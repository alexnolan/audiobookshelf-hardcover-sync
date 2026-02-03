package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/api/hardcover"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/database"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/logger"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/models"
)

// HCCollector handles collection of books from Hardcover with rate limiting
type HCCollector struct {
	hcClient      hardcover.HardcoverClientInterface
	repository    *database.Repository
	rateLimiter   *TokenBucket
	lastCollected map[string]time.Time
	mu            sync.RWMutex
}

// TokenBucket implements rate limiting
type TokenBucket struct {
	maxTokens    int
	refillRate   float64 // tokens per second
	tokens       float64
	lastRefill   time.Time
	mu           sync.Mutex
}

// NewTokenBucket creates a new rate limiter
func NewTokenBucket(maxTokens int, refillRate float64) *TokenBucket {
	return &TokenBucket{
		maxTokens:  maxTokens,
		refillRate: refillRate,
		tokens:     float64(maxTokens),
		lastRefill: time.Now(),
	}
}

// Wait blocks until a token is available
func (tb *TokenBucket) Wait(ctx context.Context) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for tb.tokens < 1 {
		// Refill tokens based on elapsed time
		now := time.Now()
		elapsed := now.Sub(tb.lastRefill).Seconds()
		tb.tokens += elapsed * tb.refillRate
		tb.lastRefill = now

		if tb.tokens < 1 {
			// Wait a bit before checking again
			select {
			case <-time.After(100 * time.Millisecond):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	tb.tokens--
	return nil
}

// NewHCCollector creates a new Hardcover collector with rate limiting
func NewHCCollector(hcClient hardcover.HardcoverClientInterface, repository *database.Repository) *HCCollector {
	// 50 req/min = ~0.833 req/sec, max burst of 50
	return &HCCollector{
		hcClient:      hcClient,
		repository:    repository,
		rateLimiter:   NewTokenBucket(50, 50.0/60.0),
		lastCollected: make(map[string]time.Time),
	}
}

// CollectAllUserBooks fetches all user books from Hardcover (rate limited)
func (c *HCCollector) CollectAllUserBooks(ctx context.Context, profileID string, progressCallback func(current, total int)) (*CollectionStats, error) {
	stats := &CollectionStats{}
	startTime := time.Now()
	log := logger.Get()

	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	// Get all user books from Hardcover API
	userBooks, err := c.hcClient.GetAllUserBooks(ctx)
	if err != nil {
		log.Error("Failed to fetch user books from Hardcover", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to fetch user books: %w", err)
	}

	total := len(userBooks)
	log.Info("Fetched user books from Hardcover", map[string]interface{}{
		"total": total,
	})

	for i, ub := range userBooks {
		if progressCallback != nil {
			progressCallback(i+1, total)
		}

		// Get author from book title (we'll need to fetch separately if needed)
		author := "" // HC doesn't return author in this query

		// Extract progress from user_book_reads (get the most recent reading session)
		var progress float64
		if len(ub.UserBookReads) > 0 && ub.UserBookReads[0].Progress != nil {
			// Hardcover stores progress as 0-100, normalize to 0-1 for consistency with ABS
			progress = *ub.UserBookReads[0].Progress / 100.0
		}

		// Convert edition ID to pointer (nullable in database)
		var editionID *int64
		if ub.EditionID > 0 {
			eid := int64(ub.EditionID)
			editionID = &eid
		}

		// Convert to database model
		hcBook := database.HardcoverUserBook{
			ProfileID:    profileID,
			HCUserBookID: int64(ub.ID),
			HCBookID:     int64(ub.BookID),
			HCEditionID:  editionID,
			Slug:         ub.Book.Slug,
			Title:        ub.Book.Title,
			Author:       author,
			Progress:     progress,
			UpdatedAt:    time.Now(),
		}

		// Store ASIN/ISBN for matching if available
		if ub.Edition.ASIN != nil {
			hcBook.ASIN = *ub.Edition.ASIN
		}
		if ub.Edition.ISBN13 != nil {
			hcBook.ISBN13 = *ub.Edition.ISBN13
		}
		if ub.Edition.ISBN10 != nil {
			hcBook.ISBN10 = *ub.Edition.ISBN10
		}

		// Upsert book
		if err := c.repository.UpsertHardcoverUserBook(hcBook); err != nil {
			log.Warn("Failed to upsert HC book", map[string]interface{}{
				"hc_id": ub.ID,
				"error": err.Error(),
			})
			stats.ErrorCount++
			continue
		}
		stats.CollectedCount++
	}

	c.mu.Lock()
	c.lastCollected[profileID] = time.Now()
	c.mu.Unlock()

	stats.Duration = time.Since(startTime)
	return stats, nil
}

// ShouldRefreshUserBooks determines if HC books need to be refreshed
func (c *HCCollector) ShouldRefreshUserBooks(profileID string, maxAge time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lastCollection, exists := c.lastCollected[profileID]
	if !exists {
		return true
	}

	return time.Since(lastCollection) > maxAge
}

// GetBookEditions retrieves all editions for a Hardcover book
func (c *HCCollector) GetBookEditions(ctx context.Context, bookID int) ([]hardcover.BookEdition, error) {
	log := logger.Get()

	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait error: %w", err)
	}

	// Fetch editions from HC API
	editions, err := c.hcClient.GetBookEditions(ctx, bookID)
	if err != nil {
		log.Error("Failed to fetch editions from Hardcover", map[string]interface{}{
			"book_id": bookID,
			"error":   err.Error(),
		})
		return nil, err
	}

	return editions, nil
}

// SearchBookByASIN searches for a book on Hardcover by ASIN
func (c *HCCollector) SearchBookByASIN(ctx context.Context, asin string) (*models.HardcoverBook, error) {
	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait error: %w", err)
	}

	return c.hcClient.SearchBookByASIN(ctx, asin)
}

// SearchBookByISBN13 searches for a book on Hardcover by ISBN-13
func (c *HCCollector) SearchBookByISBN13(ctx context.Context, isbn13 string) (*models.HardcoverBook, error) {
	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait error: %w", err)
	}

	return c.hcClient.SearchBookByISBN13(ctx, isbn13)
}

