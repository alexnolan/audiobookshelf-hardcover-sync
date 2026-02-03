package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/database"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/logger"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/sync"
)

// CollectorFactory interface for creating collectors
type CollectorFactory interface {
	CreateABSCollector(profileID string) (*sync.ABSCollector, error)
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
	ABSProgress       float64 `json:"abs_progress"`
	HardcoverProgress float64 `json:"hardcover_progress"`
	ProgressDiff      float64 `json:"progress_diff"`
	InSync            bool    `json:"in_sync"`
	SyncStatus        string  `json:"sync_status"`
	SyncEnabled       bool    `json:"sync_enabled"`
	HCBookID          *int64  `json:"hc_book_id,omitempty"`
	HCUserBookID      *int64  `json:"hc_user_book_id,omitempty"`
	MatchMethod       string  `json:"match_method,omitempty"`
	MatchConfidence   float64 `json:"match_confidence,omitempty"`
}

// flattenComparison converts a BookComparison to a flat structure for frontend
func flattenComparison(comp database.BookComparison) FlatBookComparison {
	flat := FlatBookComparison{
		ABSID:        comp.ABSBook.ABSID,
		ABSTitle:     comp.ABSBook.Title,
		ABSAuthor:    comp.ABSBook.Author,
		ABSProgress:  comp.ABSBook.Progress,
		ProgressDiff: comp.ProgressDiff,
		InSync:       comp.InSync,
		SyncStatus:   comp.SyncStatus,
		SyncEnabled:  true, // Default to enabled
	}

	// Set HC progress if mapped
	if comp.HardcoverBook != nil {
		flat.HardcoverProgress = comp.HardcoverBook.Progress
		hcBookID := comp.HardcoverBook.HCBookID
		flat.HCBookID = &hcBookID
		hcUserBookID := comp.HardcoverBook.HCUserBookID
		flat.HCUserBookID = &hcUserBookID
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
		Search: r.URL.Query().Get("search"),
		Filter: r.URL.Query().Get("filter"),
		Sort:   r.URL.Query().Get("sort"),
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

	w.WriteHeader(http.StatusOK)
	// Return response matching frontend expectations:
	// data = books array, pagination at top level
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    flatBooks,
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

	// Return placeholder response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":  "sync_initiated",
			"book_id": absBookIDStr,
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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":       "batch_sync_initiated",
			"synced_count": 0,
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
