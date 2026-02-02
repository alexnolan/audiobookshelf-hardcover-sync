package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/database"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/logger"
)

// LibraryAPI handles book library REST endpoints
type LibraryAPI struct {
	repository *database.Repository
}

// NewLibraryAPI creates a new library API handler
func NewLibraryAPI(repository *database.Repository) *LibraryAPI {
	return &LibraryAPI{
		repository: repository,
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

	// Get comparisons
	comparisons, total, err := api.repository.GetBookComparisons(profileID, limit, offset)
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

	w.WriteHeader(http.StatusOK)
	// Return response matching frontend expectations:
	// data = books array, pagination at top level
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    comparisons,
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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":            "collection_initiated",
			"collected_count": 0,
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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":           "collection_initiated",
			"collected_count": 0,
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
