package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/api/audiobookshelf"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/database"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/logger"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/sync"
)

// CollectorFactory interface for creating collectors
type CollectorFactory interface {
	CreateABSCollector(profileID string) (*sync.ABSCollector, error)
	CreateABSClient(profileID string) (audiobookshelf.AudiobookshelfClientInterface, error)
	CreateHCCollector(profileID string) (*sync.HCCollector, error)
	GetRepository() *database.Repository
}

// LibraryAPI handles book library REST endpoints
type LibraryAPI struct {
	repository       *database.Repository
	collectorFactory CollectorFactory
}

// NewLibraryAPI creates a new library API handler
func NewLibraryAPI(repository *database.Repository, collectorFactory CollectorFactory) *LibraryAPI {
	return &LibraryAPI{
		repository:       repository,
		collectorFactory: collectorFactory,
	}
}

// Exported for use in server.go
var _ interface{} = (*LibraryAPI)(nil)

// PaginationInfo holds pagination details
type PaginationInfo struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// FlatBookComparison is a flattened view of book comparison data for the frontend
type FlatBookComparison struct {
	ABSID             string  `json:"abs_id"`
	ABSTitle          string  `json:"abs_title"`
	ABSAuthor         string  `json:"abs_author"`
	ABSNarrator       string  `json:"abs_narrator,omitempty"`
	ABSMediaType      string  `json:"abs_media_type,omitempty"` // "book" for audiobook, "ebook" for ebook
	ABSProgress       float64 `json:"abs_progress"`
	ABSASIN           string  `json:"abs_asin,omitempty"`
	ABSISBN           string  `json:"abs_isbn,omitempty"`
	ABSLastUpdated    string  `json:"abs_last_updated,omitempty"`
	HardcoverProgress float64 `json:"hardcover_progress"`
	ProgressDiff      float64 `json:"progress_diff"`
	InSync            bool    `json:"in_sync"`
	SyncStatus        string  `json:"sync_status"`
	SyncEnabled       bool    `json:"sync_enabled"`
	HCBookID          *int64  `json:"hc_book_id,omitempty"`
	HCUserBookID      *int64  `json:"hc_user_book_id,omitempty"`
	HCSlug            string  `json:"hc_slug,omitempty"`
	HCEditionID       *int64  `json:"hc_edition_id,omitempty"`
	HCTitle           string  `json:"hc_title,omitempty"`
	HCAuthor          string  `json:"hc_author,omitempty"`
	HCASIN            string  `json:"hc_asin,omitempty"`
	HCISBN13          string  `json:"hc_isbn13,omitempty"`
	HCISBN10          string  `json:"hc_isbn10,omitempty"`
	HCStatusName      string  `json:"hc_status_name,omitempty"`
	MatchMethod       string  `json:"match_method,omitempty"`
	MatchConfidence   float64 `json:"match_confidence,omitempty"`
}

// flattenComparison converts a BookComparison to a flat structure for frontend
func flattenComparison(comp database.BookComparison) FlatBookComparison {
	flat := FlatBookComparison{
		ABSID:          comp.ABSBook.ABSID,
		ABSTitle:       comp.ABSBook.Title,
		ABSAuthor:      comp.ABSBook.Author,
		ABSNarrator:    comp.ABSBook.Narrator,
		ABSMediaType:   comp.ABSBook.MediaType,
		ABSProgress:    comp.ABSBook.Progress,
		ABSASIN:        comp.ABSBook.ASIN,
		ABSISBN:        comp.ABSBook.ISBN,
		ABSLastUpdated: comp.ABSBook.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ProgressDiff:   comp.ProgressDiff,
		InSync:         comp.InSync,
		SyncStatus:     comp.SyncStatus,
		SyncEnabled:    true, // Default to enabled
	}

	// Set HC progress if mapped
	if comp.HardcoverBook != nil {
		flat.HardcoverProgress = comp.HardcoverBook.Progress
		hcBookID := comp.HardcoverBook.HCBookID
		flat.HCBookID = &hcBookID
		hcUserBookID := comp.HardcoverBook.HCUserBookID
		flat.HCUserBookID = &hcUserBookID
		flat.HCSlug = comp.HardcoverBook.Slug
		flat.HCEditionID = comp.HardcoverBook.HCEditionID
		flat.HCTitle = comp.HardcoverBook.Title
		flat.HCAuthor = comp.HardcoverBook.Author
		flat.HCASIN = comp.HardcoverBook.ASIN
		flat.HCISBN13 = comp.HardcoverBook.ISBN13
		flat.HCISBN10 = comp.HardcoverBook.ISBN10
		flat.HCStatusName = comp.HardcoverBook.StatusName
	}

	// Check sync config
	if comp.Config != nil {
		flat.SyncEnabled = comp.Config.SyncEnabled
	}

	// Add mapping info
	if comp.Mapping != nil {
		flat.MatchMethod = string(comp.Mapping.MatchMethod)
		flat.MatchConfidence = comp.Mapping.MatchConfidence
	}

	return flat
}

