package sync

import (
	"context"
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

	// Get user books - placeholder for actual HC API call
	var userBooks []*models.HardcoverBook

	total := len(userBooks)
	for i, ub := range userBooks {
		if progressCallback != nil {
			progressCallback(i+1, total)
		}

		// Parse IDs from strings to int64
		// In real implementation, these would come from HC API as integers
		var hcUserBookID int64 = 0
		var hcBookID int64 = 0
		// TODO: parse ub.UserBookID and ub.ID when HC API is implemented

		// Convert to database model
		hcBook := database.HardcoverUserBook{
			ProfileID:    profileID,
			HCUserBookID: hcUserBookID,
			HCBookID:     hcBookID,
			Title:        ub.Title,
			Author:       getHCAuthorString(ub),
			Progress:     getHCProgress(ub),
			UpdatedAt:    time.Now(),
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

// Helper functions
func getHCAuthorString(book *models.HardcoverBook) string {
	if len(book.Authors) > 0 {
		return book.Authors[0].Name
	}
	return ""
}

func getHCProgress(ub *models.HardcoverBook) float64 {
	// Hardcover doesn't have progress field, default to 0
	// This would need to be fetched from a separate endpoint
	return 0
}
