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

	// Get profile's sync config for library filtering
	profile, err := c.repository.GetProfile(profileID)
	if err != nil {
		log.Warn("Failed to get profile for library filtering, will include all libraries", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
	} else if profile != nil {
		log.Info("Profile sync config loaded", map[string]interface{}{
			"profile_id":          profileID,
			"library_filter_mode": profile.SyncConfig.LibraryFilterMode,
			"filtered_libraries":  profile.SyncConfig.FilteredLibraries,
			"include_ebooks":      profile.SyncConfig.IncludeEbooks,
			"include_collections": profile.SyncConfig.IncludeCollections,
			"exclude_collections": profile.SyncConfig.ExcludeCollections,
		})
		
		// Log warning if collection filters are configured but not yet implemented
		if len(profile.SyncConfig.IncludeCollections) > 0 || len(profile.SyncConfig.ExcludeCollections) > 0 {
			log.Info("Collection filtering is configured", map[string]interface{}{
				"profile_id":          profileID,
				"include_collections": profile.SyncConfig.IncludeCollections,
				"exclude_collections": profile.SyncConfig.ExcludeCollections,
			})
		}
	}

	// Build collection filtering data if configured
	var bookCollections map[string][]string // book ID -> collection IDs
	var includeCollectionSet, excludeCollectionSet map[string]bool
	collectionFilteringEnabled := false

	if profile != nil && (len(profile.SyncConfig.IncludeCollections) > 0 || len(profile.SyncConfig.ExcludeCollections) > 0) {
		collections, err := c.absClient.GetCollections(ctx)
		if err != nil {
			log.Warn("Failed to get collections for filtering, will include all books", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			collectionFilteringEnabled = true
			bookCollections = make(map[string][]string)
			
			// Map each book ID to its collection IDs (not names)
			for _, col := range collections {
				for _, bookID := range col.BookIDs {
					bookCollections[bookID] = append(bookCollections[bookID], col.ID)
				}
			}
			
			// Build include/exclude sets using collection IDs from config
			if len(profile.SyncConfig.IncludeCollections) > 0 {
				includeCollectionSet = make(map[string]bool)
				for _, colID := range profile.SyncConfig.IncludeCollections {
					includeCollectionSet[colID] = true
				}
			}
			if len(profile.SyncConfig.ExcludeCollections) > 0 {
				excludeCollectionSet = make(map[string]bool)
				for _, colID := range profile.SyncConfig.ExcludeCollections {
					excludeCollectionSet[colID] = true
				}
			}
			
			log.Info("Collection filtering initialized", map[string]interface{}{
				"total_collections":   len(collections),
				"books_in_collections": len(bookCollections),
				"include_filter_count": len(includeCollectionSet),
				"exclude_filter_count": len(excludeCollectionSet),
			})
		}
	}

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

	// Build a map of library name to ID for filtering
	libraryNameToID := make(map[string]string)
	libraryIDToName := make(map[string]string)
	libraryNames := make([]string, 0, len(libraries))
	for _, lib := range libraries {
		libraryNameToID[lib.Name] = lib.ID
		libraryIDToName[lib.ID] = lib.Name
		libraryNames = append(libraryNames, lib.Name)
	}

	log.Info("Found ABS libraries", map[string]interface{}{
		"count":     len(libraries),
		"libraries": libraryNames,
	})

	// Determine which libraries to collect based on filter settings
	includedLibraryIDs := make(map[string]bool)
	excludedLibraryIDs := make(map[string]bool)

	if profile != nil && profile.SyncConfig.LibraryFilterMode != "" {
		filterMode := profile.SyncConfig.LibraryFilterMode
		filteredLibraries := profile.SyncConfig.FilteredLibraries

		log.Info("Applying library filter", map[string]interface{}{
			"mode":      filterMode,
			"libraries": filteredLibraries,
		})

		if filterMode == database.LibraryFilterModeInclude {
			// Only include specified libraries
			for _, libName := range filteredLibraries {
				if libID, ok := libraryNameToID[libName]; ok {
					includedLibraryIDs[libID] = true
				}
			}
		} else if filterMode == database.LibraryFilterModeExclude {
			// Exclude specified libraries
			for _, libName := range filteredLibraries {
				if libID, ok := libraryNameToID[libName]; ok {
					excludedLibraryIDs[libID] = true
				}
			}
		}
	}

	// Delete books from excluded libraries
	if len(excludedLibraryIDs) > 0 || len(includedLibraryIDs) > 0 {
		for _, lib := range libraries {
			shouldExclude := false
			if len(includedLibraryIDs) > 0 && !includedLibraryIDs[lib.ID] {
				shouldExclude = true
			}
			if excludedLibraryIDs[lib.ID] {
				shouldExclude = true
			}

			if shouldExclude {
				deleted, err := c.repository.DeleteABSBooksByLibrary(profileID, lib.ID)
				if err != nil {
					log.Warn("Failed to delete books from excluded library", map[string]interface{}{
						"library_id":   lib.ID,
						"library_name": lib.Name,
						"error":        err.Error(),
					})
				} else if deleted > 0 {
					log.Info("Deleted books from excluded library", map[string]interface{}{
						"library_id":    lib.ID,
						"library_name":  lib.Name,
						"deleted_count": deleted,
					})
				}
			}
		}
	}

	collectedLibraries := 0
	skippedLibraries := 0
	
	// Build collection filter
	collectionFilter := &CollectionFilter{
		Enabled:            collectionFilteringEnabled,
		BookCollections:    bookCollections,
		IncludeCollections: includeCollectionSet,
		ExcludeCollections: excludeCollectionSet,
	}
	
	// Clean up books that no longer match collection filter
	if collectionFilter.Enabled {
		// Get all existing books for this profile
		existingBooks, err := c.repository.GetABSBooksForProfile(profileID)
		if err != nil {
			log.Warn("Failed to get existing books for collection cleanup", map[string]interface{}{
				"profile_id": profileID,
				"error":      err.Error(),
			})
		} else {
			deletedByCollection := 0
			for _, book := range existingBooks {
				if !collectionFilter.ShouldIncludeBook(book.ABSID) {
					if err := c.repository.DeleteABSBook(profileID, book.ABSID); err != nil {
						log.Warn("Failed to delete book excluded by collection filter", map[string]interface{}{
							"abs_id": book.ABSID,
							"title":  book.Title,
							"error":  err.Error(),
						})
					} else {
						deletedByCollection++
					}
				}
			}
			if deletedByCollection > 0 {
				log.Info("Cleaned up books excluded by collection filter", map[string]interface{}{
					"profile_id":    profileID,
					"deleted_count": deletedByCollection,
				})
			}
		}
	}
	
	for _, lib := range libraries {
		// Check if library should be included
		if len(includedLibraryIDs) > 0 && !includedLibraryIDs[lib.ID] {
			log.Debug("Skipping library not in include list", map[string]interface{}{
				"library_id":   lib.ID,
				"library_name": lib.Name,
			})
			skippedLibraries++
			continue
		}
		if excludedLibraryIDs[lib.ID] {
			log.Debug("Skipping excluded library", map[string]interface{}{
				"library_id":   lib.ID,
				"library_name": lib.Name,
			})
			skippedLibraries++
			continue
		}

		libStats, err := c.CollectLibraryBooks(ctx, profileID, lib.ID, progressMap, collectionFilter, progressCallback)
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
		collectedLibraries++
	}

	log.Info("ABS collection complete", map[string]interface{}{
		"collected_libraries": collectedLibraries,
		"skipped_libraries":   skippedLibraries,
		"total_books":         stats.CollectedCount,
	})

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

// CollectionFilter holds collection filtering configuration
type CollectionFilter struct {
	Enabled            bool
	BookCollections    map[string][]string // book ID -> collection IDs
	IncludeCollections map[string]bool     // collection IDs to include (nil = all)
	ExcludeCollections map[string]bool     // collection IDs to exclude (nil = none)
}

// ShouldIncludeBook checks if a book should be included based on collection filters
func (f *CollectionFilter) ShouldIncludeBook(bookID string) bool {
	if !f.Enabled {
		return true
	}
	
	bookColls := f.BookCollections[bookID]
	
	// If include filter is set, book must be in at least one included collection
	if len(f.IncludeCollections) > 0 {
		found := false
		for _, collID := range bookColls {
			if f.IncludeCollections[collID] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// If exclude filter is set, book must not be in any excluded collection
	if len(f.ExcludeCollections) > 0 {
		for _, collID := range bookColls {
			if f.ExcludeCollections[collID] {
				return false
			}
		}
	}
	
	return true
}

// CollectLibraryBooks fetches books from a specific ABS library
func (c *ABSCollector) CollectLibraryBooks(ctx context.Context, profileID string, libraryID string, progressMap map[string]ProgressInfo, collectionFilter *CollectionFilter, progressCallback func(current, total int)) (*CollectionStats, error) {
	stats := &CollectionStats{}
	log := logger.Get()

	// Get profile's sync config to check IncludeEbooks setting
	profile, err := c.repository.GetProfile(profileID)
	if err != nil {
		log.Warn("Failed to get profile for ebook filtering, will include all items", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
	}
	includeEbooks := profile != nil && profile.SyncConfig.IncludeEbooks

	// If ebooks are not included, remove any existing ebooks from the database
	if !includeEbooks {
		deleted, err := c.repository.DeleteABSEbooksByProfile(profileID)
		if err != nil {
			log.Warn("Failed to delete existing ebooks", map[string]interface{}{
				"profile_id": profileID,
				"error":      err.Error(),
			})
		} else if deleted > 0 {
			log.Info("Deleted existing ebooks from database", map[string]interface{}{
				"profile_id":    profileID,
				"deleted_count": deleted,
			})
		}
	}

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
	skippedEbooks := 0
	skippedByCollection := 0
	for i, item := range items {
		if progressCallback != nil {
			progressCallback(i+1, total)
		}

		// Determine if this item is an ebook using the proper detection
		isEbook := item.IsEbook()

		// Skip ebooks if not included
		if isEbook && !includeEbooks {
			log.Debug("Skipping ebook item", map[string]interface{}{
				"abs_id":        item.ID,
				"title":         item.Media.Metadata.Title,
				"ebook_format":  item.Media.EbookFormat,
				"duration":      item.Media.Duration,
				"num_audio":     item.Media.NumAudioFiles,
			})
			skippedEbooks++
			continue
		}

		// Apply collection filtering
		if collectionFilter != nil && !collectionFilter.ShouldIncludeBook(item.ID) {
			log.Debug("Skipping item excluded by collection filter", map[string]interface{}{
				"abs_id":      item.ID,
				"title":       item.Media.Metadata.Title,
				"collections": collectionFilter.BookCollections[item.ID],
			})
			skippedByCollection++
			continue
		}

		// Look up progress from the progress map
		progress := ProgressInfo{}
		if progressMap != nil {
			if p, found := progressMap[item.ID]; found {
				progress = p
			}
		}

		// Determine the effective media type for storage
		// ABS only returns "book" or "podcast" at item level, but we want to distinguish ebooks
		effectiveMediaType := "audiobook"
		if isEbook {
			effectiveMediaType = "ebook"
		}

		// Log each item for debugging
		log.Debug("Processing ABS item", map[string]interface{}{
			"abs_id":           item.ID,
			"title":            item.Media.Metadata.Title,
			"author":           item.Media.Metadata.AuthorName,
			"abs_media_type":   item.MediaType,
			"effective_type":   effectiveMediaType,
			"is_ebook":         isEbook,
			"ebook_format":     item.Media.EbookFormat,
			"duration":         item.Media.Duration,
			"num_audio_files":  item.Media.NumAudioFiles,
			"library_id":       item.LibraryID,
			"progress":         progress.Progress,
			"has_progress":     progressMap != nil && progressMap[item.ID].Progress > 0,
		})

		// Convert to database model
		absBook := database.ABSBook{
			ProfileID:   profileID,
			ABSID:       item.ID,
			LibraryID:   item.LibraryID,
			MediaType:   effectiveMediaType, // Use our detected type, not ABS's
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

	log.Info("Library collection complete", map[string]interface{}{
		"library_id":             libraryID,
		"total_items":            total,
		"collected":              stats.CollectedCount,
		"skipped_ebooks":         skippedEbooks,
		"skipped_by_collection":  skippedByCollection,
		"errors":                 stats.ErrorCount,
		"include_ebooks":         includeEbooks,
	})

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