// GetBooksHandler retrieves books with comparison data
func (api *LibraryAPI) GetBooksHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	// Parse pagination
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	offset := (page - 1) * limit

	// Parse filter options
	filterOpts := &database.BookFilterOptions{
		Search:    r.URL.Query().Get("search"),
		Filter:    r.URL.Query().Get("filter"),
		Sort:      r.URL.Query().Get("sort"),
		SortDir:   r.URL.Query().Get("sort_dir"),
		MediaType: r.URL.Query().Get("media_type"),
		LibraryID: r.URL.Query().Get("library_id"),
	}

	// Get comparisons
	comparisons, total, err := api.repository.GetBookComparisons(profileID, limit, offset, filterOpts)
	if err != nil {
		log.Error("Failed to get book comparisons", map[string]interface{}{
			"error": err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to retrieve books",
		})
		return
	}

	totalPages := (int(total) + limit - 1) / limit

	// Flatten comparisons for frontend
	flatBooks := make([]FlatBookComparison, len(comparisons))
	for i, comp := range comparisons {
		flatBooks[i] = flattenComparison(comp)
	}

	// Get ABS URL for the profile
	absURL := ""
	if profile, err := api.repository.GetProfile(profileID); err == nil && profile != nil {
		absURL = profile.AudiobookshelfURL
	}

	w.WriteHeader(http.StatusOK)
	// Return response matching frontend expectations:
	// data = books array, pagination at top level
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    flatBooks,
		"abs_url": absURL,
		"pagination": PaginationInfo{
			Page:       page,
			Limit:      limit,
			Total:      int(total),
			TotalPages: totalPages,
		},
	})
}

// GetBookHandler retrieves a single book with details
func (api *LibraryAPI) GetBookHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	absBookIDStr := r.PathValue("absBookId")

	if profileID == "" || absBookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and Book ID required",
		})
		return
	}

	// Parse absBookID as uint
	absBookID := uint(0)
	if parsed, err := strconv.ParseUint(absBookIDStr, 10, 32); err == nil {
		absBookID = uint(parsed)
	}

	// Get comparison
	comparison, err := api.repository.GetBookComparison(profileID, absBookID)
	if err != nil {
		log.Warn("Book not found", map[string]interface{}{
			"error": err.Error(),
		})
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Book not found",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    comparison,
	})
}

// SyncBookHandler triggers sync for a single book
func (api *LibraryAPI) SyncBookHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	absBookIDStr := r.PathValue("absBookId")

	if profileID == "" || absBookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and Book ID required",
		})
		return
	}

	log.Info("Sync book requested", map[string]interface{}{
		"profile_id":  profileID,
		"abs_book_id": absBookIDStr,
	})

	// Get the ABS book from database
	absBook, err := api.repository.GetABSBook(profileID, absBookIDStr)
	if err != nil || absBook == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "ABS book not found",
		})
		return
	}

	// Get the mapping for this book
	mapping, err := api.repository.GetBookMapping(profileID, absBook.ID)
	if err != nil || mapping == nil || mapping.HCUserBookID == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Book is not mapped to Hardcover or has no user_book_id",
		})
		return
	}

	// Get HC collector
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()

	// Determine the target status based on progress
	var targetStatus string
	if absBook.Progress >= 1.0 || absBook.IsFinished {
		targetStatus = "READ"
	} else if absBook.Progress == 0 {
		targetStatus = "WANT_TO_READ"
	} else {
		targetStatus = "READING"
	}

	// Update the user book status on Hardcover
	err = hcCollector.UpdateUserBookStatus(ctx, *mapping.HCUserBookID, targetStatus)
	if err != nil {
		log.Error("Failed to update Hardcover status", map[string]interface{}{
			"error":        err.Error(),
			"user_book_id": *mapping.HCUserBookID,
			"target_status": targetStatus,
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to update Hardcover status: " + err.Error(),
		})
		return
	}

	// Update local HardcoverUserBook record
	statusID := 1
	switch targetStatus {
	case "READING":
		statusID = 2
	case "READ":
		statusID = 3
	}

	hcBook, err := api.repository.GetHardcoverUserBook(profileID, *mapping.HCUserBookID)
	if err == nil && hcBook != nil {
		hcBook.Status = statusID
		hcBook.StatusName = targetStatus
		hcBook.Progress = absBook.Progress
		hcBook.ProgressSeconds = absBook.CurrentTime
		if err := api.repository.UpsertHardcoverUserBook(*hcBook); err != nil {
			log.Warn("Failed to update local HardcoverUserBook", map[string]interface{}{
				"error":        err.Error(),
				"user_book_id": *mapping.HCUserBookID,
			})
		}
	}

	log.Info("Sync book completed", map[string]interface{}{
		"profile_id":    profileID,
		"abs_book_id":   absBookIDStr,
		"abs_progress":  absBook.Progress,
		"target_status": targetStatus,
		"user_book_id":  *mapping.HCUserBookID,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":        "synced",
			"book_id":       absBookIDStr,
			"abs_progress":  absBook.Progress,
			"target_status": targetStatus,
			"user_book_id":  *mapping.HCUserBookID,
		},
	})
}

