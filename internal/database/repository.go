package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/crypto"
	"github.com/drallgood/audiobookshelf-hardcover-sync/internal/logger"
)

// Repository provides database operations for users and configurations
type Repository struct {
	db        *Database
	encryptor *crypto.EncryptionManager
	logger    *logger.Logger
}

// NewRepository creates a new repository instance
func NewRepository(db *Database, encryptor *crypto.EncryptionManager, log *logger.Logger) *Repository {
	return &Repository{
		db:        db,
		encryptor: encryptor,
		logger:    log,
	}
}

// ProfileWithTokens represents a sync profile with decrypted tokens
type ProfileWithTokens struct {
	Profile             SyncProfile    `json:"profile"`
	AudiobookshelfURL   string         `json:"audiobookshelf_url"`
	AudiobookshelfToken string         `json:"audiobookshelf_token"`
	HardcoverToken      string         `json:"hardcover_token"`
	SyncConfig          SyncConfigData `json:"sync_config"`
}

// CreateProfile creates a new sync profile with encrypted configuration
func (r *Repository) CreateProfile(profileID, name, audiobookshelfURL, audiobookshelfToken, hardcoverToken string, syncConfig SyncConfigData) error {
	// Encrypt tokens
	encryptedABSToken, err := r.encryptor.Encrypt(audiobookshelfToken)
	if err != nil {
		r.logger.Error("Failed to encrypt Audiobookshelf token", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		return fmt.Errorf("failed to encrypt Audiobookshelf token: %w", err)
	}

	encryptedHCToken, err := r.encryptor.Encrypt(hardcoverToken)
	if err != nil {
		r.logger.Error("Failed to encrypt Hardcover token", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		return fmt.Errorf("failed to encrypt Hardcover token: %w", err)
	}

	// Serialize sync config
	syncConfigJSON, err := json.Marshal(syncConfig)
	if err != nil {
		r.logger.Error("Failed to marshal sync config", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		return fmt.Errorf("failed to marshal sync config: %w", err)
	}

	// Create profile and config in a transaction
	return r.db.GetDB().Transaction(func(tx *gorm.DB) error {
		// Create profile
		profile := SyncProfile{
			ID:     profileID,
			Name:   name,
			Active: true,
		}
		if err := tx.Create(&profile).Error; err != nil {
			return fmt.Errorf("failed to create sync profile: %w", err)
		}

		// Create profile config
		config := SyncProfileConfig{
			ProfileID:                    profileID,
			AudiobookshelfURL:            audiobookshelfURL,
			AudiobookshelfTokenEncrypted: encryptedABSToken,
			HardcoverTokenEncrypted:      encryptedHCToken,
			SyncConfig:                   string(syncConfigJSON),
		}
		if err := tx.Create(&config).Error; err != nil {
			return fmt.Errorf("failed to create sync profile config: %w", err)
		}

		// Create empty sync state
		syncState := ProfileSyncState{
			ProfileID: profileID,
			StateData: "{}",
		}
		if err := tx.Create(&syncState).Error; err != nil {
			return fmt.Errorf("failed to create sync state: %w", err)
		}

		r.logger.Info("Created new user", map[string]interface{}{
			"user_id": profileID,
			"name":    name,
		})

		return nil
	})
}

// GetProfile retrieves a sync profile by ID with decrypted tokens
func (r *Repository) GetProfile(profileID string) (*ProfileWithTokens, error) {
	// Get profile with config
	var profile SyncProfile
	if err := r.db.GetDB().Preload("Config").Preload("SyncState").First(&profile, "id = ? AND active = ?", profileID, true).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get sync profile: %w", err)
	}

	if profile.Config == nil {
		return nil, fmt.Errorf("sync profile config not found")
	}

	// Decrypt tokens
	audiobookshelfToken, err := r.encryptor.Decrypt(profile.Config.AudiobookshelfTokenEncrypted)
	if err != nil {
		fields := map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		}
		if isLikelyEncryptionKeyMismatch(err) {
			fields["hint"] = "encryption key mismatch suspected; ensure ENCRYPTION_KEY, DATA_DIR, paths.data_dir and volume mounts are consistent with when tokens were created"
		}

		r.logger.Error("Failed to decrypt Audiobookshelf token", fields)
		return nil, fmt.Errorf("failed to decrypt Audiobookshelf token: %w", err)
	}

	hardcoverToken, err := r.encryptor.Decrypt(profile.Config.HardcoverTokenEncrypted)
	if err != nil {
		fields := map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		}
		if isLikelyEncryptionKeyMismatch(err) {
			fields["hint"] = "encryption key mismatch suspected; ensure ENCRYPTION_KEY, DATA_DIR, paths.data_dir and volume mounts are consistent with when tokens were created"
		}

		r.logger.Error("Failed to decrypt Hardcover token", fields)
		return nil, fmt.Errorf("failed to decrypt Hardcover token: %w", err)
	}

	// Parse sync config
	var syncConfig SyncConfigData
	if profile.Config.SyncConfig != "" {
		if err := json.Unmarshal([]byte(profile.Config.SyncConfig), &syncConfig); err != nil {
			r.logger.Error("Failed to parse sync config", map[string]interface{}{
				"profile_id": profileID,
				"error":      err.Error(),
			})
			return nil, fmt.Errorf("failed to parse sync config: %w", err)
		}
	}

	return &ProfileWithTokens{
		Profile:             profile,
		AudiobookshelfURL:   profile.Config.AudiobookshelfURL,
		AudiobookshelfToken: audiobookshelfToken,
		HardcoverToken:      hardcoverToken,
		SyncConfig:          syncConfig,
	}, nil
}

// ListProfiles retrieves all active sync profiles
func (r *Repository) ListProfiles() ([]SyncProfile, error) {
	var profiles []SyncProfile
	if err := r.db.GetDB().Preload("Config").Preload("SyncState").Where("active = ?", true).Find(&profiles).Error; err != nil {
		return nil, fmt.Errorf("failed to list sync profiles: %w", err)
	}
	return profiles, nil
}

