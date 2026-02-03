package database

import (
	"time"

	"gorm.io/gorm"
)

// SyncProfile represents a sync profile in the system
type SyncProfile struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Active    bool      `gorm:"default:true" json:"active"`

	// Relationships
	Config    *SyncProfileConfig `gorm:"foreignKey:ProfileID" json:"config,omitempty"`
	SyncState *ProfileSyncState  `gorm:"foreignKey:ProfileID" json:"sync_state,omitempty"`
	BookLogs  []BookSyncLog      `gorm:"foreignKey:ProfileID" json:"book_logs,omitempty"`
}

// SyncProfileConfig holds the configuration for a specific sync profile
type SyncProfileConfig struct {
	ProfileID                  string `gorm:"primaryKey;column:profile_id" json:"profile_id"`
	AudiobookshelfURL          string `json:"audiobookshelf_url"`
	AudiobookshelfTokenEncrypted string `json:"-"` // Hidden from JSON serialization
	HardcoverTokenEncrypted    string `json:"-"` // Hidden from JSON serialization
	SyncConfig                 string `gorm:"type:text" json:"-"` // JSON string (hidden from API responses)
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`

	// Relationship
	Profile SyncProfile `gorm:"foreignKey:ProfileID" json:"-"`
}

// ProfileSyncState holds the sync state for a specific profile
type ProfileSyncState struct {
	ProfileID string     `gorm:"primaryKey;column:profile_id" json:"profile_id"`
	StateData string     `gorm:"type:text" json:"state_data"` // JSON string
	LastSync  *time.Time `json:"last_sync"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Relationship
	Profile SyncProfile `gorm:"foreignKey:ProfileID" json:"profile,omitempty"`
}

// SyncConfigData represents the structure of sync configuration
// SyncMode represents the sync operation mode
type SyncMode string

const (
	// SyncModeNeedsSync syncs only books with progress differences (default)
	SyncModeNeedsSync SyncMode = "needs_sync"
	// SyncModeAll syncs all matched books regardless of current state
	SyncModeAll SyncMode = "sync_all"
	// SyncModeCollections syncs only books in selected ABS collections
	SyncModeCollections SyncMode = "collections"
)

// LibraryFilterMode determines whether the library filter includes or excludes
type LibraryFilterMode string

const (
	LibraryFilterModeInclude LibraryFilterMode = "include"
	LibraryFilterModeExclude LibraryFilterMode = "exclude"
)

type SyncConfigData struct {
	Incremental        bool     `json:"incremental"`
	StateFile          string   `json:"state_file"`
	MinChangeThreshold int      `json:"min_change_threshold"`
	// Legacy library filtering (kept for backward compatibility during migration)
	Libraries          struct {
		Include []string `json:"include"`
		Exclude []string `json:"exclude"`
	} `json:"libraries"`
	// New library filter mode: "include" or "exclude"
	LibraryFilterMode  LibraryFilterMode `json:"library_filter_mode"`
	// Libraries to include or exclude (based on LibraryFilterMode)
	FilteredLibraries  []string          `json:"filtered_libraries"`
	SyncInterval        string   `json:"sync_interval"`
	MinimumProgress     float64  `json:"minimum_progress"`
	SyncWantToRead      bool     `json:"sync_want_to_read"`
	ProcessUnreadBooks  bool     `json:"process_unread_books"`
	SyncOwned           bool     `json:"sync_owned"`
	IncludeEbooks       bool     `json:"include_ebooks"`
	DryRun              bool     `json:"dry_run"`
	TestBookFilter      string   `json:"test_book_filter"`
	TestBookLimit       int      `json:"test_book_limit"`
	// Sync mode fields
	SyncMode            SyncMode `json:"sync_mode"`             // needs_sync, sync_all, collections
	SelectedCollections []string `json:"selected_collections"`  // ABS collection IDs for collections mode
	// Collection filtering for ABS data collection (both can be set)
	IncludeCollections  []string `json:"include_collections"`   // Only fetch books from these collections
	ExcludeCollections  []string `json:"exclude_collections"`   // Exclude books from these collections
}