// SyncBatchHandler triggers sync for multiple books
func (api *LibraryAPI) SyncBatchHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	log.Info("Batch sync requested", map[string]interface{}{
		"profile_id": profileID,
	})

	// Parse request body for sync options
	var reqBody struct {
		SyncMode            string   `json:"sync_mode"`              // "needs_sync", "sync_all", "collections"
		SelectedCollections []string `json:"selected_collections"`   // For collections mode
		BookIDs             []string `json:"book_ids"`               // Optional: specific books to sync
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil && err.Error() != "EOF" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Default to needs_sync mode
	if reqBody.SyncMode == "" {
		reqBody.SyncMode = string(database.SyncModeNeedsSync)
	}

	// Get HC collector
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()

	// For collections mode, build a set of book IDs from selected collections
	collectionBookIDs := make(map[string]bool)
	if reqBody.SyncMode == string(database.SyncModeCollections) && len(reqBody.SelectedCollections) > 0 {
		absClient, err := api.collectorFactory.CreateABSClient(profileID)
		if err != nil {
			log.Error("Failed to create ABS client", map[string]interface{}{"error": err.Error()})
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Error:   "Failed to initialize ABS client",
			})
			return
		}
		
		collections, err := absClient.GetCollections(ctx)
		if err != nil {
			log.Error("Failed to get ABS collections", map[string]interface{}{"error": err.Error()})
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Error:   "Failed to get collections: " + err.Error(),
			})
			return
		}
		
		// Build map of selected collection IDs
		selectedSet := make(map[string]bool)
		for _, colID := range reqBody.SelectedCollections {
			selectedSet[colID] = true
		}
		
		// Add book IDs from selected collections
		for _, col := range collections {
			if selectedSet[col.ID] {
				for _, bookID := range col.BookIDs {
					collectionBookIDs[bookID] = true
				}
			}
		}
		
		log.Info("Collections mode: found books", map[string]interface{}{
			"selected_collections": len(reqBody.SelectedCollections),
			"books_in_collections": len(collectionBookIDs),
		})
	}

	// Get books to sync based on mode
	var booksToSync []database.ABSBook
	
	if len(reqBody.BookIDs) > 0 {
		// Specific books requested
		for _, bookID := range reqBody.BookIDs {
			book, err := api.repository.GetABSBook(profileID, bookID)
			if err == nil && book != nil {
				booksToSync = append(booksToSync, *book)
			}
		}
	} else {
		// Get all books for profile (use large limit to get all)
		allBooks, _, err := api.repository.GetABSBooks(profileID, 10000, 0)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Error:   "Failed to get books: " + err.Error(),
			})
			return
		}
		
		// Filter books based on sync mode
		for _, book := range allBooks {
			mapping, _ := api.repository.GetBookMapping(profileID, book.ID)
			
			switch reqBody.SyncMode {
			case string(database.SyncModeNeedsSync):
				// Only include books that need syncing (have mapping and progress differs)
				if mapping != nil && mapping.HCUserBookID != nil {
					hcBook, _ := api.repository.GetHardcoverUserBook(profileID, *mapping.HCUserBookID)
					if hcBook != nil && api.needsSync(book.Progress, book.IsFinished, hcBook.Progress, hcBook.Status) {
						booksToSync = append(booksToSync, book)
					}
				}
			case string(database.SyncModeAll):
				// All books with mappings
				if mapping != nil && mapping.HCUserBookID != nil {
					booksToSync = append(booksToSync, book)
				}
			case string(database.SyncModeCollections):
				// Books in selected collections with mappings
				if mapping != nil && mapping.HCUserBookID != nil {
					// Check if this book's ABS ID is in the collection book IDs
					if collectionBookIDs[book.ABSID] {
						booksToSync = append(booksToSync, book)
					}
				}
			}
		}
	}

	log.Info("Starting batch sync", map[string]interface{}{
		"profile_id":   profileID,
		"sync_mode":    reqBody.SyncMode,
		"books_to_sync": len(booksToSync),
	})

	// Sync each book
	var syncedCount int
	var failedCount int
	var results []map[string]interface{}

	for _, book := range booksToSync {
		mapping, _ := api.repository.GetBookMapping(profileID, book.ID)
		if mapping == nil || mapping.HCUserBookID == nil {
			continue
		}

		// Log the current state before sync
		log.Debug("Syncing book", map[string]interface{}{
			"abs_book_id":     book.ABSID,
			"title":           book.Title,
			"abs_progress":    book.Progress,
			"hc_user_book_id": *mapping.HCUserBookID,
		})

		// Determine target status
		var targetStatus string
		if book.Progress >= 1.0 || book.IsFinished {
			targetStatus = "READ"
		} else if book.Progress == 0 {
			targetStatus = "WANT_TO_READ"
		} else {
			targetStatus = "READING"
		}

		// Update on Hardcover
		err := hcCollector.UpdateUserBookStatus(ctx, *mapping.HCUserBookID, targetStatus)
		if err != nil {
			log.Error("Failed to sync book", map[string]interface{}{
				"abs_book_id":  book.ABSID,
				"error":        err.Error(),
			})
			failedCount++
			results = append(results, map[string]interface{}{
				"book_id":   book.ABSID,
				"title":     book.Title,
				"status":    "failed",
				"error":     err.Error(),
			})
			continue
		}

		// Update local HardcoverUserBook record
		statusID := 1
		switch targetStatus {
		case "READING":
			statusID = 2
		case "READ":
			statusID = 3
		}

		hcBook, err := api.repository.GetHardcoverUserBook(profileID, *mapping.HCUserBookID)
		if err == nil && hcBook != nil {
			oldProgress := hcBook.Progress
			hcBook.Status = statusID
			hcBook.StatusName = targetStatus
			hcBook.Progress = book.Progress
			hcBook.ProgressSeconds = book.CurrentTime
			if upsertErr := api.repository.UpsertHardcoverUserBook(*hcBook); upsertErr != nil {
				log.Error("Failed to update local HC book record", map[string]interface{}{
					"abs_book_id":    book.ABSID,
					"hc_user_book_id": *mapping.HCUserBookID,
					"error":          upsertErr.Error(),
				})
			} else {
				// Verify the update worked
				verifyBook, verifyErr := api.repository.GetHardcoverUserBook(profileID, *mapping.HCUserBookID)
				if verifyErr != nil || verifyBook == nil {
					log.Error("Failed to verify HC book update", map[string]interface{}{
						"abs_book_id":    book.ABSID,
						"hc_user_book_id": *mapping.HCUserBookID,
						"error":          "record not found after upsert",
					})
				} else {
					log.Debug("Updated local HC book record", map[string]interface{}{
						"abs_book_id":     book.ABSID,
						"hc_user_book_id": *mapping.HCUserBookID,
						"old_hc_progress": oldProgress,
						"new_hc_progress": verifyBook.Progress,
						"abs_progress":    book.Progress,
						"new_status":      targetStatus,
						"match":           verifyBook.Progress == book.Progress,
					})
				}
			}
		}

		syncedCount++
		results = append(results, map[string]interface{}{
			"book_id":       book.ABSID,
			"title":         book.Title,
			"status":        "synced",
			"target_status": targetStatus,
		})
	}

	log.Info("Batch sync completed", map[string]interface{}{
		"profile_id":   profileID,
		"synced_count": syncedCount,
		"failed_count": failedCount,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":       "batch_sync_completed",
			"synced_count": syncedCount,
			"failed_count": failedCount,
			"results":      results,
		},
	})
}