// SearchBooks searches for books on Hardcover by title and author
func (c *HCCollector) SearchBooks(ctx context.Context, title, author string) ([]models.HardcoverBook, error) {
	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait error: %w", err)
	}

	return c.hcClient.SearchBooks(ctx, title, author)
}

// CreateUserBook creates a new user book on Hardcover with the given edition and status
func (c *HCCollector) CreateUserBook(ctx context.Context, editionID, status string) (string, error) {
	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limit wait error: %w", err)
	}

	return c.hcClient.CreateUserBook(ctx, editionID, status)
}

// UpdateUserBookStatus updates the status of a user book on Hardcover
func (c *HCCollector) UpdateUserBookStatus(ctx context.Context, userBookID int64, status string) error {
	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait error: %w", err)
	}

	return c.hcClient.UpdateUserBookStatus(ctx, hardcover.UpdateUserBookStatusInput{
		ID:     userBookID,
		Status: status,
	})
}

// InsertUserBookRead creates a reading progress entry on Hardcover
func (c *HCCollector) InsertUserBookRead(ctx context.Context, userBookID int64, editionID *int64, startedAt, finishedAt *string) (int, error) {
	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return 0, fmt.Errorf("rate limit wait error: %w", err)
	}

	input := hardcover.InsertUserBookReadInput{
		UserBookID: userBookID,
		DatesRead: hardcover.DatesReadInput{
			EditionID:  editionID,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
		},
	}

	return c.hcClient.InsertUserBookRead(ctx, input)
}

// GetUserBookReads gets the reading progress entries for a user book
func (c *HCCollector) GetUserBookReads(ctx context.Context, userBookID int64) ([]hardcover.UserBookRead, error) {
	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait error: %w", err)
	}

	return c.hcClient.GetUserBookReads(ctx, hardcover.GetUserBookReadsInput{
		UserBookID: userBookID,
	})
}