// UpdateProfile updates sync profile information
func (r *Repository) UpdateProfile(profileID, name string) error {
	result := r.db.GetDB().Model(&SyncProfile{}).
		Where("id = ? AND active = ?", profileID, true).
		Updates(map[string]interface{}{
			"name":       name,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update sync profile: %w", result.Error)
	}

	return nil
}

// UpdateUserConfig updates user configuration with encrypted tokens
// If audiobookshelfToken or hardcoverToken are empty, the existing tokens will be preserved
func (r *Repository) UpdateUserConfig(profileID, audiobookshelfURL, audiobookshelfToken, hardcoverToken string, syncConfig SyncConfigData) error {
	// Get existing config to preserve tokens and sync config if not provided
	var existingConfig SyncProfileConfig
	if err := r.db.GetDB().Where("profile_id = ?", profileID).First(&existingConfig).Error; err != nil {
		return fmt.Errorf("failed to get existing user config: %w", err)
	}

	// Use existing tokens if new ones are empty
	var encryptedABSToken, encryptedHCToken string
	var err error

	if audiobookshelfToken != "" {
		encryptedABSToken, err = r.encryptor.Encrypt(audiobookshelfToken)
		if err != nil {
			return fmt.Errorf("failed to encrypt Audiobookshelf token: %w", err)
		}
	} else {
		encryptedABSToken = existingConfig.AudiobookshelfTokenEncrypted
	}

	if hardcoverToken != "" {
		encryptedHCToken, err = r.encryptor.Encrypt(hardcoverToken)
		if err != nil {
			return fmt.Errorf("failed to encrypt Hardcover token: %w", err)
		}
	} else {
		encryptedHCToken = existingConfig.HardcoverTokenEncrypted
	}

	// Merge with existing sync config to preserve values not being updated
	var existingSyncConfig SyncConfigData
	if existingConfig.SyncConfig != "" {
		if err := json.Unmarshal([]byte(existingConfig.SyncConfig), &existingSyncConfig); err != nil {
			return fmt.Errorf("failed to unmarshal existing sync config: %w", err)
		}
	}
	
	// Only update sync config if it's not empty (has at least one field set)
	// This prevents clearing all values when only updating tokens
	finalSyncConfig := existingSyncConfig
	if !syncConfig.IsEmpty() {
		// For boolean fields, always use the new value since false is a valid value
		finalSyncConfig.Incremental = syncConfig.Incremental
		finalSyncConfig.SyncWantToRead = syncConfig.SyncWantToRead
		finalSyncConfig.ProcessUnreadBooks = syncConfig.ProcessUnreadBooks
		finalSyncConfig.SyncOwned = syncConfig.SyncOwned
		finalSyncConfig.IncludeEbooks = syncConfig.IncludeEbooks
		finalSyncConfig.DryRun = syncConfig.DryRun
		
		// For string fields, update if provided
		if syncConfig.StateFile != "" {
			finalSyncConfig.StateFile = syncConfig.StateFile
		}
		if syncConfig.MinChangeThreshold != 0 {
			finalSyncConfig.MinChangeThreshold = syncConfig.MinChangeThreshold
		}
		// SyncInterval can be empty (disabled) so always set it
		finalSyncConfig.SyncInterval = syncConfig.SyncInterval
		if syncConfig.MinimumProgress != 0 {
			finalSyncConfig.MinimumProgress = syncConfig.MinimumProgress
		}
		if syncConfig.TestBookFilter != "" {
			finalSyncConfig.TestBookFilter = syncConfig.TestBookFilter
		}
		if syncConfig.TestBookLimit != 0 {
			finalSyncConfig.TestBookLimit = syncConfig.TestBookLimit
		}
		
		// Library filtering - always update these as empty arrays are valid (means "all")
		finalSyncConfig.Libraries.Include = syncConfig.Libraries.Include
		finalSyncConfig.Libraries.Exclude = syncConfig.Libraries.Exclude
		finalSyncConfig.LibraryFilterMode = syncConfig.LibraryFilterMode
		finalSyncConfig.FilteredLibraries = syncConfig.FilteredLibraries
		
		// Sync mode and collection settings - always update
		finalSyncConfig.SyncMode = syncConfig.SyncMode
		finalSyncConfig.SelectedCollections = syncConfig.SelectedCollections
		finalSyncConfig.IncludeCollections = syncConfig.IncludeCollections
		finalSyncConfig.ExcludeCollections = syncConfig.ExcludeCollections
	}

	// Serialize sync config
	syncConfigJSON, err := json.Marshal(finalSyncConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal sync config: %w", err)
	}

	// Update config
	updates := map[string]interface{}{
		"AudiobookshelfURL":            audiobookshelfURL,
		"AudiobookshelfTokenEncrypted": encryptedABSToken,
		"HardcoverTokenEncrypted":      encryptedHCToken,
		"SyncConfig":                   string(syncConfigJSON),
		"UpdatedAt":                    time.Now(),
	}

	// Only include fields that have values to avoid overwriting with zero values
	result := r.db.GetDB().Model(&SyncProfileConfig{}).Where("profile_id = ?", profileID).Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update user config: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user config not found: %s", profileID)
	}

	r.logger.Info("Updated user config", map[string]interface{}{
		"profile_id": profileID,
	})

	return nil
}

// DeleteProfile soft deletes a sync profile by setting active to false
func (r *Repository) DeleteProfile(profileID string) error {
	result := r.db.GetDB().Model(&SyncProfile{}).Where("id = ?", profileID).Update("active", false)
	if result.Error != nil {
		return fmt.Errorf("failed to delete sync profile: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("sync profile not found: %s", profileID)
	}

	r.logger.Info("Deleted sync profile", map[string]interface{}{
		"profile_id": profileID,
	})

	return nil
}

// GetSyncState retrieves the sync state for a sync profile
func (r *Repository) GetSyncState(profileID string) (*ProfileSyncState, error) {
	var state ProfileSyncState
	if err := r.db.GetDB().Where("profile_id = ?", profileID).First(&state).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return default state if not found
			now := time.Now()
			return &ProfileSyncState{
				ProfileID: profileID,
				StateData: "{}",
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		}
		return nil, fmt.Errorf("failed to get sync state: %w", err)
	}
	return &state, nil
}