// IsEmpty checks if the SyncConfigData is empty (all fields at their zero values)
func (s SyncConfigData) IsEmpty() bool {
	return !s.Incremental &&
		s.StateFile == "" &&
		s.MinChangeThreshold == 0 &&
		len(s.Libraries.Include) == 0 &&
		len(s.Libraries.Exclude) == 0 &&
		s.LibraryFilterMode == "" &&
		len(s.FilteredLibraries) == 0 &&
		s.SyncInterval == "" &&
		s.MinimumProgress == 0 &&
		!s.SyncWantToRead &&
		!s.ProcessUnreadBooks &&
		!s.SyncOwned &&
		!s.IncludeEbooks &&
		!s.DryRun &&
		s.TestBookFilter == "" &&
		s.TestBookLimit == 0 &&
		s.SyncMode == "" &&
		len(s.SelectedCollections) == 0 &&
		len(s.IncludeCollections) == 0 &&
		len(s.ExcludeCollections) == 0
}

// BeforeCreate hook for SyncProfile
func (p *SyncProfile) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate hook for SyncProfile
func (p *SyncProfile) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = time.Now()
	return nil
}

// BeforeCreate hook for SyncProfileConfig
func (c *SyncProfileConfig) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate hook for SyncProfileConfig
func (c *SyncProfileConfig) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

// BeforeCreate hook for ProfileSyncState
func (s *ProfileSyncState) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate hook for ProfileSyncState
func (s *ProfileSyncState) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = time.Now()
	return nil
}

// BookSyncLog tracks individual book sync attempts and results
type BookSyncLog struct {
	ID            int64      `gorm:"primaryKey" json:"id"`
	ProfileID     string     `gorm:"index" json:"profile_id"`
	AudiobookID   string     `gorm:"index" json:"audiobook_id"`
	HardcoverID   *int64     `json:"hardcover_id,omitempty"`
	EditionID     *string    `json:"edition_id,omitempty"`
	Title         string     `json:"title"`
	Author        string     `json:"author"`
	Status        string     `json:"status"` // PENDING, SYNCED, SKIPPED, ERROR, NOT_FOUND
	Progress      float64    `json:"progress"`
	TargetStatus  string     `json:"target_status"` // WANT_TO_READ, IN_PROGRESS, FINISHED
	ErrorMessage  string     `gorm:"type:text" json:"error_message,omitempty"`
	LastAttempt   *time.Time `json:"last_attempt,omitempty"`
	SyncedAt      *time.Time `json:"synced_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Relationship
	Profile SyncProfile `gorm:"foreignKey:ProfileID" json:"-"`
}

// BeforeCreate hook for BookSyncLog
func (b *BookSyncLog) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate hook for BookSyncLog
func (b *BookSyncLog) BeforeUpdate(tx *gorm.DB) error {
	b.UpdatedAt = time.Now()
	return nil
}

// ============================================================================
// NEW MODELS FOR BOOK LIBRARY REARCHITECTURE
// See docs/BOOK_LIBRARY_REARCHITECTURE.md for details
// ============================================================================

// ABSBook stores AudiobookShelf book data with progress information.
// This is the local cache of book data fetched from ABS API.
type ABSBook struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ProfileID     string    `gorm:"index:idx_abs_profile_id;uniqueIndex:idx_profile_abs_unique,priority:1;not null" json:"profile_id"`
	ABSID         string    `gorm:"index:idx_abs_id;uniqueIndex:idx_profile_abs_unique,priority:2;not null" json:"abs_id"`
	LibraryID     string    `gorm:"index:idx_abs_library_id" json:"library_id"`
	MediaType     string    `gorm:"index:idx_abs_media_type" json:"media_type"` // "book" or "ebook"
	Title         string    `gorm:"not null" json:"title"`
	Author        string    `json:"author"`
	Narrator      string    `json:"narrator"`
	SeriesName    string    `json:"series_name"`
	ASIN          string    `gorm:"index:idx_abs_asin" json:"asin"`
	ISBN          string    `gorm:"index:idx_abs_isbn" json:"isbn"`
	Duration      float64   `json:"duration"`       // Total duration in seconds
	CurrentTime   float64   `json:"current_time"`   // Current playback position in seconds
	Progress      float64   `json:"progress"`       // 0.0 to 1.0
	IsFinished    bool      `json:"is_finished"`
	StartedAt     *int64    `json:"started_at,omitempty"`  // Unix timestamp
	FinishedAt    *int64    `json:"finished_at,omitempty"` // Unix timestamp
	CoverPath     string    `json:"cover_path"`
	LastFetchedAt time.Time `gorm:"index:idx_abs_last_fetched" json:"last_fetched_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Relationships
	Profile SyncProfile    `gorm:"foreignKey:ProfileID" json:"-"`
	Mapping *BookMapping   `gorm:"foreignKey:ABSBookID" json:"mapping,omitempty"`
	Config  *BookSyncConfig `gorm:"foreignKey:ABSBookID" json:"config,omitempty"`
}

