# Book Library Rearchitecture Plan

## Overview

This document outlines the plan to rearchitect the sync system from a "fetch-and-sync" model to a "store-compare-sync" model. The new architecture will:

1. **Store all book data** from both AudiobookShelf (ABS) and Hardcover (HC) in the local database
2. **Track progress history** over time for analytics and debugging
3. **Enable granular sync control** - sync individual books, batch sync, manual overrides
4. **Provide a comparison UI** - side-by-side view of ABS vs HC data with sync actions

## Current Architecture Problems

- **No persistent book storage**: Books are fetched, processed, and discarded each sync
- **State file drift**: State tracking is file-based and can get out of sync with reality
- **No comparison capability**: Cannot see what ABS has vs what Hardcover has
- **All-or-nothing sync**: Must sync entire library, cannot sync individual books
- **No conflict detection**: Always overwrites HC with ABS data without checking
- **Misleading counters**: "Books synced" counted skipped books incorrectly

## New Architecture

### Database Schema

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│   ABSBook       │     │   BookMapping    │     │  HardcoverUserBook  │
├─────────────────┤     ├──────────────────┤     ├─────────────────────┤
│ id (PK)         │     │ id (PK)          │     │ id (PK)             │
│ profile_id (FK) │────▶│ profile_id (FK)  │◀────│ profile_id (FK)     │
│ abs_id          │     │ abs_book_id (FK) │     │ hc_user_book_id     │
│ library_id      │     │ hc_user_book_id  │     │ hc_book_id          │
│ title           │     │ hc_edition_id    │     │ hc_edition_id       │
│ author          │     │ match_method     │     │ title               │
│ asin            │     │ confidence       │     │ author              │
│ isbn            │     │ manual_override  │     │ status              │
│ duration        │     │ created_at       │     │ progress            │
│ progress        │     │ updated_at       │     │ progress_seconds    │
│ is_finished     │     └──────────────────┘     │ started_at          │
│ started_at      │                              │ finished_at         │
│ finished_at     │     ┌──────────────────┐     │ last_fetched_at     │
│ last_fetched_at │     │ ProgressHistory  │     │ created_at          │
│ created_at      │     ├──────────────────┤     │ updated_at          │
│ updated_at      │     │ id (PK)          │     └─────────────────────┘
└─────────────────┘     │ profile_id (FK)  │
                        │ abs_book_id      │     ┌──────────────────┐
                        │ source (ABS/HC)  │     │ BookSyncConfig   │
                        │ progress         │     ├──────────────────┤
                        │ is_finished      │     │ id (PK)          │
                        │ recorded_at      │     │ profile_id (FK)  │
                        │ created_at       │     │ abs_book_id (FK) │
                        └──────────────────┘     │ sync_enabled     │
                                                 │ conflict_mode    │
┌─────────────────┐                              │ created_at       │
│   SyncEvent     │                              │ updated_at       │
├─────────────────┤                              └──────────────────┘
│ id (PK)         │
│ profile_id (FK) │
│ abs_book_id     │
│ event_type      │
│ trigger         │
│ old_progress    │
│ new_progress    │
│ details (JSON)  │
│ created_at      │
└─────────────────┘
```

### Component Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                           Web UI                                     │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌───────────┐ │
│  │Profiles │  │  Sync   │  │ Library │  │  Book   │  │  History  │ │
│  │  Tab    │  │ Status  │  │  Tab    │  │ Detail  │  │   View    │ │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘  └───────────┘ │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         API Layer                                    │
│  /api/profiles/{id}/books                                           │
│  /api/profiles/{id}/books/{id}/sync                                 │
│  /api/profiles/{id}/collect/abs                                     │
│  /api/profiles/{id}/collect/hardcover                               │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                       Service Layer                                  │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐           │
│  │ ABSCollector  │  │ HCCollector   │  │ SyncService   │           │
│  │               │  │ (rate-limited)│  │ (refactored)  │           │
│  └───────────────┘  └───────────────┘  └───────────────┘           │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      Repository Layer                                │
│  ABSBookRepository | HCBookRepository | MappingRepository           │
│  HistoryRepository | SyncEventRepository | ConfigRepository         │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         Database                                     │
│                    SQLite (existing)                                │
└─────────────────────────────────────────────────────────────────────┘
```

### Sync Flow (New)

```
1. COLLECT PHASE (Separate from sync)
   ┌─────────────────┐
   │ Trigger: Manual │──▶ ABSCollector.CollectAll()
   │ or Scheduled    │         │
   └─────────────────┘         ▼
                         ┌─────────────┐
                         │ Fetch from  │
                         │ ABS API     │
                         └─────────────┘
                               │
                               ▼
                         ┌─────────────┐
                         │ Upsert to   │
                         │ ABSBook     │
                         └─────────────┘
                               │
                               ▼
                         ┌─────────────┐
                         │ Auto-match  │
                         │ to HC books │
                         └─────────────┘

2. SYNC PHASE (Database-driven)
   ┌─────────────────┐
   │ Trigger: Manual │──▶ SyncService.SyncFromDatabase()
   │ or Scheduled    │         │
   └─────────────────┘         ▼
                         ┌─────────────────┐
                         │ Query ABSBooks  │
                         │ with mappings   │
                         └─────────────────┘
                               │
                               ▼
                         ┌─────────────────┐
                         │ For each book:  │
                         │ - Check config  │
                         │ - Compare prog. │
                         │ - Resolve conf. │
                         └─────────────────┘
                               │
                               ▼
                         ┌─────────────────┐
                         │ Update HC via   │
                         │ API if needed   │
                         └─────────────────┘
                               │
                               ▼
                         ┌─────────────────┐
                         │ Record history  │
                         │ and events      │
                         └─────────────────┘
```