// UpdateSyncState updates the sync state for a sync profile
func (r *Repository) UpdateSyncState(state *ProfileSyncState) error {
	state.UpdatedAt = time.Now()

	// Check if state exists
	var existingState ProfileSyncState
	result := r.db.GetDB().Where("profile_id = ?", state.ProfileID).First(&existingState)

	if result.Error == nil {
		// Update existing state - use the existing CreatedAt
		state.CreatedAt = existingState.CreatedAt

		// Update the existing record
		if err := r.db.GetDB().Model(&existingState).Updates(state).Error; err != nil {
			r.logger.Error("Failed to update sync state", map[string]interface{}{
				"profile_id": state.ProfileID,
				"error":      err.Error(),
			})
			return fmt.Errorf("failed to update sync state: %w", err)
		}
	} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// Create new state
		state.CreatedAt = time.Now()

		if err := r.db.GetDB().Create(state).Error; err != nil {
			r.logger.Error("Failed to create sync state", map[string]interface{}{
				"profile_id": state.ProfileID,
				"error":      err.Error(),
			})
			return fmt.Errorf("failed to create sync state: %w", err)
		}
	} else {
		return fmt.Errorf("failed to check for existing sync state: %w", result.Error)
	}

	r.logger.Info("Updated sync state", map[string]interface{}{
		"profile_id": state.ProfileID,
	})

	return nil
}

// UserExists checks if a sync profile exists and is active
func (r *Repository) UserExists(profileID string) (bool, error) {
	var count int64
	if err := r.db.GetDB().Model(&SyncProfile{}).Where("id = ? AND active = ?", profileID, true).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check profile existence: %w", err)
	}
	return count > 0, nil
}

// LogBookSync logs a book sync attempt or completion
func (r *Repository) LogBookSync(profileID, audiobookID, title, author, status, targetStatus string, progress float64, hardcoverID *int64, editionID *string, errorMsg string) error {
	now := time.Now()
	bookLog := BookSyncLog{
		ProfileID:    profileID,
		AudiobookID:  audiobookID,
		Title:        title,
		Author:       author,
		Status:       status,
		TargetStatus: targetStatus,
		Progress:     progress,
		HardcoverID:  hardcoverID,
		EditionID:    editionID,
		ErrorMessage: errorMsg,
		LastAttempt:  &now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// If status is SYNCED, also set SyncedAt
	if status == "SYNCED" {
		bookLog.SyncedAt = &now
	}

	// Try to update existing log first
	result := r.db.GetDB().Model(&BookSyncLog{}).
		Where("profile_id = ? AND audiobook_id = ?", profileID, audiobookID).
		Updates(bookLog)

	if result.Error != nil {
		r.logger.Error("Failed to log book sync", map[string]interface{}{
			"profile_id": profileID,
			"audiobook_id": audiobookID,
			"error": result.Error.Error(),
		})
		return fmt.Errorf("failed to log book sync: %w", result.Error)
	}

	// If no rows affected, create new record
	if result.RowsAffected == 0 {
		if err := r.db.GetDB().Create(&bookLog).Error; err != nil {
			r.logger.Error("Failed to create book sync log", map[string]interface{}{
				"profile_id": profileID,
				"audiobook_id": audiobookID,
				"error": err.Error(),
			})
			return fmt.Errorf("failed to create book sync log: %w", err)
		}
	}

	return nil
}

// GetBookSyncLogs gets sync logs for a profile
func (r *Repository) GetBookSyncLogs(profileID string, limit int, offset int) ([]BookSyncLog, int64, error) {
	var logs []BookSyncLog
	var total int64

	// Get total count
	if err := r.db.GetDB().Model(&BookSyncLog{}).Where("profile_id = ?", profileID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count book sync logs: %w", err)
	}

	// Get paginated results
	if err := r.db.GetDB().
		Where("profile_id = ?", profileID).
		Order("last_attempt DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get book sync logs: %w", err)
	}

	return logs, total, nil
}

// GetBookSyncLogsByStatus gets sync logs filtered by status
func (r *Repository) GetBookSyncLogsByStatus(profileID, status string, limit int) ([]BookSyncLog, error) {
	var logs []BookSyncLog

	if err := r.db.GetDB().
		Where("profile_id = ? AND status = ?", profileID, status).
		Order("last_attempt DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to get book sync logs by status: %w", err)
	}

	return logs, nil
}

// GetBookSyncLog gets a specific book sync log
func (r *Repository) GetBookSyncLog(profileID, audiobookID string) (*BookSyncLog, error) {
	var log BookSyncLog

	if err := r.db.GetDB().
		Where("profile_id = ? AND audiobook_id = ?", profileID, audiobookID).
		First(&log).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get book sync log: %w", err)
	}

	return &log, nil
}

// ClearOldBookSyncLogs deletes old book sync logs to prevent bloat
func (r *Repository) ClearOldBookSyncLogs(profileID string, daysOld int) error {
	cutoffTime := time.Now().AddDate(0, 0, -daysOld)

	if err := r.db.GetDB().
		Where("profile_id = ? AND updated_at < ?", profileID, cutoffTime).
		Delete(&BookSyncLog{}).Error; err != nil {
		return fmt.Errorf("failed to clear old book sync logs: %w", err)
	}

	return nil
}

// isLikelyEncryptionKeyMismatch checks if an error is likely due to encryption key mismatch
func isLikelyEncryptionKeyMismatch(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "cipher") || strings.Contains(errStr, "decrypt") || strings.Contains(errStr, "authentication")
}

// ============================================================================
// ABS BOOK REPOSITORY METHODS
// ============================================================================

// UpsertABSBook creates or updates an ABS book record
func (r *Repository) UpsertABSBook(book ABSBook) error {
	if book.ProfileID == "" || book.ABSID == "" {
		return errors.New("profile_id and abs_id are required")
	}

	now := time.Now()
	book.UpdatedAt = now
	book.LastFetchedAt = now

	// Use ON CONFLICT to upsert
	return r.db.GetDB().Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "profile_id"}, {Name: "abs_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "author", "narrator", "series_name", "asin", "isbn", "duration", "current_time", "progress", "is_finished", "started_at", "finished_at", "cover_path", "library_id", "last_fetched_at", "updated_at"}),
		},
	).Create(&book).Error
}