// BeforeCreate hook for ABSBook
func (b *ABSBook) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = now
	}
	if b.LastFetchedAt.IsZero() {
		b.LastFetchedAt = now
	}
	return nil
}

// BeforeUpdate hook for ABSBook
func (b *ABSBook) BeforeUpdate(tx *gorm.DB) error {
	b.UpdatedAt = time.Now()
	return nil
}

// HardcoverUserBook stores Hardcover user_book data with progress information.
// This is the local cache of user's book data fetched from Hardcover API.
type HardcoverUserBook struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ProfileID       string    `gorm:"index:idx_hc_profile_id;uniqueIndex:idx_profile_hc_unique,priority:1;not null" json:"profile_id"`
	HCUserBookID    int64     `gorm:"index:idx_hc_user_book_id;uniqueIndex:idx_profile_hc_unique,priority:2;not null" json:"hc_user_book_id"`
	HCBookID        int64     `gorm:"index:idx_hc_book_id" json:"hc_book_id"`
	HCEditionID     *int64    `gorm:"index:idx_hc_edition_id" json:"hc_edition_id,omitempty"`
	Slug            string    `json:"slug"`              // Book slug for URL (e.g., "green-wing")
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	ASIN            string    `gorm:"index:idx_hc_asin" json:"asin"`
	ISBN13          string    `gorm:"index:idx_hc_isbn13" json:"isbn_13"`
	ISBN10          string    `gorm:"index:idx_hc_isbn10" json:"isbn_10"`
	Status          int       `json:"status"`            // Hardcover status ID (1=want_to_read, 2=reading, 3=finished, etc.)
	StatusName      string    `json:"status_name"`       // Human-readable status
	Progress        float64   `json:"progress"`          // 0.0 to 1.0
	ProgressSeconds float64   `json:"progress_seconds"`  // Current position in seconds (for audiobooks)
	Rating          *float64  `json:"rating,omitempty"`  // User's rating
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	LastFetchedAt   time.Time `gorm:"index:idx_hc_last_fetched" json:"last_fetched_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Relationships
	Profile SyncProfile `gorm:"foreignKey:ProfileID" json:"-"`
}

// BeforeCreate hook for HardcoverUserBook
func (b *HardcoverUserBook) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = now
	}
	if b.LastFetchedAt.IsZero() {
		b.LastFetchedAt = now
	}
	return nil
}

// BeforeUpdate hook for HardcoverUserBook
func (b *HardcoverUserBook) BeforeUpdate(tx *gorm.DB) error {
	b.UpdatedAt = time.Now()
	return nil
}

// MatchMethod represents how a book was matched between ABS and Hardcover
type MatchMethod string

const (
	MatchMethodASIN       MatchMethod = "asin"
	MatchMethodISBN       MatchMethod = "isbn"
	MatchMethodTitleAuthor MatchMethod = "title_author"
	MatchMethodManual     MatchMethod = "manual"
	MatchMethodNone       MatchMethod = "none"
)

// BookMapping links an ABS book to a Hardcover user_book/edition.
// This enables tracking which ABS books correspond to which Hardcover records.
type BookMapping struct {
	ID             uint        `gorm:"primaryKey" json:"id"`
	ProfileID      string      `gorm:"index:idx_mapping_profile_id;uniqueIndex:idx_mapping_unique,priority:1;not null" json:"profile_id"`
	ABSBookID      uint        `gorm:"index:idx_mapping_abs_book_id;uniqueIndex:idx_mapping_unique,priority:2;not null" json:"abs_book_id"`
	HCUserBookID   *int64      `gorm:"index:idx_mapping_hc_user_book_id" json:"hc_user_book_id,omitempty"`
	HCEditionID    *int64      `gorm:"index:idx_mapping_hc_edition_id" json:"hc_edition_id,omitempty"`
	HCBookID       *int64      `json:"hc_book_id,omitempty"`
	MatchMethod    MatchMethod `gorm:"type:varchar(20);default:'none'" json:"match_method"`
	MatchConfidence float64    `json:"match_confidence"` // 0.0 to 1.0
	ManualOverride bool        `gorm:"default:false" json:"manual_override"`
	MatchDetails   string      `gorm:"type:text" json:"match_details,omitempty"` // JSON with match details
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`

	// Relationships
	Profile SyncProfile `gorm:"foreignKey:ProfileID" json:"-"`
	ABSBook ABSBook     `gorm:"foreignKey:ABSBookID" json:"abs_book,omitempty"`
}