## Implementation Phases

### Phase 1: Database Foundation
- [ ] Create new models in `internal/database/models.go`
- [ ] Add migration logic (AutoMigrate + seed from state files)
- [ ] Add repository methods for CRUD operations
- [ ] Add proper indexes for query performance

### Phase 2: Data Collection
- [ ] Create `internal/sync/abs_collector.go`
- [ ] Create `internal/sync/hc_collector.go` with rate limiting
- [ ] Add collection progress reporting
- [ ] Implement automatic book matching

### Phase 3: Sync Refactoring
- [ ] Add `SyncFromDatabase()` method to sync service
- [ ] Add `SyncSingleBook()` for individual book sync
- [ ] Add `SyncBatch()` for multi-book sync
- [ ] Implement conflict detection and resolution
- [ ] Record sync events and progress history

### Phase 4: API Layer
- [ ] Add book list endpoint with pagination/filtering
- [ ] Add book detail endpoint
- [ ] Add sync trigger endpoints (single, batch)
- [ ] Add collection trigger endpoints
- [ ] Add mapping override endpoint
- [ ] Add history endpoint

### Phase 5: Web UI
- [ ] Add "Library" tab to navigation
- [ ] Build book table with comparison columns
- [ ] Add sync status indicators and actions
- [ ] Add book detail modal with history chart
- [ ] Add bulk selection and batch operations
- [ ] Add collection trigger buttons with progress

## Configuration Changes

### New SyncConfigData Fields

```go
type SyncConfigData struct {
    // ... existing fields ...
    
    // Conflict resolution: prefer_abs | prefer_newest | prefer_hardcover | manual
    ConflictResolution string `json:"conflict_resolution"`
    
    // How often to refresh data from Hardcover (e.g., "24h")
    HCRefreshInterval string `json:"hc_refresh_interval"`
    
    // Threshold for "in sync" (percentage difference)
    InSyncThreshold float64 `json:"in_sync_threshold"`
    
    // Auto-collect from ABS before sync
    AutoCollectABS bool `json:"auto_collect_abs"`
}
```

### Per-Book Configuration

```go
type BookSyncConfig struct {
    // Override profile conflict resolution for this book
    ConflictResolution string `json:"conflict_resolution,omitempty"`
    
    // Disable sync for this book
    SyncEnabled bool `json:"sync_enabled"`
}
```

## API Endpoints

### Book Library

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/profiles/{id}/books` | List books with comparison data |
| GET | `/api/profiles/{id}/books/{bookId}` | Get book detail with history |
| POST | `/api/profiles/{id}/books/{bookId}/sync` | Sync single book |
| POST | `/api/profiles/{id}/books/sync` | Batch sync books |
| PUT | `/api/profiles/{id}/books/{bookId}/mapping` | Update book mapping |
| PUT | `/api/profiles/{id}/books/{bookId}/config` | Update book config |
| GET | `/api/profiles/{id}/books/{bookId}/history` | Get progress history |

### Data Collection

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/profiles/{id}/collect/abs` | Collect books from ABS |
| POST | `/api/profiles/{id}/collect/hardcover` | Collect books from HC |
| GET | `/api/profiles/{id}/collect/status` | Get collection status |

### History Management

| Method | Path | Description |
|--------|------|-------------|
| DELETE | `/api/profiles/{id}/history` | Purge old history |

## Rate Limiting Strategy

### Hardcover API Limits
- 60 requests per minute
- 30 second query timeout

### Implementation
- Token bucket rate limiter: 50 req/min (buffer for other operations)
- Batch collection with progress reporting
- Stale data detection: only refresh books not fetched in 24h
- On-demand single book refresh for detail views

## History Retention

### Policy
- Keep last 30 days of history
- OR keep last 100 entries per book (whichever is larger)
- Manual purge option via API
- Automatic purge on scheduled basis (optional)

### Purge Query
```sql
DELETE FROM progress_history
WHERE profile_id = ?
  AND created_at < datetime('now', '-30 days')
  AND id NOT IN (
    SELECT id FROM (
      SELECT id FROM progress_history
      WHERE profile_id = ? AND abs_book_id = progress_history.abs_book_id
      ORDER BY created_at DESC
      LIMIT 100
    )
  )
```

## Migration Plan

### From State Files
1. On first startup after upgrade, detect existing state files
2. For each book in state file:
   - Create ABSBook record with stored progress
   - Attempt to match to existing BookSyncLog entries for HC info
3. Mark state file as migrated (rename to `.migrated`)

### Backward Compatibility
- Keep BookSyncLog table for historical reference
- State files remain as backup/fallback
- Gradual migration, no big-bang cutover

## Success Metrics

1. **Accurate sync counts**: "Synced" = actually sent to Hardcover
2. **Visible comparison**: Can see ABS vs HC progress for any book
3. **Granular control**: Can sync individual books on demand
4. **Conflict awareness**: Know when HC has different data
5. **Historical tracking**: Can see progress changes over time
