# Quick Reference Guide - Book Library Rearchitecture

## 📚 Documentation Files

| File | Purpose | Read When |
|------|---------|-----------|
| **PROJECT_COMPLETE.md** | Final status & sign-off | You want the executive summary |
| **IMPLEMENTATION_SUMMARY.md** | Complete overview | You want total project overview |
| **IMPLEMENTATION_COMPLETE.md** | Backend details | You're implementing the backend |
| **UI_INTEGRATION_COMPLETE.md** | Frontend details | You're integrating the UI |

## 🎯 Quick Start for Integration

### Step 1: Register API Routes (1-2 hours)
```go
// In main router setup, add:
libraryAPI := api.NewLibraryAPI(database, hcCollector, syncService)

router.GET("/api/profiles/:id/books", libraryAPI.GetBooksHandler)
router.GET("/api/profiles/:id/books/:absBookId", libraryAPI.GetBookHandler)
router.POST("/api/profiles/:id/books/:absBookId/sync", libraryAPI.SyncBookHandler)
// ... etc (see library.go for all endpoints)
```

### Step 2: Database Migrations (30 mins)
```sql
-- Run these migrations to create 6 new tables:
-- abs_books, hardcover_user_books, book_mappings,
-- progress_histories, sync_events, book_sync_configs
```

### Step 3: Verify Frontend (30 mins)
- [ ] Library tab appears in navigation
- [ ] Profile selector loads profiles
- [ ] Books load when profile selected
- [ ] Filtering/sorting works
- [ ] Modals open/close properly

### Step 4: End-to-End Test (1-2 hours)
- [ ] Create test profile
- [ ] Collect books
- [ ] View in Library
- [ ] Sync a book
- [ ] Check history

## 🔑 Key Concepts

### Store-Compare-Sync
```
STORE: Collectors fetch books and store in database
COMPARE: Query database to compare ABS vs HC progress
SYNC: Apply conflict resolution and update HC
```

### Conflict Resolution Modes
| Mode | Behavior |
|------|----------|
| `prefer_abs` | Always use ABS progress (default) |
| `prefer_hardcover` | Always use HC progress |
| `prefer_newest` | Use whichever is more recent |
| `manual` | Don't sync, flag for review |

### Progress Threshold
- Default: 1% difference
- Less than threshold = "in sync"
- Greater than threshold = "needs sync"
- Configurable per profile

## 📊 Database Schema (6 Tables)

```
abs_books
├─ id (PK)
├─ profile_id (FK)
├─ abs_id (unique + indexed)
├─ title, author
├─ progress (float 0-1)
└─ created_at, updated_at

hardcover_user_books
├─ id (PK)
├─ profile_id (FK)
├─ hc_id
├─ title, author
├─ progress (float 0-1)
└─ created_at, updated_at

book_mappings
├─ id (PK)
├─ profile_id (FK)
├─ abs_id (FK) → abs_books
├─ hc_id (FK) → hardcover_user_books
├─ match_method (ASIN/ISBN/manual)
├─ confidence (0-100)
└─ created_at

progress_histories
├─ id (PK)
├─ profile_id, abs_id
├─ source (ABS/HC)
├─ progress (float)
└─ created_at (auto-cleanup: 30d/100 entries)

sync_events
├─ id (PK)
├─ profile_id, abs_id
├─ event_type (progress_update, error, etc)
├─ old_progress, new_progress
├─ trigger (scheduled, manual, API, web)
├─ details, error
└─ created_at

book_sync_configs
├─ id (PK)
├─ profile_id, abs_id
├─ sync_enabled (boolean)
├─ conflict_mode (optional override)
└─ created_at, updated_at
```

## 🔗 API Endpoints Reference

### Books
```
GET    /api/profiles/:id/books
GET    /api/profiles/:id/books/:absBookId
POST   /api/profiles/:id/books/:absBookId/sync
POST   /api/profiles/:id/books/sync-batch
```

### Collections
```
POST   /api/profiles/:id/collect/abs
POST   /api/profiles/:id/collect/hardcover
```

### History
```
GET    /api/profiles/:id/books/:absBookId/history
GET    /api/profiles/:id/sync-summary
```

### Mapping
```
POST   /api/profiles/:id/books/:absBookId/map
DELETE /api/profiles/:id/books/:absBookId/map
```

