# AudiobookShelf-Hardcover Sync Project Instructions

## Project Overview
This is a **complete rewrite** of the original [drallgood/audiobookshelf-hardcover-sync](https://github.com/drallgood/audiobookshelf-hardcover-sync) project, forked and maintained by [alexnolan](https://github.com/alexnolan/audiobookshelf-hardcover-sync).

The application is a Go-based service that syncs reading progress and book data between AudiobookShelf and Hardcover. It features a modern web UI, multi-user support, database-driven architecture, and enterprise security features while maintaining 100% backward compatibility with the original single-user mode.

**Key Technologies:**
- Backend: Go with GORM (SQLite/PostgreSQL/MySQL support)
- GraphQL client for Hardcover API
- REST client for AudiobookShelf API
- Web UI: Vanilla JavaScript with modern CSS
- Authentication: Local (bcrypt) + OIDC/Keycloak
- Encryption: AES-256-GCM for token storage
- Containerization: Docker with multi-stage builds

## Architecture Overview

### Core Design: Store-Compare-Sync
The application follows a "store-compare-sync" pattern:
1. **Collect** - Fetch book data from ABS and Hardcover, store in local database
2. **Compare** - Query database to compare progress between sources
3. **Sync** - Update Hardcover when differences detected, respecting conflict resolution rules

### Key Components
- **Database Layer** (`internal/database/`) - Multi-database support (SQLite default, PostgreSQL, MySQL)
- **Collectors** (`internal/sync/`) - `ABSCollector` and `HCCollector` fetch data into database
- **Sync Service** (`internal/sync/service.go`) - Orchestrates sync operations from database
- **Multi-User Service** (`internal/multiuser/`) - Profile management and orchestration
- **Authentication** (`internal/auth/`) - Local auth (bcrypt) + OIDC/Keycloak integration
- **Encryption** (`internal/crypto/`) - AES-256-GCM token encryption
- **API Layer** (`internal/api/`) - REST API handlers for all operations
- **Server** (`internal/server/`) - HTTP server with routing and middleware
- **Web UI** (`web/static/`) - Modern SPA with profile, library, and tools tabs
- **CLI Tools** (`cmd/`) - edition-tool, image-tool, hardcover-lookup utilities

### Database Models
- `SyncProfile` - User profile with relationships to config and state
- `SyncProfileConfig` - Profile configuration with encrypted tokens
- `ProfileSyncState` - Sync state per profile (JSON string)
- `ABSBook` - AudiobookShelf book data with progress
- `HardcoverUserBook` - Hardcover user_book data with progress
- `BookMapping` - Links ABS books to Hardcover editions with match confidence
- `ProgressHistory` - Timestamped progress snapshots for tracking changes
- `SyncEvent` - Audit log of sync operations
- `BookSyncConfig` - Per-book sync settings (override profile defaults)
- `BookSyncLog` - Legacy sync log (kept for compatibility)
- `User` (auth) - Authentication user accounts with roles
- `OAuthState` (auth) - OAuth state tracking for OIDC flows

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

### Repository Information
- **Fork**: Complete rewrite from [drallgood/audiobookshelf-hardcover-sync](https://github.com/drallgood/audiobookshelf-hardcover-sync)
- **Maintained by**: [alexnolan](https://github.com/alexnolan/audiobookshelf-hardcover-sync)
- **License**: Apache 2.0
- **Container Registry**: ghcr.io/drallgood/audiobookshelf-hardcover-sync (maintained for compatibility)
- **Documentation**: Extensive docs in `docs/` directory covering architecture, authentication, migration

### Code Structure
- `cmd/audiobookshelf-hardcover-sync/` - Main entry point
- `cmd/edition-tool/` - Interactive tool for creating Hardcover audiobook editions
- `cmd/hardcover-lookup/` - Search tool for author, narrator, publisher IDs
- `cmd/image-tool/` - Cover image management and upload utility
- `internal/api/` - HTTP handlers and API layer
- `internal/api/audiobookshelf/` - ABS REST client
- `internal/api/hardcover/` - Hardcover GraphQL client
- `internal/api/audnex/` - Audnex API client for metadata enrichment
- `internal/auth/` - Authentication system (local + OIDC)
- `internal/config/` - Configuration loading
- `internal/crypto/` - AES-256-GCM encryption for tokens
- `internal/database/` - GORM models and repository
- `internal/sync/` - Sync service, collectors, state management
- `internal/multiuser/` - Multi-profile orchestration
- `internal/server/` - HTTP server with routing
- `internal/logger/` - Structured logging
- `internal/cache/` - Caching layer for API responses
- `internal/edition/` - Edition creation logic
- `internal/mismatch/` - Mismatch detection and export
- `web/static/` - Web UI assets (HTML, CSS, JS)
- `docs/` - Feature documentation (authentication, database, migration guides)
- `helm/` - Kubernetes Helm charts for deployment

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

#### Sync Modes
Three sync modes available via `SyncMode` configuration:
- `needs_sync` (default) - Syncs only books with progress differences
- `sync_all` - Syncs all matched books regardless of current state
- `collections` - Syncs only books in selected ABS collections

#### Conflict Resolution
Profile-level setting with per-book overrides:
- `prefer_abs` - Always use ABS progress (default)
- `prefer_newest` - Use whichever is more recent
- `prefer_hardcover` - Always use Hardcover progress
- `manual` - Skip and flag for user review

#### "In Sync" Detection
Books considered "in sync" if progress differs by less than threshold (default 1% or 60 seconds).

#### DNF (Did Not Finish) Handling
- `preserve_dnf` (default: true) - Respects DNF status in Hardcover
- When enabled, books marked as DNF in Hardcover are skipped during sync
- Prevents accidental status overrides

#### Finished Book Handling
- Books with `IsFinished=true` and valid `FinishedAt` treated as 100% progress
- Uses actual completion date from ABS instead of sync date
- Prevents finished books from flipping between statuses

#### Single Book Sync
```go
func (s *Service) SyncSingleBook(ctx context.Context, absBookID string, force bool) error
```

#### Batch Sync
```go
func (s *Service) SyncBatch(ctx context.Context, absBookIDs []string) error
```

#### Incremental Sync
- Profile-specific state files: `./data/sync_state.{profileID}.json`
- Tracks last sync time, progress snapshots
- Only syncs changed books on subsequent runs
- Configurable via `sync.incremental` (default: true)

### Testing
- All new features should include comprehensive test coverage
- Use `go test -v ./...` to run all tests
- Test files: `*_test.go` in same package as source
- Use table-driven tests and subtests
- Mock external APIs using interfaces
- Test database operations with in-memory SQLite
- Test utilities in `internal/testutils/` for common mocks
- Authentication tests use mock providers

### Configuration

#### Configuration Methods (Priority Order)
1. **Environment Variables** (highest priority)
2. **config.yaml** (medium priority)  
3. **Default values** (lowest priority)

#### Key Configuration Sections

**Server Settings:**
- `server.host` - HTTP server bind address (default: "0.0.0.0")
- `server.port` - HTTP server port (default: 8765)

**Authentication:**
- `authentication.enabled` - Enable/disable authentication (default: false)
- `authentication.session.secret` - Session signing key (auto-generated)
- `authentication.session.max_age` - Session expiration in seconds (default: 86400)
- `authentication.default_admin` - Default admin credentials
- `authentication.oidc` - OIDC/Keycloak provider configuration

**Database:**
- `database.type` - Database type: sqlite (default), postgres, mysql
- `database.path` - SQLite file path (default: "./data/audiobookshelf-hardcover-sync.db")
- `database.host`, `database.port`, `database.name` - For PostgreSQL/MySQL

**Encryption:**
- `encryption.key_file` - Path to encryption key (default: "./data/encryption.key")
- `ENCRYPTION_KEY` env var - Base64-encoded 32-byte key (optional, auto-generated)

**Rate Limiting:**
- `rate_limit.rate` - Min time between requests (default: "1200ms")
- `rate_limit.burst` - Max burst size (default: 2)
- `rate_limit.max_concurrent` - Max concurrent requests (default: 5)

**Sync Configuration (Profile-Level):**
- `sync_interval` - Periodic sync interval (e.g., "1h", "30m")
- `minimum_progress` - Min progress to sync (default: 0.01 = 1%)
- `sync_want_to_read` - Sync 0% books as "Want to Read" (default: true)
- `process_unread_books` - Include unread books in sync (default: true)
- `sync_owned` - Mark synced books as owned (default: true)
- `include_ebooks` - Include ebook media type (default: false)
- `preserve_dnf` - Preserve DNF status (default: true)
- `incremental` - Enable incremental sync (default: true)
- `min_change_threshold` - Min progress change in seconds (default: 60)

**Library Filtering:**
- `library_filter_mode` - "include" or "exclude" (default: empty = all libraries)
- `filtered_libraries` - Array of library names to include/exclude
- `include_collections` - ABS collection IDs to include
- `exclude_collections` - ABS collection IDs to exclude

**Paths:**
- `paths.data_dir` - Data directory (default: "./data")
- `paths.cache_dir` - Cache directory (default: "./cache")
- `paths.mismatch_output_dir` - Mismatch files (default: "./mismatches")

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
- **Edition Management**: Search Hardcover, create editions, manual mapping
- **Progress History**: View complete sync history per book
- **Add to Hardcover**: Create missing books/editions directly from UI

#### Tools Tab (New)
- **Edition Tool**: Interactive wizard for creating audiobook editions on Hardcover
- **Image Tool**: Upload and manage cover images
- **Hardcover Lookup**: Search for author, narrator, publisher IDs
- Integrated directly into web UI for convenience

#### Authentication UI
- Login page at `/login` with username/password or OAuth providers
- Session management with HTTP-only cookies
- CSRF protection for all authenticated endpoints
- Role-based UI features (admin sees all profiles, users see their own)

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
- **Migration Guide**: `MIGRATION.md` for upgrading from single-user to multi-user
- **Authentication Guide**: `docs/AUTHENTICATION.md` for auth setup
- **Database Guide**: `docs/DATABASE.md` for schema and repository patterns

### CLI Tools

#### edition-tool
Interactive wizard for creating missing audiobook editions on Hardcover:
```bash
go run cmd/edition-tool/main.go
# OR
./edition-tool
```
Guides through edition creation with auto-population from ABS data.

#### hardcover-lookup
Search and verify author, narrator, publisher IDs on Hardcover:
```bash
go run cmd/hardcover-lookup/main.go
# OR
./hardcover-lookup
```
Interactive search for people and publishers by name.

#### image-tool
Cover image management and upload utility:
```bash
go run cmd/image-tool/main.go
# OR
./image-tool
```
Uploads cover images to Hardcover for books/editions.

### Security Considerations

#### Token Encryption
- All API tokens (ABS, Hardcover) encrypted with AES-256-GCM
- Encryption key auto-generated on first run, stored in `data/encryption.key`
- **CRITICAL**: Encryption key must persist with database or tokens become unrecoverable
- Docker: Mount `/app/data` volume to persist encryption key and database together

#### Authentication
- Local auth uses bcrypt for password hashing (cost factor 10)
- Sessions use HTTP-only cookies with configurable expiration
- CSRF protection on all authenticated endpoints
- Role-based access control: Admin, User, Viewer
- OIDC/Keycloak integration for enterprise SSO
- Session secrets auto-generated if not configured

#### PII Protection
- **NEVER commit** AudiobookShelf API response files (contain PII)
- Added to `.gitignore`: `audiobookshelf_raw_response.json`, `*_response.json`
- Mismatch export files sanitized to remove sensitive data

#### Database Security
- Tokens stored encrypted, never in plaintext
- Use `internal/crypto` package for all encryption/decryption
- Support for external databases (PostgreSQL, MySQL) with TLS connections

### Docker & Deployment
- Multi-stage Docker build with scratch base image
- Support for environment file configuration
- GitHub Container Registry for image publishing (ghcr.io/drallgood/audiobookshelf-hardcover-sync)
- Docker Compose for easy local development
- **Kubernetes Support**: Helm charts in `helm/` directory
- **Beta Releases**: Auto-deployment from `develop` branch with `beta-{build}-{sha}` tags
- **ArgoCD Integration**: Support for automated beta deployments
- Volume mounts: `/app/config`, `/app/data`, `/app/mismatches`

### API Endpoints

#### Authentication (No Auth Required)
- `GET /health` - Health check
- `GET /login` - Serve login page
- `POST /api/auth/login` - Handle login form submission
- `GET /api/auth/me` - Check authentication status
- `GET /auth/callback/{provider}` - OAuth callback handler
- `GET /auth/oauth/{provider}` - OAuth login initiation
- `POST /api/auth/logout` - Logout and clear session
- `GET /api/status` - General status check (all profiles)
- `POST /api/sync` - Legacy sync endpoint (backward compatibility)

#### Profile Management (Authenticated)
- `GET /api/profiles` - List all profiles
- `POST /api/profiles` - Create new profile
- `GET /api/profiles/{id}` - Get profile details
- `PUT /api/profiles/{id}` - Update profile
- `DELETE /api/profiles/{id}` - Delete profile
- `PUT /api/profiles/{id}/config` - Update profile configuration
- `GET /api/profiles/{id}/status` - Get sync status
- `POST /api/profiles/{id}/sync` - Start sync operation
- `DELETE /api/profiles/{id}/sync` - Cancel sync operation
- `POST /api/profiles/{id}/purge` - Purge all profile data
- `GET /api/profiles/{id}/summary` - Get profile summary
- `GET /api/profiles/{id}/book-syncs` - List book sync logs
- `GET /api/profiles/{id}/book-syncs/{audiobookId}` - Get single book sync log
- `GET /api/profiles/{id}/sync-events` - Get sync event history
- `GET /api/profiles/{id}/abs/libraries` - List ABS libraries
- `GET /api/profiles/{id}/abs/collections` - List ABS collections

#### Library/Book Management (Authenticated)
- `GET /api/profiles/{id}/books` - List all books (paginated, filterable)
- `GET /api/profiles/{id}/books/{absBookId}` - Get book details
- `POST /api/profiles/{id}/books/{absBookId}/sync` - Sync single book
- `POST /api/profiles/{id}/books/sync-batch` - Sync multiple books
- `POST /api/profiles/{id}/collect/abs` - Collect ABS data
- `POST /api/profiles/{id}/collect/hardcover` - Collect Hardcover data
- `POST /api/profiles/{id}/auto-match` - Auto-match books
- `GET /api/profiles/{id}/sync-summary` - Get sync summary statistics
- `GET /api/profiles/{id}/library-summary` - Get library summary
- `GET /api/profiles/{id}/books/{absBookId}/history` - Get progress history
- `POST /api/profiles/{id}/books/{absBookId}/mapping` - Create manual mapping
- `DELETE /api/profiles/{id}/books/{absBookId}/mapping` - Delete mapping
- `GET /api/profiles/{id}/editions` - Search editions
- `PUT /api/profiles/{id}/books/{absBookId}/edition` - Update edition mapping
- `GET /api/profiles/{id}/books/{absBookId}/search-hardcover` - Search Hardcover for book
- `GET /api/profiles/{id}/search-hardcover` - Search Hardcover by query
- `POST /api/profiles/{id}/books/{absBookId}/add-to-hardcover` - Add book to Hardcover

#### Edition/People Management (Authenticated)
- `GET /api/profiles/{id}/search-people` - Search for authors/narrators
- `GET /api/profiles/{id}/search-publishers` - Search for publishers
- `POST /api/profiles/{id}/editions` - Create new edition
- `GET /api/profiles/{id}/editions/prepopulate` - Get prepopulated edition data
- `POST /api/profiles/{id}/upload-image` - Upload cover image

#### Static Files
- `GET /` - Serve web UI (index.html, library.html, etc.)
- All static assets served from `web/static/`

### Backward Compatibility

#### Single-User Mode (Legacy)
The application maintains 100% backward compatibility with the original single-user mode:
- Set `AUDIOBOOKSHELF_URL`, `AUDIOBOOKSHELF_TOKEN`, `HARDCOVER_TOKEN` environment variables
- No authentication required when auth is disabled
- Legacy `/api/sync` endpoint still functional
- Original configuration format still supported
- Automatic migration from single-user config to multi-user database on first run

#### Environment Variable Migration
All original environment variables are still supported:
- `AUDIOBOOKSHELF_URL` → profile config
- `AUDIOBOOKSHELF_TOKEN` → encrypted in database
- `HARDCOVER_TOKEN` → encrypted in database
- `LOG_LEVEL`, `LOG_FORMAT` → logging config
- `SYNC_INTERVAL`, `MINIMUM_PROGRESS`, `SYNC_WANT_TO_READ` → sync config
- See `MIGRATION.md` for complete mapping

#### Breaking Changes from Original
- `HARDCOVER_SYNC_DELAY_MS` removed (replaced with token bucket rate limiting)
- `AUDIOBOOK_MATCH_MODE` removed (improved matching is default)
- Progress tracking now uses seconds instead of percentages (automatic conversion)

### Development Workflow

#### Local Development Setup
```bash
# Clone repository
git clone https://github.com/alexnolan/audiobookshelf-hardcover-sync
cd audiobookshelf-hardcover-sync

# Install dependencies
go mod download

# Copy config example
cp config.example.yaml config.yaml
# Edit config.yaml with your settings

# Run locally
make run
# OR
go run cmd/audiobookshelf-hardcover-sync/main.go
```

#### Running Tests
```bash
# Run all tests
make test

# Run specific package tests
go test -v ./internal/sync/...

# Run with coverage
go test -v -cover ./...

# Run with race detector
go test -race ./...
```

#### Linting
```bash
# Run linter
make lint

# Auto-fix linting issues
go fmt ./...
goimports -w .
```

#### Building
```bash
# Build binary
make build

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o build/sync-linux-amd64 cmd/audiobookshelf-hardcover-sync/main.go

# Build Docker image
make docker-build

# Build with version info
go build -ldflags "-X main.Version=v1.2.3" cmd/audiobookshelf-hardcover-sync/main.go
```

#### Working with Database
```bash
# Run migrations (automatic on startup)
# Database auto-migrates to latest schema

# Inspect database
sqlite3 data/audiobookshelf-hardcover-sync.db

# Useful queries
SELECT * FROM sync_profiles;
SELECT * FROM book_mappings WHERE match_confidence > 0.8;
SELECT * FROM progress_history WHERE abs_book_id = 'xxx' ORDER BY created_at DESC;
```

#### Hot Reload for Development
```bash
# Install air for hot reload
go install github.com/cosmtrek/air@latest

# Run with hot reload
air
```

#### Debugging
- Set `LOG_LEVEL=debug` for verbose logging
- Use `DRY_RUN=true` to test without making changes to Hardcover
- Check `data/sync_state.{profileID}.json` for state issues
- View `mismatches/` directory for books that couldn't be matched
- Use browser DevTools to inspect web UI API calls

#### Common Development Tasks
1. **Adding a new API endpoint**: Update `internal/server/server.go` with route, add handler in `internal/api/`
2. **Adding a database model**: Add to `internal/database/models.go`, create migration logic
3. **Adding a config option**: Update `internal/config/config.go`, add to `config.example.yaml`
4. **Adding a web UI feature**: Update HTML in `web/static/`, add API handlers as needed
5. **Modifying sync logic**: Update `internal/sync/service.go` and test thoroughly

### Architecture Patterns

#### Store-Compare-Sync Pattern
1. **Collect Phase**: Fetch data from ABS and Hardcover into local database
2. **Compare Phase**: Query database to identify books needing sync
3. **Sync Phase**: Apply changes to Hardcover based on comparison results

This pattern enables:
- Book-level control and conflict resolution
- Progress history tracking
- Offline analysis and debugging
- Incremental syncs based on state

#### Collector Pattern
Collectors are responsible for fetching data from external APIs and storing in database:
- `ABSCollector` - Fetches from AudiobookShelf REST API
- `HCCollector` - Fetches from Hardcover GraphQL API
- Both implement rate limiting and error handling
- Support for incremental collection based on last fetch time

#### Repository Pattern
All database operations go through `internal/database/repository.go`:
- CRUD operations for all models
- Bulk upsert with `ON CONFLICT` handling
- Transaction support for atomic operations
- Returns errors, callers decide how to handle

#### Service Layer Pattern
Business logic in service layer (`internal/sync/service.go`, `internal/multiuser/service.go`):
- Orchestrates collectors and repository
- Implements sync logic and conflict resolution
- Handles state management and history tracking
- Independent of HTTP layer for testability

### Key Design Decisions

#### Why Database-Driven?
Original was stateless; each run re-fetched everything. Database enables:
- Incremental syncs (faster, less API usage)
- Progress history and audit trails
- Per-book configuration and overrides
- Multi-user support with isolated data
- Offline analysis and debugging

#### Why GraphQL for Hardcover?
Hardcover API is GraphQL-first:
- More efficient queries (request only needed fields)
- Better schema documentation
- Type safety with generated types
- Single endpoint for all operations

#### Why Multi-User Architecture?
Users requested:
- Multiple AudiobookShelf instances
- Multiple Hardcover accounts
- Household/family sharing scenarios
- Separate sync configurations per user

#### Why Token Encryption?
Security best practice:
- Tokens are sensitive credentials
- Database files could be backed up to cloud
- Compliance requirements (GDPR, etc.)
- Defense in depth security

### Performance Considerations

#### Rate Limiting
- Hardcover API: 60 req/min limit, we use 50 req/min for safety
- Token bucket implementation with burst support
- Per-profile rate limiting for multi-user
- Exponential backoff on rate limit errors

#### Caching
- Cache Hardcover edition searches (`internal/cache/`)
- TTL-based expiration (configurable)
- In-memory cache, not persisted
- Reduces redundant API calls

#### Database Optimization
- Indexes on frequently queried columns
- Bulk upserts instead of individual inserts
- Lazy loading of relationships
- Connection pooling for PostgreSQL/MySQL

#### Incremental Sync
- Only fetch changed books from ABS
- Track last sync time per profile
- Skip books already in sync
- Reduces sync time from minutes to seconds