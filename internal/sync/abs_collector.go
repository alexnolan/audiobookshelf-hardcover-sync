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
	CollectedCount int           `json:"collected_count"`
	UpdatedCount   int           `json:"updated_count"`
	ErrorCount     int           `json:"error_count"`
	Duration       time.Duration `json:"duration"`
}

// CollectAllBooks fetches all books from all ABS libraries
func (c *ABSCollector) CollectAllBooks(ctx context.Context, profileID string, progressCallback func(current, total int)) (*CollectionStats, error) {
	stats := &CollectionStats{}
	startTime := time.Now()

	log := logger.Get()

	// First, get user progress data
	userProgress, err := c.absClient.GetUserProgress(ctx)
	if err != nil {
		log.Warn("Failed to get user progress, will continue without progress data", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Build progress map keyed by library item ID
	progressMap := make(map[string]ProgressInfo)
	if userProgress != nil {
		for _, mp := range userProgress.MediaProgress {
			progressMap[mp.LibraryItemID] = ProgressInfo{
				Progress:    mp.Progress,
				CurrentTime: mp.CurrentTime,
				Duration:    mp.Duration,
				IsFinished:  mp.IsFinished,
				StartedAt:   mp.StartedAt,
				FinishedAt:  mp.FinishedAt,
			}
		}
		log.Info("Loaded user progress data", map[string]interface{}{
			"progress_entries": len(progressMap),
		})
	}

	// Get libraries
	libraries, err := c.absClient.GetLibraries(ctx)
	if err != nil {
		log.Error("Failed to get ABS libraries", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}

	for _, lib := range libraries {
		libStats, err := c.CollectLibraryBooks(ctx, profileID, lib.ID, progressMap, progressCallback)
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

// ProgressInfo holds user progress data for a book
type ProgressInfo struct {
	Progress    float64
	CurrentTime float64
	Duration    float64
	IsFinished  bool
	StartedAt   int64
	FinishedAt  int64
}

// CollectLibraryBooks fetches books from a specific ABS library
func (c *ABSCollector) CollectLibraryBooks(ctx context.Context, profileID string, libraryID string, progressMap map[string]ProgressInfo, progressCallback func(current, total int)) (*CollectionStats, error) {
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

		// Look up progress from the progress map
		progress := ProgressInfo{}
		if progressMap != nil {
			if p, found := progressMap[item.ID]; found {
				progress = p
			}
		}

		// Log each item for debugging
		log.Debug("Processing ABS item", map[string]interface{}{
			"abs_id":      item.ID,
			"title":       item.Media.Metadata.Title,
			"author":      item.Media.Metadata.AuthorName,
			"duration":    item.Media.Duration,
			"library_id":  item.LibraryID,
			"progress":    progress.Progress,
			"has_progress": progressMap != nil && progressMap[item.ID].Progress > 0,
		})

		// Convert to database model
		absBook := database.ABSBook{
			ProfileID:   profileID,
			ABSID:       item.ID,
			LibraryID:   item.LibraryID,
			Title:       item.Media.Metadata.Title,
			Author:      item.Media.Metadata.AuthorName,
			Narrator:    item.Media.Metadata.NarratorName,
			SeriesName:  item.Media.Metadata.SeriesName,
			ASIN:        item.Media.Metadata.ASIN,
			ISBN:        item.Media.Metadata.ISBN,
			Duration:    progress.Duration,
			CurrentTime: progress.CurrentTime,
			Progress:    progress.Progress,
			IsFinished:  progress.IsFinished,
			CoverPath:   item.Media.CoverPath,
			UpdatedAt:   time.Now(),
		}

		// Use media duration if available and progress duration is 0
		if absBook.Duration == 0 && item.Media.Duration > 0 {
			absBook.Duration = item.Media.Duration
		}

		// Handle progress timestamps
		if progress.StartedAt > 0 {
			startedAt := progress.StartedAt
			absBook.StartedAt = &startedAt
		}
		if progress.FinishedAt > 0 {
			finishedAt := progress.FinishedAt
			absBook.FinishedAt = &finishedAt
		}

		// Upsert book
		if err := c.repository.UpsertABSBook(absBook); err != nil {
			log.Warn("Failed to upsert ABS book", map[string]interface{}{
				"abs_id": item.ID,
				"title":  item.Media.Metadata.Title,
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

	matchedCount := 0
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
				} else {
					matchedCount++
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
				} else {
					matchedCount++
				}
				continue
			}
		}

		// Try title match (80% confidence - exact match only)
		if book.Title != "" {
			hcBook, err := c.repository.GetHardcoverUserBookByTitle(profileID, book.Title)
			if err == nil && hcBook != nil {
				hcUserBookID := int64(hcBook.HCUserBookID)
				mapping := database.BookMapping{
					ProfileID:       profileID,
					ABSBookID:       book.ID,
					HCUserBookID:    &hcUserBookID,
					MatchMethod:     database.MatchMethodTitleAuthor,
					MatchConfidence: 0.80,
				}
				if err := c.repository.UpsertBookMapping(mapping); err != nil {
					log.Warn("Failed to create title mapping", map[string]interface{}{
						"error": err.Error(),
					})
				} else {
					matchedCount++
				}
			}
		}
	}

	log.Info("Auto-matching completed", map[string]interface{}{
		"profile_id":    profileID,
		"unmapped":      len(unmapped),
		"matched_count": matchedCount,
	})

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
