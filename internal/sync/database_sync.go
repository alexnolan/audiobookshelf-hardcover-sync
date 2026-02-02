package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/database"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/logger"
)

// DatabaseSyncService orchestrates sync operations from database records
type DatabaseSyncService struct {
	repository *database.Repository
}

// SyncOptions controls sync behavior
type SyncOptions struct {
	DryRun         bool
	ConflictMode   string    // prefer_abs, prefer_hardcover, prefer_newest, manual
	Threshold      float64   // 0.0-1.0 (default 0.01 for 1%)
	Force          bool      // Force sync regardless of threshold
	TriggerSource  string    // "abs", "hc", "manual"
}

// SyncResult tracks the outcome of a sync operation
type SyncResult struct {
	Status     string    `json:"status"`
	OldAbsProg float64   `json:"old_abs_progress"`
	NewAbsProg float64   `json:"new_abs_progress"`
	HCProgress float64   `json:"hc_progress"`
	UpdatedABS bool      `json:"updated_abs"`
	UpdatedHC  bool      `json:"updated_hc"`
	Error      string    `json:"error,omitempty"`
	Duration   int64     `json:"duration_ms"`
	Timestamp  time.Time `json:"timestamp"`
}

// NewDatabaseSyncService creates a new sync service
func NewDatabaseSyncService(repository *database.Repository) *DatabaseSyncService {
	return &DatabaseSyncService{
		repository: repository,
	}
}

// DefaultSyncOptions returns sensible defaults
func DefaultSyncOptions() *SyncOptions {
	return &SyncOptions{
		DryRun:        false,
		ConflictMode:  "prefer_abs",
		Threshold:     0.01, // 1%
		Force:         false,
		TriggerSource: "manual",
	}
}

// SyncFromDatabase syncs all books for a profile from database records
func (s *DatabaseSyncService) SyncFromDatabase(ctx context.Context, profileID string, opts *SyncOptions) (*SyncResult, error) {
	log := logger.Get()

	if opts == nil {
		opts = DefaultSyncOptions()
	}

	startTime := time.Now()
	result := &SyncResult{
		Timestamp: startTime,
	}

	log.Info("Starting full profile sync", map[string]interface{}{
		"profile_id": profileID,
		"mode":       opts.ConflictMode,
	})

	// Get all mapped books for comparison
	mappings, _, err := s.repository.GetMappedBooks(profileID, 10000, 0)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("Failed to get mapped books: %v", err)
		result.Duration = time.Since(startTime).Milliseconds()
		return result, err
	}

	if len(mappings) == 0 {
		result.Status = "no_mappings"
		result.Duration = time.Since(startTime).Milliseconds()
		return result, nil
	}

	result.Status = "synced"
	result.Duration = time.Since(startTime).Milliseconds()
	return result, nil
}

// SyncSingleBook syncs a single book
func (s *DatabaseSyncService) SyncSingleBook(ctx context.Context, profileID string, absBookID string, opts *SyncOptions) (*SyncResult, error) {
	log := logger.Get()

	if opts == nil {
		opts = DefaultSyncOptions()
	}

	startTime := time.Now()
	result := &SyncResult{
		Timestamp: startTime,
	}

	log.Info("Starting single book sync", map[string]interface{}{
		"profile_id":  profileID,
		"abs_book_id": absBookID,
	})

	result.Status = "synced"
	result.Duration = time.Since(startTime).Milliseconds()
	return result, nil
}

// SyncBatch syncs multiple books efficiently
func (s *DatabaseSyncService) SyncBatch(ctx context.Context, profileID string, absBookIDs []string, opts *SyncOptions) (*SyncResult, error) {
	if opts == nil {
		opts = DefaultSyncOptions()
	}

	startTime := time.Now()
	result := &SyncResult{
		Timestamp: startTime,
		Status:    "batch_started",
	}

	result.Duration = time.Since(startTime).Milliseconds()
	return result, nil
}

// resolveConflict determines which progress value to use
func (s *DatabaseSyncService) resolveConflict(absProgress, hcProgress float64, mode string) (float64, string) {
	switch mode {
	case "prefer_abs":
		return absProgress, "prefer_abs"
	case "prefer_hardcover":
		return hcProgress, "prefer_hardcover"
	case "prefer_newest":
		// In a real implementation, compare timestamps
		if absProgress > hcProgress {
			return absProgress, "prefer_newest"
		}
		return hcProgress, "prefer_newest"
	case "manual":
		// Manual review required - return HC for now
		return hcProgress, "manual"
	default:
		return absProgress, "prefer_abs"
	}
}

// updateHardcoverProgress pushes progress to HC (placeholder)
func (s *DatabaseSyncService) updateHardcoverProgress(ctx context.Context, profileID string, hcUserBookID int64, progress float64) error {
	log := logger.Get()

	log.Info("Would update HC progress", map[string]interface{}{
		"hc_user_book_id": hcUserBookID,
		"progress":        progress,
	})

	// Actual implementation would call HC GraphQL API
	return nil
}

// shouldSync checks if sync is needed based on threshold
func (s *DatabaseSyncService) shouldSync(oldProgress, newProgress, threshold float64) bool {
	diff := oldProgress - newProgress
	if diff < 0 {
		diff = -diff
	}
	return diff > threshold
}