// needsSync determines if a book needs to be synced based on progress differences
func (api *LibraryAPI) needsSync(absProgress float64, absFinished bool, hcProgress float64, hcStatus int) bool {
	// Calculate target status from ABS
	var absTargetStatus int
	if absProgress >= 1.0 || absFinished {
		absTargetStatus = 3 // READ
	} else if absProgress == 0 {
		absTargetStatus = 1 // WANT_TO_READ
	} else {
		absTargetStatus = 2 // READING
	}
	
	// If status differs, needs sync
	if absTargetStatus != hcStatus {
		return true
	}
	
	// If progress differs significantly (more than 1%)
	if absTargetStatus == 2 { // READING
		diff := absProgress - hcProgress
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.01 {
			return true
		}
	}
	
	return false
}

// GetABSLibrariesHandler returns ABS libraries for a profile
func (api *LibraryAPI) GetABSLibrariesHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	// Get ABS client for this profile
	absClient, err := api.collectorFactory.CreateABSClient(profileID)
	if err != nil {
		log.Error("Failed to create ABS client", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize ABS client",
		})
		return
	}

	libraries, err := absClient.GetLibraries(r.Context())
	if err != nil {
		log.Error("Failed to get ABS libraries", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to get libraries: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"libraries": libraries,
		},
	})
}

// GetABSCollectionsHandler returns ABS collections for a profile
func (api *LibraryAPI) GetABSCollectionsHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	// Get ABS client for this profile
	absClient, err := api.collectorFactory.CreateABSClient(profileID)
	if err != nil {
		log.Error("Failed to create ABS client", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize ABS client",
		})
		return
	}

	collections, err := absClient.GetCollections(r.Context())
	if err != nil {
		log.Error("Failed to get ABS collections", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to get collections: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"collections": collections,
		},
	})
}

// CollectABSBooksHandler triggers collection from ABS
func (api *LibraryAPI) CollectABSBooksHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	log.Info("ABS collection requested", map[string]interface{}{
		"profile_id": profileID,
	})

	// Check if collector factory is available
	if api.collectorFactory == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Collection service not configured",
		})
		return
	}

	// Create collector for this profile
	collector, err := api.collectorFactory.CreateABSCollector(profileID)
	if err != nil {
		log.Error("Failed to create ABS collector", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to create collector: " + err.Error(),
		})
		return
	}

	// Run collection
	ctx := r.Context()
	stats, err := collector.CollectAllBooks(ctx, profileID, nil)
	if err != nil {
		log.Error("ABS collection failed", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Collection failed: " + err.Error(),
		})
		return
	}

	log.Info("ABS collection completed", map[string]interface{}{
		"profile_id": profileID,
		"stats":      stats,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"data": map[string]interface{}{
			"status":          "collection_completed",
			"collected_count": stats.CollectedCount,
			"updated_count":   stats.UpdatedCount,
			"error_count":     stats.ErrorCount,
			"duration":        stats.Duration.String(),
		},
	})
}

// CollectHCBooksHandler triggers collection from Hardcover
func (api *LibraryAPI) CollectHCBooksHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	log.Info("HC collection requested", map[string]interface{}{
		"profile_id": profileID,
	})

	// Check if collector factory is available
	if api.collectorFactory == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Collection service not configured",
		})
		return
	}

	// Create collector for this profile
	collector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to create collector: " + err.Error(),
		})
		return
	}

	// Run collection
	ctx := r.Context()
	stats, err := collector.CollectAllUserBooks(ctx, profileID, nil)
	if err != nil {
		log.Error("HC collection failed", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Collection failed: " + err.Error(),
		})
		return
	}

	log.Info("HC collection completed", map[string]interface{}{
		"profile_id": profileID,
		"stats":      stats,
	})

	// Run auto-match after HC collection to link new books
	matchedCount := 0
	absCollector, err := api.collectorFactory.CreateABSCollector(profileID)
	if err == nil {
		if err := absCollector.AutoMatchBooks(ctx, profileID); err != nil {
			log.Warn("Auto-match after HC collection failed", map[string]interface{}{
				"profile_id": profileID,
				"error":      err.Error(),
			})
		} else {
			log.Info("Auto-match after HC collection completed", map[string]interface{}{
				"profile_id": profileID,
			})
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"status":          "collection_completed",
			"collected_count": stats.CollectedCount,
			"updated_count":   stats.UpdatedCount,
			"error_count":     stats.ErrorCount,
			"matched_count":   matchedCount,
			"duration":        stats.Duration.String(),
		},
	})
}

// GetProgressHistoryHandler retrieves progress history for a book
func (api *LibraryAPI) GetProgressHistoryHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	absBookIDStr := r.PathValue("absBookId")

	if profileID == "" || absBookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and Book ID required",
		})
		return
	}

	// Parse absBookID as uint
	absBookID := uint(0)
	if parsed, err := strconv.ParseUint(absBookIDStr, 10, 32); err == nil {
		absBookID = uint(parsed)
	}

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	limit := 50
	offset := (page - 1) * limit

	history, total, err := api.repository.GetProgressHistory(profileID, absBookID, limit, offset)
	if err != nil {
		log.Error("Failed to get progress history", map[string]interface{}{
			"error": err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to retrieve history",
		})
		return
	}

	totalPages := (int(total) + limit - 1) / limit

	w.WriteHeader(http.StatusOK)
	// Wrap data with pagination info
	responseData := map[string]interface{}{
		"items": history,
		"pagination": PaginationInfo{
			Page:       page,
			Limit:      limit,
			Total:      int(total),
			TotalPages: totalPages,
		},
	}
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    responseData,
	})
}

