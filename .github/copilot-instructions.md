# AudiobookShelf-Hardcover Sync Project Instructions

## Project Overview
This is a Go application that syncs reading progress and book data between AudiobookShelf and Hardcover. It uses GraphQL for Hardcover API interactions and REST for AudiobookShelf.

## Architecture Overview

### Core Design: Store-Compare-Sync
The application follows a "store-compare-sync" pattern:
1. **Collect** - Fetch book data from ABS and Hardcover, store in local database
2. **Compare** - Query database to compare progress between sources
3. **Sync** - Update Hardcover when differences detected, respecting conflict resolution rules

### Key Components
- **Database Layer** (`internal/database/`) - SQLite with GORM, stores books, mappings, history
- **Collectors** (`internal/sync/`) - `ABSCollector` and `HCCollector` fetch data into database
- **Sync Service** (`internal/sync/service.go`) - Orchestrates sync operations from database
- **API Layer** (`internal/api/`) - REST endpoints for web UI
- **Web UI** (`web/static/`) - Profile management, book library, sync controls

### Database Models
- `ABSBook` - AudiobookShelf book data with progress
- `HardcoverUserBook` - Hardcover user_book data with progress
- `BookMapping` - Links ABS books to Hardcover editions with match confidence
- `ProgressHistory` - Timestamped progress snapshots for tracking changes
- `SyncEvent` - Audit log of sync operations
- `BookSyncConfig` - Per-book sync settings (override profile defaults)
- `SyncProfile` - Multi-user profile with encrypted tokens
- `BookSyncLog` - Legacy sync log (kept for compatibility)

### Data Flow
```
ABS API ──▶ ABSCollector ──▶ ABSBook table
                                   │
                                   ▼
                            BookMapping table ◀── Auto-match (ASIN/ISBN/title)
                                   │
                                   ▼
HC API ◀── HCCollector ◀── HardcoverUserBook table
                │
                ▼
         SyncService.SyncFromDatabase()
                │
                ▼
         ProgressHistory + SyncEvent (audit trail)
```

## Development Guidelines

### Code Structure
- `cmd/audiobookshelf-hardcover-sync/` - Main entry point
- `internal/api/` - HTTP handlers and API layer
- `internal/api/audiobookshelf/` - ABS REST client
- `internal/api/hardcover/` - Hardcover GraphQL client
- `internal/config/` - Configuration loading
- `internal/database/` - GORM models and repository
- `internal/sync/` - Sync service, collectors, state management
- `internal/multiuser/` - Multi-profile orchestration
- `internal/logger/` - Structured logging
- `web/static/` - Web UI assets (HTML, CSS, JS)
- `docs/` - Feature documentation

### API Definitions

- Use GraphQL for Hardcover API, see `docs/hardcover-schema.graphql` for schema
- Use REST for AudiobookShelf API, documented in `docs/openapi.yaml`
- AudiobookShelf git project https://github.com/advplyr/audiobookshelf/tree/master

#### Hardcover API Limitations
- API is rate-limited to **60 requests per minute** - use token bucket rate limiter
- API tokens expire after 1 year, reset on January 1st
- Query timeout: 30 seconds
- Query depth limit: 3
- Disabled operators: `_like`, `_ilike`, `_regex`, `_similar` and variants
- **Important**: Ownership stored in `lists` table (via "Owned" list), NOT `user_books.owned`

#### Rate Limiting Strategy
- Use 50 req/min limit (buffer for concurrent operations)
- `HCCollector` implements token bucket rate limiter
- Batch operations with progress reporting
- Stale data detection: only refresh books not fetched in configurable interval

### Database Patterns

#### Models Location
All models in `internal/database/models.go`. Use GORM tags for schema.

#### Repository Pattern
- CRUD methods in `internal/database/repository.go`
- Use bulk upsert with `ON CONFLICT` for efficient updates
- Return errors, let caller decide how to handle

#### Indexes
Add indexes for frequently queried columns:
```go
type ABSBook struct {
    // ...
    ProfileID string `gorm:"index:idx_abs_profile_id"`
    ABSID     string `gorm:"index:idx_abs_id;uniqueIndex:idx_profile_abs_unique,priority:2"`
}
```

#### History Retention
- Keep 30 days OR 100 entries per book (whichever is larger)
- Implement `PurgeOldHistory()` with proper WHERE clause

### Sync Patterns

#### Conflict Resolution
Profile-level setting with per-book overrides:
- `prefer_abs` - Always use ABS progress (default)
- `prefer_newest` - Use whichever is more recent
- `prefer_hardcover` - Always use Hardcover progress
- `manual` - Skip and flag for user review

#### "In Sync" Detection
Books considered "in sync" if progress differs by less than threshold (default 1% or 60 seconds).

#### Single Book Sync
```go
func (s *Service) SyncSingleBook(ctx context.Context, absBookID string, force bool) error
```

#### Batch Sync
```go
func (s *Service) SyncBatch(ctx context.Context, absBookIDs []string) error
```

### Testing
- All new features should include comprehensive test coverage
- Use `go test -v ./...` to run all tests
- Test files: `*_test.go` in same package as source
- Use table-driven tests and subtests
- Mock external APIs using interfaces
- Test database operations with in-memory SQLite

### Web UI Patterns

#### API Response Format
```json
{
  "success": true,
  "data": { ... },
  "error": "optional error message"
}
```

#### Pagination
```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 1027,
    "total_pages": 21
  }
}
```

#### Book Library UI (Matching Tab)
- Renamed from "Library" to "Matching" tab for clarity
- Show side-by-side ABS vs HC progress
- Color coding: green (in sync), yellow (minor diff), red (major diff), gray (not matched)
- Support multi-select for batch operations
- Lazy load book details on expand
- Sortable columns: Title, Author, Diff, Updated (click headers to sort)
- Filter dropdowns: Status, Media Type, ABS Library
- localStorage persistence for tab selection, filters, sort preferences
- Search with 500ms debounce

#### Profile Data Management
- Purge feature: Delete all ABS books, HC books, mappings, configs, history for a profile
- Allows users to reset and re-collect data fresh
- Available via "Purge Data" button on profile cards

#### Collection Filtering
- Include/Exclude collections use collection **IDs** (not names)
- `ABSCollector` builds book ID → collection ID mappings
- `CollectionFilter.ShouldIncludeBook()` checks membership by ID

### Release Process
- Semantic versioning: v1.2.3
- Update version in `cmd/audiobookshelf-hardcover-sync/main.go`
- Update `CHANGELOG.md`
- Tag and push: `git tag v1.2.3 && git push origin v1.2.3`
- Create GitHub release with `gh release create`

### Makefile Tasks
- `make build` - Build binary with version info
- `make run` - Build and run locally
- `make test` - Run all tests
- `make lint` - Run linting tools
- `make docker-build` - Build Docker image
- `make docker-run` - Run in Docker container

### Code Style
- Follow idiomatic Go conventions (https://go.dev/doc/effective_go)
- Use named functions over long anonymous ones
- Organize logic into small, composable functions
- Prefer interfaces for dependencies to enable mocking
- Use gofmt or goimports to enforce formatting
- Avoid unnecessary abstraction; keep things simple

### Documentation
- Keep README.md updated with new features
- Update CHANGELOG.md for all releases
- Document configuration options thoroughly
- Feature docs in `docs/` directory

### Docker & Deployment
- Multi-stage Docker build with scratch base image
- Support for environment file configuration
- GitHub Container Registry for image publishing
- Docker Compose for easy local development