// BeforeCreate hook for BookMapping
func (m *BookMapping) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate hook for BookMapping
func (m *BookMapping) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}

// ProgressSource indicates where a progress record came from
type ProgressSource string

const (
	ProgressSourceABS       ProgressSource = "abs"
	ProgressSourceHardcover ProgressSource = "hardcover"
)

// ProgressHistory stores timestamped progress snapshots for tracking changes over time.
// This enables progress analytics and debugging sync issues.
type ProgressHistory struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	ProfileID    string         `gorm:"index:idx_history_profile_id;index:idx_history_composite,priority:1;not null" json:"profile_id"`
	ABSBookID    *uint          `gorm:"index:idx_history_abs_book_id;index:idx_history_composite,priority:2" json:"abs_book_id,omitempty"`
	HCUserBookID *int64         `gorm:"index:idx_history_hc_user_book_id" json:"hc_user_book_id,omitempty"`
	Source       ProgressSource `gorm:"type:varchar(20);not null" json:"source"`
	Progress     float64        `json:"progress"`      // 0.0 to 1.0
	CurrentTime  float64        `json:"current_time"`  // Position in seconds
	IsFinished   bool           `json:"is_finished"`
	RecordedAt   time.Time      `gorm:"index:idx_history_recorded_at;index:idx_history_composite,priority:3;not null" json:"recorded_at"`
	CreatedAt    time.Time      `json:"created_at"`

	// Relationships
	Profile SyncProfile `gorm:"foreignKey:ProfileID" json:"-"`
	ABSBook *ABSBook    `gorm:"foreignKey:ABSBookID" json:"abs_book,omitempty"`
}

// BeforeCreate hook for ProgressHistory
func (h *ProgressHistory) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}
	if h.RecordedAt.IsZero() {
		h.RecordedAt = now
	}
	return nil
}

// SyncEventType represents the type of sync operation that occurred
type SyncEventType string

const (
	SyncEventTypeProgressUpdate SyncEventType = "progress_update"
	SyncEventTypeStatusChange   SyncEventType = "status_change"
	SyncEventTypeSkipped        SyncEventType = "skipped"
	SyncEventTypeError          SyncEventType = "error"
	SyncEventTypeNotFound       SyncEventType = "not_found"
	SyncEventTypeManualSync     SyncEventType = "manual_sync"
)

// SyncTrigger indicates what triggered the sync
type SyncTrigger string

const (
	SyncTriggerScheduled SyncTrigger = "scheduled"
	SyncTriggerManual    SyncTrigger = "manual"
	SyncTriggerAPI       SyncTrigger = "api"
	SyncTriggerWebUI     SyncTrigger = "web_ui"
)