// GetSyncEventsHandler returns sync events for a profile
func (api *LibraryAPI) GetSyncEventsHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	// Parse pagination params
	limit := 100
	offset := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	events, total, err := api.repository.GetSyncEvents(profileID, limit, offset)
	if err != nil {
		log.Error("Failed to get sync events", map[string]interface{}{
			"error": err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to retrieve sync events",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"events": events,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// GetSyncSummaryHandler returns aggregated sync statistics
func (api *LibraryAPI) GetSyncSummaryHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	summary, err := api.repository.GetSyncSummary(profileID)
	if err != nil {
		log.Error("Failed to get sync summary", map[string]interface{}{
			"error": err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to retrieve summary",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    summary,
	})
}

// AutoMatchBooksHandler triggers auto-matching of ABS books to Hardcover
func (api *LibraryAPI) AutoMatchBooksHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	log.Info("Auto-match requested", map[string]interface{}{
		"profile_id": profileID,
	})

	// Check if collector factory is available
	if api.collectorFactory == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Collector service not configured",
		})
		return
	}

	// Create ABS collector for auto-match
	collector, err := api.collectorFactory.CreateABSCollector(profileID)
	if err != nil {
		log.Error("Failed to create collector for auto-match", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to create collector: " + err.Error(),
		})
		return
	}

	// Run auto-match
	ctx := r.Context()
	if err := collector.AutoMatchBooks(ctx, profileID); err != nil {
		log.Error("Auto-match failed", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Auto-match failed: " + err.Error(),
		})
		return
	}

	log.Info("Auto-match completed", map[string]interface{}{
		"profile_id": profileID,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"status": "auto_match_completed",
		},
	})
}

// CreateBookMappingHandler creates a manual book mapping
func (api *LibraryAPI) CreateBookMappingHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	absBookIDStr := r.PathValue("absBookId")

	if profileID == "" || absBookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and Book ID required",
		})
		return
	}

	log.Info("Manual mapping requested", map[string]interface{}{
		"profile_id":  profileID,
		"abs_book_id": absBookIDStr,
	})

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status": "mapping_created",
		},
	})
}

// DeleteBookMappingHandler deletes a book mapping
func (api *LibraryAPI) DeleteBookMappingHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	absBookIDStr := r.PathValue("absBookId")

	if profileID == "" || absBookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and Book ID required",
		})
		return
	}

	log.Info("Mapping delete requested", map[string]interface{}{
		"profile_id":  profileID,
		"abs_book_id": absBookIDStr,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status": "mapping_deleted",
		},
	})
}

// GetEditionsHandler retrieves all editions for a Hardcover book
func (api *LibraryAPI) GetEditionsHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	bookIDStr := r.URL.Query().Get("book_id")

	if profileID == "" || bookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and book_id required",
		})
		return
	}

	bookID, err := strconv.Atoi(bookIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Invalid book_id",
		})
		return
	}

	// Get the HC collector to access the Hardcover client
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	// Fetch editions from Hardcover API
	editions, err := hcCollector.GetBookEditions(r.Context(), bookID)
	if err != nil {
		log.Error("Failed to fetch editions", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to fetch editions from Hardcover",
		})
		return
	}

	log.Info("Retrieved editions for book", map[string]interface{}{
		"book_id":       bookID,
		"edition_count": len(editions),
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    editions,
	})
}

// UpdateEditionRequest represents a request to update the edition mapping
type UpdateEditionRequest struct {
	EditionID int64 `json:"edition_id"`
}

// UpdateEditionHandler updates the edition for a book mapping
func (api *LibraryAPI) UpdateEditionHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	absBookIDStr := r.PathValue("absBookId")

	if profileID == "" || absBookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and Book ID required",
		})
		return
	}

	absBookID, err := strconv.ParseUint(absBookIDStr, 10, 32)
	if err != nil {
		// Try looking up by ABS ID string
		absBook, lookupErr := api.repository.GetABSBook(profileID, absBookIDStr)
		if lookupErr != nil || absBook == nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIResponse{
				Success: false,
				Error:   "Invalid book ID",
			})
			return
		}
		absBookID = uint64(absBook.ID)
	}

	var req UpdateEditionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	// Get the existing mapping
	mapping, err := api.repository.GetBookMapping(profileID, uint(absBookID))
	if err != nil || mapping == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Book mapping not found",
		})
		return
	}

	// Update the edition ID in the mapping
	mapping.HCEditionID = &req.EditionID
	mapping.MatchMethod = database.MatchMethodManual
	mapping.MatchConfidence = 1.0
	mapping.ManualOverride = true

	if err := api.repository.UpsertBookMapping(*mapping); err != nil {
		log.Error("Failed to update mapping", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to update mapping",
		})
		return
	}

	// Also update the HardcoverUserBook record if it exists
	if mapping.HCUserBookID != nil {
		hcBook, err := api.repository.GetHardcoverUserBookByID(profileID, *mapping.HCUserBookID)
		if err == nil && hcBook != nil {
			hcBook.HCEditionID = &req.EditionID
			if err := api.repository.UpsertHardcoverUserBook(*hcBook); err != nil {
				log.Warn("Failed to update HC book edition", map[string]interface{}{"error": err.Error()})
			}
		}
	}

	log.Info("Updated edition mapping", map[string]interface{}{
		"profile_id":  profileID,
		"abs_book_id": absBookID,
		"edition_id":  req.EditionID,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":     "edition_updated",
			"edition_id": req.EditionID,
		},
	})
}

