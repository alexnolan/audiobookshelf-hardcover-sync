package sync

import (
	"context"
	"time"

	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/api/audiobookshelf"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/database"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/logger"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/models"
)

// ABSCollector handles collection of books from AudiobookShelf
type ABSCollector struct {
	absClient  *audiobookshelf.Client
	repository *database.Repository
}

// NewABSCollector creates a new ABS collector
func NewABSCollector(absClient *audiobookshelf.Client, repository *database.Repository) *ABSCollector {
	return &ABSCollector{
		absClient:  absClient,
		repository: repository,
	}
}

// CollectionStats holds statistics about a collection operation
type CollectionStats struct {
	CollectedCount int
	UpdatedCount   int
	ErrorCount     int
	Duration       time.Duration
}

// CollectAllBooks fetches all books from all ABS libraries
func (c *ABSCollector) CollectAllBooks(ctx context.Context, profileID string, progressCallback func(current, total int)) (*CollectionStats, error) {
	stats := &CollectionStats{}
	startTime := time.Now()

	log := logger.Get()

	// Get libraries
	libraries, err := c.absClient.GetLibraries(ctx)
	if err != nil {
		log.Error("Failed to get ABS libraries", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}

	for _, lib := range libraries {
		libStats, err := c.CollectLibraryBooks(ctx, profileID, lib.ID, progressCallback)
		if err != nil {
			log.Warn("Failed to collect library books", map[string]interface{}{
				"library_id": lib.ID,
				"error":      err.Error(),
			})
			stats.ErrorCount++
			continue
		}
		stats.CollectedCount += libStats.CollectedCount
		stats.UpdatedCount += libStats.UpdatedCount
		stats.ErrorCount += libStats.ErrorCount
	}

	stats.Duration = time.Since(startTime)
	return stats, nil
}

// CollectLibraryBooks fetches books from a specific ABS library
func (c *ABSCollector) CollectLibraryBooks(ctx context.Context, profileID string, libraryID string, progressCallback func(current, total int)) (*CollectionStats, error) {
	stats := &CollectionStats{}
	log := logger.Get()

	// Get library items (books)
	items, err := c.absClient.GetLibraryItems(ctx, libraryID)
	if err != nil {
		log.Error("Failed to get library items", map[string]interface{}{
			"library_id": libraryID,
			"error":      err.Error(),
		})
		return nil, err
	}

	total := len(items)
	for i, item := range items {
		if progressCallback != nil {
			progressCallback(i+1, total)
		}

		// Convert to database model
		absBook := database.ABSBook{
			ProfileID:   profileID,
			ABSID:       item.ID,
			Title:       item.Media.Metadata.Title,
			Author:      getAuthorsString(item.Media.Metadata),
			ASIN:        getASINFromMetadata(item.Media.Metadata),
			ISBN:        getISBNFromMetadata(item.Media.Metadata),
			Progress:    getProgressFromBook(item),
			UpdatedAt:   time.Now(),
		}

		// Upsert book
		if err := c.repository.UpsertABSBook(absBook); err != nil {
			log.Warn("Failed to upsert ABS book", map[string]interface{}{
				"abs_id": item.ID,
				"error":  err.Error(),
			})
			stats.ErrorCount++
			continue
		}
		stats.CollectedCount++
	}

	// Auto-match books
	if err := c.AutoMatchBooks(ctx, profileID); err != nil {
		log.Warn("Failed to auto-match books", map[string]interface{}{
			"error": err.Error(),
		})
	}

	return stats, nil
}

// AutoMatchBooks attempts to auto-match ABS books to Hardcover editions
func (c *ABSCollector) AutoMatchBooks(ctx context.Context, profileID string) error {
	log := logger.Get()

	// Get unmapped books
	unmapped, _, err := c.repository.GetUnmappedBooks(profileID, 1000, 0)
	if err != nil {
		return err
	}

	for _, book := range unmapped {
		// Try ASIN match first (99% confidence)
		if book.ASIN != "" {
			hcBook, err := c.repository.GetHardcoverUserBookByASIN(profileID, book.ASIN)
			if err == nil && hcBook != nil {
				hcUserBookID := int64(hcBook.HCUserBookID)
				mapping := database.BookMapping{
					ProfileID:       profileID,
					ABSBookID:       book.ID,
					HCUserBookID:    &hcUserBookID,
					MatchMethod:     database.MatchMethodASIN,
					MatchConfidence: 0.99,
				}
				if err := c.repository.UpsertBookMapping(mapping); err != nil {
					log.Warn("Failed to create ASIN mapping", map[string]interface{}{
						"error": err.Error(),
					})
				}
				continue
			}
		}

		// Try ISBN match (95% confidence)
		if book.ISBN != "" {
			hcBook, err := c.repository.GetHardcoverUserBookByISBN(profileID, book.ISBN)
			if err == nil && hcBook != nil {
				hcUserBookID := int64(hcBook.HCUserBookID)
				mapping := database.BookMapping{
					ProfileID:       profileID,
					ABSBookID:       book.ID,
					HCUserBookID:    &hcUserBookID,
					MatchMethod:     database.MatchMethodISBN,
					MatchConfidence: 0.95,
				}
				if err := c.repository.UpsertBookMapping(mapping); err != nil {
					log.Warn("Failed to create ISBN mapping", map[string]interface{}{
						"error": err.Error(),
					})
				}
			}
		}
	}

	return nil
}

// Helper functions
func getAuthorsString(metadata models.AudiobookshelfMetadataStruct) string {
	return metadata.AuthorName
}

func getASINFromMetadata(metadata models.AudiobookshelfMetadataStruct) string {
	return metadata.ASIN
}

func getISBNFromMetadata(metadata models.AudiobookshelfMetadataStruct) string {
	return metadata.ISBN
}

func getProgressFromBook(book models.AudiobookshelfBook) float64 {
	if book.Media.Duration == 0 {
		return 0
	}
	progress := book.Progress.CurrentTime / book.Media.Duration
	if progress < 0 {
		return 0
	}
	if progress > 1 {
		return 1
	}
	return progress
}