// UpsertABSBooks bulk upserts multiple ABS books
func (r *Repository) UpsertABSBooks(books []ABSBook) error {
	if len(books) == 0 {
		return nil
	}

	now := time.Now()
	for i := range books {
		books[i].UpdatedAt = now
		books[i].LastFetchedAt = now
	}

	clause := clause.OnConflict{
		Columns:   []clause.Column{{Name: "profile_id"}, {Name: "abs_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "author", "narrator", "series_name", "asin", "isbn", "duration", "current_time", "progress", "is_finished", "started_at", "finished_at", "cover_path", "library_id", "last_fetched_at", "updated_at"}),
	}
	return r.db.GetDB().Clauses(clause).CreateInBatches(books, 100).Error
}

// GetABSBook retrieves an ABS book by profile and ABS ID
func (r *Repository) GetABSBook(profileID, absID string) (*ABSBook, error) {
	var book ABSBook
	if err := r.db.GetDB().
		Where("profile_id = ? AND abs_id = ?", profileID, absID).
		Preload("Mapping").
		Preload("Config").
		First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &book, nil
}

// GetABSBookByID retrieves an ABS book by primary key
func (r *Repository) GetABSBookByID(id uint) (*ABSBook, error) {
	var book ABSBook
	if err := r.db.GetDB().
		Preload("Mapping").
		Preload("Config").
		First(&book, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &book, nil
}

// GetABSBooks retrieves all ABS books for a profile with pagination
func (r *Repository) GetABSBooks(profileID string, limit int, offset int) ([]ABSBook, int64, error) {
	var books []ABSBook
	var total int64

	if err := r.db.GetDB().Model(&ABSBook{}).Where("profile_id = ?", profileID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.GetDB().
		Where("profile_id = ?", profileID).
		Preload("Mapping").
		Preload("Config").
		Order("title ASC").
		Limit(limit).
		Offset(offset).
		Find(&books).Error; err != nil {
		return nil, 0, err
	}

	return books, total, nil
}

// GetABSBooksForProfile retrieves all ABS books for a profile (no pagination)
func (r *Repository) GetABSBooksForProfile(profileID string) ([]ABSBook, error) {
	var books []ABSBook
	if err := r.db.GetDB().
		Where("profile_id = ?", profileID).
		Find(&books).Error; err != nil {
		return nil, err
	}
	return books, nil
}

// GetABSBooksByLibrary retrieves all ABS books in a specific library
func (r *Repository) GetABSBooksByLibrary(profileID, libraryID string) ([]ABSBook, error) {
	var books []ABSBook
	if err := r.db.GetDB().
		Where("profile_id = ? AND library_id = ?", profileID, libraryID).
		Preload("Mapping").
		Preload("Config").
		Order("title ASC").
		Find(&books).Error; err != nil {
		return nil, err
	}
	return books, nil
}

// DeleteABSBook deletes an ABS book record
func (r *Repository) DeleteABSBook(profileID, absID string) error {
	return r.db.GetDB().Where("profile_id = ? AND abs_id = ?", profileID, absID).Delete(&ABSBook{}).Error
}

// DeleteABSBooksByProfile deletes all ABS books for a profile
func (r *Repository) DeleteABSBooksByProfile(profileID string) error {
	return r.db.GetDB().Where("profile_id = ?", profileID).Delete(&ABSBook{}).Error
}

// DeleteABSEbooksByProfile deletes all ebooks for a profile (media_type = 'ebook')
func (r *Repository) DeleteABSEbooksByProfile(profileID string) (int64, error) {
	result := r.db.GetDB().Where("profile_id = ? AND LOWER(media_type) = ?", profileID, "ebook").Delete(&ABSBook{})
	return result.RowsAffected, result.Error
}

// DeleteABSBooksByLibrary deletes all ABS books for a specific library within a profile
func (r *Repository) DeleteABSBooksByLibrary(profileID, libraryID string) (int64, error) {
	result := r.db.GetDB().Where("profile_id = ? AND library_id = ?", profileID, libraryID).Delete(&ABSBook{})
	return result.RowsAffected, result.Error
}

// PurgeProfileDataResult contains counts of deleted records from each table
type PurgeProfileDataResult struct {
	ABSBooks        int64 `json:"abs_books"`
	HardcoverBooks  int64 `json:"hardcover_books"`
	BookMappings    int64 `json:"book_mappings"`
	ProgressHistory int64 `json:"progress_history"`
	SyncEvents      int64 `json:"sync_events"`
	BookSyncLogs    int64 `json:"book_sync_logs"`
	BookSyncConfigs int64 `json:"book_sync_configs"`
}

// PurgeProfileData deletes all data associated with a profile from all tables.
// This is a destructive operation that removes:
// - ABS books
// - Hardcover user books
// - Book mappings
// - Progress history
// - Sync events
// - Book sync logs
// - Book sync configs
// - Profile sync state
// The profile itself is NOT deleted, only its associated data.
func (r *Repository) PurgeProfileData(profileID string) (*PurgeProfileDataResult, error) {
	result := &PurgeProfileDataResult{}

	// Use a transaction to ensure all deletes succeed or none do
	err := r.db.GetDB().Transaction(func(tx *gorm.DB) error {
		var res *gorm.DB

		// Delete book sync configs first (references ABSBook)
		res = tx.Where("profile_id = ?", profileID).Delete(&BookSyncConfig{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete book sync configs: %w", res.Error)
		}
		result.BookSyncConfigs = res.RowsAffected

		// Delete book mappings (references ABSBook)
		res = tx.Where("profile_id = ?", profileID).Delete(&BookMapping{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete book mappings: %w", res.Error)
		}
		result.BookMappings = res.RowsAffected

		// Delete progress history
		res = tx.Where("profile_id = ?", profileID).Delete(&ProgressHistory{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete progress history: %w", res.Error)
		}
		result.ProgressHistory = res.RowsAffected

		// Delete sync events
		res = tx.Where("profile_id = ?", profileID).Delete(&SyncEvent{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete sync events: %w", res.Error)
		}
		result.SyncEvents = res.RowsAffected

		// Delete book sync logs
		res = tx.Where("profile_id = ?", profileID).Delete(&BookSyncLog{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete book sync logs: %w", res.Error)
		}
		result.BookSyncLogs = res.RowsAffected

		// Delete ABS books
		res = tx.Where("profile_id = ?", profileID).Delete(&ABSBook{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete ABS books: %w", res.Error)
		}
		result.ABSBooks = res.RowsAffected

		// Delete Hardcover user books
		res = tx.Where("profile_id = ?", profileID).Delete(&HardcoverUserBook{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete Hardcover user books: %w", res.Error)
		}
		result.HardcoverBooks = res.RowsAffected

		// Reset profile sync state (keep the record but clear the state)
		res = tx.Model(&ProfileSyncState{}).Where("profile_id = ?", profileID).Updates(map[string]interface{}{
			"state_data":  "{}",
			"last_sync":   nil,
			"updated_at":  time.Now(),
		})
		if res.Error != nil {
			return fmt.Errorf("failed to reset profile sync state: %w", res.Error)
		}

		return nil
	})

	if err != nil {
		r.logger.Error("Failed to purge profile data", map[string]interface{}{
			"profile_id": profileID,
			"error":      err.Error(),
		})
		return nil, err
	}

	r.logger.Info("Purged profile data", map[string]interface{}{
		"profile_id":        profileID,
		"abs_books":         result.ABSBooks,
		"hardcover_books":   result.HardcoverBooks,
		"book_mappings":     result.BookMappings,
		"progress_history":  result.ProgressHistory,
		"sync_events":       result.SyncEvents,
		"book_sync_logs":    result.BookSyncLogs,
		"book_sync_configs": result.BookSyncConfigs,
	})

	return result, nil
}

// ============================================================================
// HARDCOVER USER BOOK REPOSITORY METHODS
// ============================================================================

// UpsertHardcoverUserBook creates or updates a Hardcover user book record
func (r *Repository) UpsertHardcoverUserBook(book HardcoverUserBook) error {
	if book.ProfileID == "" || book.HCUserBookID == 0 {
		return errors.New("profile_id and hc_user_book_id are required")
	}

	now := time.Now()
	book.UpdatedAt = now
	book.LastFetchedAt = now

	clause := clause.OnConflict{
		Columns:   []clause.Column{{Name: "profile_id"}, {Name: "hc_user_book_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"hc_book_id", "hc_edition_id", "slug", "title", "author", "asin", "isbn13", "isbn10", "status", "status_name", "progress", "progress_seconds", "rating", "started_at", "finished_at", "last_fetched_at", "updated_at"}),
	}
	return r.db.GetDB().Clauses(clause).Create(&book).Error
}

// UpsertHardcoverUserBooks bulk upserts multiple Hardcover user books
func (r *Repository) UpsertHardcoverUserBooks(books []HardcoverUserBook) error {
	if len(books) == 0 {
		return nil
	}

	now := time.Now()
	for i := range books {
		books[i].UpdatedAt = now
		books[i].LastFetchedAt = now
	}

	clause := clause.OnConflict{
		Columns:   []clause.Column{{Name: "profile_id"}, {Name: "hc_user_book_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"hc_book_id", "hc_edition_id", "slug", "title", "author", "asin", "isbn13", "isbn10", "status", "status_name", "progress", "progress_seconds", "rating", "started_at", "finished_at", "last_fetched_at", "updated_at"}),
	}
	return r.db.GetDB().Clauses(clause).CreateInBatches(books, 100).Error
}

// GetHardcoverUserBook retrieves a Hardcover user book by profile and user book ID
func (r *Repository) GetHardcoverUserBook(profileID string, userBookID int64) (*HardcoverUserBook, error) {
	var book HardcoverUserBook
	if err := r.db.GetDB().
		Where("profile_id = ? AND hc_user_book_id = ?", profileID, userBookID).
		First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &book, nil
}

// GetHardcoverUserBooks retrieves all Hardcover user books for a profile with pagination
func (r *Repository) GetHardcoverUserBooks(profileID string, limit int, offset int) ([]HardcoverUserBook, int64, error) {
	var books []HardcoverUserBook
	var total int64

	if err := r.db.GetDB().Model(&HardcoverUserBook{}).Where("profile_id = ?", profileID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.GetDB().
		Where("profile_id = ?", profileID).
		Order("title ASC").
		Limit(limit).
		Offset(offset).
		Find(&books).Error; err != nil {
		return nil, 0, err
	}

	return books, total, nil
}

// GetHardcoverUserBookByASIN retrieves a Hardcover user book by ASIN
func (r *Repository) GetHardcoverUserBookByASIN(profileID, asin string) (*HardcoverUserBook, error) {
	var book HardcoverUserBook
	if err := r.db.GetDB().
		Where("profile_id = ? AND asin = ?", profileID, asin).
		First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &book, nil
}

// GetHardcoverUserBookByISBN retrieves a Hardcover user book by ISBN
func (r *Repository) GetHardcoverUserBookByISBN(profileID, isbn string) (*HardcoverUserBook, error) {
	var book HardcoverUserBook
	if err := r.db.GetDB().
		Where("profile_id = ? AND (isbn13 = ? OR isbn10 = ?)", profileID, isbn, isbn).
		First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &book, nil
}

// GetHardcoverUserBookByTitle retrieves a Hardcover user book by exact title match
func (r *Repository) GetHardcoverUserBookByTitle(profileID, title string) (*HardcoverUserBook, error) {
	var book HardcoverUserBook
	if err := r.db.GetDB().
		Where("profile_id = ? AND LOWER(title) = LOWER(?)", profileID, title).
		First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &book, nil
}

// GetHardcoverUserBookByID retrieves a Hardcover user book by HCUserBookID
func (r *Repository) GetHardcoverUserBookByID(profileID string, hcUserBookID int64) (*HardcoverUserBook, error) {
	var book HardcoverUserBook
	if err := r.db.GetDB().
		Where("profile_id = ? AND hc_user_book_id = ?", profileID, hcUserBookID).
		First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &book, nil
}

// ============================================================================
// BOOK MAPPING REPOSITORY METHODS
// ============================================================================

// UpsertBookMapping creates or updates a book mapping record
func (r *Repository) UpsertBookMapping(mapping BookMapping) error {
	if mapping.ProfileID == "" || mapping.ABSBookID == 0 {
		return errors.New("profile_id and abs_book_id are required")
	}

	now := time.Now()
	mapping.UpdatedAt = now

	clause := clause.OnConflict{
		Columns:   []clause.Column{{Name: "profile_id"}, {Name: "abs_book_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"hc_user_book_id", "hc_edition_id", "hc_book_id", "match_method", "match_confidence", "manual_override", "match_details", "updated_at"}),
	}
	return r.db.GetDB().Clauses(clause).Create(&mapping).Error
}

// GetBookMapping retrieves a book mapping by profile and ABS book ID
func (r *Repository) GetBookMapping(profileID string, absBookID uint) (*BookMapping, error) {
	var mapping BookMapping
	if err := r.db.GetDB().
		Where("profile_id = ? AND abs_book_id = ?", profileID, absBookID).
		Preload("ABSBook").
		First(&mapping).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &mapping, nil
}

// GetMappedBooks retrieves all mapped books for a profile
func (r *Repository) GetMappedBooks(profileID string, limit int, offset int) ([]BookMapping, int64, error) {
	var mappings []BookMapping
	var total int64

	if err := r.db.GetDB().Model(&BookMapping{}).
		Where("profile_id = ? AND hc_user_book_id IS NOT NULL", profileID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.GetDB().
		Where("profile_id = ? AND hc_user_book_id IS NOT NULL", profileID).
		Preload("ABSBook").
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&mappings).Error; err != nil {
		return nil, 0, err
	}

	return mappings, total, nil
}

// GetUnmappedBooks retrieves all unmapped ABS books for a profile
func (r *Repository) GetUnmappedBooks(profileID string, limit int, offset int) ([]ABSBook, int64, error) {
	var books []ABSBook
	var total int64

	// Books with no mapping
	db := r.db.GetDB().Model(&ABSBook{}).
		Where("profile_id = ?", profileID).
		Where("id NOT IN (SELECT DISTINCT abs_book_id FROM book_mappings WHERE profile_id = ? AND hc_user_book_id IS NOT NULL)", profileID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := db.
		Order("title ASC").
		Limit(limit).
		Offset(offset).
		Find(&books).Error; err != nil {
		return nil, 0, err
	}

	return books, total, nil
}

// DeleteBookMapping deletes a book mapping
func (r *Repository) DeleteBookMapping(profileID string, absBookID uint) error {
	return r.db.GetDB().Where("profile_id = ? AND abs_book_id = ?", profileID, absBookID).Delete(&BookMapping{}).Error
}

// ============================================================================
// PROGRESS HISTORY REPOSITORY METHODS
// ============================================================================

// InsertProgressHistory records a progress snapshot
func (r *Repository) InsertProgressHistory(history ProgressHistory) error {
	if history.ProfileID == "" || (history.ABSBookID == nil && history.HCUserBookID == nil) {
		return errors.New("profile_id and either abs_book_id or hc_user_book_id are required")
	}

	if history.CreatedAt.IsZero() {
		history.CreatedAt = time.Now()
	}
	if history.RecordedAt.IsZero() {
		history.RecordedAt = time.Now()
	}

	return r.db.GetDB().Create(&history).Error
}

// GetProgressHistory retrieves progress history for an ABS book with pagination
func (r *Repository) GetProgressHistory(profileID string, absBookID uint, limit int, offset int) ([]ProgressHistory, int64, error) {
	var histories []ProgressHistory
	var total int64

	if err := r.db.GetDB().Model(&ProgressHistory{}).
		Where("profile_id = ? AND abs_book_id = ?", profileID, absBookID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.GetDB().
		Where("profile_id = ? AND abs_book_id = ?", profileID, absBookID).
		Order("recorded_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&histories).Error; err != nil {
		return nil, 0, err
	}

	return histories, total, nil
}

// GetProgressHistoryRange retrieves progress history within a time range
func (r *Repository) GetProgressHistoryRange(profileID string, absBookID uint, startTime, endTime time.Time) ([]ProgressHistory, error) {
	var histories []ProgressHistory
	if err := r.db.GetDB().
		Where("profile_id = ? AND abs_book_id = ? AND recorded_at BETWEEN ? AND ?", profileID, absBookID, startTime, endTime).
		Order("recorded_at ASC").
		Find(&histories).Error; err != nil {
		return nil, err
	}
	return histories, nil
}

// PurgeOldProgressHistory deletes old progress history records to prevent bloat.
// Keeps records up to maxDays old OR keeps at least minEntries, whichever is larger.
func (r *Repository) PurgeOldProgressHistory(profileID string, maxDays int, minEntries int) error {
	// First, find all abs_book_id values for this profile
	var absBookIDs []uint
	if err := r.db.GetDB().
		Distinct("abs_book_id").
		Where("profile_id = ?", profileID).
		Model(&ProgressHistory{}).
		Pluck("abs_book_id", &absBookIDs).Error; err != nil {
		return err
	}

	cutoffTime := time.Now().AddDate(0, 0, -maxDays)

	// For each book, delete old entries keeping minimum
	for _, absBookID := range absBookIDs {
		// Count total entries
		var total int64
		if err := r.db.GetDB().
			Where("profile_id = ? AND abs_book_id = ?", profileID, absBookID).
			Model(&ProgressHistory{}).
			Count(&total).Error; err != nil {
			return err
		}

		// Calculate how many to delete
		toDelete := total - int64(minEntries)
		if toDelete > 0 {
			// Delete oldest entries, but keep entries newer than cutoff
			if err := r.db.GetDB().
				Where("profile_id = ? AND abs_book_id = ? AND recorded_at < ?", profileID, absBookID, cutoffTime).
				Order("recorded_at ASC").
				Limit(int(toDelete)).
				Delete(&ProgressHistory{}).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

// ============================================================================
// SYNC EVENT REPOSITORY METHODS
// ============================================================================

// InsertSyncEvent records a sync operation
func (r *Repository) InsertSyncEvent(event SyncEvent) error {
	if event.ProfileID == "" || event.EventType == "" {
		return errors.New("profile_id and event_type are required")
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	return r.db.GetDB().Create(&event).Error
}

// GetSyncEvents retrieves sync events for a profile with pagination
func (r *Repository) GetSyncEvents(profileID string, limit int, offset int) ([]SyncEvent, int64, error) {
	var events []SyncEvent
	var total int64

	if err := r.db.GetDB().Model(&SyncEvent{}).Where("profile_id = ?", profileID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.GetDB().
		Where("profile_id = ?", profileID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&events).Error; err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// GetSyncEventsForBook retrieves all sync events for a specific book
func (r *Repository) GetSyncEventsForBook(profileID string, absBookID uint) ([]SyncEvent, error) {
	var events []SyncEvent
	if err := r.db.GetDB().
		Where("profile_id = ? AND abs_book_id = ?", profileID, absBookID).
		Order("created_at DESC").
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// ============================================================================
// BOOK SYNC CONFIG REPOSITORY METHODS
// ============================================================================

// UpsertBookSyncConfig creates or updates a book sync configuration
func (r *Repository) UpsertBookSyncConfig(config BookSyncConfig) error {
	if config.ProfileID == "" || config.ABSBookID == 0 {
		return errors.New("profile_id and abs_book_id are required")
	}

	now := time.Now()
	config.UpdatedAt = now

	clause := clause.OnConflict{
		Columns:   []clause.Column{{Name: "profile_id"}, {Name: "abs_book_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"sync_enabled", "conflict_mode", "notes", "updated_at"}),
	}
	return r.db.GetDB().Clauses(clause).Create(&config).Error
}

// GetBookSyncConfig retrieves a book sync configuration
func (r *Repository) GetBookSyncConfig(profileID string, absBookID uint) (*BookSyncConfig, error) {
	var config BookSyncConfig
	if err := r.db.GetDB().
		Where("profile_id = ? AND abs_book_id = ?", profileID, absBookID).
		Preload("ABSBook").
		First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

// GetBookSyncConfigs retrieves all sync configurations for a profile
func (r *Repository) GetBookSyncConfigs(profileID string) ([]BookSyncConfig, error) {
	var configs []BookSyncConfig
	if err := r.db.GetDB().
		Where("profile_id = ?", profileID).
		Preload("ABSBook").
		Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

// DeleteBookSyncConfig deletes a book sync configuration
func (r *Repository) DeleteBookSyncConfig(profileID string, absBookID uint) error {
	return r.db.GetDB().Where("profile_id = ? AND abs_book_id = ?", profileID, absBookID).Delete(&BookSyncConfig{}).Error
}

// ============================================================================
// BOOK COMPARISON QUERY METHODS
// ============================================================================

// GetBookComparison retrieves a full comparison view for an ABS book
func (r *Repository) GetBookComparison(profileID string, absBookID uint) (*BookComparison, error) {
	var absBook ABSBook
	if err := r.db.GetDB().
		Where("profile_id = ? AND id = ?", profileID, absBookID).
		Preload("Mapping").
		Preload("Config").
		First(&absBook).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	comparison := &BookComparison{
		ABSBook: absBook,
		Mapping: absBook.Mapping,
		Config:  absBook.Config,
	}

	// Fetch matching Hardcover book if mapping exists
	if absBook.Mapping != nil && absBook.Mapping.HCUserBookID != nil {
		var hcBook HardcoverUserBook
		if err := r.db.GetDB().
			Where("profile_id = ? AND hc_user_book_id = ?", profileID, *absBook.Mapping.HCUserBookID).
			First(&hcBook).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		} else if err == nil {
			comparison.HardcoverBook = &hcBook
		}
	}

	// Calculate progress diff and sync status
	if comparison.HardcoverBook != nil {
		comparison.ProgressDiff = comparison.ABSBook.Progress - comparison.HardcoverBook.Progress
		comparison.InSync = math.Abs(comparison.ProgressDiff) < 0.01 // 1% threshold
		if !comparison.InSync {
			comparison.SyncStatus = "needs_sync"
		} else {
			comparison.SyncStatus = "in_sync"
		}
	} else if absBook.Mapping == nil {
		comparison.SyncStatus = "not_matched"
	} else if absBook.Config != nil && !absBook.Config.SyncEnabled {
		comparison.SyncStatus = "disabled"
	} else {
		comparison.SyncStatus = "not_matched"
	}

	return comparison, nil
}

// BookFilterOptions defines filter, sort, and search options for book queries
type BookFilterOptions struct {
	Search       string // Search query for title/author
	Filter       string // Filter: in_sync, needs_sync, unmapped, disabled
	Sort         string // Sort: title, progress_diff, last_updated
	SortDir      string // Sort direction: asc, desc
	MediaType    string // Filter by media type: audiobook, ebook
	LibraryID    string // Filter by ABS library ID
	CollectionID string // Filter by ABS collection ID
}

// GetBookComparisons retrieves all comparisons for a profile with pagination and filtering
func (r *Repository) GetBookComparisons(profileID string, limit int, offset int, opts *BookFilterOptions) ([]BookComparison, int64, error) {
	var absBooks []ABSBook

	// Determine sort direction
	sortDir := "ASC"
	if opts != nil && opts.SortDir == "desc" {
		sortDir = "DESC"
	}

	// Determine sort order
	orderBy := "title " + sortDir // default
	if opts != nil && opts.Sort != "" {
		switch opts.Sort {
		case "author":
			orderBy = "author " + sortDir
		case "progress_diff":
			orderBy = "progress " + sortDir // Will be recalculated, sort by abs progress for now
		case "last_updated":
			orderBy = "updated_at " + sortDir
		default:
			orderBy = "title " + sortDir
		}
	}

	// Build query - fetch ALL books first (filtering happens after sync status computation)
	bookQuery := r.db.GetDB().Model(&ABSBook{}).
		Where("profile_id = ?", profileID).
		Preload("Mapping").
		Preload("Config")

	// Apply search filter if provided
	if opts != nil && opts.Search != "" {
		searchPattern := "%" + opts.Search + "%"
		bookQuery = bookQuery.Where("(title LIKE ? OR author LIKE ?)", searchPattern, searchPattern)
	}

	// Apply media type filter if provided
	if opts != nil && opts.MediaType != "" {
		switch opts.MediaType {
		case "audiobook":
			// Audiobooks have media_type = "audiobook" or "book" (legacy) or empty/null (for backwards compatibility)
			bookQuery = bookQuery.Where("(LOWER(media_type) = ? OR LOWER(media_type) = ? OR media_type IS NULL OR media_type = '')", "audiobook", "book")
		case "ebook":
			bookQuery = bookQuery.Where("LOWER(media_type) = ?", "ebook")
		}
	}

	// Apply library filter if provided
	if opts != nil && opts.LibraryID != "" {
		bookQuery = bookQuery.Where("library_id = ?", opts.LibraryID)
	}

	if err := bookQuery.
		Order(orderBy).
		Find(&absBooks).Error; err != nil {
		return nil, 0, err
	}

	// Build comparisons and apply status filter
	allComparisons := make([]BookComparison, 0, len(absBooks))
	for _, absBook := range absBooks {
		comp := BookComparison{
			ABSBook: absBook,
			Mapping: absBook.Mapping,
			Config:  absBook.Config,
		}

		// Fetch Hardcover book if mapped
		if absBook.Mapping != nil && absBook.Mapping.HCUserBookID != nil {
			var hcBook HardcoverUserBook
			if err := r.db.GetDB().
				Where("profile_id = ? AND hc_user_book_id = ?", profileID, *absBook.Mapping.HCUserBookID).
				First(&hcBook).Error; err == nil {
				comp.HardcoverBook = &hcBook
				comp.ProgressDiff = absBook.Progress - hcBook.Progress
				comp.InSync = math.Abs(comp.ProgressDiff) < 0.01
				if !comp.InSync {
					comp.SyncStatus = "needs_sync"
				} else {
					comp.SyncStatus = "in_sync"
				}
			}
		}

		if comp.HardcoverBook == nil {
			if absBook.Config != nil && !absBook.Config.SyncEnabled {
				comp.SyncStatus = "disabled"
			} else {
				comp.SyncStatus = "not_matched"
			}
		}

		// Apply status filter if specified
		if opts != nil && opts.Filter != "" {
			switch opts.Filter {
			case "in_sync":
				if comp.SyncStatus != "in_sync" {
					continue
				}
			case "needs_sync":
				if comp.SyncStatus != "needs_sync" {
					continue
				}
			case "unmapped":
				if comp.SyncStatus != "not_matched" {
					continue
				}
			case "disabled":
				if comp.SyncStatus != "disabled" {
					continue
				}
			}
		}

		allComparisons = append(allComparisons, comp)
	}

	// Get total count after filtering
	total := int64(len(allComparisons))

	// Apply pagination
	start := offset
	if start > len(allComparisons) {
		start = len(allComparisons)
	}
	end := start + limit
	if end > len(allComparisons) {
		end = len(allComparisons)
	}

	return allComparisons[start:end], total, nil
}

// GetSyncSummary retrieves aggregated sync statistics for a profile
func (r *Repository) GetSyncSummary(profileID string) (*SyncSummary, error) {
	summary := &SyncSummary{}
	var count int64

	// Total ABS books
	if err := r.db.GetDB().Model(&ABSBook{}).
		Where("profile_id = ?", profileID).
		Count(&count).Error; err != nil {
		return nil, err
	}
	summary.TotalABSBooks = int(count)

	// Total HC books
	if err := r.db.GetDB().Model(&HardcoverUserBook{}).
		Where("profile_id = ?", profileID).
		Count(&count).Error; err != nil {
		return nil, err
	}
	summary.TotalHCBooks = int(count)

	// Mapped books (with valid HC link)
	if err := r.db.GetDB().Model(&BookMapping{}).
		Where("profile_id = ? AND hc_user_book_id IS NOT NULL", profileID).
		Count(&count).Error; err != nil {
		return nil, err
	}
	summary.MappedBooks = int(count)

	// Unmapped books
	summary.UnmappedBooks = summary.TotalABSBooks - summary.MappedBooks

	// In sync books (needs complex query)
	if err := r.db.GetDB().Raw(
		`SELECT COUNT(*) as count FROM (
			SELECT a.id FROM abs_books a
			LEFT JOIN book_mappings m ON a.id = m.abs_book_id AND a.profile_id = m.profile_id
			LEFT JOIN hardcover_user_books h ON m.hc_user_book_id = h.hc_user_book_id AND h.profile_id = a.profile_id
			WHERE a.profile_id = ?
			AND m.hc_user_book_id IS NOT NULL
			AND ABS(a.progress - h.progress) < 0.01
		) as in_sync_count`,
		profileID).Scan(&summary.InSyncBooks).Error; err != nil {
		return nil, err
	}

	// Needs sync books
	if err := r.db.GetDB().Raw(
		`SELECT COUNT(*) as count FROM (
			SELECT a.id FROM abs_books a
			LEFT JOIN book_mappings m ON a.id = m.abs_book_id AND a.profile_id = m.profile_id
			LEFT JOIN hardcover_user_books h ON m.hc_user_book_id = h.hc_user_book_id AND h.profile_id = a.profile_id
			WHERE a.profile_id = ?
			AND m.hc_user_book_id IS NOT NULL
			AND ABS(a.progress - h.progress) >= 0.01
		) as needs_sync_count`,
		profileID).Scan(&summary.NeedsSyncBooks).Error; err != nil {
		return nil, err
	}

	// Sync disabled books
	if err := r.db.GetDB().Model(&BookSyncConfig{}).
		Where("profile_id = ? AND sync_enabled = ?", profileID, false).
		Count(&count).Error; err != nil {
		return nil, err
	}
	summary.SyncDisabledBooks = int(count)

	return summary, nil
}