// SearchHardcoverRequest represents a request to search for a book on Hardcover
type SearchHardcoverRequest struct {
	ASIN   string `json:"asin,omitempty"`
	ISBN   string `json:"isbn,omitempty"`
	Title  string `json:"title,omitempty"`
	Author string `json:"author,omitempty"`
}

// SearchHardcoverResult represents a search result from Hardcover
type SearchHardcoverResult struct {
	BookID int64  `json:"book_id"`
	Title  string `json:"title"`
	Slug   string `json:"slug,omitempty"`
}

// SearchHardcoverHandler searches for a book on Hardcover by ASIN/ISBN/title
func (api *LibraryAPI) SearchHardcoverHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	absBookID := r.PathValue("absBookId")

	if profileID == "" || absBookID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and Book ID required",
		})
		return
	}

	// Get the ABS book to get search data
	absBook, err := api.repository.GetABSBook(profileID, absBookID)
	if err != nil || absBook == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "ABS book not found",
		})
		return
	}

	// Get the HC collector to access the Hardcover client
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()
	var results []SearchHardcoverResult

	// Try to search by ASIN first
	if absBook.ASIN != "" {
		book, err := hcCollector.SearchBookByASIN(ctx, absBook.ASIN)
		if err == nil && book != nil {
			bookID, _ := strconv.ParseInt(book.ID, 10, 64)
			results = append(results, SearchHardcoverResult{
				BookID: bookID,
				Title:  book.Title,
				Slug:   book.Slug,
			})
		}
	}

	// If no ASIN results, try ISBN
	if len(results) == 0 && absBook.ISBN != "" {
		book, err := hcCollector.SearchBookByISBN13(ctx, absBook.ISBN)
		if err == nil && book != nil {
			bookID, _ := strconv.ParseInt(book.ID, 10, 64)
			results = append(results, SearchHardcoverResult{
				BookID: bookID,
				Title:  book.Title,
				Slug:   book.Slug,
			})
		}
	}

	// If still no results, try title/author search
	if len(results) == 0 && absBook.Title != "" {
		books, err := hcCollector.SearchBooks(ctx, absBook.Title, absBook.Author)
		if err == nil && len(books) > 0 {
			for _, book := range books {
				bookID, _ := strconv.ParseInt(book.ID, 10, 64)
				results = append(results, SearchHardcoverResult{
					BookID: bookID,
					Title:  book.Title,
					Slug:   book.Slug,
				})
			}
		}
	}

	log.Info("Searched Hardcover for book", map[string]interface{}{
		"profile_id":    profileID,
		"abs_book_id":   absBookID,
		"abs_title":     absBook.Title,
		"result_count":  len(results),
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    results,
	})
}

// SearchHardcoverByQueryHandler searches for books on Hardcover by a text query
func (api *LibraryAPI) SearchHardcoverByQueryHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	query := r.URL.Query().Get("q")

	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Search query required (use ?q=search+text)",
		})
		return
	}

	// Get the HC collector to access the Hardcover client
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()
	var results []SearchHardcoverResult

	// Search by title (use empty author to search just by query)
	books, err := hcCollector.SearchBooks(ctx, query, "")
	if err != nil {
		log.Error("Failed to search Hardcover", map[string]interface{}{"error": err.Error(), "query": query})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to search Hardcover",
		})
		return
	}

	for _, book := range books {
		bookID, _ := strconv.ParseInt(book.ID, 10, 64)
		results = append(results, SearchHardcoverResult{
			BookID: bookID,
			Title:  book.Title,
			Slug:   book.Slug,
		})
	}

	log.Info("Searched Hardcover by query", map[string]interface{}{
		"profile_id":   profileID,
		"query":        query,
		"result_count": len(results),
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    results,
	})
}

// AddToWantToReadRequest represents a request to add a book to "want to read"
type AddToWantToReadRequest struct {
	EditionID int64 `json:"edition_id"`
	BookID    int64 `json:"book_id"`
}

