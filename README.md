# AudiobookShelf-Hardcover Sync

[![Trivy Scan](https://github.com/alexnolan/audiobookshelf-hardcover-sync/actions/workflows/trivy.yml/badge.svg)](https://github.com/alexnolan/audiobookshelf-hardcover-sync/actions/workflows/trivy.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/alexnolan/audiobookshelf-hardcover-sync)](https://goreportcard.com/report/github.com/alexnolan/audiobookshelf-hardcover-sync)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/alexnolan/audiobookshelf-hardcover-sync?label=latest%20release)](https://github.com/alexnolan/audiobookshelf-hardcover-sync/releases/latest)

> **Note**: This is a complete rewrite and enhancement of the original [drallgood/audiobookshelf-hardcover-sync](https://github.com/drallgood/audiobookshelf-hardcover-sync) project. While maintaining full backward compatibility, this fork introduces a modern web UI, multi-user support, database-driven architecture, and advanced book management features.

Automatically synchronize your [AudiobookShelf](https://www.audiobookshelf.org/) library with [Hardcover](https://hardcover.app/), including reading progress, book status, and ownership information. Built with Go for reliability and performance.

---

## ✨ What's New in This Fork

This fork represents a **complete architectural rewrite** with major enhancements:

- 🎨 **Modern Web UI** - Beautiful, responsive interface for managing profiles and books
- 👥 **Multi-User/Multi-Profile** - Support multiple AudiobookShelf and Hardcover accounts
- 🗄️ **Database-Driven** - SQLite backend with GORM for robust data management
- 📚 **Store-Compare-Sync** - Advanced book-level control with conflict resolution
- 🔐 **Enterprise Security** - AES-256-GCM encryption, authentication (local + OIDC/Keycloak)
- 🎯 **Per-Book Control** - Individual sync settings and conflict resolution strategies
- 📊 **Progress Tracking** - Complete history and audit trails for all operations
- 🚀 **REST API** - Full programmatic control via RESTful endpoints
- ⚡ **Enhanced Performance** - Intelligent caching and incremental sync
- 🔧 **Developer Tools** - Edition creation, image management, ID lookup utilities

All while maintaining **100% backward compatibility** with the original single-user mode.

---

## 🌟 Key Features

### 🎯 Core Sync Capabilities
- **Automatic Progress Sync** - Keep your reading progress synchronized across platforms
- **Smart Status Management** - Automatically sets "Want to Read", "Currently Reading", and "Read" status
- **Ownership Tracking** - Marks synced books as "owned" in Hardcover
- **Incremental Sync** - Efficient state-based syncing processes only changed books
- **DNF Preservation** - Respects "Did Not Finish" status in Hardcover
- **Finished Book Detection** - Accurate completion tracking with date preservation
- **Library Filtering** - Include/exclude specific AudiobookShelf libraries
- **Collection Support** - Filter by AudiobookShelf collections

### 🌐 Web Interface & Multi-User
- **Modern Dashboard** - Intuitive web UI at `http://localhost:8765`
- **Profile Management** - Create and manage multiple sync profiles
- **Book Library View** - Side-by-side comparison of AudiobookShelf vs Hardcover progress
- **Real-Time Status** - Live sync monitoring with auto-refresh
- **Batch Operations** - Sync multiple books simultaneously
- **Manual Mapping** - Override automatic book matching when needed
- **Progress History** - View complete sync history per book
- **Sync Configuration** - Per-profile and per-book sync settings

### 🔒 Security & Authentication
- **Token Encryption** - AES-256-GCM encryption for API tokens at rest
- **Local Authentication** - Username/password with bcrypt hashing
- **OIDC/Keycloak** - Enterprise SSO integration
- **Role-Based Access** - Admin, User, and Viewer roles
- **Session Management** - Secure session handling with CSRF protection
- **Optional Auth** - Can run without authentication for home use

### 🎨 Advanced Features
- **Conflict Resolution** - Multiple strategies: prefer ABS, prefer Hardcover, prefer newest, manual review
- **Smart Matching** - Automatic book matching by ASIN, ISBN, or title/author
- **Progress Threshold** - Configurable "in sync" detection (default 1% difference)
- **Audit Trail** - Complete history of all sync operations
- **Dry Run Mode** - Test syncs without making changes
- **Rate Limiting** - Respects Hardcover API limits (60 req/min)
- **Multi-Database** - SQLite (default), PostgreSQL, MySQL/MariaDB support

### 🛠️ Developer Tools
- **edition-tool** - Interactive tool for creating missing audiobook editions on Hardcover
- **image-tool** - Cover image management and upload utility
- **hardcover-lookup** - Search and verify author, narrator, publisher IDs
- **REST API** - Full programmatic control via `/api/*` endpoints
- **GraphQL Client** - Direct Hardcover GraphQL API integration

---

## 🚀 Quick Start

### Option 1: Docker (Recommended)

The fastest way to get started with the web UI and multi-user support:

```bash
# Create directories
mkdir -p ~/audiobookshelf-hardcover-sync/{config,data}
cd ~/audiobookshelf-hardcover-sync

# Create docker-compose.yml
cat > docker-compose.yml <<'EOF'
version: '3.8'

services:
  audiobookshelf-hardcover-sync:
    image: ghcr.io/drallgood/audiobookshelf-hardcover-sync:latest
    container_name: abs-hardcover-sync
    restart: unless-stopped
    ports:
      - "8765:8765"
    volumes:
      - ./config:/app/config
      - ./data:/data
    environment:
      - ENABLE_WEB_UI=true
      - LOG_LEVEL=info
      - DATABASE_PATH=/data/audiobookshelf-hardcover-sync.db
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:8765/health"]
      interval: 30s
      timeout: 10s
      retries: 3
EOF

# Start the service
docker compose up -d

# View logs
docker compose logs -f
```

**Access the web interface** at `http://localhost:8765` and create your first sync profile!

### Option 2: Docker CLI

```bash
docker run -d \
  --name abs-hardcover-sync \
  -p 8765:8765 \
  -v $(pwd)/config:/app/config \
  -v $(pwd)/data:/data \
  -e ENABLE_WEB_UI=true \
  ghcr.io/drallgood/audiobookshelf-hardcover-sync:latest
```

### Option 3: Binary Installation

1. **Download the latest release** from [GitHub Releases](https://github.com/alexnolan/audiobookshelf-hardcover-sync/releases)

2. **Extract and configure**:
   ```bash
   tar -xzf audiobookshelf-hardcover-sync-*.tar.gz
   cd audiobookshelf-hardcover-sync
   cp config.example.yaml config.yaml
   # Edit config.yaml with your settings
   ```

3. **Run with web UI enabled**:
   ```bash
   ENABLE_WEB_UI=true ./audiobookshelf-hardcover-sync --server-only
   ```

4. **Access the dashboard** at `http://localhost:8765`

### Option 4: Legacy Single-User Mode

For backward compatibility with the original version:

```bash
# Configure via config.yaml or environment variables
export AUDIOBOOKSHELF_URL="https://your-abs-instance.com"
export AUDIOBOOKSHELF_TOKEN="your-abs-token"
export HARDCOVER_TOKEN="your-hardcover-token"

# Run sync
./audiobookshelf-hardcover-sync
```

---

## 📖 Usage Guide

### Using the Web Interface (Recommended)

1. **Create a Profile**
   - Navigate to the "Add Profile" tab
   - Enter a profile name (e.g., "My Books")
   - Add your AudiobookShelf URL and token
   - Add your Hardcover API token
   - Configure sync settings (conflict resolution, intervals, etc.)
   - Click "Create Profile"

2. **Manage Your Library**
   - Switch to the "Matching" tab
   - Select your profile from the dropdown
   - Click "Collect ABS Books" to fetch your AudiobookShelf library
   - Click "Collect Hardcover Books" to fetch your Hardcover progress
   - View side-by-side comparison of progress across platforms
   - Books are color-coded:
     - 🟢 Green: In sync (< 1% difference)
     - 🟡 Yellow: Minor difference (1-5%)
     - 🔴 Red: Major difference (> 5%)
     - ⚪ Gray: Not matched to Hardcover

3. **Sync Books**
   - Click "Sync All" to sync all books
   - Or use checkboxes to select specific books and click "Sync Selected"
   - Click on any book row to view detailed progress history
   - Configure per-book sync settings via the settings icon

4. **Monitor Progress**
   - Check the "Sync Status" tab for real-time sync operations
   - View "Book Sync Logs" for historical sync events
   - Each profile syncs independently

### Command-Line Interface

The application supports several CLI flags for different operation modes:

#### Available Flags

```bash
# Show help
./audiobookshelf-hardcover-sync --help

# Show version
./audiobookshelf-hardcover-sync --version

# Run sync once and exit (useful for cron jobs)
./audiobookshelf-hardcover-sync --once

# Start server without periodic sync (web UI only)
./audiobookshelf-hardcover-sync --server-only

# Configuration file path
./audiobookshelf-hardcover-sync --config /path/to/config.yaml

# Override config with CLI flags
./audiobookshelf-hardcover-sync \
  --audiobookshelf-url https://abs.example.com \
  --audiobookshelf-token your-token \
  --hardcover-token your-token \
  --sync-interval 1h

# Dry run mode (no changes will be made)
./audiobookshelf-hardcover-sync --dry-run

# Testing options
./audiobookshelf-hardcover-sync \
  --test-book-filter "Harry Potter" \
  --test-book-limit 5
```

#### Common Usage Patterns

**One-time sync** (e.g., from cron):
```bash
./audiobookshelf-hardcover-sync --once --config /etc/abs-sync/config.yaml
```

**Web UI with auto-sync**:
```bash
ENABLE_WEB_UI=true ./audiobookshelf-hardcover-sync
```

**Web UI without auto-sync** (manual control only):
```bash
ENABLE_WEB_UI=true ./audiobookshelf-hardcover-sync --server-only
```

**Legacy single-user mode** (periodic sync, no web UI):
```bash
./audiobookshelf-hardcover-sync --config config.yaml
```

### Using the REST API

The application provides a comprehensive REST API for programmatic control:

#### Profile Management
```bash
# List all profiles
curl http://localhost:8765/api/profiles

# Create a new profile
curl -X POST http://localhost:8765/api/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Profile",
    "audiobookshelf_url": "https://abs.example.com",
    "audiobookshelf_token": "your-token",
    "hardcover_token": "your-token",
    "conflict_resolution": "prefer_abs",
    "sync_interval": "1h"
  }'

# Update a profile
curl -X PUT http://localhost:8765/api/profiles/{id} \
  -H "Content-Type: application/json" \
  -d '{"name": "Updated Name"}'

# Delete a profile
curl -X DELETE http://localhost:8765/api/profiles/{id}
```

#### Sync Operations
```bash
# Start sync for a profile
curl -X POST http://localhost:8765/api/profiles/{id}/sync

# Get sync status
curl http://localhost:8765/api/profiles/{id}/status

# Cancel running sync
curl -X DELETE http://localhost:8765/api/profiles/{id}/sync

# Purge all data for a profile (books, mappings, history)
curl -X POST http://localhost:8765/api/profiles/{id}/purge
```

#### Book Management
```bash
# List books for a profile
curl http://localhost:8765/api/profiles/{id}/books

# Get specific book details
curl http://localhost:8765/api/profiles/{id}/books/{absBookId}

# Sync a single book
curl -X POST http://localhost:8765/api/profiles/{id}/books/{absBookId}/sync

# Sync multiple books
curl -X POST http://localhost:8765/api/profiles/{id}/books/sync-batch \
  -H "Content-Type: application/json" \
  -d '{"abs_book_ids": ["book1", "book2"]}'

# Get book sync history
curl http://localhost:8765/api/profiles/{id}/books/{absBookId}/history

# Manually map a book to Hardcover edition
curl -X POST http://localhost:8765/api/profiles/{id}/books/{absBookId}/mapping \
  -H "Content-Type: application/json" \
  -d '{"hardcover_edition_id": 12345, "confidence": 100}'
```

#### Collection Operations
```bash
# Collect AudiobookShelf books
curl -X POST http://localhost:8765/api/profiles/{id}/collect/abs

# Collect Hardcover books
curl -X POST http://localhost:8765/api/profiles/{id}/collect/hardcover
```

For complete API documentation, see the [OpenAPI specification](docs/openapi.yaml).

### Using Developer Tools

#### Edition Tool - Create Missing Audiobook Editions

```bash
# Interactive mode
./edition-tool

# With pre-filled data
./edition-tool \
  --title "The Hobbit" \
  --authors "J.R.R. Tolkien" \
  --narrators "Andy Serkis" \
  --publisher "HarperCollins" \
  --asin "B09B6QWPQJ" \
  --duration 11.08
```

#### Image Tool - Upload Cover Images

```bash
# Upload a cover image for an edition
./image-tool \
  --edition-id 12345 \
  --image-path ./covers/book.jpg \
  --hardcover-token "your-token"
```

#### Hardcover Lookup - Search IDs

```bash
# Search for an author
./hardcover-lookup author "J.R.R. Tolkien"

# Search for a narrator
./hardcover-lookup narrator "Andy Serkis"

# Search for a publisher
./hardcover-lookup publisher "HarperCollins"
```

---

## ⚙️ Configuration

### Configuration Methods

The application supports three configuration methods (in order of precedence):

1. **Environment Variables** - Highest priority
2. **Config File** (`config.yaml`) - Standard configuration
3. **Web UI** - Profile-specific settings (for multi-user mode)

### Configuration File

Create a `config.yaml` file based on the example:

```bash
cp config.example.yaml config.yaml
```

#### Essential Settings

```yaml
# Server configuration
server:
  port: "8765"
  enable_web_ui: true  # Enable web UI for multi-user mode

# Rate limiting (respect Hardcover API limits)
rate_limit:
  rate: "1500ms"       # ~40 requests per minute
  burst: 2
  max_concurrent: 3

# Logging
logging:
  level: "info"        # debug, info, warn, error
  format: "console"    # console or json

# Database (SQLite default)
database:
  type: "sqlite"
  path: "./data/database.db"

# Authentication (optional)
authentication:
  enabled: false       # Set to true for multi-user with auth
  session:
    secret: ""         # Auto-generated if empty
    max_age: 86400     # 24 hours
  default_admin:
    username: "admin"
    email: "admin@localhost"
    password: ""       # Required if auth enabled
  # Optional: Keycloak/OIDC
  keycloak:
    enabled: false
    issuer: ""
    client_id: ""
    client_secret: ""
```

#### Single-User Legacy Settings

For backward compatibility with the original project:

```yaml
# AudiobookShelf connection
audiobookshelf:
  url: "https://your-audiobookshelf-instance.com"
  token: "your-audiobookshelf-token"

# Hardcover connection
hardcover:
  token: "your-hardcover-token"

# Sync configuration
sync:
  sync_interval: "1h"
  minimum_progress: 0.01       # Skip books with < 1% progress
  sync_want_to_read: true      # Sync 0% books as "Want to Read"
  sync_owned: true             # Mark synced books as owned
  preserve_dnf: true           # Don't overwrite DNF status
  include_ebooks: false        # Skip ebooks by default
  incremental: true            # Only sync changed books
  min_change_threshold: 60     # Minimum 60s change to trigger sync
  dry_run: false               # Set true to test without changes
  
  # Library filtering
  libraries:
    include: []                # Include only these (empty = all)
    exclude: []                # Exclude these (empty = none)
```

### Environment Variables

All settings can be overridden via environment variables:

#### Core Settings
```bash
# Server
export SERVER_PORT=8765
export ENABLE_WEB_UI=true
export LOG_LEVEL=info

# Database
export DATABASE_TYPE=sqlite
export DATABASE_PATH=/data/database.db

# Encryption (multi-user mode)
export ENCRYPTION_KEY=base64-encoded-32-byte-key
export DATA_DIR=/data
```

#### Legacy Single-User Mode
```bash
# AudiobookShelf
export AUDIOBOOKSHELF_URL=https://abs.example.com
export AUDIOBOOKSHELF_TOKEN=your-token

# Hardcover
export HARDCOVER_TOKEN=your-token

# Sync settings
export SYNC_INTERVAL=1h
export SYNC_WANT_TO_READ=true
export SYNC_OWNED=true
export PRESERVE_DNF=true
export INCLUDE_EBOOKS=false
export DRY_RUN=false
```

#### Authentication (Optional)
```bash
export AUTH_ENABLED=true
export AUTH_DEFAULT_ADMIN_PASSWORD=secure-password
export AUTH_SESSION_SECRET=random-secret-key

# Keycloak/OIDC (optional)
export AUTH_KEYCLOAK_ENABLED=true
export AUTH_KEYCLOAK_ISSUER=https://keycloak.example.com/realms/myrealm
export AUTH_KEYCLOAK_CLIENT_ID=audiobookshelf-sync
export AUTH_KEYCLOAK_CLIENT_SECRET=your-secret
```

### Getting API Tokens

#### AudiobookShelf Token

1. Open your AudiobookShelf instance
2. Go to **Settings → Users → Your User**
3. Scroll to **API Token** section
4. Click **Generate New Token**
5. Copy the token

#### Hardcover Token

1. Go to [Hardcover API Settings](https://hardcover.app/settings/api)
2. Log in to your account
3. Click **Generate New Token**
4. Copy the token

---

## 🐳 Docker Deployment

### Docker Compose (Production)

Create a production-ready `docker-compose.yml`:

```yaml
version: '3.8'

services:
  audiobookshelf-hardcover-sync:
    image: ghcr.io/drallgood/audiobookshelf-hardcover-sync:latest
    container_name: abs-hardcover-sync
    restart: unless-stopped
    stop_grace_period: 10s
    init: true
    
    ports:
      - "8765:8765"
    
    volumes:
      # Configuration
      - ./config:/app/config:ro
      # Persistent data
      - ./data:/data
      # Optional: separate cache
      - ./cache:/data/cache
    
    environment:
      # Server
      - ENABLE_WEB_UI=true
      - SERVER_PORT=8765
      - LOG_LEVEL=info
      - LOG_FORMAT=json
      
      # Paths
      - DATABASE_PATH=/data/db/sync.db
      - CACHE_DIR=/data/cache
      - SYNC_STATE_FILE=/data/sync_state.json
      
      # Security (use secrets in production!)
      - ENCRYPTION_KEY=${ENCRYPTION_KEY}
      - AUTH_ENABLED=${AUTH_ENABLED:-false}
      - AUTH_DEFAULT_ADMIN_PASSWORD=${AUTH_ADMIN_PASSWORD}
    
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:8765/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    
    # Optional: resource limits
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 512M
        reservations:
          memory: 128M
    
    networks:
      - abs-hc-network

networks:
  abs-hc-network:
    driver: bridge
```

### Docker with External Database

For PostgreSQL or MySQL/MariaDB:

```yaml
services:
  audiobookshelf-hardcover-sync:
    image: ghcr.io/drallgood/audiobookshelf-hardcover-sync:latest
    environment:
      - DATABASE_TYPE=postgresql
      - DATABASE_HOST=postgres
      - DATABASE_PORT=5432
      - DATABASE_NAME=abs_sync
      - DATABASE_USER=syncuser
      - DATABASE_PASSWORD=${DB_PASSWORD}
      - DATABASE_SSL_MODE=prefer
    depends_on:
      - postgres
  
  postgres:
    image: postgres:16-alpine
    environment:
      - POSTGRES_DB=abs_sync
      - POSTGRES_USER=syncuser
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

### Multi-Architecture Support

Images are built for multiple architectures:

```bash
# Explicit platform selection
docker pull --platform linux/amd64 ghcr.io/drallgood/audiobookshelf-hardcover-sync:latest
docker pull --platform linux/arm64 ghcr.io/drallgood/audiobookshelf-hardcover-sync:latest
```

---

## ☸️ Kubernetes Deployment

### Helm Chart Installation

The easiest way to deploy to Kubernetes:

```bash
# Add the Helm repository
helm repo add abs-hc-sync https://alexnolan.github.io/audiobookshelf-hardcover-sync
helm repo update

# Install with default values
helm install my-sync abs-hc-sync/audiobookshelf-hardcover-sync

# Install with custom values
helm install my-sync abs-hc-sync/audiobookshelf-hardcover-sync \
  --set webUI.enabled=true \
  --set persistence.enabled=true \
  --set persistence.size=10Gi \
  --set resources.limits.memory=512Mi
```

### Custom Values

Create a `values.yaml` file:

```yaml
# Enable web UI
webUI:
  enabled: true

# Resource limits
resources:
  requests:
    memory: "128Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"

# Persistence
persistence:
  enabled: true
  storageClass: "standard"
  size: 10Gi

# Ingress (optional)
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: abs-sync.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: abs-sync-tls
      hosts:
        - abs-sync.example.com

# Authentication
authentication:
  enabled: true
  defaultAdmin:
    username: admin
    email: admin@example.com
    password: "changeme"  # Use existingSecret in production
  
  # Keycloak/OIDC
  keycloak:
    enabled: true
    issuer: "https://keycloak.example.com/realms/myrealm"
    clientId: "abs-sync"
    clientSecret: ""  # Use existingSecret
    redirectUri: "https://abs-sync.example.com/auth/callback"

# Existing secrets (recommended for production)
existingSecret:
  name: "abs-sync-secrets"
  keys:
    encryptionKey: "encryption-key"
    authAdminPassword: "admin-password"
    keycloakClientSecret: "keycloak-secret"
```

Apply the configuration:

```bash
helm install my-sync abs-hc-sync/audiobookshelf-hardcover-sync -f values.yaml
```

### Manual Kubernetes Deployment

See [helm/audiobookshelf-hardcover-sync/](helm/audiobookshelf-hardcover-sync/) for complete manifests.

---

## 🔐 Security

### Token Encryption

All API tokens are encrypted at rest using **AES-256-GCM**:

- Encryption keys are auto-generated and stored in the data directory
- Override with `ENCRYPTION_KEY` environment variable (base64-encoded 32-byte key)
- Tokens are never logged or exposed in API responses (masked)

### Authentication & Authorization

Optional authentication system with three user roles:

- **Admin** - Full access, user management, system configuration
- **User** - Sync operations, personal settings, status monitoring
- **Viewer** - Read-only access to sync status

#### Enable Authentication

```yaml
# config.yaml
authentication:
  enabled: true
  default_admin:
    username: "admin"
    email: "admin@localhost"
    password: "your-secure-password"  # Change this!
```

Or via environment:

```bash
export AUTH_ENABLED=true
export AUTH_DEFAULT_ADMIN_PASSWORD=your-secure-password
```

#### Keycloak/OIDC Integration

For enterprise SSO:

```yaml
authentication:
  enabled: true
  keycloak:
    enabled: true
    issuer: "https://keycloak.example.com/realms/myrealm"
    client_id: "audiobookshelf-sync"
    client_secret: "your-client-secret"
    redirect_uri: "https://abs-sync.example.com/auth/callback"
    scopes: "openid profile email"
    role_claim: "realm_access.roles"
```

See [docs/AUTHENTICATION.md](docs/AUTHENTICATION.md) for complete setup guide.

### Best Practices

- ✅ Use environment variables or secrets for sensitive data
- ✅ Enable authentication for multi-user deployments
- ✅ Use HTTPS/TLS in production (configure reverse proxy or ingress)
- ✅ Rotate API tokens regularly
- ✅ Keep encryption keys backed up and secure
- ✅ Use strong admin passwords (12+ characters)
- ✅ Review audit logs in `sync_events` table
- ❌ Don't commit tokens or secrets to version control
- ❌ Don't expose the application directly to the internet without authentication

---

## 📊 Monitoring & Troubleshooting

### Health Checks

The application provides health check endpoints:

```bash
# Basic health check
curl http://localhost:8765/health

# Detailed health check with component status
curl http://localhost:8765/healthz
```

### Logs

View application logs:

```bash
# Docker Compose
docker compose logs -f

# Docker
docker logs -f abs-hardcover-sync

# Binary
./audiobookshelf-hardcover-sync 2>&1 | tee app.log
```

Set log level:

```yaml
logging:
  level: "debug"  # debug, info, warn, error
  format: "json"  # json or console
```

### Database Inspection

Access the SQLite database:

```bash
# Enter database shell
sqlite3 data/database.db

# Common queries
.tables                                    # List all tables
SELECT * FROM sync_profiles;               # View profiles
SELECT * FROM abs_books LIMIT 10;          # View AudiobookShelf books
SELECT * FROM book_mappings;               # View book mappings
SELECT * FROM progress_histories           # View sync history
  WHERE abs_book_id = 'your-book-id'
  ORDER BY created_at DESC
  LIMIT 10;
SELECT * FROM sync_events                  # View sync events (audit log)
  ORDER BY created_at DESC
  LIMIT 20;
```

### Common Issues

#### Problem: "Failed to decrypt token"

**Cause**: Encryption key mismatch (usually after volume recreation in Docker)

**Solution**:
1. Ensure the encryption key is persistent:
   ```yaml
   # docker-compose.yml
   environment:
     - ENCRYPTION_KEY=your-base64-encoded-key
   ```
2. Or delete the database and re-create profiles:
   ```bash
   rm data/database.db
   docker compose restart
   ```

#### Problem: "Book not syncing"

**Cause**: May be below progress threshold, disabled, or in conflict

**Solution**:
1. Check book configuration in web UI (click book → settings icon)
2. Verify sync is enabled for the book
3. Check conflict resolution strategy
4. Review sync events: `SELECT * FROM sync_events WHERE abs_book_id = 'book-id'`
5. Force sync: Click "Force Sync" button in web UI

#### Problem: "Rate limit exceeded"

**Cause**: Too many Hardcover API requests

**Solution**:
1. Increase rate limit interval in config:
   ```yaml
   rate_limit:
     rate: "2000ms"  # Slower rate
   ```
2. Reduce concurrent requests:
   ```yaml
   rate_limit:
     max_concurrent: 2
   ```

#### Problem: Web UI not loading

**Cause**: Web UI not enabled

**Solution**:
```bash
# Enable via environment variable
export ENABLE_WEB_UI=true

# Or in config.yaml
server:
  enable_web_ui: true
```

### Performance Tuning

For large libraries (1000+ books):

```yaml
# Increase cache size
cache:
  max_size: 10000
  ttl: 24h

# Optimize database
database:
  connection_pool:
    max_open_conns: 50
    max_idle_conns: 10
    conn_max_lifetime: 120

# Adjust sync settings
sync:
  incremental: true              # Only sync changed books
  min_change_threshold: 120      # Require 2+ minutes change
  batch_size: 50                 # Process in batches
```

---

## 🔧 Migration from Original Project

If you're migrating from the original [drallgood/audiobookshelf-hardcover-sync](https://github.com/drallgood/audiobookshelf-hardcover-sync):

### Automatic Migration

The application automatically migrates single-user configurations:

1. Detects existing `config.yaml`
2. Creates a backup: `config.yaml.backup.YYYYMMDD-HHMMSS`
3. Creates "Default Profile" with your settings in the database
4. Preserves all sync state and functionality

### Manual Migration Steps

1. **Stop the old version**:
   ```bash
   docker compose down
   # or kill the binary process
   ```

2. **Update Docker image** (if using Docker):
   ```yaml
   # docker-compose.yml
   services:
     audiobookshelf-hardcover-sync:
       image: ghcr.io/drallgood/audiobookshelf-hardcover-sync:latest  # Changed
   ```

3. **Enable web UI** (optional but recommended):
   ```yaml
   environment:
     - ENABLE_WEB_UI=true
   ```

4. **Start the new version**:
   ```bash
   docker compose up -d
   ```

5. **Verify migration**:
   - Access web UI at `http://localhost:8765`
   - Check "Profiles" tab for "Default Profile"
   - Verify tokens are working (click "Test Connection")
   - Check sync status

### Configuration Changes

Some configuration keys have changed:

| Old (v2.x) | New (v3.x) | Notes |
|------------|------------|-------|
| `app.sync_interval` | `sync.sync_interval` | Moved to sync section |
| `app.minimum_progress` | `sync.minimum_progress` | Moved to sync section |
| `app.sync_want_to_read` | `sync.sync_want_to_read` | Moved to sync section |
| `app.sync_owned` | `sync.sync_owned` | Moved to sync section |
| `app.dry_run` | `sync.dry_run` | Moved to sync section |

The application will automatically read both old and new formats and log warnings for deprecated settings.

---

## 🛠️ Development

### Prerequisites

- Go 1.24 or later
- Make
- Docker (optional, for containerized builds)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/alexnolan/audiobookshelf-hardcover-sync.git
cd audiobookshelf-hardcover-sync

# Install dependencies
go mod download

# Build all binaries
make build

# Build specific tools
make build-edition-tool
make build-image-tool
make build-hardcover-lookup

# Run tests
make test

# Run linting
make lint

# Build Docker image
make docker-build
```

### Project Structure

```
.
├── cmd/                                    # Main applications
│   ├── audiobookshelf-hardcover-sync/     # Main sync application
│   ├── edition-tool/                      # Edition creation tool
│   ├── image-tool/                        # Image upload tool
│   └── hardcover-lookup/                  # ID lookup tool
├── internal/                              # Private application code
│   ├── api/                               # REST API handlers
│   │   ├── audiobookshelf/                # AudiobookShelf client
│   │   ├── hardcover/                     # Hardcover GraphQL client
│   │   └── library.go                     # Book library API
│   ├── auth/                              # Authentication system
│   ├── config/                            # Configuration loading
│   ├── crypto/                            # Encryption utilities
│   ├── database/                          # Database layer (GORM)
│   ├── sync/                              # Sync engine
│   │   ├── service.go                     # Sync orchestration
│   │   ├── abs_collector.go              # AudiobookShelf collector
│   │   ├── hc_collector.go               # Hardcover collector
│   │   └── database_sync.go              # Database-driven sync
│   ├── multiuser/                         # Multi-profile management
│   └── logger/                            # Structured logging
├── web/static/                            # Web UI assets
│   ├── index.html                         # Main dashboard
│   ├── login.html                         # Login page
│   ├── app.js                             # Frontend JavaScript
│   └── styles.css                         # Styling
├── docs/                                  # Documentation
│   ├── AUTHENTICATION.md                  # Auth setup guide
│   ├── QUICK_REFERENCE.md                 # Quick reference
│   ├── hardcover-schema.graphql           # Hardcover GraphQL schema
│   └── openapi.yaml                       # REST API specification
├── helm/                                  # Kubernetes Helm chart
└── test/                                  # Tests
```

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package
go test -v ./internal/sync/...

# Run integration tests
go test -v -tags=integration ./test/integration/...
```

### Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Add tests for new functionality
5. Run tests and linting: `make test lint`
6. Commit with clear messages: `git commit -m "Add feature: description"`
7. Push to your fork: `git push origin feature/my-feature`
8. Open a Pull Request

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

---

## 📚 Documentation

- **[Quick Reference](docs/QUICK_REFERENCE.md)** - Fast reference for integration and deployment
- **[Authentication Guide](docs/AUTHENTICATION.md)** - Setup authentication and OIDC
- **[Database Schema](docs/DATABASE.md)** - Complete database documentation
- **[API Reference](docs/openapi.yaml)** - OpenAPI/Swagger specification
- **[Changelog](CHANGELOG.md)** - Version history and release notes
- **[Migration Guide](MIGRATION.md)** - Migrating from v2.x to v3.x
- **[Security Policy](SECURITY.md)** - Security policies and vulnerability reporting
- **[Release Process](RELEASE.md)** - How releases are created

---

## 🐛 Troubleshooting & Support

### Getting Help

1. **Check the documentation** - Most questions are answered in the docs
2. **Search existing issues** - Someone may have had the same problem
3. **Enable debug logging** - Set `LOG_LEVEL=debug` for detailed output
4. **Check the database** - Use `sqlite3` to inspect sync state
5. **Open an issue** - [GitHub Issues](https://github.com/alexnolan/audiobookshelf-hardcover-sync/issues)

### Reporting Bugs

When opening an issue, please include:

- Application version (`docker logs` or binary output)
- Configuration (redact tokens!)
- Steps to reproduce
- Expected vs actual behavior
- Relevant log output (set `LOG_LEVEL=debug`)
- Database state (if applicable)

### Feature Requests

Feature requests are welcome! Please:

- Check if the feature already exists or is planned
- Describe the use case and problem it solves
- Suggest implementation approach (optional)
- Label the issue as `enhancement`

---

## 📝 License

This project is licensed under the **Apache License 2.0** - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- **Original Project**: [drallgood/audiobookshelf-hardcover-sync](https://github.com/drallgood/audiobookshelf-hardcover-sync) - Thank you for the excellent foundation!
- **[AudiobookShelf](https://www.audiobookshelf.org/)** - Self-hosted audiobook and podcast server
- **[Hardcover](https://hardcover.app/)** - Modern book tracking and discovery platform
- All contributors who have helped improve this project

---

## 🔗 Links

- **GitHub Repository**: [alexnolan/audiobookshelf-hardcover-sync](https://github.com/alexnolan/audiobookshelf-hardcover-sync)
- **Docker Images**: [GitHub Container Registry](https://github.com/alexnolan/audiobookshelf-hardcover-sync/pkgs/container/audiobookshelf-hardcover-sync)
- **Issues & Discussions**: [GitHub Issues](https://github.com/alexnolan/audiobookshelf-hardcover-sync/issues)
- **Original Project**: [drallgood/audiobookshelf-hardcover-sync](https://github.com/drallgood/audiobookshelf-hardcover-sync)

---

<div align="center">

**Made with ❤️ for audiobook enthusiasts**

*If you find this project useful, please consider giving it a ⭐ on GitHub!*

</div>