// SyncEvent is an audit log of sync operations for tracking and debugging.
type SyncEvent struct {
	ID           uint          `gorm:"primaryKey" json:"id"`
	ProfileID    string        `gorm:"index:idx_event_profile_id;not null" json:"profile_id"`
	ABSBookID    *uint         `gorm:"index:idx_event_abs_book_id" json:"abs_book_id,omitempty"`
	HCUserBookID *int64        `gorm:"index:idx_event_hc_user_book_id" json:"hc_user_book_id,omitempty"`
	EventType    SyncEventType `gorm:"type:varchar(30);not null" json:"event_type"`
	Trigger      SyncTrigger   `gorm:"type:varchar(20);not null" json:"trigger"`
	OldProgress  *float64      `json:"old_progress,omitempty"`
	NewProgress  *float64      `json:"new_progress,omitempty"`
	OldStatus    *int          `json:"old_status,omitempty"`
	NewStatus    *int          `json:"new_status,omitempty"`
	Details      string        `gorm:"type:text" json:"details,omitempty"` // JSON with additional details
	ErrorMessage string        `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time     `gorm:"index:idx_event_created_at" json:"created_at"`

	// Relationships
	Profile SyncProfile `gorm:"foreignKey:ProfileID" json:"-"`
	ABSBook *ABSBook    `gorm:"foreignKey:ABSBookID" json:"abs_book,omitempty"`
}

// BeforeCreate hook for SyncEvent
func (e *SyncEvent) BeforeCreate(tx *gorm.DB) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return nil
}

// ConflictResolutionMode determines how to resolve conflicts when progress differs
type ConflictResolutionMode string

const (
	ConflictModePreferABS       ConflictResolutionMode = "prefer_abs"
	ConflictModePreferHardcover ConflictResolutionMode = "prefer_hardcover"
	ConflictModePreferNewest    ConflictResolutionMode = "prefer_newest"
	ConflictModeManual          ConflictResolutionMode = "manual"
)

// BookSyncConfig stores per-book sync configuration, allowing overrides of profile defaults.
type BookSyncConfig struct {
	ID             uint                    `gorm:"primaryKey" json:"id"`
	ProfileID      string                  `gorm:"index:idx_config_profile_id;uniqueIndex:idx_config_unique,priority:1;not null" json:"profile_id"`
	ABSBookID      uint                    `gorm:"index:idx_config_abs_book_id;uniqueIndex:idx_config_unique,priority:2;not null" json:"abs_book_id"`
	SyncEnabled    bool                    `gorm:"default:true" json:"sync_enabled"`
	ConflictMode   ConflictResolutionMode  `gorm:"type:varchar(30)" json:"conflict_mode,omitempty"` // Empty means use profile default
	Notes          string                  `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`

	// Relationships
	Profile SyncProfile `gorm:"foreignKey:ProfileID" json:"-"`
	ABSBook ABSBook     `gorm:"foreignKey:ABSBookID" json:"abs_book,omitempty"`
}

// BeforeCreate hook for BookSyncConfig
func (c *BookSyncConfig) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate hook for BookSyncConfig
func (c *BookSyncConfig) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

// ============================================================================
// HELPER TYPES FOR QUERIES
// ============================================================================

// BookComparison represents a joined view of ABS and Hardcover book data for comparison.
// This is not a database model, but a result type for queries.
type BookComparison struct {
	ABSBook           ABSBook            `json:"abs_book"`
	HardcoverBook     *HardcoverUserBook `json:"hardcover_book,omitempty"`
	Mapping           *BookMapping       `json:"mapping,omitempty"`
	Config            *BookSyncConfig    `json:"config,omitempty"`
	ProgressDiff      float64            `json:"progress_diff"`      // ABS progress - HC progress
	InSync            bool               `json:"in_sync"`            // True if progress diff < threshold
	SyncStatus        string             `json:"sync_status"`        // "in_sync", "needs_sync", "not_matched", "disabled"
}

// SyncSummary provides aggregated statistics about sync status for a profile.
type SyncSummary struct {
	TotalABSBooks      int `json:"total_abs_books"`
	TotalHCBooks       int `json:"total_hc_books"`
	MappedBooks        int `json:"mapped_books"`
	UnmappedBooks      int `json:"unmapped_books"`
	InSyncBooks        int `json:"in_sync_books"`
	NeedsSyncBooks     int `json:"needs_sync_books"`
	SyncDisabledBooks  int `json:"sync_disabled_books"`
}