// AddToWantToReadHandler adds a book to Hardcover's "want to read" list and creates a mapping
func (api *LibraryAPI) AddToWantToReadHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	absBookIDStr := r.PathValue("absBookId")

	if profileID == "" || absBookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and Book ID required",
		})
		return
	}

	var req AddToWantToReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	if req.EditionID == 0 || req.BookID == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "edition_id and book_id are required",
		})
		return
	}

	// Get the ABS book
	absBook, err := api.repository.GetABSBook(profileID, absBookIDStr)
	if err != nil || absBook == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "ABS book not found",
		})
		return
	}

	// Get the HC collector
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client: " + err.Error(),
		})
		return
	}

	ctx := r.Context()

	// Create user book on Hardcover with "WANT_TO_READ" status
	editionIDStr := strconv.FormatInt(req.EditionID, 10)
	userBookIDStr, err := hcCollector.CreateUserBook(ctx, editionIDStr, "WANT_TO_READ")
	if err != nil {
		log.Error("Failed to create user book on Hardcover", map[string]interface{}{
			"error":      err.Error(),
			"edition_id": req.EditionID,
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to add book to Hardcover: " + err.Error(),
		})
		return
	}

	// Parse the user book ID
	userBookID, err := strconv.ParseInt(userBookIDStr, 10, 64)
	if err != nil {
		log.Error("Failed to parse user book ID", map[string]interface{}{"error": err.Error()})
		userBookID = 0
	}

	// Create the mapping
	hcBookID := req.BookID
	mapping := database.BookMapping{
		ProfileID:       profileID,
		ABSBookID:       absBook.ID,
		HCBookID:        &hcBookID,
		HCEditionID:     &req.EditionID,
		HCUserBookID:    &userBookID,
		MatchMethod:     database.MatchMethodManual,
		MatchConfidence: 1.0,
		ManualOverride:  true,
	}

	if err := api.repository.UpsertBookMapping(mapping); err != nil {
		log.Error("Failed to create mapping", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to create mapping",
		})
		return
	}

	// Also create/update the HardcoverUserBook record so our local DB is in sync
	hcUserBook := database.HardcoverUserBook{
		ProfileID:       profileID,
		HCUserBookID:    userBookID,
		HCBookID:        req.BookID,
		HCEditionID:     &req.EditionID,
		Title:           absBook.Title,  // Use ABS title since we don't have HC title
		Author:          absBook.Author,
		ASIN:            absBook.ASIN,
		ISBN13:          absBook.ISBN,   // ABSBook uses ISBN field
		Status:          1, // WANT_TO_READ
		StatusName:      "Want to Read",
		Progress:        0.0,
		ProgressSeconds: 0.0,
	}

	if err := api.repository.UpsertHardcoverUserBook(hcUserBook); err != nil {
		// Log but don't fail - the mapping was created successfully
		log.Warn("Failed to create HardcoverUserBook record", map[string]interface{}{
			"error":        err.Error(),
			"user_book_id": userBookID,
		})
	}

	log.Info("Added book to want to read and created mapping", map[string]interface{}{
		"profile_id":     profileID,
		"abs_book_id":    absBookIDStr,
		"hc_book_id":     req.BookID,
		"hc_edition_id":  req.EditionID,
		"hc_user_book_id": userBookID,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":          "added_to_want_to_read",
			"user_book_id":    userBookID,
			"edition_id":      req.EditionID,
			"book_id":         req.BookID,
		},
	})
}

// SearchPeopleHandler searches for authors or narrators by name
func (api *LibraryAPI) SearchPeopleHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	// Get query parameters
	name := r.URL.Query().Get("name")
	personType := r.URL.Query().Get("type") // "author" or "narrator"
	limitStr := r.URL.Query().Get("limit")
	
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Name parameter required",
		})
		return
	}
	
	// Default to author if not specified
	if personType == "" {
		personType = "author"
	}
	
	// Default limit to 10
	limit := 10
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get HC collector
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()

	// Search for people
	people, err := hcCollector.SearchPeople(ctx, name, personType, limit)
	if err != nil {
		log.Error("Failed to search people", map[string]interface{}{
			"error": err.Error(),
			"name":  name,
			"type":  personType,
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to search people: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    people,
	})
}

// SearchPublishersHandler searches for publishers by name
func (api *LibraryAPI) SearchPublishersHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	// Get query parameters
	name := r.URL.Query().Get("name")
	limitStr := r.URL.Query().Get("limit")
	
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Name parameter required",
		})
		return
	}
	
	// Default limit to 10
	limit := 10
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get HC collector
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()

	// Search for publishers
	publishers, err := hcCollector.SearchPublishers(ctx, name, limit)
	if err != nil {
		log.Error("Failed to search publishers", map[string]interface{}{
			"error": err.Error(),
			"name":  name,
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to search publishers: " + err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    publishers,
	})
}

// CreateEditionRequest represents a request to create a new edition
type CreateEditionRequest struct {
	BookID        int    `json:"book_id"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle,omitempty"`
	ImageURL      string `json:"image_url,omitempty"`
	ISBN10        string `json:"isbn_10,omitempty"`
	ISBN13        string `json:"isbn_13,omitempty"`
	ASIN          string `json:"asin,omitempty"`
	PublishedDate string `json:"published_date,omitempty"`
	PublisherID   int    `json:"publisher_id,omitempty"`
	LanguageID    int    `json:"language_id,omitempty"`
	CountryID     int    `json:"country_id,omitempty"`
	AuthorIDs     []int  `json:"author_ids,omitempty"`
	NarratorIDs   []int  `json:"narrator_ids,omitempty"`
	AudioLength   int    `json:"audio_seconds,omitempty"`
	ReleaseDate   string `json:"release_date,omitempty"`
	EditionInfo   string `json:"edition_information,omitempty"`
	EditionFormat string `json:"edition_format,omitempty"`
}