## 🖥️ UI Components

### Main Navigation
```
[Profiles] [Library] [Sync Status] [Book Sync Logs] [Add Profile]
```

### Library Tab Sections
1. **Profile Selector** - Choose which profile to view
2. **Summary Stats** - Total/Mapped/In Sync/Needs Sync/Unmapped
3. **Search & Controls** - Search, filter, sort, action buttons
4. **Books Table** - List of books with progress comparison
5. **Pagination** - Page navigation

### Modals
1. **Book Detail** - Full info, history, sync events
2. **Book Config** - Enable/disable, override settings
3. **Manual Mapping** - Enter HC edition ID

## 🚀 Deployment Checklist

### Pre-Integration
- [ ] Read all 4 documentation files
- [ ] Review Go code in `internal/api/library.go`
- [ ] Review JavaScript in `web/static/app.js`
- [ ] Review HTML in `web/static/index.html`

### Integration
- [ ] Register API routes
- [ ] Create database migrations
- [ ] Wire up collectors and sync service
- [ ] Add UI route handlers
- [ ] Test locally

### Validation
- [ ] All routes respond with correct data
- [ ] Database tables created successfully
- [ ] UI elements visible and functional
- [ ] No console errors in browser
- [ ] API responses match documented format

### Testing
- [ ] Create test profile
- [ ] Collect ABS books
- [ ] Collect HC books
- [ ] View Library tab
- [ ] Test sync operation
- [ ] Verify history recorded
- [ ] Test conflict resolution

### Production
- [ ] Code review completed
- [ ] Performance tested
- [ ] Security review completed
- [ ] Deploy to staging
- [ ] Smoke testing
- [ ] Deploy to production
- [ ] Monitor error logs

## 📈 Performance Metrics to Monitor

### Collection
- ABS collection time (depends on library size)
- HC collection time (rate limited to 50 req/min)
- Database insert speed (bulk upsert)

### Sync
- Single book sync latency (< 1 second target)
- Batch sync throughput (books/second)
- HC API calls (should be <= 50/min)

### UI
- Library tab load time (< 1 second)
- Pagination performance (50 books/page)
- Modal open/close speed (instant)

## 🐛 Troubleshooting

### Problem: Library tab shows "No books found"
**Solution**: 
1. Verify profile is selected
2. Run collection (ABS or HC)
3. Check database has records
4. Check browser console for errors

### Problem: Sync fails with error
**Solution**:
1. Check profile tokens are valid
2. Verify network connectivity
3. Review sync_events table for details
4. Check HC API rate limit not exceeded

### Problem: Performance is slow
**Solution**:
1. Check database indexes are created
2. Verify pagination is working (50 books/page)
3. Check HC rate limiter (50 req/min)
4. Review database query times

### Problem: Mapping not working
**Solution**:
1. Verify HC edition ID is numeric
2. Check profile selector not empty
3. Review confidence level (0-100)
4. Verify HC book exists in database

## 💾 Database Backup

Before deploying, backup existing database:
```bash
sqlite3 sync_state.db ".backup sync_state_backup.db"
```

Recovery:
```bash
sqlite3 sync_state.db ".restore sync_state_backup.db"
```

## 📞 Support Contacts

### For Backend Issues
See: `IMPLEMENTATION_COMPLETE.md` → Backend Architecture section

### For UI Issues  
See: `UI_INTEGRATION_COMPLETE.md` → UI Workflow section

### For Integration Help
See: `IMPLEMENTATION_SUMMARY.md` → Integration Checklist section

## 🎓 Learning Resources

### Code Files to Review
1. `internal/api/library.go` - API implementation
2. `internal/sync/database_sync.go` - Sync logic
3. `web/static/app.js` - Frontend logic

### Concepts to Understand
1. Store-Compare-Sync pattern
2. Conflict resolution strategies
3. Rate limiting (token bucket)
4. RESTful API design
5. Pagination patterns

## ✅ Done!

All implementation is complete. You're ready to integrate and deploy.

- **Backend**: ✅ Ready to integrate
- **API**: ✅ Ready to route
- **Frontend**: ✅ Ready to use
- **Documentation**: ✅ Complete

Good luck with deployment! 🚀