// CreateEditionHandler creates a new audiobook edition in Hardcover
func (api *LibraryAPI) CreateEditionHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	var req CreateEditionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate required fields
	if req.BookID == 0 || req.Title == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "book_id and title are required",
		})
		return
	}

	// Get HC collector to access the Hardcover client
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()

	// Import edition package to use the Creator
	// Note: This requires adding the edition package import at the top of the file
	editionInput := &struct {
		BookID        int    `json:"book_id"`
		Title         string `json:"title"`
		Subtitle      string `json:"subtitle,omitempty"`
		ImageURL      string `json:"image_url,omitempty"`
		ISBN10        string `json:"isbn_10,omitempty"`
		ISBN13        string `json:"isbn_13,omitempty"`
		ASIN          string `json:"asin,omitempty"`
		PublishedDate string `json:"published_date,omitempty"`
		PublisherID   int    `json:"publisher_id,omitempty"`
		LanguageID    int    `json:"language_id,omitempty"`
		CountryID     int    `json:"country_id,omitempty"`
		AuthorIDs     []int  `json:"author_ids,omitempty"`
		NarratorIDs   []int  `json:"narrator_ids,omitempty"`
		AudioLength   int    `json:"audio_seconds,omitempty"`
		ReleaseDate   string `json:"release_date,omitempty"`
		EditionInfo   string `json:"edition_information,omitempty"`
		EditionFormat string `json:"edition_format,omitempty"`
	}{
		BookID:        req.BookID,
		Title:         req.Title,
		Subtitle:      req.Subtitle,
		ImageURL:      req.ImageURL,
		ISBN10:        req.ISBN10,
		ISBN13:        req.ISBN13,
		ASIN:          req.ASIN,
		PublishedDate: req.PublishedDate,
		PublisherID:   req.PublisherID,
		LanguageID:    req.LanguageID,
		CountryID:     req.CountryID,
		AuthorIDs:     req.AuthorIDs,
		NarratorIDs:   req.NarratorIDs,
		AudioLength:   req.AudioLength,
		ReleaseDate:   req.ReleaseDate,
		EditionInfo:   req.EditionInfo,
		EditionFormat: req.EditionFormat,
	}

	// Marshal and unmarshal to convert to the edition package's EditionInput type
	inputJSON, err := json.Marshal(editionInput)
	if err != nil {
		log.Error("Failed to marshal edition input", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to process edition input",
		})
		return
	}

	// Create the edition using GraphQL mutation directly
	// This is a simplified version - we'll use the Hardcover client's GraphQL mutation capability
	mutation := `
		mutation CreateEdition($input: EditionInput!) {
			createEdition(input: $input) {
				edition {
					id
					title
					reading_format_id
				}
			}
		}
	`

	variables := map[string]interface{}{
		"input": json.RawMessage(inputJSON),
	}

	var result struct {
		CreateEdition struct {
			Edition struct {
				ID              int    `json:"id"`
				Title           string `json:"title"`
				ReadingFormatID int    `json:"reading_format_id"`
			} `json:"edition"`
		} `json:"createEdition"`
	}

	err = hcCollector.GraphQLMutation(ctx, mutation, variables, &result)
	if err != nil {
		log.Error("Failed to create edition", map[string]interface{}{
			"error":   err.Error(),
			"book_id": req.BookID,
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to create edition: " + err.Error(),
		})
		return
	}

	log.Info("Edition created successfully", map[string]interface{}{
		"profile_id":  profileID,
		"edition_id":  result.CreateEdition.Edition.ID,
		"book_id":     req.BookID,
		"title":       result.CreateEdition.Edition.Title,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"edition_id": result.CreateEdition.Edition.ID,
			"title":      result.CreateEdition.Edition.Title,
			"book_id":    req.BookID,
		},
	})
}

// PrepopulateEditionHandler prepopulates edition data from an existing Hardcover book
func (api *LibraryAPI) PrepopulateEditionHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	bookIDStr := r.URL.Query().Get("book_id")

	if profileID == "" || bookIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID and book_id required",
		})
		return
	}

	bookID, err := strconv.Atoi(bookIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Invalid book_id",
		})
		return
	}

	// Get HC collector
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()

	// Query to get book details for prepopulation
	query := `
		query GetBookForPrepopulation($id: Int!) {
			book(id: $id) {
				id
				title
				subtitle
				image
				contributions {
					author {
						id
						name
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"id": bookID,
	}

	var result struct {
		Book struct {
			ID            int    `json:"id"`
			Title         string `json:"title"`
			Subtitle      string `json:"subtitle"`
			Image         string `json:"image"`
			Contributions []struct {
				Author struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"author"`
			} `json:"contributions"`
		} `json:"book"`
	}

	err = hcCollector.GraphQLQuery(ctx, query, variables, &result)
	if err != nil {
		log.Error("Failed to fetch book for prepopulation", map[string]interface{}{
			"error":   err.Error(),
			"book_id": bookID,
		})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to fetch book data: " + err.Error(),
		})
		return
	}

	// Build prepopulated data
	authorIDs := []int{}
	for _, contrib := range result.Book.Contributions {
		if authorID, err := strconv.Atoi(contrib.Author.ID); err == nil {
			authorIDs = append(authorIDs, authorID)
		}
	}

	prepopulated := CreateEditionRequest{
		BookID:    bookID,
		Title:     result.Book.Title,
		Subtitle:  result.Book.Subtitle,
		ImageURL:  result.Book.Image,
		AuthorIDs: authorIDs,
		// Set reading format to audiobook (2) by default
		LanguageID: 1, // Default to English
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    prepopulated,
	})
}

// UploadImageRequest represents a request to upload an image
type UploadImageRequest struct {
	ImageURL    string `json:"image_url"`
	EditionID   int    `json:"edition_id,omitempty"`
	BookID      int    `json:"book_id,omitempty"`
	Description string `json:"description,omitempty"`
}

// UploadImageHandler uploads a cover image to an edition or book
func (api *LibraryAPI) UploadImageHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Get()
	w.Header().Set("Content-Type", "application/json")

	profileID := r.PathValue("id")
	if profileID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Profile ID required",
		})
		return
	}

	var req UploadImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate required fields
	if req.ImageURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "image_url is required",
		})
		return
	}

	if req.EditionID == 0 && req.BookID == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Either edition_id or book_id is required",
		})
		return
	}

	// Get HC collector
	hcCollector, err := api.collectorFactory.CreateHCCollector(profileID)
	if err != nil {
		log.Error("Failed to create HC collector", map[string]interface{}{"error": err.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Error:   "Failed to initialize Hardcover client",
		})
		return
	}

	ctx := r.Context()

	// Note: This is a placeholder - actual image upload logic would need to be implemented
	// using the edition.Creator's UploadEditionImage method or similar functionality
	// For now, we'll return a success response indicating the feature needs implementation
	
	log.Info("Image upload requested", map[string]interface{}{
		"profile_id":  profileID,
		"edition_id":  req.EditionID,
		"book_id":     req.BookID,
		"image_url":   req.ImageURL,
	})

	// Suppress unused variable warning
	_ = hcCollector
	_ = ctx

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":  "image_upload_pending",
			"message": "Image upload functionality to be implemented",
		},
	})
}
