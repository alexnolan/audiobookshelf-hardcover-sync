// Sync Profile Management App
console.info('Sync UI loaded', { build: '2025-08-16 02:30:00+02:00' });
// Global image error handler for cover fallbacks
window.__absHandleImageError = function(img) {
    try {
        const raw = (img.dataset && img.dataset.fallbacks) ? img.dataset.fallbacks : '';
        const list = raw.split('|').filter(Boolean);
        let idx = parseInt(img.dataset.fbIdx || '0', 10);
        if (Number.isNaN(idx)) idx = 0;
        if (idx < list.length - 1) {
            idx += 1;
            img.dataset.fbIdx = String(idx);
            img.src = list[idx];
        } else {
            // Stop further error loops
            img.onerror = null;
            // Final fallback (in case list didn't include it)
            img.src = '/cover-placeholder.svg';
        }
    } catch (e) {
        console.warn('Image fallback handler error:', e);
        img.onerror = null;
        img.src = '/cover-placeholder.svg';
    }
};

class SyncProfileApp {
    constructor() {
        this.users = [];
        this.statuses = {};
        this.currentEditUser = null;
        this.refreshInterval = null;
        this.currentUser = null;
        this.authEnabled = false;
        this.hasRedirectedToLogin = false;
        
        this.init();
    }

    // Coerce various representations to boolean with a sensible default
    toBool(value, defaultValue = true) {
        if (value === true || value === false) return value;
        if (typeof value === 'string') {
            const v = value.trim().toLowerCase();
            if (v === 'true') return true;
            if (v === 'false') return false;
            if (v === '1') return true;
            if (v === '0') return false;
        }
        if (value === 1) return true;
        if (value === 0) return false;
        return !!defaultValue;
    }

    // Format a timestamp to relative time (e.g., "5 minutes ago") with fallback
    formatRelativeTime(ts) {
        try {
            if (!ts) return '';
            const date = (ts instanceof Date) ? ts : new Date(ts);
            const now = new Date();
            const diffMs = date.getTime() - now.getTime();
            const seconds = Math.round(diffMs / 1000);
            const absSec = Math.abs(seconds);
            const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
            const divisions = [
                { amount: 60, name: 'seconds' },
                { amount: 60, name: 'minutes' },
                { amount: 24, name: 'hours' },
                { amount: 7, name: 'days' },
                { amount: 4.34524, name: 'weeks' },
                { amount: 12, name: 'months' },
                { amount: Number.POSITIVE_INFINITY, name: 'years' }
            ];
            let duration = seconds;
            for (const division of divisions) {
                if (Math.abs(duration) < division.amount) {
                    return rtf.format(Math.round(duration), division.name);
                }
                duration /= division.amount;
            }
            return date.toLocaleString();
        } catch (_) {
            return new Date(ts).toLocaleString();
        }
    }

    async init() {
        try {
            // Set up event listeners first so UI is responsive
            this.setupEventListeners();
            
            // Restore persisted tab selection
            const savedTab = loadPersisted(STORAGE_KEYS.activeTab, 'users');
            if (savedTab && savedTab !== 'users') {
                // Use setTimeout to allow DOM to be ready
                setTimeout(() => this.showTab(savedTab), 0);
            }
            
            // Check authentication status
            const isAuthenticated = await this.checkAuthStatus();
            
            // If auth is enabled but user is not authenticated, we'll be redirected to login
            if (this.authEnabled && !isAuthenticated) {
                // Don't load any data, just show the login UI
                this.updateUserInfo();
                return;
            }
            
            // If we get here, either auth is disabled or user is authenticated
            try {
                // Load data in parallel for better performance
                await Promise.all([
                    this.loadProfiles(),
                    this.loadStatuses()
                ]);
                
                // Start auto-refresh only if we have data to refresh
                if (this.users.length > 0) {
                    this.startAutoRefresh();
                }
            } catch (error) {
                console.error('Error loading data:', error);
                this.showToast('Failed to load data', 'error');
            }
            
            // Ensure UI is up to date
            this.updateUserInfo();
            
        } catch (error) {
            console.error('Error initializing app:', error);
            this.showToast('Failed to initialize application', 'error');
            
            // Make sure we show appropriate UI even if there's an error
            this.updateUserInfo();
        }
    }

    async checkAuthStatus() {
        try {
            // Prevent login loops by checking if we're already on login page
            if (window.location.pathname.endsWith('/login')) {
                // We're on login page, don't do auth checks that might redirect
                this.authEnabled = true;
                this.currentUser = null;
                this.updateUserInfo();
                return false;
            }
            
            // Load current user to determine auth status
            const userLoaded = await this.loadCurrentUser();
            
            if (userLoaded) {
                // User is authenticated
                this.updateUserInfo();
                return true;
            }
            
            // No user loaded - check if we need to redirect to login
            if (this.authEnabled && !this.hasRedirectedToLogin) {
                console.log('Auth enabled but no user, redirecting to login');
                this.hasRedirectedToLogin = true;
                this.redirectToLogin();
                return false;
            }
            
            // Update UI and return status
            this.updateUserInfo();
            return false;
        } catch (error) {
            console.error('Error checking auth status:', error);
            this.authEnabled = false;
            this.currentUser = null;
            this.updateUserInfo();
            return false;
        }
    }

    async loadCurrentUser() {
        try {
            const response = await fetch('/api/auth/me', {
                method: 'GET',
                credentials: 'include',
                headers: {
                    'Accept': 'application/json',
                    'Cache-Control': 'no-cache',
                    'Pragma': 'no-cache'
                }
            });
            
            if (response.ok) {
                const data = await response.json();
                
                // Handle new authentication response format
                this.authEnabled = data.auth_enabled !== false; // Default to true if not specified
                
                if (data.authenticated && data.user) {
                    this.currentUser = data.user;
                    console.log('User authenticated:', this.currentUser);
                    return true;
                } else {
                    // Not authenticated but auth is enabled
                    this.currentUser = null;
                    console.log('User not authenticated, auth enabled:', this.authEnabled);
                    return false;
                }
            } else {
                // If we get an error, assume auth is enabled but user not authenticated
                this.currentUser = null;
                this.authEnabled = true;
                console.log('Auth error, assuming auth enabled');
                return false;
            }
        } catch (error) {
            console.error('Error loading current user:', error);
            // On error, assume auth is disabled
            this.currentUser = null;
            this.authEnabled = false;
            return false;
        }
    }

    redirectToLogin() {
        // Only redirect if we're not already on the login page
        if (!window.location.pathname.endsWith('/login')) {
            const currentPath = window.location.pathname + window.location.search;
            window.location.href = `/login?redirect=${encodeURIComponent(currentPath)}`;
        }
    }

    updateUserInfo() {
        try {
            const userInfoElement = document.getElementById('user-info');
            if (!userInfoElement) {
                console.warn('User info element not found');
                return;
            }

            // Debug logging - show full user object for troubleshooting
            console.log('Current user object:', this.currentUser);
            console.log('Updating user info:', { 
                authEnabled: this.authEnabled, 
                currentUser: this.currentUser || 'No user',
                path: window.location.pathname
            });

            if (this.authEnabled && this.currentUser) {
                // User is authenticated - show user info and logout button
                // Keycloak might provide different user properties, so we'll check multiple possibilities
                const username = this.currentUser.preferred_username || 
                               this.currentUser.name || 
                               this.currentUser.email || 
                               this.currentUser.username || 
                               'User';
                const userInitial = username.charAt(0).toUpperCase();
                
                userInfoElement.innerHTML = `
                    <div class="user-info">
                        <div class="user-avatar">${userInitial}</div>
                        <span>${this.escapeHtml(username)}</span>
                    </div>
                    <button class="logout-btn" onclick="app.logout()">
                        <span class="btn-icon">🚪</span> Logout
                    </button>
                `;
                
                // Make sure the user is on the right page
                if (window.location.pathname.endsWith('/login')) {
                    window.location.href = '/';
                }
            } else if (this.authEnabled) {
                // Auth is enabled but no user - show login button
                userInfoElement.innerHTML = `
                    <button class="login-btn" onclick="app.redirectToLogin()">
                        <span class="btn-icon">🔑</span> Login
                    </button>
                `;
                
                // If we're not on the login page and auth is required, redirect
                if (!window.location.pathname.endsWith('/login')) {
                    this.redirectToLogin();
                }
            } else {
                // Auth is not enabled - clear the user info area
                userInfoElement.innerHTML = '';
            }
            
            // Trigger a reflow to ensure UI updates
            userInfoElement.offsetHeight;
            
        } catch (error) {
            console.error('Error updating user info:', error);
        }
    }

    async logout() {
        try {
            // Clear local state first to update UI immediately
            this.currentUser = null;
            this.authEnabled = true;
            this.updateUserInfo();
            
            // Show loading state
            this.showLoading();
            
            // Call the logout API
            const response = await fetch('/api/auth/logout', {
                method: 'POST',
                credentials: 'include',
                headers: {
                    'Cache-Control': 'no-cache',
                    'Pragma': 'no-cache'
                }
            });
            
            // Hide loading state
            this.hideLoading();
            
            // Handle response
            if (response.ok) {
                // Clear any remaining data
                this.users = [];
                this.statuses = {};
                
                // Stop any auto-refresh
                this.stopAutoRefresh();
                
                // Redirect to login page
                window.location.href = '/login';
            } else {
                const errorData = await response.json().catch(() => ({}));
                console.error('Logout failed:', response.status, errorData);
                this.showToast('Logout failed. Please try again.', 'error');
                
                // Still redirect to login page even if API call fails
                window.location.href = '/login';
            }
        } catch (error) {
            console.error('Logout error:', error);
            this.hideLoading();
            this.showToast('Logout failed. Please try again.', 'error');
            
            // Still redirect to login page on error
            window.location.href = '/login';
        }
    }

    setupEventListeners() {
        // Prevent duplicate event listeners
        if (this.eventListenersSetup) return;
        this.eventListenersSetup = true;
        
        // Tab switching
        document.querySelectorAll('.tab-button').forEach(button => {
            button.addEventListener('click', (e) => {
                const tabName = e.target.getAttribute('onclick').match(/'([^']+)'/)[1];
                this.showTab(tabName);
            });
        });

        // Add profile form (inline version)
        const addUserForm = document.getElementById('inline-add-user-form');
        if (addUserForm) {
            addUserForm.addEventListener('submit', (e) => {
                e.preventDefault();
                handleAddUser(e); // Use the global handler
            });
        }

        // Edit profile form
        const editUserForm = document.getElementById('edit-user-form');
        if (editUserForm) {
            editUserForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.handleEditProfile(e);
            });
        }

        // Modal close on background click
        const editModal = document.getElementById('edit-user-modal');
        if (editModal) {
            editModal.addEventListener('click', (e) => {
                if (e.target.id === 'edit-user-modal') {
                    this.closeEditModal();
                }
            });
        }
    }

    showTab(tabName) {
        // Save tab selection to localStorage
        savePersisted(STORAGE_KEYS.activeTab, tabName);
        
        // Update tab buttons
        document.querySelectorAll('.tab-button').forEach(btn => btn.classList.remove('active'));
        document.querySelector(`[onclick="showTab('${tabName}')"]`).classList.add('active');

        // Update tab content
        document.querySelectorAll('.tab-content').forEach(content => content.classList.remove('active'));
        document.getElementById(`${tabName}-tab`).classList.add('active');

        // Refresh data when switching to relevant tabs
        if (tabName === 'users') {
            this.loadProfiles();
        } else if (tabName === 'matching') {
            // Matching tab - data loaded via loadLibraryBooks()
            updateLibraryProfileSelect();
        } else if (tabName === 'stats') {
            // Stats tab - populate profile dropdown and load logs
            populateHistoryProfileDropdown();
        }
    }

    /**
     * Renders the list of sync profiles in the UI with improved visual design
     */
    renderProfiles() {
        const usersList = document.getElementById('users-list');
        if (!usersList) return;

        if (!this.users || this.users.length === 0) {
            usersList.innerHTML = `
                <div class="empty-state" style="grid-column: 1 / -1; text-align: center; padding: 2rem;">
                    <h3>No sync profiles found</h3>
                    <p>Click on "Add Profile" to create a new sync profile.</p>
                </div>
            `;
            return;
        }

        usersList.innerHTML = this.users.map(user => {
            const lastSyncISO = user.last_sync || null;
            const lastSync = lastSyncISO ? this.formatRelativeTime(lastSyncISO) : 'Never';
            const statusClass = user.active ? 'active' : 'inactive';
            const statusIcon = user.active ? '✓' : '✗';
            
            // Library stats placeholder - will be updated async
            const statsId = `profile-stats-${this.escapeHtml(user.id)}`;
            
            return `
                <div class="user-card">
                    <div class="user-card-header">
                        <h3>${this.escapeHtml(user.name || user.id)}</h3>
                        <span class="status-badge ${statusClass}" title="${user.active ? 'Active' : 'Inactive'}">
                            ${statusIcon} ${user.active ? 'Active' : 'Inactive'}
                        </span>
                    </div>
                    
                    <div class="user-card-body">
                        <div class="user-info">
                            <div class="user-info-item">
                                <strong>Profile ID:</strong>
                                <span class="user-id">${this.escapeHtml(user.id)}</span>
                            </div>
                            <div class="user-info-item">
                                <strong>Last Synced:</strong>
                                <span class="last-sync" title="${lastSyncISO ? new Date(lastSyncISO).toLocaleString() : 'Never'}">${lastSync}</span>
                            </div>
                            <div class="user-info-item" id="${statsId}">
                                <strong>Library:</strong>
                                <span class="text-small text-muted">Loading stats...</span>
                            </div>
                        </div>
                        
                        <div class="user-card-actions">
                            <button class="btn btn-sm btn-icon" onclick="app.editProfile('${this.escapeHtml(user.id)}')" title="Edit Profile">
                                <span class="icon">✏️</span> Edit
                            </button>
                            <button class="btn btn-sm btn-icon btn-warning" onclick="app.purgeProfileData('${this.escapeHtml(user.id)}')" title="Purge all data for this profile and start fresh">
                                <span class="icon">🔄</span> Purge Data
                            </button>
                            <button class="btn btn-sm btn-icon btn-danger" onclick="app.deleteProfile('${this.escapeHtml(user.id)}')" title="Delete Profile">
                                <span class="icon">🗑️</span> Delete
                            </button>
                        </div>
                    </div>
                </div>
            `;
        }).join('');
        
        // Load library stats for each profile
        this.users.forEach(user => this.loadProfileStats(user.id));
    }
    
    async loadProfileStats(profileId) {
        try {
            const response = await fetch(`/api/profiles/${profileId}/sync-summary`);
            const data = await response.json();
            
            const statsEl = document.getElementById(`profile-stats-${this.escapeHtml(profileId)}`);
            if (!statsEl) return;
            
            if (data.success && data.data) {
                const s = data.data;
                const inSyncPct = s.mapped_books > 0 ? Math.round((s.in_sync_books / s.mapped_books) * 100) : 0;
                statsEl.innerHTML = `
                    <strong>Library:</strong>
                    <span class="text-small text-secondary">
                        📚 ${s.total_abs_books} books 
                        | ✓ ${s.mapped_books} mapped 
                        | ⇄ ${inSyncPct}% synced
                        ${s.unmapped_books > 0 ? `| <span class="text-danger">✗ ${s.unmapped_books} unmapped</span>` : ''}
                    </span>
                `;
            } else {
                statsEl.innerHTML = `
                    <strong>Library:</strong>
                    <span class="text-small text-muted">No data yet</span>
                `;
            }
        } catch (error) {
            console.error('Failed to load profile stats:', error);
        }
    }

    async loadProfiles() {
        try {
            this.showLoading();
            
            // Check authentication status first
            if (this.authEnabled && !this.currentUser) {
                this.showToast('Please log in to view profiles', 'error');
                this.redirectToLogin();
                return;
            }
            
            const response = await fetch('/api/profiles', {
                method: 'GET',
                credentials: 'include', // Include session cookies
                headers: {
                    'Content-Type': 'application/json'
                }
            });
            
            // Handle authentication errors specifically
            if (response.status === 401 || response.status === 403) {
                this.showToast('Authentication required. Please log in.', 'error');
                this.redirectToLogin();
                return;
            }
            
            const data = await response.json();

            if (response.ok && data.success) {
                this.users = data.data;
                this.renderProfiles();
                updateProfileSelectForBookLogs(); // Update the history profile dropdown
                updateLibraryProfileSelect(); // Update the library profile dropdown
            } else {
                // Handle different types of errors
                if (data.error && data.error.code === 'authentication_required') {
                    this.showToast('Authentication required. Please log in.', 'error');
                    this.redirectToLogin();
                } else {
                    this.showToast('Failed to load sync profiles: ' + (data.error?.message || data.error || 'Unknown error'), 'error');
                }
            }
        } catch (error) {
            this.showToast('Error loading sync profiles: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }

    async loadStatuses() {
        try {
            this.showLoading();
            const statuses = {};
            const summaryPromises = [];
            
            // First, get the list of profiles if not already loaded
            if (!this.users || this.users.length === 0) {
                await this.loadProfiles();
            }
            
            // If no users, render empty status
            if (!this.users || this.users.length === 0) {
                this.statuses = {};
                this.renderStatuses();
                return;
            }
            
            // Fetch status for each profile
            for (const user of this.users) {
                try {
                    const statusResponse = await fetch(`/api/profiles/${user.id}/status`);
                    if (statusResponse.ok) {
                        const statusData = await statusResponse.json();
                        if (statusData.success) {
                            const hasSummary = statusData.data?.last_sync_summary || 
                                            (statusData.data?.books_synced !== undefined && 
                                             (statusData.data?.mismatches?.length > 0 || 
                                              statusData.data?.books_not_found?.length > 0));
                            
                            statuses[user.id] = {
                                ...statusData.data,
                                profile_id: user.id,
                                profile_name: user.name || `Profile ${user.id}`,
                                books_not_found: statusData.data?.books_not_found || [],
                                mismatches: statusData.data?.mismatches || [],
                                has_summary: hasSummary,
                                // Ensure we have the total books processed
                                books_total: statusData.data?.books_total || 0,
                                books_synced: statusData.data?.books_synced || 0,
                                last_sync: statusData.data?.last_sync
                            };
                            
                            // Always try to fetch the summary for completed/error states
                            if (statusData.data?.state === 'completed' || statusData.data?.state === 'error') {
                                summaryPromises.push(this.fetchSyncSummary(user.id, statuses));
                            }
                        }
                    }
                } catch (error) {
                    console.error(`Error fetching status for profile ${user.id}:`, error);
                }
            }
            
            // Wait for all summary fetches to complete
            await Promise.all(summaryPromises);
            
            this.statuses = statuses;
            this.renderStatuses();
            this.renderSyncSummary();
            
        } catch (error) {
            console.error('Error in loadStatuses:', error);
            this.showToast('Error loading statuses: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }
    
    async fetchSyncSummary(profileId, statuses) {
        try {
            console.log(`Fetching sync summary for profile ${profileId}...`);
            const response = await fetch(`/api/profiles/${profileId}/summary`);
            if (response.ok) {
                const result = await response.json();
                console.log('Raw sync summary response:', result);
                
                // The API returns the data directly in the response, not in a 'data' property
                const summaryData = result.success ? result.data || result : result;
                
                if (statuses[profileId]) {
                    const booksSynced = summaryData.books_synced || statuses[profileId].books_synced || 0;
                    const booksNotFound = summaryData.books_not_found || statuses[profileId].books_not_found || [];
                    const mismatches = summaryData.mismatches || statuses[profileId].mismatches || [];
                    const totalBooks = summaryData.total_books_processed !== undefined 
                        ? summaryData.total_books_processed 
                        : statuses[profileId].books_total || 0;
                    
                    // Update the status with the summary data
                    statuses[profileId] = {
                        ...statuses[profileId],
                        books_synced: booksSynced,
                        books_not_found: booksNotFound,
                        mismatches: mismatches,
                        has_summary: true,
                        books_total: totalBooks,
                        last_sync: statuses[profileId].last_sync || new Date().toISOString()
                    };
                    
                    console.log(`Updated sync summary for profile ${profileId}:`, statuses[profileId]);
                    
                    // Force a re-render of the statuses to show the updated summary
                    this.statuses = { ...statuses };
                }
            } else {
                console.error(`Failed to fetch summary for profile ${profileId}:`, response.status, response.statusText);
            }
        } catch (error) {
            console.error(`Error fetching summary for profile ${profileId}:`, error);
            // Don't show toast here to avoid multiple toasts for multiple failures
        }
    }
    
    renderStatuses() {
        const container = document.getElementById('sync-status');
        if (!container) return;

        if (Object.keys(this.statuses).length === 0) {
            container.innerHTML = `
                <div class="text-center" style="grid-column: 1 / -1; padding: 40px;">
                    <h3>No sync statuses available</h3>
                    <p>Add a new sync profile and start syncing to see status information.</p>
                </div>
            `;
            return;
        }

        // Debug: Log status data to console for troubleshooting
        console.log('Rendering statuses with data:', this.statuses);

        // Convert statuses object to array and filter out any null/undefined entries
        const statusArray = Object.entries(this.statuses).filter(([_, status]) => status);
        
        container.innerHTML = statusArray.map(([profileId, status]) => {
            const progress = status.progress || 0;
            const booksSynced = status.books_synced || 0;
            const booksTotal = status.books_total || 0;
            const booksSkipped = status.books_skipped || 0;
            const booksNotFound = status.books_not_found?.length || 0;
            const mismatches = status.mismatches?.length || 0;
            const syncSettings = status.sync_settings || {};
            
            // Determine if we should show the View Details button
            const hasSummary = status.has_summary || 
                             (status.status === 'completed' && 
                              (booksSynced > 0 || booksNotFound > 0 || mismatches > 0 || booksSkipped > 0));
            
            const progressPercent = booksTotal > 0 ? Math.round((booksSynced / booksTotal) * 100) : 0;
            const lastSync = status.last_sync || status.lastSync || null;
            const statusText = status.status || 'idle';
            const profileName = status.profile_name || status.profile_id || 'Unknown Profile';
            
            // Build sync settings display
            const settingsHtml = this.buildSyncSettingsHtml(syncSettings);

            return `
                <div class="status-card ${statusText.toLowerCase()}">
                    <div class="status-header">
                        <h3>${this.escapeHtml(profileName)}</h3>
                        <span class="status-badge">${statusText}</span>
                    </div>
                    <div class="status-info">
                        ${lastSync ? `
                            <div><strong>Last Sync:</strong> <span title="${new Date(lastSync).toLocaleString()}">${this.formatRelativeTime(lastSync)}</span></div>
                        ` : ''}
                        ${progress > 0 ? `
                            <div><strong>Progress:</strong> ${progress}%</div>
                        ` : ''}
                        ${booksTotal > 0 ? `
                            <div class="sync-counters">
                                <div class="counter-row">
                                    <span class="counter-label">Total Processed:</span>
                                    <span class="counter-value">${booksTotal}</span>
                                </div>
                                <div class="counter-row">
                                    <span class="counter-label">Actually Synced:</span>
                                    <span class="counter-value success">${booksSynced}</span>
                                </div>
                                ${booksSkipped > 0 ? `
                                <div class="counter-row">
                                    <span class="counter-label">Skipped:</span>
                                    <span class="counter-value muted">${booksSkipped}</span>
                                </div>
                                ` : ''}
                                ${booksNotFound > 0 ? `
                                <div class="counter-row">
                                    <span class="counter-label">Not Found:</span>
                                    <span class="counter-value warning">${booksNotFound}</span>
                                </div>
                                ` : ''}
                                ${mismatches > 0 ? `
                                <div class="counter-row">
                                    <span class="counter-label">Mismatches:</span>
                                    <span class="counter-value warning">${mismatches}</span>
                                </div>
                                ` : ''}
                            </div>
                        ` : ''}
                        ${settingsHtml}
                        ${status.message ? `
                            <div class="status-message">${this.escapeHtml(status.message)}</div>
                        ` : ''}
                        ${status.error ? `
                            <div class="status-error">Error: ${this.escapeHtml(status.error)}</div>
                        ` : ''}
                    </div>
                    <div class="status-actions">
                        ${statusText.toLowerCase() === 'syncing' ? `
                            <button class="btn btn-warning" onclick="app.cancelSync('${profileId}')">
                                Cancel Sync
                            </button>
                        ` : `
                            <button class="btn btn-primary" onclick="app.startSync('${profileId}')">
                                ${statusText.toLowerCase() === 'error' ? 'Retry Sync' : 'Start Sync'}
                            </button>
                        `}
                        <button class="btn btn-secondary" onclick="viewProfileLibrary('${profileId}')">
                            📚 View Library
                        </button>
                        ${hasSummary ? `
                            <button class="btn btn-secondary" onclick="app.showSyncSummary('${profileId}')">
                                View Details
                            </button>
                        ` : ''}
                    </div>
                </div>
            `;
        }).join('');
    }
    
    buildSyncSettingsHtml(settings) {
        if (!settings || Object.keys(settings).length === 0) {
            return '';
        }
        
        const settingItems = [];
        
        // Key boolean settings with icons
        if (settings.dry_run) {
            settingItems.push('<span class="setting-tag warning" title="Dry Run Mode - No changes will be made">🔒 Dry Run</span>');
        }
        if (settings.process_unread_books) {
            settingItems.push('<span class="setting-tag" title="0% progress books are processed">📖 0% Books</span>');
        } else {
            settingItems.push('<span class="setting-tag muted" title="0% progress books are skipped">📖 Skip 0%</span>');
        }
        if (settings.sync_want_to_read) {
            settingItems.push('<span class="setting-tag" title="0% books synced as Want to Read">📚 Want to Read</span>');
        }
        if (settings.sync_owned) {
            settingItems.push('<span class="setting-tag" title="Owned status will be synced">✅ Owned</span>');
        }
        if (settings.include_ebooks) {
            settingItems.push('<span class="setting-tag" title="Ebooks are included">📱 Ebooks</span>');
        }
        if (settings.incremental) {
            settingItems.push('<span class="setting-tag" title="Incremental sync enabled">⚡ Incremental</span>');
        }
        if (settings.minimum_progress > 0) {
            const pct = Math.round(settings.minimum_progress * 100);
            settingItems.push(`<span class="setting-tag muted" title="Minimum progress threshold">${pct}% min</span>`);
        }
        
        if (settingItems.length === 0) {
            return '';
        }
        
        return `
            <div class="sync-settings-display">
                <div class="settings-label">Settings:</div>
                <div class="settings-tags">${settingItems.join('')}</div>
            </div>
        `;
    }
    
    buildDetailedSettingsHtml(settings) {
        if (!settings || Object.keys(settings).length === 0) {
            return '';
        }
        
        const rows = [];
        
        // Build detailed settings table
        rows.push(`<tr><td>Process 0% Progress Books</td><td>${settings.process_unread_books ? '✅ Yes' : '❌ No'}</td><td class="setting-desc">Process books with 0% listening progress for matching and sync</td></tr>`);
        rows.push(`<tr><td>Sync as "Want to Read"</td><td>${settings.sync_want_to_read ? '✅ Yes' : '❌ No'}</td><td class="setting-desc">Sync 0% progress books to Hardcover with "Want to Read" status</td></tr>`);
        rows.push(`<tr><td>Sync Owned Status</td><td>${settings.sync_owned ? '✅ Yes' : '❌ No'}</td><td class="setting-desc">Add synced audiobooks to "Owned" list in Hardcover</td></tr>`);
        rows.push(`<tr><td>Include Ebooks</td><td>${settings.include_ebooks ? '✅ Yes' : '❌ No'}</td><td class="setting-desc">Fetch ebook media type from ABS</td></tr>`);
        rows.push(`<tr><td>Incremental Sync</td><td>${settings.incremental ? '✅ Yes' : '❌ No'}</td><td class="setting-desc">Only sync books with changes since last sync</td></tr>`);
        rows.push(`<tr><td>Minimum Progress</td><td>${Math.round((settings.minimum_progress || 0) * 100)}%</td><td class="setting-desc">Minimum progress before syncing a book</td></tr>`);
        if (settings.dry_run) {
            rows.push(`<tr class="warning-row"><td>Dry Run Mode</td><td>⚠️ Enabled</td><td class="setting-desc">No changes will be made to Hardcover</td></tr>`);
        }
        
        return `
            <div class="detailed-settings-section">
                <h4>Sync Configuration</h4>
                <table class="settings-table">
                    <thead>
                        <tr><th>Setting</th><th>Value</th><th>Description</th></tr>
                    </thead>
                    <tbody>
                        ${rows.join('')}
                    </tbody>
                </table>
            </div>
        `;
    }
    
    showSyncSummary(profileId) {
        const status = this.statuses[profileId];
        if (!status) {
            console.error('No status found for profile:', profileId);
            return;
        }
        
        console.log('Showing sync summary for profile:', profileId, status);
        
        const container = document.getElementById('sync-summary-container');
        const content = document.getElementById('sync-summary-content');
        const tabsContainer = document.getElementById('sync-summary-tabs');
        
        if (!container || !content || !tabsContainer) {
            console.error('Missing required DOM elements for sync summary');
            return;
        }
        
        // Update the tabs to show the current profile
        tabsContainer.innerHTML = `
            <button class="tab-button active" data-profile="${profileId}">
                ${this.escapeHtml(status.profile_name || `Profile ${profileId}`)}
            </button>`;
        
        // Get the last sync time if available
        const lastSync = status.last_sync || status.lastSync;
        const lastSyncDate = lastSync ? new Date(lastSync).toLocaleString() : 'Never';
        
        // Build summary HTML
        const summary = status.last_sync_summary || status.lastSyncSummary || null;
        const booksSynced = (summary && typeof summary.books_synced === 'number') ? summary.books_synced : (status.books_synced || 0);
        const booksTotal = (summary && typeof summary.total_books_processed === 'number') ? summary.total_books_processed : (status.books_total || 0);
        const booksSkipped = status.books_skipped || 0;
        const syncSettings = status.sync_settings || {};
        // Prefer top-level mismatches; only fall back to summary mismatches if top-level is empty
        let mismatchesArr = Array.isArray(status.mismatches) && status.mismatches.length > 0
            ? status.mismatches
            : (Array.isArray(summary?.mismatches) ? summary.mismatches : []);
        // De-duplicate by book_id just in case both sources are present
        if (Array.isArray(summary?.mismatches) && summary.mismatches.length > 0 && Array.isArray(status.mismatches) && status.mismatches.length > 0) {
            const byId = new Map();
            [...status.mismatches, ...summary.mismatches].forEach(m => {
                const id = m.book_id || m.id;
                if (!byId.has(id)) byId.set(id, m);
            });
            mismatchesArr = Array.from(byId.values());
        }
        // Resolve the Audiobookshelf base URL for this profile (used to build ABS book links)
        const profileEntry = Array.isArray(this.users)
            ? this.users.find(u => (u.id || (u.profile && u.profile.id)) === profileId)
            : null;
        // Profiles returned by /api/profiles include Config as `config` with `audiobookshelf_url`
        const __absBaseUrl = profileEntry && profileEntry.config && profileEntry.config.audiobookshelf_url
            ? String(profileEntry.config.audiobookshelf_url).replace(/\/+$/, '')
            : '';
        
        // Build sync settings section for the detailed view
        const settingsHtml = this.buildDetailedSettingsHtml(syncSettings);
        
        let html = `
            <div class="sync-summary">
                <div class="summary-header">
                    <h3>Sync Summary: ${this.escapeHtml(status.profile_name || 'Unknown Profile')}</h3>
                    <div class="last-sync">Last Sync: ${lastSyncDate}</div>
                </div>
                ${settingsHtml}
                <div class="summary-stats">
                    <div class="stat-item success">
                        <span class="stat-value">${booksSynced}</span>
                        <span class="stat-label">Books Synced</span>
                    </div>
                    <div class="stat-item info">
                        <span class="stat-value">${booksTotal}</span>
                        <span class="stat-label">Total Processed</span>
                    </div>
                    ${booksSkipped > 0 ? `
                    <div class="stat-item muted">
                        <span class="stat-value">${booksSkipped}</span>
                        <span class="stat-label">Books Skipped</span>
                    </div>
                    ` : ''}`;
        
        // Add books not found stat if any
        if (status.books_not_found?.length > 0) {
            html += `
                    <div class="stat-item warning">
                        <span class="stat-value">${status.books_not_found.length}</span>
                        <span class="stat-label">Books Not Found</span>
                    </div>`;
        }
        
        // Add mismatches stat if any
        if (mismatchesArr.length > 0) {
            html += `
                    <div class="stat-item warning">
                        <span class="stat-value">${mismatchesArr.length}</span>
                        <span class="stat-label">Potential Mismatches</span>
                    </div>`;
        }
        
        html += `
                </div>`; // Close summary-stats
        
        // Add books not found section
        if (status.books_not_found?.length > 0) {
            const booksHtml = status.books_not_found.map(book => {
                const title = this.escapeHtml(book.title || 'Unknown Title');
                const subtitle = book.subtitle ? `<div class="book-subtitle">${this.escapeHtml(book.subtitle)}</div>` : '';
                const author = book.author ? `<div><strong>Author:</strong> ${this.escapeHtml(book.author)}</div>` : '';
                const publishedYear = book.published_year ? `<div><strong>Published:</strong> ${this.escapeHtml(book.published_year)}</div>` : '';
                const publisher = book.publisher ? `<div><strong>Publisher:</strong> ${this.escapeHtml(book.publisher)}</div>` : '';
                const asin = book.asin ? `<div><strong>ASIN:</strong> ${this.escapeHtml(book.asin)}</div>` : '';
                const isbn = book.isbn ? `<div><strong>ISBN:</strong> ${this.escapeHtml(book.isbn)}</div>` : '';
                const libraryId = book.library_id ? `<div><strong>Library ID:</strong> ${this.escapeHtml(book.library_id)}</div>` : '';
                const error = book.error ? `<div class="book-error">${this.escapeHtml(book.error)}</div>` : '';
                const reason = book.reason ? `<div class="book-reason"><strong>Reason:</strong> ${this.escapeHtml(book.reason)}</div>` : '';
                
                return `
                    <div class="book-item">
                        <div class="book-title">${title}</div>
                        ${subtitle}
                        <div class="book-meta">
                            ${author}
                            ${publishedYear}
                            ${publisher}
                            ${asin}
                            ${isbn}
                            ${libraryId}
                        </div>
                        ${error}
                        ${reason}
                    </div>`;
            }).join('');
            
            html += `
                <div class="summary-section">
                    <h4>Books Not Found in Hardcover</h4>
                    <div class="book-list">${booksHtml}
                    </div>
                </div>`;
        } else {
            html += `
                <div class="summary-section">
                    <p>All books were found in Hardcover.</p>
                </div>`;
        }
        
        // Add mismatches section if any
        if (mismatchesArr.length > 0) {
            const mismatchesHtml = mismatchesArr.map(mismatch => {
                // Get data from the mismatch object with proper fallbacks
                const absData = {
                    ...mismatch,
                    // Map any ABS-specific fields here if needed
                };
                // If ABS author is missing but Hardcover provided one, use Hardcover author as a fallback
                if ((!absData.author || absData.author === 'Unknown Author') && mismatch.hardcover_author) {
                    absData.author = mismatch.hardcover_author;
                }
                
                // Log mismatch data for debugging
                console.log('Mismatch data:', { absData, mismatch });
                
                // Extract author information
                const hardcoverAuthor = mismatch.hardcover_author || mismatch.author || 'Unknown Author';
                
                // Create direct link to the book on Hardcover (only if we have a book-level match via slug/path/url)
                const hardcoverBookUrl = (() => {
                    const data = mismatch.hardcover_data || {};
                    // Prefer explicit URL fields from backend
                    if (mismatch.hardcover_url && typeof mismatch.hardcover_url === 'string') return mismatch.hardcover_url;
                    if (data.url && typeof data.url === 'string') return data.url;
                    if (data.slug_url && typeof data.slug_url === 'string') return data.slug_url;
                    // Construct from slug or path
                    if (mismatch.hardcover_slug && typeof mismatch.hardcover_slug === 'string') return `https://hardcover.app/books/${mismatch.hardcover_slug}`;
                    if (data.slug && typeof data.slug === 'string') return `https://hardcover.app/books/${data.slug}`;
                    if (data.path && typeof data.path === 'string') return `https://hardcover.app${data.path}`;
                    // No book-level match -> no link
                    return '';
                })();

                // Base Hardcover data: ONLY what HC provided (no ABS fallbacks)
                const hcData = {
                    title: mismatch.hardcover_title || undefined,
                    author: mismatch.hardcover_author || undefined,
                    published_year: mismatch.hardcover_published_year || undefined,
                    publisher: mismatch.hardcover_publisher || undefined,
                    asin: mismatch.hardcover_asin || undefined,
                    isbn: mismatch.hardcover_isbn || undefined,
                    format: mismatch.hardcover_format || undefined,
                    language: mismatch.hardcover_language || undefined,
                    page_count: mismatch.hardcover_page_count || undefined,
                    description: mismatch.hardcover_description || undefined,
                    cover_url: mismatch.hardcover_cover_url || undefined,
                    id: mismatch.hardcover_book_id || undefined,
                    slug: mismatch.hardcover_slug || (mismatch.hardcover_data && mismatch.hardcover_data.slug) || undefined,
                    path: (mismatch.hardcover_data && mismatch.hardcover_data.path) || undefined,
                    // Include raw hardcover_data first so our computed URL can override legacy forms
                    ...(mismatch.hardcover_data || {}),
                    // Use the direct URL if available, otherwise construct it from slug/path only
                    url: hardcoverBookUrl
                };
                
                // Remove any duplicate or empty fields
                Object.keys(hcData).forEach(key => {
                    if (hcData[key] === undefined || hcData[key] === '') {
                        delete hcData[key];
                    }
                });

                // Normalize legacy Hardcover URL forms: prefer slug/path; otherwise drop link
                if (typeof hcData === 'object' && hcData) {
                    const legacyRe = /^https?:\/\/hardcover\.app\/book\/[0-9]+\/?$/i;
                    if (hcData.url && legacyRe.test(hcData.url)) {
                        if (hcData.slug) {
                            hcData.url = `https://hardcover.app/books/${hcData.slug}`;
                        } else if (hcData.path) {
                            hcData.url = `https://hardcover.app${hcData.path}`;
                        } else {
                            hcData.url = '';
                        }
                    }
                }
                
                // If we have a hardcover_book object, use its properties
                if (mismatch.hardcover_book) {
                    const book = mismatch.hardcover_book;
                    Object.assign(hcData, {
                        title: book.title || hcData.title,
                        author: book.author_display || book.author || hcData.author,
                        // Preserve authors array for multi-author rendering
                        authors: Array.isArray(book.authors) && book.authors.length > 0 ? book.authors : hcData.authors,
                        published_year: book.published_year || hcData.published_year,
                        publisher: book.publisher || hcData.publisher,
                        isbn: book.isbn || book.isbn13 || hcData.isbn,
                        format: book.format || hcData.format,
                        language: book.language || hcData.language,
                        page_count: book.page_count || hcData.page_count,
                        description: book.description || hcData.description,
                        cover_url: book.cover_url || book.cover_image_url || hcData.cover_url,
                        slug: book.slug || hcData.slug,
                        path: book.path || hcData.path,
                        // Prefer slug/path-derived URL or precomputed hcData.url over legacy /book/<id>
                        url: (
                            hcData.url ||
                            (book.slug ? `https://hardcover.app/books/${book.slug}` : (book.path ? `https://hardcover.app${book.path}` : '')) ||
                            book.url ||
                            hcData.url
                        )
                    });
                }
                
                const hasAbsData = absData && (absData.title || absData.author);
                const hasHcData = hcData && (hcData.title || hcData.author);
                
                const displayTitle = this.escapeHtml(absData?.title || 'Unknown Title');
                const displaySubtitle = absData?.subtitle ? this.escapeHtml(absData.subtitle) : '';
                
                // Helper function to clean and extract book data with deep fallbacks
                const extractBookData = (data, source) => {
                    if (!data) return {};
                    
                    // Clean the data by removing empty/undefined values and trimming strings
                    const cleanData = {};
                    Object.entries(data).forEach(([key, value]) => {
                        if (value !== undefined && value !== null && value !== '') {
                            if (typeof value === 'string') {
                                const trimmed = value.trim();
                                if (trimmed) cleanData[key] = trimmed;
                            } else if (Array.isArray(value) && value.length > 0) {
                                cleanData[key] = value;
                            } else if (typeof value === 'object' && value !== null) {
                                cleanData[key] = value;
                            } else if (value !== '') {
                                cleanData[key] = value;
                            }
                        }
                    });

                    // Extract and transform fields with fallbacks
                    const extracted = {
                        // Title with fallbacks
                        title: cleanData.title || cleanData.name || 'Unknown Title',
                        // Optional subtitle
                        subtitle: cleanData.subtitle,
                        
                        // Author with multiple fallback fields (include Hardcover's author_display)
                        author: cleanData.author || 
                               cleanData.author_display ||
                               cleanData.author_name || 
                               cleanData.authors?.[0]?.name ||
                               (Array.isArray(cleanData.authors) && cleanData.authors.length > 0 ? 
                                   cleanData.authors[0] : 'Unknown Author'),
                        
                        // Narrator (ABS)
                        narrator: cleanData.narrator || 
                                  cleanData.reader || 
                                  (Array.isArray(cleanData.narrators) ? cleanData.narrators.join(', ') : cleanData.narrators),
                        
                        // Published year from various date formats (prefer incoming published_year)
                        published_year: cleanData.published_year ||
                                      cleanData.publishedYear || 
                                      (cleanData.published_date ? 
                                          cleanData.published_date.split('-')[0] : 
                                          cleanData.publication_date?.split('-')[0]),
                        
                        // Publisher with fallback to series
                        publisher: cleanData.publisher || 
                                 (cleanData.series && cleanData.series.publisher) ||
                                 cleanData.series?.publisher,
                        
                        // Format with intelligent detection
                        format: cleanData.format || 
                               (cleanData.mediaType ? 
                                   `${cleanData.mediaType.charAt(0).toUpperCase()}${cleanData.mediaType.slice(1)}` : 
                                   source === 'abs' ? 'Audiobook' : 'Book'),
                        
                        // Language with fallback
                        language: cleanData.language || 
                                (cleanData.languages && cleanData.languages[0]) ||
                                cleanData.language_code,
                        
                        // Page count with fallbacks
                        page_count: cleanData.numPages || 
                                  cleanData.pageCount || 
                                  cleanData.pages,
                        
                        // Description with fallback to subtitle
                        description: cleanData.description || 
                                   cleanData.overview ||
                                   cleanData.summary,

                        // Duration fields (ABS)
                        duration_seconds: (typeof cleanData.duration_seconds === 'number' ? cleanData.duration_seconds :
                                           typeof cleanData.durationSeconds === 'number' ? cleanData.durationSeconds :
                                           (typeof cleanData.duration === 'number' ? cleanData.duration : undefined)),
                        duration: (typeof cleanData.duration === 'string' ? cleanData.duration :
                                   cleanData.length || cleanData.length_readable),
                        
                        // Cover image with multiple possible fields (accept raw cover_url too)
                        cover_url: cleanData.coverImageUrl || 
                                 cleanData.cover_image_url ||
                                 cleanData.cover_url ||
                                 cleanData.cover?.medium ||
                                 cleanData.cover?.large ||
                                 cleanData.image_url,
                        
                        // Identifiers with fallbacks
                        // Do NOT cross-fallback between ASIN and ISBN
                        asin: cleanData.asin,
                        isbn: cleanData.isbn || cleanData.isbn13 || cleanData.isbn_13 || cleanData.isbn10 || cleanData.isbn_10,
                        isbn10: cleanData.isbn10 || cleanData.isbn_10,
                        isbn13: cleanData.isbn13 || cleanData.isbn_13,
                        
                        // URLs with controlled generation: don't fall back to Amazon for title link
                        url: (() => {
                            const legacyRe = /^https?:\/\/hardcover\.app\/book\/[0-9]+\/?$/i;
                            // Start with provided URL when present
                            let u = cleanData.url || '';
                            // ABS keeps its abs_url
                            if (!u && source === 'abs' && cleanData.abs_url) u = cleanData.abs_url;
                            // For HC prefer slug/path when building from scratch
                            if (!u && source === 'hc') {
                                if (cleanData.slug && typeof cleanData.slug === 'string') u = `https://hardcover.app/books/${cleanData.slug}`;
                                else if (cleanData.path && typeof cleanData.path === 'string') u = `https://hardcover.app${cleanData.path}`;
                            }
                            // Normalize legacy HC book ID URLs
                            if (source === 'hc' && u && legacyRe.test(u)) {
                                if (cleanData.slug && typeof cleanData.slug === 'string') u = `https://hardcover.app/books/${cleanData.slug}`;
                                else if (cleanData.path && typeof cleanData.path === 'string') u = `https://hardcover.app${cleanData.path}`;
                                else u = '';
                            }
                            return u || '';
                        })(),
                        // Preserve ABS direct link for buttons and other UI
                        abs_url: cleanData.abs_url,
                        // Additional metadata
                        genres: cleanData.genres || cleanData.categories,
                        // Preserve authors array if provided (used for multi-author rendering)
                        authors: Array.isArray(cleanData.authors) && cleanData.authors.length > 0 ? cleanData.authors : undefined,
                        series: cleanData.series,
                        // ABS-specific additional metadata
                        library_id: cleanData.library_id,
                        folder_id: cleanData.folder_id,
                        release_date: cleanData.release_date,
                        // ABS-specific tracking/context
                        book_id: cleanData.book_id || cleanData.id,
                        timestamp: cleanData.timestamp,
                        created_at: cleanData.created_at,
                        reason: cleanData.reason,
                        attempts: cleanData.attempts,
                        
                        // Status information based on source
                        status: source === 'abs' ? 'In Audiobookshelf' : 'On Hardcover',
                        statusType: source === 'abs' ? 'success' : 'info'
                    };
                    
                    // Clean up any remaining undefined values
                    Object.keys(extracted).forEach(key => {
                        if (extracted[key] === undefined || 
                            (Array.isArray(extracted[key]) && extracted[key].length === 0)) {
                            delete extracted[key];
                        }
                    });
                    
                    return extracted;
                };
                
                // Build a list of cover image fallbacks (ordered, unique)
                const computeCoverFallbacks = (cleanData, rawData, source) => {
                    try {
                        const isHttp = (u) => typeof u === 'string' && /^https?:\/\//.test(u);

                        // Primary candidates from the current source
                        const candidates = [
                            cleanData && cleanData.cover_url,
                            cleanData && cleanData.image_url,
                            rawData && rawData.cover_url,
                            rawData && rawData.cover_image_url,
                            rawData && (rawData.cover?.large),
                            rawData && (rawData.cover?.medium)
                        ].filter(isHttp);

                        // If rendering ABS, append Hardcover URLs as fallbacks
                        if (source === 'abs' && typeof hcData === 'object' && hcData) {
                            const hcCandidates = [
                                hcData.cover_url,
                                hcData.cover_image_url,
                                hcData.image_url,
                                hcData.cover && hcData.cover.large,
                                hcData.cover && hcData.cover.medium
                            ].filter(isHttp);
                            candidates.push(...hcCandidates);
                        }

                        // Append local placeholder last
                        candidates.push('/cover-placeholder.svg');

                        // De-duplicate while preserving order
                        const seen = new Set();
                        const unique = [];
                        for (const u of candidates) {
                            if (!seen.has(u)) { seen.add(u); unique.push(u); }
                        }
                        return unique;
                    } catch (e) {
                        console.warn('computeCoverFallbacks error:', e);
                        return ['/cover-placeholder.svg'];
                    }
                };
                
                // Helper function to render a book's details
                const renderBookDetails = (data, source) => {
                    try {
                        console.log(`Rendering ${source} data:`, data);
                        if (!data || (typeof data === 'object' && Object.keys(data).length === 0)) {
                            console.log(`No data for ${source}`);
                            return `
                            <div class="comparison-details">
                                <div><em>No data available</em></div>
                            </div>`;
                        }
                        
                        // Extract and clean the data
                        const cleanData = extractBookData(data, source);
                        const details = [];
                        
                        // Set up author information with safe fallbacks and support for multiple authors
                        let authorToShow = (() => {
                            // Prefer explicit authors array when present (map objects to name)
                            if (Array.isArray(cleanData.authors) && cleanData.authors.length > 0) {
                                const names = cleanData.authors
                                    .map(a => (typeof a === 'string' ? a : (a && a.name ? a.name : '')))
                                    .filter(Boolean);
                                if (names.length > 0) return names.join(', ');
                            }
                            return cleanData.author || 'Unknown Author';
                        })();
                        
                        // HC-specific notices: show edition/book status ABOVE the image
                        if (source === 'hc') {
                            const hasBookMatch = !!(cleanData.url || cleanData.slug || cleanData.path || cleanData.id);
                            if (hasBookMatch) {
                                details.push(`
                                    <div class="mb-3">
                                        <div class="edition-warning rounded-md px-3 py-2 text-sm flex items-start gap-2">
                                            <i class="fas fa-exclamation-triangle mt-0.5 edition-warning-icon" aria-hidden="true"></i>
                                            <div>
                                                <div class="edition-warning-title"><strong>Edition not matched.</strong></div>
                                                <div class="edition-warning-text">Create or link the correct Hardcover edition.</div>
                                            </div>
                                        </div>
                                    </div>
                                `);
                            } else {
                                details.push(`
                                    <div class="mb-3">
                                        <div class="book-not-found rounded-md px-3 py-2 text-sm flex items-start gap-2">
                                            <i class="fas fa-info-circle mt-0.5 book-not-found-icon" aria-hidden="true"></i>
                                            <div>
                                                <div class="book-not-found-title"><strong>Book not found on Hardcover.</strong></div>
                                                <div class="book-not-found-text">Try searching on Hardcover to create it.</div>
                                            </div>
                                        </div>
                                    </div>
                                `);
                            }
                        }

                        // Add cover image near the top of each column
                        {
                            const fallbacks = computeCoverFallbacks(cleanData, data, source);
                            const initialSrc = fallbacks[0] || '/cover-placeholder.svg';
                            const fbAttr = this.escapeHtml(fallbacks.join('|'));
                            const altText = this.escapeHtml(cleanData.title || 'Book cover');
                            details.push(`
                                <div class="mb-3 text-center">
                                    <img src="${initialSrc}"
                                         data-fallbacks="${fbAttr}"
                                         data-fb-idx="0"
                                         alt="${altText}"
                                         class="book-cover mx-auto"
                                         loading="lazy"
                                         decoding="async"
                                         fetchpriority="low"
                                         onerror="window.__absHandleImageError && window.__absHandleImageError(this)"
                                         style="max-height: 300px; max-width: 100%; border-radius: 4px; box-shadow: 0 2px 8px rgba(0,0,0,0.1);">
                                </div>
                            `);
                        }
                        
                        // Use provided author URL only; do not fabricate cross-site search links
                        let authorUrl = cleanData.author_url;
                        
                        // Add title with link if URL is available
                        if (cleanData.title) {
                            const titleText = this.escapeHtml(cleanData.title);
                            
                            // Create title display with optional author
                            let titleHtml = `<div class="mb-2"><strong>Title</strong><span> `;
                            
                            if (cleanData.url) {
                                titleHtml += `<a href="${this.escapeHtml(cleanData.url)}" target="_blank" rel="noopener noreferrer" class="font-medium text-blue-600 hover:underline">${titleText} <i class="fas fa-external-link-alt" style="font-size: 0.8em;"></i></a>`;
                            } else {
                                titleHtml += titleText;
                            }
                            
                            // Add author if available
                            if (authorToShow && authorToShow !== 'Unknown Author') {
                                if (authorUrl) {
                                    titleHtml += ` <span class=\"text-gray-600\">by</span> <a href="${this.escapeHtml(authorUrl)}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:underline">${this.escapeHtml(authorToShow)} <i class=\"fas fa-external-link-alt\" style=\"font-size: 0.8em;\"></i></a>`;
                                } else {
                                    titleHtml += ` <span class=\"text-gray-600\">by</span> <span class=\"text-gray-800\">${this.escapeHtml(authorToShow)}</span>`;
                                }
                            }
                            
                            titleHtml += `</span></div>`;
                            details.push(titleHtml);
                            // Subtitle directly under the title, labeled like other fields
                            if (cleanData.subtitle) {
                                details.push(`<div class="mb-2"><strong>Subtitle</strong><span class="text-gray-800"> ${this.escapeHtml(cleanData.subtitle)}</span></div>`);
                            }
                        } else if (authorToShow && authorToShow !== 'Unknown Author') {
                            // If no title but we have an author, show just the author
                            if (authorUrl) {
                                details.push(`<div class=\"mb-2\"><strong>Author</strong><span> <a href="${this.escapeHtml(authorUrl)}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:underline">${this.escapeHtml(authorToShow)} <i class=\"fas fa-external-link-alt\" style=\"font-size: 0.8em;\"></i></a></span></div>`);
                            } else {
                                details.push(`<div class=\"mb-2\"><strong>Author</strong><span class=\"text-gray-800\">${this.escapeHtml(authorToShow)}</span></div>`);
                            }
                        }
                        
                        // Add metadata in a clean, consistent format
                        const metadata = [];
                        // For ABS (and HC), group metadata into sections for clarity
                        const identifiers = (source === 'abs' || source === 'hc') ? [] : null;
                        const metaSection = (source === 'abs' || source === 'hc') ? [] : null;
                        const tracking = (source === 'abs' || source === 'hc') ? [] : null;
                        
                        // Published: ABS shows date/year; HC only when an edition (release_date) exists
                        if (
                            (source !== 'hc' && (cleanData.release_date || cleanData.published_year)) ||
                            (source === 'hc' && !!cleanData.release_date)
                        ) {
                            const target = (source === 'abs' || source === 'hc') ? metaSection : metadata;
                            const publishedVal = (cleanData.release_date || cleanData.published_year).toString();
                            target.push({
                                label: 'Published',
                                value: this.escapeHtml(publishedVal)
                            });
                        }
                        
                        // Add publisher if available
                        // - ABS: always show when present
                        // - HC: only show when an edition (release_date) is present
                        if (
                            (source === 'abs' && cleanData.publisher) ||
                            (source === 'hc' && cleanData.publisher && !!cleanData.release_date)
                        ) {
                            const target = (source === 'abs' || source === 'hc') ? metaSection : metadata;
                            target.push({
                                label: 'Publisher',
                                value: this.escapeHtml(cleanData.publisher)
                            });
                        }
                        
                        // Omit format row; it's not meaningful for our comparison UI

                        // ABS-specific: add Narrator, ASIN, identifiers and tracking when present
                        if (source === 'abs') {
                            if (cleanData.narrator) {
                                metaSection.push({
                                    label: 'Narrator',
                                    value: this.escapeHtml(cleanData.narrator)
                                });
                            }
                            if (cleanData.asin) {
                                const asinEsc = this.escapeHtml(cleanData.asin);
                                const audibleHref = `https://www.audible.com/pd?asin=${asinEsc}`;
                                identifiers.push({
                                    label: 'ASIN',
                                    value: `<a href="${audibleHref}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:underline" title="Open on Audible"><code class="text-inherit">${asinEsc}</code> <i class="fas fa-external-link-alt" style="font-size: 0.8em;"></i></a>`
                                });
                            }
                            // Show ISBN fields if available
                            const isbnVal = data.isbn || cleanData.isbn;
                            const isbn10Val = data.isbn_10 || cleanData.isbn10 || cleanData.isbn_10;
                            const isbn13Val = data.isbn_13 || cleanData.isbn13 || cleanData.isbn_13;
                            if (isbnVal) {
                                identifiers.push({ label: 'ISBN', value: this.escapeHtml(isbnVal.toString()) });
                            }
                            if (isbn10Val) {
                                identifiers.push({ label: 'ISBN-10', value: this.escapeHtml(isbn10Val.toString()) });
                            }
                            if (isbn13Val) {
                                identifiers.push({ label: 'ISBN-13', value: this.escapeHtml(isbn13Val.toString()) });
                            }

                            // Show Library ID, Folder ID when available
                            if (data.library_id || cleanData.library_id) {
                                tracking.push({
                                    label: 'Library ID',
                                    value: `<code>${this.escapeHtml((data.library_id || cleanData.library_id).toString())}</code>`
                                });
                            }
                            if (data.folder_id || cleanData.folder_id) {
                                tracking.push({
                                    label: 'Folder ID',
                                    value: `<code>${this.escapeHtml((data.folder_id || cleanData.folder_id).toString())}</code>`
                                });
                            }
                            if (cleanData.book_id) {
                                const __bookIdStr = cleanData.book_id.toString();
                                // Prefer UI route with configured base URL
                                let __absBookUrl = __absBaseUrl
                                    ? `${__absBaseUrl}/item/${encodeURIComponent(__bookIdStr)}`
                                    : '';
                                // Fallback: use abs_url if provided by API
                                if (!__absBookUrl && typeof cleanData.abs_url === 'string' && cleanData.abs_url) {
                                    __absBookUrl = cleanData.abs_url;
                                }
                                // Fallback: derive UI link from cover_url (api/items/<id>/cover -> /audiobookshelf/item/<id>)
                                if (!__absBookUrl && typeof cleanData.cover_url === 'string' && cleanData.cover_url) {
                                    try {
                                        const u = new URL(cleanData.cover_url);
                                        const base = `${u.origin}/audiobookshelf`;
                                        __absBookUrl = `${base}/item/${encodeURIComponent(__bookIdStr)}`;
                                    } catch (_) {
                                        // ignore URL parse errors; leave as plain code
                                    }
                                }
                                const __valueHtml = __absBookUrl
                                    ? `<a href="${this.escapeHtml(__absBookUrl)}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:underline" title="Open in Audiobookshelf"><code class="text-inherit">${this.escapeHtml(__bookIdStr)}</code> <i class="fas fa-external-link-alt" style="font-size: 0.8em;"></i></a>`
                                    : `<code>${this.escapeHtml(__bookIdStr)}</code>`;
                                tracking.push({
                                    label: 'Book ID',
                                    value: __valueHtml
                                });
                            }
                            // Note: HC tracking is handled in the HC block below
                            // Remove Detected row as requested
                            if (typeof cleanData.attempts === 'number') {
                                tracking.push({
                                    label: 'Attempts',
                                    value: this.escapeHtml(String(cleanData.attempts))
                                });
                            }
                            // Remove Reason row as requested
                        }
                        // HC-specific tracking rows (mirrors ABS layout)
                        if (source === 'hc' && data && tracking) {
                            if (data.id) {
                                const __hcBookIdStr = data.id.toString();
                                // Build preferred HC link order: explicit url > slug > path
                                let __hcUrl = '';
                                if (typeof data.url === 'string' && data.url) {
                                    __hcUrl = data.url;
                                } else if (typeof data.slug === 'string' && data.slug) {
                                    __hcUrl = `https://hardcover.app/books/${encodeURIComponent(data.slug)}`;
                                } else if (typeof data.path === 'string' && data.path) {
                                    __hcUrl = `https://hardcover.app${data.path}`;
                                }
                                const __hcValueHtml = __hcUrl
                                    ? `<a href="${this.escapeHtml(__hcUrl)}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:underline" title="Open on Hardcover"><code class="text-inherit">${this.escapeHtml(__hcBookIdStr)}</code> <i class="fas fa-external-link-alt" style="font-size: 0.8em;"></i></a>`
                                    : `<code>${this.escapeHtml(__hcBookIdStr)}</code>`;
                                tracking.push({ label: 'Book ID', value: __hcValueHtml });
                            }
                        }
                        
                        // Add page count if available
                        if (cleanData.page_count) {
                            const target = (source === 'abs' || source === 'hc') ? metaSection : metadata;
                            target.push({
                                label: 'Pages',
                                value: cleanData.page_count.toString()
                            });
                        }
                        
                        // Add language if available
                        if (cleanData.language) {
                            const target = (source === 'abs' || source === 'hc') ? metaSection : metadata;
                            target.push({
                                label: 'Language',
                                value: this.escapeHtml(cleanData.language)
                            });
                        }
                        
                        // Add duration or length
                        if (cleanData.duration_seconds) {
                            const hours = Math.floor(cleanData.duration_seconds / 3600);
                            const minutes = Math.floor((cleanData.duration_seconds % 3600) / 60);
                            const durationText = hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`;
                            const target = (source === 'abs' || source === 'hc') ? metaSection : metadata;
                            target.push({
                                label: 'Duration',
                                value: durationText
                            });
                        } else if (cleanData.duration) {
                            const target = (source === 'abs' || source === 'hc') ? metaSection : metadata;
                            target.push({
                                label: 'Duration',
                                value: cleanData.duration
                            });
                        }
                        
                        // Add genres if available
                        if (cleanData.genres && cleanData.genres.length > 0) {
                            const genres = Array.isArray(cleanData.genres) 
                                ? cleanData.genres.map(g => this.escapeHtml(g)).join(', ')
                                : this.escapeHtml(cleanData.genres);
                            const target = (source === 'abs' || source === 'hc') ? metaSection : metadata;
                            target.push({
                                label: 'Genres',
                                value: genres,
                                class: 'text-sm text-gray-600'
                            });
                        }
                        
                        // Add series information if available
                        if (cleanData.series) {
                            const seriesInfo = [];
                            if (cleanData.series.name) {
                                seriesInfo.push(`<span class="font-medium">${this.escapeHtml(cleanData.series.name)}</span>`);
                            }
                            if (cleanData.series.sequence) {
                                seriesInfo.push(`(Book ${cleanData.series.sequence})`);
                            }
                            
                            if (seriesInfo.length > 0) {
                                const target = (source === 'abs' || source === 'hc') ? metaSection : metadata;
                                target.push({
                                    label: 'Series',
                                    value: seriesInfo.join(' ')
                                });
                            }
                        }
                        
                        // Render metadata
                        if (source === 'abs' || (source === 'hc' && ((identifiers && identifiers.length) || (metaSection && metaSection.length) || (tracking && tracking.length)))) {
                            const sections = [
                                { title: 'Identifiers', items: identifiers || [] },
                                { title: 'Metadata', items: metaSection || [] },
                                { title: 'Tracking', items: tracking || [] }
                            ].filter(s => s.items && s.items.length > 0);

                            sections.forEach((section, idx) => {
                                // Divider between sections
                                if (idx > 0) {
                                    details.push(`<div class="my-2 border-t border-gray-200"></div>`);
                                }
                                // Section header
                                details.push(`
                                    <h4 class="mt-2 mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">${this.escapeHtml(section.title)}</h4>
                                `);
                                // Section items
                                section.items.forEach(item => {
                                    details.push(`
                                        <div>
                                            <strong>${item.label}</strong>
                                            <span class="${item.class || 'text-gray-800'}">${item.value}</span>
                                        </div>
                                    `);
                                });
                            });
                        } else {
                            // Non-ABS: flat list rendering
                            if (metadata.length > 0) {
                                metadata.forEach(item => {
                                    details.push(`
                                        <div>
                                            <strong>${item.label}</strong>
                                            <span class="${item.class || 'text-gray-800'}">${item.value}</span>
                                        </div>
                                    `);
                                });
                            }
                        }
                        
                        
                        
                        // Do not render a separate Author row if it was already shown with the title to avoid duplicates
                        // However, if there is no title, still show the author-only row
                        if (!cleanData.title && authorToShow && authorToShow !== 'Unknown Author') {
                            const authorText = this.escapeHtml(authorToShow);
                            if (authorUrl) {
                                details.push(`<div class=\"mb-2\"><strong>Author</strong><span> <a href="${this.escapeHtml(authorUrl)}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:underline">${authorText} <i class=\"fas fa-external-link-alt\" style=\"font-size: 0.8em;\"></i></a></span></div>`);
                            } else {
                                details.push(`<div class=\"mb-2\"><strong>Author</strong><span class=\"text-gray-800\">${authorText}</span></div>`);
                            }
                        }

                        // Do not add a separate HC button; title already links when available
                        
                        // Add Hardcover ID (HC side) to Identifiers section so HC matches ABS layout
                        if (source === 'hc' && data.id && identifiers) {
                            const hcIdLink = data.url ? data.url : (data.slug ? `https://hardcover.app/books/${data.slug}` : (data.path ? `https://hardcover.app${data.path}` : ''));
                            const idHtml = hcIdLink
                                ? `<a href="${hcIdLink}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:underline" title="Open on Hardcover"><code class="text-inherit">${this.escapeHtml(data.id)}</code> <i class="fas fa-external-link-alt" style="font-size: 0.8em;"></i></a>`
                                : `<code>${this.escapeHtml(data.id)}</code>`;
                            identifiers.push({ label: 'Hardcover ID', value: idHtml });
                        }
                        
                        // Add description if available (with markdown link support and read more/less)
                        if (cleanData.description) {
                            const maxLength = 300;
                            // First escape HTML, then handle markdown links
                            const escapeHtml = (str) => {
                                return str
                                    .replace(/&/g, '&amp;')
                                    .replace(/</g, '&lt;')
                                    .replace(/>/g, '&gt;')
                                    .replace(/"/g, '&quot;')
                                    .replace(/'/g, '&#039;');
                            };
                            
                            // Convert markdown links to HTML
                            const processMarkdownLinks = (text) => {
                                return text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, 
                                    (match, text, url) => {
                                        return `<a href="${escapeHtml(url)}" target="_blank" rel="noopener noreferrer" class="text-blue-600 hover:underline">${escapeHtml(text)} <i class="fas fa-external-link-alt" style="font-size: 0.7em;"></i></a>`;
                                    }
                                );
                            };
                            
                            const escapedDescription = escapeHtml(cleanData.description);
                            const processedDescription = processMarkdownLinks(escapedDescription);
                            const isLong = processedDescription.length > maxLength;
                            
                            // Create short description by truncating at the last space before maxLength
                            let shortDesc = processedDescription;
                            if (isLong) {
                                const lastSpace = processedDescription.lastIndexOf(' ', maxLength);
                                shortDesc = processedDescription.substring(0, lastSpace > 0 ? lastSpace : maxLength) + '...';
                            }
                            
                            details.push(`
                                <div class="mt-3">
                                    <div class="font-medium text-gray-700 mb-1">Description:</div>
                                    <div class="text-gray-800 text-sm description-container">
                                        <span class="description-text">${shortDesc}</span>
                                        ${isLong ? 
                                            `<span class="description-full hidden">${processedDescription}</span>
                                            <a href="#" class="text-blue-600 hover:underline read-more">Read more</a>` 
                                            : ''
                                        }
                                    </div>
                                </div>
                            `);
                        }
                        
                        // Add action buttons in a consistent, accessible way
                        const buttons = [];
                        
                        // Do not add a separate Hardcover button; the title already links to Hardcover on the HC side
                        
                        // Removed external ASIN action button per request
                        
                        // Add Audiobookshelf button for ABS source
                        if (source === 'abs') {
                            const bookUrl = (cleanData.book_id && __absBaseUrl)
                                ? `${__absBaseUrl}/item/${encodeURIComponent(cleanData.book_id.toString())}`
                                : (cleanData.abs_url || cleanData.url || '');
                            if (bookUrl) {
                                buttons.push({
                                    url: bookUrl,
                                    icon: 'headphones',
                                    label: 'Open in Audiobookshelf',
                                    style: 'secondary',
                                    title: 'Open this book in Audiobookshelf'
                                });
                            }
                        }
                        
                        // Add Goodreads button only on ABS side (prefer ISBN13 > ISBN10 > ISBN)
                        if (source === 'abs') {
                            const grIsbn = cleanData.isbn13 || cleanData.isbn_13 || cleanData.isbn10 || cleanData.isbn_10 || cleanData.isbn;
                            if (grIsbn) {
                                buttons.push({
                                    url: `https://www.goodreads.com/search?q=${grIsbn}`,
                                    icon: 'goodreads',
                                    label: 'View on Goodreads',
                                    style: 'secondary',
                                    title: `Find ${cleanData.title || 'this book'} on Goodreads`
                                });
                            }
                        }
                        
                        // Render buttons with consistent styling
                        if (buttons.length > 0) {
                            const buttonClasses = {
                                primary: 'bg-indigo-600 hover:bg-indigo-700 text-white border-transparent',
                                secondary: 'bg-white hover:bg-gray-50 text-gray-700 border-gray-300',
                                danger: 'bg-red-600 hover:bg-red-700 text-white border-transparent'
                            };
                            
                            const iconMap = {
                                book: 'book',
                                amazon: 'amazon',
                                headphones: 'headphones',
                                goodreads: 'book-open',
                                external: 'external-link-alt'
                            };
                            
                            details.push(`
                                <div class="mt-4 flex flex-wrap gap-2">
                                    ${buttons.map(btn => `
                                        <a href="${this.escapeHtml(btn.url)}" 
                                           target="_blank" 
                                           rel="noopener noreferrer"
                                           title="${btn.title || ''}"
                                           class="inline-flex items-center px-3 py-1.5 border rounded-md text-xs font-medium shadow-sm focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 ${buttonClasses[btn.style] || buttonClasses.secondary}">
                                            <i class="${btn.icon.startsWith('fa-') ? btn.icon : `fa${btn.icon === 'amazon' ? 'b' : 's'} fa-${iconMap[btn.icon] || iconMap.external}`} mr-1"></i> 
                                            ${btn.label}
                                        </a>
                                    `).join('\n')}
                                </div>
                            `);
                        }
                        
                        // Add status if available (e.g., "Not in Library")
                        if (data.status) {
                            const statusType = data.statusType || 'info';
                            details.push(`
                                <div class="mt-3">
                                    <span class="status-badge inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                                        statusType === 'success' ? 'bg-green-100 text-green-800' : 
                                        statusType === 'warning' ? 'bg-yellow-100 text-yellow-800' : 
                                        'bg-blue-100 text-blue-800'
                                    }">
                                        ${this.escapeHtml(data.status)}
                                    </span>
                                </div>
                            `);
                        }
                        
                        return `
                            <div class="comparison-details">
                                ${details.join('')}
                            </div>`;
                    } catch (error) {
                        console.error('Error in renderBookDetails:', error);
                        return '<div class="comparison-details"><em>Error loading details</em></div>';
                    }
                };
                
                // Do not inject status into Hardcover details to avoid duplicate display
                // The mismatch reason is already shown once below the comparison.
                
                // Generate the book details HTML first
                // Avoid rendering placeholder authors
                if (absData && absData.author === 'Unknown Author') {
                    delete absData.author;
                }

                const absDetails = renderBookDetails({
                    ...absData,
                    format: absData?.format || 'Audiobook', // Use format from data if available
                    // Do not pass a status to avoid extra badge rendering on ABS side
                    url: absData?.abs_url || absData?.link || absData?.url || '',
                    author: absData?.author || absData?.hardcover_author || absData?.author_name || 'Unknown Author'
                }, 'abs');
                
                // Resolve Hardcover author; if it's unknown, fall back to ABS author
                let hcAuthorResolved = hcData.author || hcData.hardcover_author || hcData.author_name;
                if (!hcAuthorResolved || hcAuthorResolved === 'Unknown Author') {
                    hcAuthorResolved = absData.author || mismatch.author || hcAuthorResolved;
                }
                if (hcAuthorResolved === 'Unknown Author') {
                    hcAuthorResolved = undefined;
                }

                const hcDetails = renderBookDetails({
                    ...hcData,
                    format: hcData.format || 'Book',
                    // Do not pass a status to prevent duplicate status badge in HC column
                    url: hcData.url || (hcData.id ? `https://hardcover.app/book/${hcData.id}` : ''),
                    author: hcAuthorResolved || 'Unknown Author'
                }, 'hc');
                
                // Determine ABS link for header title if available
                const __absHeaderUrl = absData?.abs_url || absData?.link || absData?.url || '';

                return `
                    <div class="mismatch-item" data-book-id="${mismatch.id || mismatch.book_id || 'book-' + Math.random().toString(36).substr(2, 9)}">
                        <div class="mismatch-header">
                            <div class="mismatch-title">${__absHeaderUrl ? `<a href="${this.escapeHtml(__absHeaderUrl)}" target="_blank" rel="noopener noreferrer" class="text-blue-700 hover:underline">${displayTitle} <i class=\"fas fa-external-link-alt\" style=\"font-size: 0.8em;\"></i></a>` : displayTitle}</div>
                            ${displaySubtitle ? `<div class=\"mismatch-subtitle\"><strong>Subtitle</strong><span class=\"text-gray-800\"> ${displaySubtitle}</span></div>` : ''}
                        </div>
                        
                        <div class="mismatch-columns mismatch-comparison">
                            <!-- Audiobookshelf Column -->
                            <div class="mismatch-col abs comparison-column">
                                <div class="mismatch-col-title abs comparison-header">
                                    <i class="fas fa-book-open"></i>
                                    <span>Audiobookshelf</span>
                                </div>
                                ${absDetails}
                            </div>
                            
                            <!-- Arrow Divider -->
                            <div class="mismatch-divider comparison-arrow">
                                <div class="mismatch-divider-dot">
                                    <i class="fas fa-arrow-right"></i>
                                </div>
                            </div>
                            
                            <!-- Hardcover Column -->
                            <div class="mismatch-col hc comparison-column">
                                <div class="mismatch-col-title hc comparison-header">
                                    <i class="fas fa-book"></i>
                                    <span>Hardcover</span>
                                </div>
                                ${hcDetails}
                            </div>
                        </div>
                        
                        ${mismatch.reason ? `
                            <div class="mismatch-reason">
                                <strong>Note:</strong> ${this.escapeHtml(mismatch.reason)}
                            </div>` : ''}
                    </div>`;
            }).join('');
            
            html += `
                <div class="summary-section">
                    <div class="section-header">
                        <h3>Potential Mismatches</h3>
                        <p class="mismatch-help">These books were found but may have some discrepancies. Please verify the details.</p>
                    </div>
                    <div class="summary-stats">
                        <div class="mismatches-container">
                            ${mismatchesHtml}
                        </div>
                    </div>
                </div>`;
        }
        
        // Close the sync-summary div
        html += `
            </div>`;
            
        // Update the content and show the container
        content.innerHTML = html;
        container.style.display = 'block';
        container.scrollIntoView({ behavior: 'smooth' });
        
        // Show the sync tab
        this.showTab('sync');
    }

    renderSyncSummary() {
        // Currently empty, but can be used to render a summary of all syncs
    }

    async handleAddProfile(event) {
        const formData = new FormData(event.target);
        const profileData = {
            id: formData.get('id'),
            name: formData.get('name'),
            audiobookshelf_url: formData.get('audiobookshelf_url'),
            audiobookshelf_token: formData.get('audiobookshelf_token'),
            hardcover_token: formData.get('hardcover_token'),
            sync_config: {
                incremental: formData.get('incremental') === 'on',
                state_file: `./data/${formData.get('id')}_sync_state.json`,
                min_change_threshold: 60,
                libraries: {
                    include: this.parseCommaSeparated(formData.get('include_libraries')),
                    exclude: this.parseCommaSeparated(formData.get('exclude_libraries'))
                },
                sync_interval: formData.get('sync_interval'),
                minimum_progress: parseFloat(formData.get('minimum_progress')),
                sync_want_to_read: formData.get('sync_want_to_read') === 'on',
                process_unread_books: formData.get('process_unread_books') === 'on',
                sync_owned: formData.get('sync_owned') === 'on',
                include_ebooks: formData.get('include_ebooks') === 'on',
                dry_run: false,
                test_book_filter: '',
                test_book_limit: 0
            }
        };

        try {
            this.showLoading();
            const response = await fetch('/api/profiles', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(profileData)
            });

            const data = await response.json();

            if (data.success) {
                this.showToast('Profile created successfully!', 'success');
                event.target.reset();
                this.loadProfiles();
                this.showTab('profiles');
            } else {
                this.showToast('Failed to create profile: ' + data.error, 'error');
            }
        } catch (error) {
            this.showToast('Error creating profile: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }

    async editProfile(profileId) {
        try {
            this.showLoading();
            
            // Check authentication status first
            if (this.authEnabled && !this.currentUser) {
                this.showToast('Please log in to edit profiles', 'error');
                this.redirectToLogin();
                return;
            }
            
            const response = await fetch(`/api/profiles/${profileId}`, {
                method: 'GET',
                credentials: 'include', // Include session cookies
                headers: {
                    'Content-Type': 'application/json'
                }
            });
            
            // Handle authentication errors specifically
            if (response.status === 401 || response.status === 403) {
                this.showToast('Authentication required. Please log in.', 'error');
                this.redirectToLogin();
                return;
            }
            
            const data = await response.json();

            if (response.ok && data.success) {
                this.currentEditUser = data.data;
                this.showEditModal();
            } else {
                // Handle different types of errors
                if (data.error && data.error.code === 'authentication_required') {
                    this.showToast('Authentication required. Please log in.', 'error');
                    this.redirectToLogin();
                } else if (response.status === 400) {
                    this.showToast('Invalid profile ID: ' + profileId, 'error');
                } else {
                    this.showToast('Failed to load profile data: ' + (data.error?.message || data.error || 'Unknown error'), 'error');
                }
            }
        } catch (error) {
            this.showToast('Error loading profile data: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }

    showEditModal() {
        const user = this.currentEditUser;
        const config = user.sync_config || {};
        
        // Basic user fields - use correct data structure from ProfileWithTokens
        document.getElementById('edit-user-id').value = user.profile.id;
        document.getElementById('edit-user-name').value = user.profile.name;
        document.getElementById('edit-abs-url').value = user.audiobookshelf_url;
        
        // Sync configuration fields
        document.getElementById('edit-incremental').checked = this.toBool(config.incremental, false);
        document.getElementById('edit-sync-interval').value = config.sync_interval || '';
        document.getElementById('edit-minimum-progress').value = config.minimum_progress || 0.01;
        document.getElementById('edit-process-unread-books').checked = this.toBool(config.process_unread_books, true);
        document.getElementById('edit-sync-want-to-read').checked = this.toBool(config.sync_want_to_read, true);
        document.getElementById('edit-sync-owned').checked = this.toBool(config.sync_owned, true);
        
        // Update sync want to read state based on process unread setting
        handleProcessUnreadChange();
        
        const includeEbooksEl = document.getElementById('edit-include-ebooks');
        if (includeEbooksEl) {
            includeEbooksEl.checked = this.toBool(config.include_ebooks, false);
        }
        
        // Library filter mode and selections - support both old and new format
        const libraries = config.libraries || {};
        const libraryFilterMode = config.library_filter_mode || '';
        const filteredLibraries = config.filtered_libraries || [];
        
        // Migrate from old format if needed
        if (!libraryFilterMode && (libraries.include?.length > 0 || libraries.exclude?.length > 0)) {
            // Old format - convert to new
            if (libraries.include?.length > 0) {
                this.pendingLibraryFilterMode = 'include';
                this.pendingFilteredLibraries = libraries.include;
            } else if (libraries.exclude?.length > 0) {
                this.pendingLibraryFilterMode = 'exclude';
                this.pendingFilteredLibraries = libraries.exclude;
            }
        } else {
            this.pendingLibraryFilterMode = libraryFilterMode;
            this.pendingFilteredLibraries = filteredLibraries;
        }
        
        // Set library filter mode dropdown
        const libraryFilterModeEl = document.getElementById('edit-library-filter-mode');
        if (libraryFilterModeEl) {
            libraryFilterModeEl.value = this.pendingLibraryFilterMode || '';
            this.handleLibraryFilterModeChange();
        }
        
        // Collection filters for ABS data collection
        this.pendingIncludeCollections = config.include_collections || [];
        this.pendingExcludeCollections = config.exclude_collections || [];
        
        // Load libraries and collections for the profile
        this.loadLibrariesForProfile(user.profile.id);
        this.loadDataCollectionCollections(user.profile.id);
        
        // Sync mode settings
        const syncModeEl = document.getElementById('edit-sync-mode');
        if (syncModeEl) {
            syncModeEl.value = config.sync_mode || 'needs_sync';
            // Load collections if in collections mode
            this.handleSyncModeChange();
            // Set selected collections after loading
            if (config.selected_collections && config.selected_collections.length > 0) {
                this.pendingSelectedCollections = config.selected_collections;
            }
        }
        
        document.getElementById('edit-user-modal').style.display = 'block';
    }

    async loadLibrariesForProfile(profileId) {
        const librariesSelect = document.getElementById('edit-filtered-libraries');
        
        if (!librariesSelect) return;
        
        try {
            librariesSelect.innerHTML = '<option value="">Loading...</option>';
            
            const response = await fetch(`/api/profiles/${profileId}/abs/libraries`, {
                method: 'GET',
                credentials: 'include',
                headers: { 'Content-Type': 'application/json' }
            });
            
            if (!response.ok) {
                throw new Error('Failed to load libraries');
            }
            
            const data = await response.json();
            if (data.success && data.data && data.data.libraries) {
                librariesSelect.innerHTML = '';
                
                data.data.libraries.forEach(lib => {
                    const opt = document.createElement('option');
                    opt.value = lib.name;
                    opt.textContent = lib.name;
                    if (this.pendingFilteredLibraries && this.pendingFilteredLibraries.includes(lib.name)) {
                        opt.selected = true;
                    }
                    librariesSelect.appendChild(opt);
                });
                
                this.pendingFilteredLibraries = null;
            } else {
                librariesSelect.innerHTML = '<option value="">No libraries found</option>';
            }
        } catch (error) {
            console.error('Error loading libraries:', error);
            librariesSelect.innerHTML = '<option value="">Error loading</option>';
        }
    }

    handleLibraryFilterModeChange() {
        const modeEl = document.getElementById('edit-library-filter-mode');
        const selectGroup = document.getElementById('library-select-group');
        const selectLabel = document.getElementById('library-select-label');
        
        if (!modeEl || !selectGroup) return;
        
        if (modeEl.value === '') {
            selectGroup.style.display = 'none';
        } else {
            selectGroup.style.display = 'block';
            if (selectLabel) {
                selectLabel.textContent = modeEl.value === 'include' 
                    ? 'Libraries to Include:' 
                    : 'Libraries to Exclude:';
            }
        }
    }

    async loadDataCollectionCollections(profileId) {
        const includeSelect = document.getElementById('edit-include-collections');
        const excludeSelect = document.getElementById('edit-exclude-collections');
        
        if (!includeSelect || !excludeSelect) return;
        
        try {
            includeSelect.innerHTML = '<option value="">Loading...</option>';
            excludeSelect.innerHTML = '<option value="">Loading...</option>';
            
            const response = await fetch(`/api/profiles/${profileId}/abs/collections`, {
                method: 'GET',
                credentials: 'include',
                headers: { 'Content-Type': 'application/json' }
            });
            
            if (!response.ok) {
                throw new Error('Failed to load collections');
            }
            
            const data = await response.json();
            if (data.success && data.data && data.data.collections) {
                includeSelect.innerHTML = '';
                excludeSelect.innerHTML = '';
                
                data.data.collections.forEach(col => {
                    const bookCount = col.book_ids ? col.book_ids.length : 0;
                    const displayText = `${col.name} (${bookCount} books)`;
                    
                    // Include collections select
                    const includeOpt = document.createElement('option');
                    includeOpt.value = col.id;
                    includeOpt.textContent = displayText;
                    if (this.pendingIncludeCollections && this.pendingIncludeCollections.includes(col.id)) {
                        includeOpt.selected = true;
                    }
                    includeSelect.appendChild(includeOpt);
                    
                    // Exclude collections select
                    const excludeOpt = document.createElement('option');
                    excludeOpt.value = col.id;
                    excludeOpt.textContent = displayText;
                    if (this.pendingExcludeCollections && this.pendingExcludeCollections.includes(col.id)) {
                        excludeOpt.selected = true;
                    }
                    excludeSelect.appendChild(excludeOpt);
                });
                
                this.pendingIncludeCollections = null;
                this.pendingExcludeCollections = null;
            } else {
                includeSelect.innerHTML = '<option value="">No collections found</option>';
                excludeSelect.innerHTML = '<option value="">No collections found</option>';
            }
        } catch (error) {
            console.error('Error loading collections:', error);
            includeSelect.innerHTML = '<option value="">Error loading</option>';
            excludeSelect.innerHTML = '<option value="">Error loading</option>';
        }
    }

    async handleSyncModeChange() {
        const syncModeEl = document.getElementById('edit-sync-mode');
        const collectionsGroup = document.getElementById('collections-select-group');
        const collectionsSelect = document.getElementById('edit-selected-collections');
        
        if (!syncModeEl || !collectionsGroup) return;
        
        if (syncModeEl.value === 'collections') {
            collectionsGroup.style.display = 'block';
            // Load collections for current profile
            const profileId = document.getElementById('edit-user-id').value;
            if (profileId) {
                await this.loadCollectionsForProfile(profileId, collectionsSelect);
            }
        } else {
            collectionsGroup.style.display = 'none';
        }
    }

    async loadCollectionsForProfile(profileId, selectEl) {
        try {
            selectEl.innerHTML = '<option value="">Loading collections...</option>';
            
            const response = await fetch(`/api/profiles/${profileId}/abs/collections`, {
                method: 'GET',
                credentials: 'include',
                headers: { 'Content-Type': 'application/json' }
            });
            
            if (!response.ok) {
                throw new Error('Failed to load collections');
            }
            
            const data = await response.json();
            if (data.success && data.data && data.data.collections) {
                selectEl.innerHTML = '';
                data.data.collections.forEach(col => {
                    const option = document.createElement('option');
                    option.value = col.id;
                    const bookCount = col.book_ids ? col.book_ids.length : 0;
                    option.textContent = `${col.name} (${bookCount} books)`;
                    selectEl.appendChild(option);
                });
                
                // Apply pending selections if any
                if (this.pendingSelectedCollections && this.pendingSelectedCollections.length > 0) {
                    Array.from(selectEl.options).forEach(opt => {
                        if (this.pendingSelectedCollections.includes(opt.value)) {
                            opt.selected = true;
                        }
                    });
                    this.pendingSelectedCollections = null;
                }
                
                if (selectEl.options.length === 0) {
                    selectEl.innerHTML = '<option value="">No collections found</option>';
                }
            } else {
                selectEl.innerHTML = '<option value="">No collections found</option>';
            }
        } catch (error) {
            console.error('Error loading collections:', error);
            selectEl.innerHTML = '<option value="">Error loading collections</option>';
        }
    }

    closeEditModal() {
        const modal = document.getElementById('edit-user-modal');
        if (modal) {
            modal.style.display = 'none';
        }
        // Ensure loading overlay is hidden when modal is closed
        this.hideLoading();
        this.currentEditUser = null;
    }

    async handleEditProfile(event) {
        const formData = new FormData(event.target);
        const userId = formData.get('id');
        
        // Update user name
        const userUpdateData = {
            name: formData.get('name')
        };

        // Get selected collections for sync mode
        const selectedCollectionsEl = document.getElementById('edit-selected-collections');
        const selectedCollections = selectedCollectionsEl ? 
            Array.from(selectedCollectionsEl.selectedOptions).map(opt => opt.value) : [];

        // Get library filter mode and selected libraries
        const libraryFilterMode = formData.get('library_filter_mode') || '';
        const filteredLibrariesEl = document.getElementById('edit-filtered-libraries');
        const filteredLibraries = filteredLibrariesEl ? 
            Array.from(filteredLibrariesEl.selectedOptions).map(opt => opt.value).filter(v => v) : [];

        // Get collection filters for data collection
        const includeCollectionsEl = document.getElementById('edit-include-collections');
        const excludeCollectionsEl = document.getElementById('edit-exclude-collections');
        const includeCollections = includeCollectionsEl ? 
            Array.from(includeCollectionsEl.selectedOptions).map(opt => opt.value).filter(v => v) : [];
        const excludeCollections = excludeCollectionsEl ? 
            Array.from(excludeCollectionsEl.selectedOptions).map(opt => opt.value).filter(v => v) : [];

        // Build library config - support both old and new format for compatibility
        const libraryConfig = { include: [], exclude: [] };
        if (libraryFilterMode === 'include') {
            libraryConfig.include = filteredLibraries;
        } else if (libraryFilterMode === 'exclude') {
            libraryConfig.exclude = filteredLibraries;
        }

        // Update user config with form data
        const configUpdateData = {
            audiobookshelf_url: formData.get('audiobookshelf_url'),
            audiobookshelf_token: formData.get('audiobookshelf_token') || this.currentEditUser.audiobookshelf_token,
            hardcover_token: formData.get('hardcover_token') || this.currentEditUser.hardcover_token,
            sync_config: {
                incremental: formData.get('incremental') === 'on',
                state_file: `./data/${userId}_sync_state.json`,
                min_change_threshold: 60,
                // Old format for backward compatibility
                libraries: libraryConfig,
                // New format
                library_filter_mode: libraryFilterMode,
                filtered_libraries: filteredLibraries,
                sync_interval: formData.get('sync_interval'),
                minimum_progress: parseFloat(formData.get('minimum_progress')),
                sync_want_to_read: formData.get('sync_want_to_read') === 'on',
                process_unread_books: formData.get('process_unread_books') === 'on',
                sync_owned: formData.get('sync_owned') === 'on',
                include_ebooks: formData.get('include_ebooks') === 'on',
                sync_mode: formData.get('sync_mode') || 'needs_sync',
                selected_collections: selectedCollections,
                // Collection filters for data collection
                include_collections: includeCollections,
                exclude_collections: excludeCollections,
                dry_run: false,
                test_book_filter: '',
                test_book_limit: 0
            }
        };

        try {
            this.showLoading();
            
            // Update user
            const userResponse = await fetch(`/api/profiles/${userId}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(userUpdateData)
            });

            const userData = await userResponse.json();
            if (!userData.success) {
                throw new Error(userData.error);
            }

            // Update config
            const configResponse = await fetch(`/api/profiles/${userId}/config`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(configUpdateData)
            });

            const configData = await configResponse.json();
            if (!configData.success) {
                throw new Error(configData.error);
            }

            this.showToast('Profile updated successfully!', 'success');
            this.closeEditModal();
            this.loadProfiles();
        } catch (error) {
            this.showToast('Error updating profile: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }

    async deleteProfile(profileId) {
        if (!confirm('Are you sure you want to delete this sync profile? This action cannot be undone.')) {
            return;
        }

        try {
            this.showLoading();
            const response = await fetch(`/api/profiles/${profileId}`, {
                method: 'DELETE'
            });

            const data = await response.json();

            if (data.success) {
                this.showToast('Profile deleted successfully!', 'success');
                this.loadProfiles();
                this.loadStatuses();
            } else {
                this.showToast('Failed to delete profile: ' + (data.error || 'Unknown error'), 'error');
            }
        } catch (error) {
            this.showToast('Error deleting profile: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }

    async purgeProfileData(profileId) {
        const confirmMsg = 'Are you sure you want to purge ALL data for this profile?\n\n' +
            'This will delete:\n' +
            '• All AudiobookShelf book records\n' +
            '• All Hardcover book records\n' +
            '• All book mappings\n' +
            '• All progress history\n' +
            '• All sync events and logs\n\n' +
            'The profile configuration will be kept. This allows you to start fresh with a clean sync.\n\n' +
            'This action cannot be undone.';
        
        if (!confirm(confirmMsg)) {
            return;
        }

        try {
            this.showLoading();
            const response = await fetch(`/api/profiles/${profileId}/purge`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            const data = await response.json();

            if (data.success) {
                const deleted = data.data?.deleted || {};
                const totalDeleted = (deleted.abs_books || 0) + 
                                   (deleted.hardcover_books || 0) + 
                                   (deleted.book_mappings || 0) +
                                   (deleted.progress_history || 0) +
                                   (deleted.sync_events || 0) +
                                   (deleted.book_sync_logs || 0) +
                                   (deleted.book_sync_configs || 0);
                
                this.showToast(`Profile data purged successfully! ${totalDeleted} records deleted.`, 'success');
                this.loadProfiles();
                this.loadStatuses();
            } else {
                this.showToast('Failed to purge profile data: ' + (data.error || 'Unknown error'), 'error');
            }
        } catch (error) {
            this.showToast('Error purging profile data: ' + error.message, 'error');
        } finally {
            this.hideLoading();
        }
    }

    async startSync(profileId) {
        if (!profileId) {
            console.error('No profile ID provided for sync');
            this.showToast('Error: No profile ID provided', 'error');
            return;
        }

        try {
            this.showLoading();
            const response = await fetch(`/api/profiles/${profileId}/sync`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            const result = await response.json();
            
            if (response.ok) {
                this.showToast('Sync started successfully', 'success');
                // Update the specific profile status
                if (result.data) {
                    this.statuses[profileId] = {
                        ...result.data,
                        profile_id: profileId,
                        profile_name: this.statuses[profileId]?.profile_name || profileId
                    };
                    this.renderStatuses();
                } else {
                    // If no data in response, refresh all statuses
                    await this.loadStatuses();
                }
            } else {
                throw new Error(result.error || 'Failed to start sync');
            }
        } catch (error) {
            console.error('Error starting sync:', error);
            this.showToast(`Error: ${error.message}`, 'error');
            
            // Update UI to show error state
            if (profileId && this.statuses[profileId]) {
                this.statuses[profileId].status = 'error';
                this.statuses[profileId].error = error.message;
                this.renderStatuses();
            }
        } finally {
            this.hideLoading();
        }
    }

    async cancelSync(profileId) {
        if (!confirm('Are you sure you want to cancel the sync?')) {
            return;
        }

        if (!profileId) {
            console.error('No profile ID provided for cancel');
            this.showToast('Error: No profile ID provided', 'error');
            return;
        }

        try {
            this.showLoading();
            const response = await fetch(`/api/profiles/${profileId}/sync`, {
                method: 'DELETE'
            });

            const result = await response.json();
            
            if (response.ok) {
                this.showToast('Sync cancelled', 'info');
                // Update the specific profile status
                if (result.data) {
                    this.statuses[profileId] = {
                        ...result.data,
                        profile_id: profileId,
                        profile_name: this.statuses[profileId]?.profile_name || profileId,
                        status: 'cancelled'
                    };
                    this.renderStatuses();
                } else {
                    // If no data in response, refresh all statuses
                    await this.loadStatuses();
                }
            } else {
                throw new Error(result.error || 'Failed to cancel sync');
            }
        } catch (error) {
            console.error('Error cancelling sync:', error);
            this.showToast(`Error: ${error.message}`, 'error');
            
            // Update UI to show error state
            if (profileId && this.statuses[profileId]) {
                this.statuses[profileId].status = 'error';
                this.statuses[profileId].error = error.message;
                this.renderStatuses();
            }
        } finally {
            this.hideLoading();
        }
    }

    startAutoRefresh() {
        // Refresh statuses every 30 seconds when on library tab
        this.refreshInterval = setInterval(() => {
            const libraryTab = document.getElementById('library-tab');
            if (libraryTab && libraryTab.classList.contains('active')) {
                // Could refresh library stats here if needed
            }
        }, 30000);
    }

    stopAutoRefresh() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }

    showLoading() {
        const overlay = document.getElementById('loading-overlay');
        if (overlay) {
            overlay.classList.add('active');
            // Ensure the overlay is visible by setting display to flex
            overlay.style.display = 'flex';
        }
    }

    hideLoading() {
        const overlay = document.getElementById('loading-overlay');
        if (overlay) {
            overlay.classList.remove('active');
            // Hide the overlay completely after a short delay to allow for fade-out
            setTimeout(() => {
                if (overlay && !overlay.classList.contains('active')) {
                    overlay.style.display = 'none';
                }
            }, 300); // Match this with the CSS transition time
        }
    }

    showToast(message, type = 'info') {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `
            ${this.escapeHtml(message)}
            <button class="toast-close" onclick="this.parentElement.remove()">&times;</button>
        `;

        container.appendChild(toast);

        // Auto-remove after 5 seconds
        setTimeout(() => {
            if (toast.parentElement) {
                toast.remove();
            }
        }, 5000);
    }

    parseCommaSeparated(value) {
        if (!value || value.trim() === '') {
            return [];
        }
        return value.split(',').map(item => item.trim()).filter(item => item.length > 0);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Global functions for HTML onclick handlers
function showTab(tabName) {
    app.showTab(tabName);
}

function refreshUsers() {
    app.loadProfiles();
}

function refreshStatus() {
    app.loadStatuses();
}

// Toggle the inline Add Profile form
function toggleAddProfileForm() {
    const section = document.getElementById('add-profile-section');
    if (section.style.display === 'none') {
        section.style.display = 'block';
    } else {
        section.style.display = 'none';
    }
}

// Handle inline add user form submission
async function handleAddUser(event) {
    event.preventDefault();
    const form = event.target;
    const formData = new FormData(form);
    
    const data = {
        id: formData.get('id'),
        name: formData.get('name'),
        audiobookshelf_url: formData.get('audiobookshelf_url'),
        audiobookshelf_token: formData.get('audiobookshelf_token'),
        hardcover_token: formData.get('hardcover_token'),
    };
    
    try {
        const response = await fetch('/api/profiles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data),
        });
        
        const result = await response.json();
        if (response.ok && result.success) {
            app.showToast('Profile created successfully!', 'success');
            form.reset();
            toggleAddProfileForm();
            app.loadProfiles();
        } else {
            app.showToast(result.error || 'Failed to create profile', 'error');
        }
    } catch (error) {
        console.error('Error creating profile:', error);
        app.showToast('Failed to create profile', 'error');
    }
}

// Populate the history tab profile dropdown
function populateHistoryProfileDropdown() {
    const profileSelect = document.getElementById('profile-select');
    if (!profileSelect || !app.users) return;
    
    const currentValue = profileSelect.value;
    profileSelect.innerHTML = '<option value="">Select a Profile...</option>';
    app.users.forEach(user => {
        const option = document.createElement('option');
        option.value = user.id;
        option.textContent = user.name || user.id;
        profileSelect.appendChild(option);
    });
    
    // Restore selection if it still exists, or auto-select if only one profile
    if (currentValue && app.users.some(u => u.id === currentValue)) {
        profileSelect.value = currentValue;
    } else if (app.users.length === 1) {
        profileSelect.value = app.users[0].id;
        loadBookSyncLogs();
    }
}

function togglePassword(inputId) {
    const input = document.getElementById(inputId);
    const button = input.nextElementSibling;
    
    if (input.type === 'password') {
        input.type = 'text';
        button.textContent = '🙈';
    } else {
        input.type = 'password';
        button.textContent = '👁️';
    }
}

function closeEditModal() {
    app.closeEditModal();
}

// Book Sync Logs Functions - Now shows sync summary and events
async function loadBookSyncLogs() {
    const profileSelect = document.getElementById('profile-select');
    const container = document.getElementById('book-logs-container');
    
    const profileId = profileSelect.value;
    
    if (!profileId) {
        container.innerHTML = '<p class="empty-state">Please select a profile</p>';
        return;
    }
    
    container.innerHTML = '<div class="empty-state"><div class="loading-spinner"></div><p>Loading sync history...</p></div>';
    
    try {
        // Fetch sync summary, sync events, logs, and schedule in parallel
        const [summaryRes, eventsRes, logsRes, scheduleRes] = await Promise.all([
            fetch(`/api/profiles/${profileId}/sync-summary`),
            fetch(`/api/profiles/${profileId}/sync-events?limit=50`),
            fetch(`/api/profiles/${profileId}/book-syncs?limit=50`),
            fetch(`/api/profiles/${profileId}/sync-schedule`)
        ]);
        
        const summaryData = await summaryRes.json();
        const eventsData = await eventsRes.json();
        const logsData = await logsRes.json();
        const scheduleData = await scheduleRes.json();
        
        displaySyncHistory(
            summaryData.success ? summaryData.data : null,
            eventsData.success ? eventsData.data?.events : [],
            logsData.success ? (logsData.data?.logs || []) : [],
            scheduleData.success ? scheduleData.data : null
        );
    } catch (error) {
        console.error('Failed to load sync history:', error);
        container.innerHTML = `<p class="empty-state text-danger">Failed to load sync history: ${error.message}</p>`;
    }
}

// renderSyncTimeline creates a visual timeline of past and future sync operations
function renderSyncTimeline(scheduleData) {
    if (!scheduleData || !scheduleData.events || scheduleData.events.length === 0) {
        return '';
    }

    const now = new Date();
    const events = scheduleData.events;
    
    // Sort events by time
    events.sort((a, b) => new Date(a.time) - new Date(b.time));
    
    // Separate past and future events
    const pastEvents = events.filter(e => e.type === 'past' && new Date(e.time) <= now);
    const futureEvents = events.filter(e => e.type === 'future' && new Date(e.time) > now);
    
    let html = `
        <div class="sync-timeline-container" style="margin-bottom: 2rem;">
            <h3 style="margin: 0 0 1rem 0; font-size: 1.1rem;">🕐 Sync Schedule Timeline</h3>
            <div class="sync-timeline-info" style="margin-bottom: 1rem; padding: 0.75rem; background: #f5f5f5; border-radius: 6px; font-size: 0.9rem;">
                <div style="display: flex; gap: 2rem; flex-wrap: wrap;">
                    <div>
                        <strong>Sync Interval:</strong> ${scheduleData.sync_interval || 'Not configured'}
                    </div>
                    <div>
                        <strong>Past Syncs:</strong> ${pastEvents.length}
                    </div>
                    <div>
                        <strong>Upcoming Syncs:</strong> ${futureEvents.length}
                    </div>
                </div>
            </div>
            <div class="sync-timeline">
    `;
    
    // Render past events (last 5)
    const recentPast = pastEvents.slice(-5).reverse();
    if (recentPast.length > 0) {
        html += '<div class="timeline-section past-events" style="margin-bottom: 1.5rem;">';
        html += '<h4 style="font-size: 0.95rem; margin: 0 0 0.75rem 0; color: #666;">Recent Syncs</h4>';
        html += '<div class="timeline-events">';
        
        recentPast.forEach(event => {
            const eventDate = new Date(event.time);
            const timeAgo = getTimeAgo(eventDate);
            const statusIcon = event.status === 'error' ? '❌' : '✅';
            const statusClass = event.status === 'error' ? 'error' : 'success';
            
            html += `
                <div class="timeline-event ${statusClass}" style="display: flex; align-items: center; padding: 0.75rem; margin-bottom: 0.5rem; background: white; border-left: 4px solid ${event.status === 'error' ? '#f44336' : '#4CAF50'}; border-radius: 4px; box-shadow: 0 1px 3px rgba(0,0,0,0.1);">
                    <div style="font-size: 1.5rem; margin-right: 1rem;">${statusIcon}</div>
                    <div style="flex: 1;">
                        <div style="font-weight: 500;">${event.description}</div>
                        <div style="font-size: 0.85rem; color: #666; margin-top: 0.25rem;">
                            ${eventDate.toLocaleString()} (${timeAgo})
                        </div>
                    </div>
                </div>
            `;
        });
        
        html += '</div></div>';
    }
    
    // Current time marker
    html += `
        <div class="timeline-now" style="display: flex; align-items: center; margin: 1rem 0; padding: 0.5rem 0;">
            <div style="flex: 1; height: 2px; background: linear-gradient(to right, #2196F3, transparent);"></div>
            <div style="padding: 0.25rem 1rem; background: #2196F3; color: white; border-radius: 20px; font-size: 0.85rem; font-weight: 600; white-space: nowrap;">
                ⏰ NOW
            </div>
            <div style="flex: 1; height: 2px; background: linear-gradient(to left, #2196F3, transparent);"></div>
        </div>
    `;
    
    // Render future events (next 5)
    const upcomingFuture = futureEvents.slice(0, 5);
    if (upcomingFuture.length > 0) {
        html += '<div class="timeline-section future-events">';
        html += '<h4 style="font-size: 0.95rem; margin: 0 0 0.75rem 0; color: #666;">Upcoming Syncs</h4>';
        html += '<div class="timeline-events">';
        
        upcomingFuture.forEach((event, index) => {
            const eventDate = new Date(event.time);
            const timeUntil = getTimeUntil(eventDate);
            const isNext = index === 0;
            
            html += `
                <div class="timeline-event future" style="display: flex; align-items: center; padding: 0.75rem; margin-bottom: 0.5rem; background: ${isNext ? '#E3F2FD' : 'white'}; border-left: 4px solid ${isNext ? '#2196F3' : '#9E9E9E'}; border-radius: 4px; box-shadow: 0 1px 3px rgba(0,0,0,0.1);">
                    <div style="font-size: 1.5rem; margin-right: 1rem;">📅</div>
                    <div style="flex: 1;">
                        <div style="font-weight: 500;">${isNext ? '⭐ ' : ''}${event.description}</div>
                        <div style="font-size: 0.85rem; color: #666; margin-top: 0.25rem;">
                            ${eventDate.toLocaleString()} (${timeUntil})
                        </div>
                    </div>
                </div>
            `;
        });
        
        html += '</div></div>';
    }
    
    html += '</div></div>';
    return html;
}

// Helper function to get "X ago" string
function getTimeAgo(date) {
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    
    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins} min${diffMins !== 1 ? 's' : ''} ago`;
    if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
    if (diffDays < 7) return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;
    return `${Math.floor(diffDays / 7)} week${Math.floor(diffDays / 7) !== 1 ? 's' : ''} ago`;
}

// Helper function to get "in X" string
function getTimeUntil(date) {
    const now = new Date();
    const diffMs = date - now;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    
    if (diffMins < 1) return 'imminently';
    if (diffMins < 60) return `in ${diffMins} min${diffMins !== 1 ? 's' : ''}`;
    if (diffHours < 24) return `in ${diffHours} hour${diffHours !== 1 ? 's' : ''}`;
    if (diffDays < 7) return `in ${diffDays} day${diffDays !== 1 ? 's' : ''}`;
    return `in ${Math.floor(diffDays / 7)} week${Math.floor(diffDays / 7) !== 1 ? 's' : ''}`;
}

function displaySyncHistory(summary, events, logs, scheduleData) {
    const container = document.getElementById('book-logs-container');
    let html = '';
    
    // Sync Schedule Timeline section
    if (scheduleData && scheduleData.events && scheduleData.events.length > 0) {
        html += renderSyncTimeline(scheduleData);
    }
    
    // Summary section
    if (summary) {
        const syncPct = summary.mapped_books > 0 ? Math.round((summary.in_sync_books / summary.mapped_books) * 100) : 0;
        html += `
            <div class="library-summary">
                <h3>📊 Library Summary</h3>
                <div class="summary-grid">
                    <div class="summary-card info">
                        <div class="value">${summary.total_abs_books}</div>
                        <div class="label">Total Books</div>
                    </div>
                    <div class="summary-card success">
                        <div class="value">${summary.mapped_books}</div>
                        <div class="label">Mapped</div>
                    </div>
                    <div class="summary-card danger">
                        <div class="value">${summary.unmapped_books}</div>
                        <div class="label">Unmapped</div>
                    </div>
                    <div class="summary-card success">
                        <div class="value">${summary.in_sync_books}</div>
                        <div class="label">In Sync</div>
                    </div>
                    <div class="summary-card warning">
                        <div class="value">${summary.needs_sync_books}</div>
                        <div class="label">Needs Sync</div>
                    </div>
                    <div class="summary-card muted">
                        <div class="value">${summary.sync_disabled_books}</div>
                        <div class="label">Sync Disabled</div>
                    </div>
                </div>
            </div>
        `;
    }
    
    // Sync Events section
    if (events && events.length > 0) {
        html += `
            <div class="mb-2">
                <h3 class="text-primary" style="margin: 0 0 1rem 0; font-size: 1.1rem;">📋 Recent Sync Events</h3>
                <table class="data-table">
                    <thead>
                        <tr>
                            <th>Event</th>
                            <th>Trigger</th>
                            <th class="text-center">Progress</th>
                            <th>Time</th>
                            <th>Details</th>
                        </tr>
                    </thead>
                    <tbody>
        `;
        
        const eventTypeColors = {
            'progress_update': '#4CAF50',
            'status_change': '#2196F3',
            'skipped': '#FF9800',
            'error': '#f44336',
            'not_found': '#9E9E9E',
            'manual_sync': '#9C27B0'
        };
        
        events.forEach(event => {
            const eventColor = eventTypeColors[event.event_type] || '#666';
            const progressDisplay = event.new_progress != null ? `${(event.new_progress * 100).toFixed(1)}%` : '-';
            const timeDisplay = event.created_at ? new Date(event.created_at).toLocaleString() : '-';
            const detailsDisplay = event.details ? (event.details.length > 40 ? event.details.substring(0, 37) + '...' : event.details) : (event.error_message || '-');
            
            html += `
                <tr>
                    <td>
                        <span class="status-badge" style="background: ${eventColor};">
                            ${event.event_type || 'unknown'}
                        </span>
                    </td>
                    <td class="text-small">${event.trigger || '-'}</td>
                    <td class="text-center text-small">${progressDisplay}</td>
                    <td class="text-small text-muted">${timeDisplay}</td>
                    <td class="text-small text-muted" title="${escapeHtml(event.details || event.error_message || '')}">${escapeHtml(detailsDisplay)}</td>
                </tr>
            `;
        });
        
        html += '</tbody></table></div>';
    }
    
    // Book Sync Logs section (fallback/legacy)
    if (logs && logs.length > 0) {
        html += `
            <div>
                <h3 class="text-primary" style="margin: 0 0 1rem 0; font-size: 1.1rem;">📚 Book Sync Logs</h3>
        `;
        html += displayBookSyncLogsTable(logs);
        html += '</div>';
    }
    
    // Show empty state if no data
    if (!summary && (!events || events.length === 0) && (!logs || logs.length === 0)) {
        html = `
            <div class="empty-state">
                <div class="icon">📭</div>
                <h3>No Sync History Yet</h3>
                <p>Go to the <strong>Library</strong> tab to collect books and sync progress.</p>
            </div>
        `;
    }
    
    container.innerHTML = html;
}

function displayBookSyncLogsTable(logs) {
    const statusColorMap = {
        'SYNCED': '#4caf50',
        'SKIPPED': '#ff9800',
        'ERROR': '#d32f2f',
        'NOT_FOUND': '#2196f3',
        'PENDING': '#9c27b0'
    };
    
    let html = '<table class="data-table">';
    html += `<thead>
        <tr>
            <th>Title</th>
            <th>Author</th>
            <th class="text-center">Status</th>
            <th class="text-right">Progress</th>
            <th>Last Attempt</th>
        </tr>
    </thead>
    <tbody>`;
    
    logs.forEach(log => {
        const status = log.status || 'UNKNOWN';
        const statusColor = statusColorMap[status] || '#999';
        const progress = log.progress ? (log.progress * 100).toFixed(1) + '%' : '-';
        const lastAttempt = log.last_attempt ? new Date(log.last_attempt).toLocaleString() : 'Never';
        
        html += `<tr>
            <td>${escapeHtml(log.title || '')}</td>
            <td>${escapeHtml(log.author || '')}</td>
            <td class="text-center">
                <span class="status-badge" style="background-color: ${statusColor};">
                    ${status}
                </span>
            </td>
            <td class="text-right">${progress}</td>
            <td class="text-small text-muted">${lastAttempt}</td>
        </tr>`;
    });
    
    html += '</tbody></table>';
    return html;
}

function displayBookSyncLogs(logs) {
    const container = document.getElementById('book-logs-container');
    
    if (!logs || logs.length === 0) {
        container.innerHTML = '<p class="empty-state">No sync history found. Run a sync to see results here.</p>';
        return;
    }
    
    container.innerHTML = displayBookSyncLogsTable(logs);
}

// Legacy function for backwards compatibility
function displayBookSyncLogsLegacy(logs) {
    const container = document.getElementById('book-logs-container');
    
    if (!logs || logs.length === 0) {
        container.innerHTML = '<p class="empty-state">No sync history found. Run a sync to see results here.</p>';
        return;
    }
    
    const statusColorMap = {
        'SYNCED': '#4caf50',
        'SKIPPED': '#ff9800',
        'ERROR': '#d32f2f',
        'NOT_FOUND': '#2196f3',
        'PENDING': '#9c27b0'
    };
    
    let html = '<table class="data-table">';
    html += `<thead>
        <tr>
            <th>Title</th>
            <th>Author</th>
            <th class="text-center">Status</th>
            <th class="text-right">Progress</th>
            <th>Last Attempt</th>
            <th>Error</th>
        </tr>
    </thead>
    <tbody>`;
    
    logs.forEach(log => {
        // Use lowercase JSON field names from Go model
        const status = log.status || 'UNKNOWN';
        const statusColor = statusColorMap[status] || '#999';
        const progress = log.progress ? (log.progress * 100).toFixed(1) + '%' : '-';
        const lastAttempt = log.last_attempt ? new Date(log.last_attempt).toLocaleString() : 'Never';
        const errorMsg = log.error_message || '-';
        const errorDisplay = errorMsg.length > 50 ? errorMsg.substring(0, 47) + '...' : errorMsg;
        
        html += `<tr>
            <td>${escapeHtml(log.title || '')}</td>
            <td>${escapeHtml(log.author || '')}</td>
            <td class="text-center">
                <span class="status-badge" style="background-color: ${statusColor};">
                    ${status}
                </span>
            </td>
            <td class="text-right">${progress}</td>
            <td class="text-small text-muted">${lastAttempt}</td>
            <td class="text-small text-danger" title="${escapeHtml(errorMsg)}">${escapeHtml(errorDisplay)}</td>
        </tr>`;
    });
    
    html += '</tbody></table>';
    container.innerHTML = html;
}

function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Update profile select when users are loaded
function updateProfileSelectForBookLogs() {
    const select = document.getElementById('profile-select');
    if (!select) return;
    
    const users = app.users || [];
    
    let html = '<option value="">Select a Profile...</option>';
    users.forEach(user => {
        html += `<option value="${user.id}">${user.name || user.id}</option>`;
    });
    
    select.innerHTML = html;
    
    // Auto-select if only one profile
    if (users.length === 1) {
        select.value = users[0].id;
        loadBookSyncLogs();
    }
}

// Update library profile selector and auto-select single profile
function updateLibraryProfileSelect() {
    const select = document.getElementById('library-profile-select');
    if (!select) return;
    
    const users = app.users || [];
    
    // Preserve current selection from state or localStorage
    const savedProfile = libraryState.profileId || loadPersisted(STORAGE_KEYS.libraryProfile, '');
    
    select.innerHTML = '<option value="">Select a Profile...</option>';
    users.forEach(user => {
        const option = document.createElement('option');
        option.value = user.id;
        option.text = user.name || user.id;
        select.appendChild(option);
    });
    
    // Restore selection or auto-select if only one profile
    if (savedProfile && users.some(u => u.id === savedProfile)) {
        select.value = savedProfile;
        // Always trigger profile change to ensure data loads
        onLibraryProfileChange(savedProfile);
    } else if (users.length === 1) {
        select.value = users[0].id;
        onLibraryProfileChange(users[0].id);
    }
}

// Handle library profile change
async function onLibraryProfileChange(profileId) {
    libraryState.profileId = profileId;
    savePersisted(STORAGE_KEYS.libraryProfile, profileId);
    
    // Reset library/collection filters when profile changes
    libraryState.absLibrary = '';
    libraryState.collection = '';
    libraryState.absLibraries = [];
    libraryState.absCollections = [];
    libraryState.profileSettings = null;
    
    if (profileId) {
        // First load profile settings for filtering
        await loadProfileSettings(profileId);
        // Then load ABS libraries and collections with filtering applied
        await Promise.all([
            loadABSLibraries(profileId),
            loadABSCollections(profileId)
        ]);
    }
    
    loadLibraryBooks();
}

// Load profile settings for filtering dropdowns
async function loadProfileSettings(profileId) {
    if (!profileId) return;
    
    try {
        const response = await fetch(`/api/profiles/${profileId}`);
        const data = await response.json();
        if (data.success && data.data) {
            const config = data.data.sync_config || {};
            libraryState.profileSettings = {
                libraryFilterMode: config.library_filter_mode || '',
                filteredLibraries: config.filtered_libraries || [],
                includeCollections: config.include_collections || [],
                excludeCollections: config.exclude_collections || []
            };
        }
    } catch (error) {
        console.error('Failed to load profile settings:', error);
        libraryState.profileSettings = null;
    }
}

// Load ABS libraries for dropdown (filtered by profile settings)
async function loadABSLibraries(profileId) {
    if (!profileId) return;
    
    try {
        const response = await fetch(`/api/profiles/${profileId}/abs/libraries`);
        const data = await response.json();
        if (data.success && data.data) {
            // Handle both { data: [...] } and { data: { libraries: [...] } } formats
            let libraries = Array.isArray(data.data) ? data.data : (data.data.libraries || []);
            libraries = Array.isArray(libraries) ? libraries : [];
            
            // Filter libraries based on profile settings
            const settings = libraryState.profileSettings;
            if (settings && settings.filteredLibraries && settings.filteredLibraries.length > 0) {
                if (settings.libraryFilterMode === 'include') {
                    // Only show libraries that are included
                    libraries = libraries.filter(lib => settings.filteredLibraries.includes(lib.name));
                } else if (settings.libraryFilterMode === 'exclude') {
                    // Show all libraries except excluded ones
                    libraries = libraries.filter(lib => !settings.filteredLibraries.includes(lib.name));
                }
            }
            
            libraryState.absLibraries = libraries;
        } else {
            libraryState.absLibraries = [];
        }
    } catch (error) {
        console.error('Failed to load ABS libraries:', error);
        libraryState.absLibraries = [];
    }
}

// Load ABS collections for dropdown (filtered by profile settings)
async function loadABSCollections(profileId) {
    if (!profileId) return;
    
    try {
        const response = await fetch(`/api/profiles/${profileId}/abs/collections`);
        const data = await response.json();
        if (data.success && data.data) {
            // Handle both { data: [...] } and { data: { collections: [...] } } formats
            let collections = Array.isArray(data.data) ? data.data : (data.data.collections || []);
            collections = Array.isArray(collections) ? collections : [];
            
            // Filter collections based on profile settings
            const settings = libraryState.profileSettings;
            if (settings) {
                // If includeCollections is set, only show those collections
                if (settings.includeCollections && settings.includeCollections.length > 0) {
                    collections = collections.filter(col => settings.includeCollections.includes(col.id));
                }
                // If excludeCollections is set, hide those collections
                else if (settings.excludeCollections && settings.excludeCollections.length > 0) {
                    collections = collections.filter(col => !settings.excludeCollections.includes(col.id));
                }
            }
            
            libraryState.absCollections = collections;
        } else {
            libraryState.absCollections = [];
        }
    } catch (error) {
        console.error('Failed to load ABS collections:', error);
        libraryState.absCollections = [];
    }
}

// Initialize the app when the page loads
let app;
document.addEventListener('DOMContentLoaded', () => {
    app = new SyncProfileApp();
    app.init();
    
    // Add event delegation for read more/less functionality
    document.addEventListener('click', (e) => {
        const readMoreLink = e.target.closest('.read-more');
        if (!readMoreLink) return;
        
        e.preventDefault();
        const container = readMoreLink.closest('.description-container');
        if (!container) return;
        
        const text = container.querySelector('.description-text');
        const fullText = container.querySelector('.description-full');
        
        if (text && fullText) {
            if (fullText.classList.contains('hidden')) {
                // Show full text
                text.classList.add('hidden');
                fullText.classList.remove('hidden');
                readMoreLink.textContent = 'Read less';
                
                // Scroll the full text into view if it's near the bottom of the viewport
                const containerRect = container.getBoundingClientRect();
                const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
                
                if (containerRect.bottom > viewportHeight - 100) {
                    container.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                }
            } else {
                // Show short text
                text.classList.remove('hidden');
                fullText.classList.add('hidden');
                readMoreLink.textContent = 'Read more';
                
                // Scroll the read more link into view if it's near the bottom
                const linkRect = readMoreLink.getBoundingClientRect();
                const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
                
                if (linkRect.bottom > viewportHeight - 100) {
                    readMoreLink.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                }
            }
        }
    });
});

// Clean up on page unload
window.addEventListener('beforeunload', () => {
    if (app) {
        app.stopAutoRefresh();
    }
});
// ============================================================
// LIBRARY TAB STATE AND FUNCTIONS
// ============================================================

// Storage keys for persistence
const STORAGE_KEYS = {
    activeTab: 'abs-hc-sync-activeTab',
    libraryProfile: 'abs-hc-sync-libraryProfile',
    libraryFilter: 'abs-hc-sync-libraryFilter',
    libraryMediaType: 'abs-hc-sync-libraryMediaType',
    librarySort: 'abs-hc-sync-librarySort',
    librarySortDir: 'abs-hc-sync-librarySortDir',
    libraryABSLibrary: 'abs-hc-sync-libraryABSLibrary',
    libraryCollection: 'abs-hc-sync-libraryCollection',
    statsProfile: 'abs-hc-sync-statsProfile',
    statsFilter: 'abs-hc-sync-statsFilter',
};

// Load persisted value from localStorage
function loadPersisted(key, defaultValue = '') {
    try {
        return localStorage.getItem(key) || defaultValue;
    } catch (e) {
        return defaultValue;
    }
}

// Save value to localStorage
function savePersisted(key, value) {
    try {
        localStorage.setItem(key, value);
    } catch (e) {
        console.warn('Failed to save to localStorage:', e);
    }
}

// Global library state
let libraryState = {
    currentPage: 1,
    pageSize: 50,
    filter: loadPersisted(STORAGE_KEYS.libraryFilter, ''),
    sort: loadPersisted(STORAGE_KEYS.librarySort, 'title'),
    sortDir: loadPersisted(STORAGE_KEYS.librarySortDir, 'asc'),
    searchQuery: '',
    mediaType: loadPersisted(STORAGE_KEYS.libraryMediaType, ''),
    absLibrary: loadPersisted(STORAGE_KEYS.libraryABSLibrary, ''),
    collection: loadPersisted(STORAGE_KEYS.libraryCollection, ''),
    profileId: loadPersisted(STORAGE_KEYS.libraryProfile, ''),
    currentBookDetail: null,
    searchTimeout: null,
    absUrl: '',
    absLibraries: [],
    absCollections: [],
};

// Debounced search function
function debouncedSearch(value) {
    // Clear any pending search
    if (libraryState.searchTimeout) {
        clearTimeout(libraryState.searchTimeout);
    }
    
    // Set new timeout
    libraryState.searchTimeout = setTimeout(() => {
        libraryState.searchQuery = value;
        libraryState.currentPage = 1;
        loadLibraryBooks();
    }, 500); // 500ms debounce
}

// Handle filter dropdown changes with persistence
function onLibraryFilterChange(field, value) {
    libraryState[field] = value;
    libraryState.currentPage = 1;
    
    // Save to localStorage
    const storageKeyMap = {
        filter: STORAGE_KEYS.libraryFilter,
        mediaType: STORAGE_KEYS.libraryMediaType,
        absLibrary: STORAGE_KEYS.libraryABSLibrary,
        collection: STORAGE_KEYS.libraryCollection,
    };
    if (storageKeyMap[field]) {
        savePersisted(storageKeyMap[field], value);
    }
    
    loadLibraryBooks();
}

// Toggle sort column and direction
function toggleSort(column) {
    if (libraryState.sort === column) {
        // Toggle direction
        libraryState.sortDir = libraryState.sortDir === 'asc' ? 'desc' : 'asc';
    } else {
        // New column, default to ascending for title/author, descending for diff/updated
        libraryState.sort = column;
        libraryState.sortDir = (column === 'title' || column === 'author') ? 'asc' : 'desc';
    }
    
    // Save to localStorage
    savePersisted(STORAGE_KEYS.librarySort, libraryState.sort);
    savePersisted(STORAGE_KEYS.librarySortDir, libraryState.sortDir);
    
    loadLibraryBooks();
}

// Format last updated timestamp
function formatLastUpdated(timestamp) {
    if (!timestamp) return '-';
    try {
        const date = new Date(timestamp);
        const now = new Date();
        const diffMs = now - date;
        const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
        
        if (diffDays === 0) {
            // Today - show time
            return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
        } else if (diffDays === 1) {
            return 'Yesterday';
        } else if (diffDays < 7) {
            return `${diffDays}d ago`;
        } else {
            // Show date
            return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
        }
    } catch (e) {
        return '-';
    }
}

// Load library books with pagination and filtering
async function loadLibraryBooks() {
    if (!libraryState.profileId) {
        document.getElementById('library-inner').innerHTML = '<p class="empty-state">Please select a profile from the dropdown above.</p>';
        return;
    }

    // Remember if search input had focus
    const searchInput = document.getElementById('library-search');
    const hadFocus = searchInput && document.activeElement === searchInput;
    const cursorPosition = hadFocus ? searchInput.selectionStart : 0;

    try {
        const params = new URLSearchParams({
            page: libraryState.currentPage,
            limit: libraryState.pageSize,
            filter: libraryState.filter,
            sort: libraryState.sort,
            sort_dir: libraryState.sortDir || 'asc',
            search: libraryState.searchQuery,
        });
        if (libraryState.mediaType) {
            params.set('media_type', libraryState.mediaType);
        }
        if (libraryState.absLibrary) {
            params.set('library_id', libraryState.absLibrary);
        }
        if (libraryState.collection) {
            params.set('collection_id', libraryState.collection);
        }

        const response = await fetch(`/api/profiles/${libraryState.profileId}/books?${params}`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        
        const data = await response.json();
        if (data.success) {
            // Store ABS URL for linking
            libraryState.absUrl = data.abs_url || '';
            renderLibraryBooks(data.data, data.pagination);
            await loadSyncSummary();
            
            // Restore focus and cursor position if search was active
            if (hadFocus) {
                const newSearchInput = document.getElementById('library-search');
                if (newSearchInput) {
                    newSearchInput.focus();
                    newSearchInput.setSelectionRange(cursorPosition, cursorPosition);
                }
            }
        } else {
            app.showToast(data.error || 'Failed to load books', 'error');
        }
    } catch (error) {
        console.error('Error loading library books:', error);
        app.showToast('Failed to load books', 'error');
    }
}

// Render books table and controls
function renderLibraryBooks(books, pagination) {
    console.log('renderLibraryBooks called with:', { books, pagination });
    console.log('Books is array:', Array.isArray(books));
    console.log('Books length:', books ? books.length : 'null/undefined');
    
    let html = `
        <div class="mb-2">
            <div class="stats-row" id="library-stats">
                <!-- Stats will be loaded by loadSyncSummary() -->
            </div>

            <div class="library-controls">
                <div class="library-controls-row search-row">
                    <input type="text" id="library-search" placeholder="Search by title or author..." 
                        oninput="debouncedSearch(this.value)"
                        value="${escapeHtml(libraryState.searchQuery)}"
                        class="form-input search-input">
                </div>
                
                <div class="library-controls-row filters-row">
                    <select id="library-filter" onchange="onLibraryFilterChange('filter', this.value)" class="form-select">
                        <option value="" ${libraryState.filter === '' ? 'selected' : ''}>All Books</option>
                        <option value="in_sync" ${libraryState.filter === 'in_sync' ? 'selected' : ''}>In Sync</option>
                        <option value="needs_sync" ${libraryState.filter === 'needs_sync' ? 'selected' : ''}>Needs Sync</option>
                        <option value="unmapped" ${libraryState.filter === 'unmapped' ? 'selected' : ''}>Unmapped</option>
                        <option value="disabled" ${libraryState.filter === 'disabled' ? 'selected' : ''}>Disabled</option>
                    </select>

                    <select id="library-media-type" onchange="onLibraryFilterChange('mediaType', this.value)" class="form-select">
                        <option value="" ${(libraryState.mediaType || '') === '' ? 'selected' : ''}>All Types</option>
                        <option value="audiobook" ${libraryState.mediaType === 'audiobook' ? 'selected' : ''}>🎧 Audiobooks</option>
                        <option value="ebook" ${libraryState.mediaType === 'ebook' ? 'selected' : ''}>📱 eBooks</option>
                    </select>

                    <select id="library-abs-library" onchange="onLibraryFilterChange('absLibrary', this.value)" class="form-select">
                        <option value="">All Libraries</option>
                        ${(libraryState.absLibraries || []).map(lib => 
                            `<option value="${escapeHtml(lib.id)}" ${libraryState.absLibrary === lib.id ? 'selected' : ''}>${escapeHtml(lib.name)}</option>`
                        ).join('')}
                    </select>

                    <select id="library-collection" onchange="onLibraryFilterChange('collection', this.value)" class="form-select">
                        <option value="">All Collections</option>
                        ${(libraryState.absCollections || []).map(col => 
                            `<option value="${escapeHtml(col.id)}" ${libraryState.collection === col.id ? 'selected' : ''}>${escapeHtml(col.name)}</option>`
                        ).join('')}
                    </select>
                </div>

                <div class="library-controls-row buttons-row">
                    <button class="btn btn-primary" onclick="collectABSBooks()">📥 Collect ABS</button>
                    <button class="btn btn-primary" onclick="collectHardcoverBooks()">📥 Collect HC</button>
                    <button class="btn btn-secondary" onclick="autoMatchBooks()">🔗 Auto-Match</button>
                </div>
            </div>

            <div class="table-responsive">
                <table class="data-table">
                    <thead>
                        <tr>
                            <th class="text-center" title="Media Type">Type</th>
                            <th class="sortable ${libraryState.sort === 'title' ? 'sort-' + libraryState.sortDir : ''}" 
                                onclick="toggleSort('title')" title="Sort by title">Title</th>
                            <th class="sortable ${libraryState.sort === 'author' ? 'sort-' + libraryState.sortDir : ''}" 
                                onclick="toggleSort('author')" title="Sort by author">Author</th>
                            <th>Narrator</th>
                            <th class="text-center" title="AudiobookShelf Progress"><img src="https://www.audiobookshelf.org/Logo.png" alt="ABS"></th>
                            <th class="text-center" title="Sync Status (click to sync)">🔄</th>
                            <th class="text-center" title="Hardcover Book">📚</th>
                            <th class="text-center" title="Hardcover Edition (click to change)">📖</th>
                            <th class="text-center sortable ${libraryState.sort === 'progress_diff' ? 'sort-' + libraryState.sortDir : ''}" 
                                onclick="toggleSort('progress_diff')" title="Sort by progress difference">Diff</th>
                            <th class="text-center sortable ${libraryState.sort === 'last_updated' ? 'sort-' + libraryState.sortDir : ''}" 
                                onclick="toggleSort('last_updated')" title="Sort by last updated">Updated</th>
                        </tr>
                    </thead>
                    <tbody>`;

    if (!books || books.length === 0) {
        html += '<tr class="empty-row"><td colspan="10">No books found</td></tr>';
    } else {
        books.forEach(book => {
            const absProgress = (book.abs_progress * 100).toFixed(1);
            const hcProgress = (book.hardcover_progress * 100).toFixed(1);
            const diff = Math.abs(book.progress_diff * 100).toFixed(1);
            
            // Determine media type icon
            const mediaType = (book.abs_media_type || 'book').toLowerCase();
            let mediaTypeIcon, mediaTypeTitle;
            if (mediaType === 'ebook') {
                mediaTypeIcon = '📱';
                mediaTypeTitle = 'eBook';
            } else {
                mediaTypeIcon = '🎧';
                mediaTypeTitle = 'Audiobook';
            }
            
            // Build ABS tooltip with metadata
            const absTooltipParts = [`Progress: ${absProgress}%`];
            if (book.abs_asin) absTooltipParts.push(`ASIN: ${book.abs_asin}`);
            if (book.abs_isbn) absTooltipParts.push(`ISBN: ${book.abs_isbn}`);
            if (book.abs_narrator) absTooltipParts.push(`Narrator: ${book.abs_narrator}`);
            
            // Build ABS link - opens audiobook page in new tab
            const absUrl = libraryState.absUrl ? `${libraryState.absUrl}/item/${book.abs_id}` : null;
            if (absUrl) {
                absTooltipParts.push('Click to open in AudiobookShelf');
            }
            const absTooltip = absTooltipParts.join('\n');
            const absLinkStart = absUrl 
                ? `<a href="${absUrl}" target="_blank" rel="noopener noreferrer" style="text-decoration: none; color: inherit; cursor: pointer;" title="${escapeHtml(absTooltip)}">` 
                : `<span title="${escapeHtml(absTooltip)}">`;
            const absLinkEnd = absUrl ? '</a>' : '</span>';
            
            // Build HC Book tooltip with metadata
            const isMapped = book.hc_book_id || book.hc_slug;
            const hcBookTooltipParts = [];
            if (isMapped) {
                hcBookTooltipParts.push(`Progress: ${hcProgress}%`);
                if (book.hc_title) hcBookTooltipParts.push(`Title: ${book.hc_title}`);
                if (book.hc_status_name) hcBookTooltipParts.push(`Status: ${book.hc_status_name}`);
                if (book.match_method) hcBookTooltipParts.push(`Match: ${book.match_method} (${(book.match_confidence * 100).toFixed(0)}%)`);
                hcBookTooltipParts.push('Click to open book in Hardcover');
            } else {
                hcBookTooltipParts.push('Not mapped to Hardcover');
            }
            const hcBookTooltip = hcBookTooltipParts.join('\n');
            
            // Build HC Edition tooltip with metadata
            const hcEditionTooltipParts = [];
            if (isMapped && book.hc_edition_id) {
                hcEditionTooltipParts.push(`Edition ID: ${book.hc_edition_id}`);
                if (book.hc_asin) hcEditionTooltipParts.push(`ASIN: ${book.hc_asin}`);
                if (book.hc_isbn13) hcEditionTooltipParts.push(`ISBN-13: ${book.hc_isbn13}`);
                if (book.hc_isbn10) hcEditionTooltipParts.push(`ISBN-10: ${book.hc_isbn10}`);
                hcEditionTooltipParts.push('Click to change edition');
            } else if (isMapped) {
                hcEditionTooltipParts.push('No edition set');
                hcEditionTooltipParts.push('Click to select edition');
            } else {
                hcEditionTooltipParts.push('Not mapped');
            }
            const hcEditionTooltip = hcEditionTooltipParts.join('\n');
            
            // Build HC Book link - always goes to book page
            let hcBookUrl = null;
            if (book.hc_slug) {
                hcBookUrl = `https://hardcover.app/books/${book.hc_slug}`;
            } else if (book.hc_book_id) {
                hcBookUrl = `https://hardcover.app/books/${book.hc_book_id}`;
            }
            
            // Build HC Edition link - goes to specific edition page
            let hcEditionUrl = null;
            if (book.hc_slug && book.hc_edition_id) {
                hcEditionUrl = `https://hardcover.app/books/${book.hc_slug}/editions/${book.hc_edition_id}`;
            }
            
            // Sync status indicator - clickable to perform sync
            let syncIndicator;
            let syncTooltipParts = [];
            if (!isMapped) {
                // Not mapped - red X, clickable to search and add to Hardcover
                syncTooltipParts.push('Not mapped to Hardcover');
                if (book.abs_asin) syncTooltipParts.push(`ABS ASIN: ${book.abs_asin}`);
                if (book.abs_isbn) syncTooltipParts.push(`ABS ISBN: ${book.abs_isbn}`);
                syncTooltipParts.push('Click to find on Hardcover');
                syncIndicator = `<span class="sync-icon unmapped" title="${escapeHtml(syncTooltipParts.join('\n'))}" onclick="openAddToHardcover('${book.abs_id}', '${escapeHtml(book.abs_title)}')">✗</span>`;
            } else if (book.in_sync) {
                // In sync - green double arrow, clickable to re-sync
                syncTooltipParts.push('✔ In sync');
                if (book.match_method) syncTooltipParts.push(`Match: ${book.match_method}`);
                syncTooltipParts.push('Click to re-sync');
                syncIndicator = `<span class="sync-icon synced" title="${escapeHtml(syncTooltipParts.join('\n'))}" onclick="syncSingleBook('${book.abs_id}')">⇄</span>`;
            } else {
                // Needs sync - amber double arrow, clickable to sync
                syncTooltipParts.push(`⚠ Needs sync (${diff}% diff)`);
                if (book.match_method) syncTooltipParts.push(`Match: ${book.match_method}`);
                syncTooltipParts.push('Click to sync');
                syncIndicator = `<span class="sync-icon needs-sync" title="${escapeHtml(syncTooltipParts.join('\n'))}" onclick="syncSingleBook('${book.abs_id}')">⇄</span>`;
            }
            
            // Edition cell - clickable to select/change edition
            let editionCell;
            if (isMapped) {
                const editionDisplay = book.hc_edition_id ? `#${book.hc_edition_id}` : '-';
                const editionLinkHtml = hcEditionUrl 
                    ? `<a href="${hcEditionUrl}" target="_blank" rel="noopener noreferrer" class="link-info" onclick="event.stopPropagation();">${editionDisplay}</a>`
                    : editionDisplay;
                editionCell = `<span class="edition-cell" title="${escapeHtml(hcEditionTooltip)}" onclick="openEditionSelector('${book.abs_id}', ${book.hc_book_id || 'null'}, '${escapeHtml(book.hc_slug || '')}')">
                    ${editionLinkHtml} <span class="edit-icon">✏️</span>
                </span>`;
            } else {
                editionCell = `<span class="text-muted" title="${escapeHtml(hcEditionTooltip)}">-</span>`;
            }
            
            html += `<tr>
                <td class="text-center" title="${mediaTypeTitle}" style="font-size: 1.2rem;">${mediaTypeIcon}</td>
                <td>${escapeHtml(book.abs_title)}</td>
                <td>${escapeHtml(book.abs_author || '')}</td>
                <td class="text-secondary text-small">${escapeHtml(book.abs_narrator || '')}</td>
                <td class="text-center">
                    ${absLinkStart}
                    <div class="progress-bar-mini">
                        <div class="fill success" style="width: ${absProgress}%;"></div>
                    </div>
                    <small class="text-secondary">${absProgress}%${absUrl ? ' 🔗' : ''}</small>
                    ${absLinkEnd}
                </td>
                <td class="text-center">${syncIndicator}</td>
                <td class="text-center">
                    ${isMapped ? `<a href="${hcBookUrl}" target="_blank" rel="noopener noreferrer" class="progress-link" title="${escapeHtml(hcBookTooltip)}">` : `<span title="${escapeHtml(hcBookTooltip)}">`}
                    <div class="progress-bar-mini">
                        <div class="fill ${isMapped ? 'info' : 'muted'}" style="width: ${hcProgress}%;"></div>
                    </div>
                    <small class="text-secondary">${isMapped ? `${hcProgress}% 🔗` : '-'}</small>
                    ${isMapped ? '</a>' : '</span>'}
                </td>
                <td class="text-center">${editionCell}</td>
                <td class="text-center"><strong>${diff}%</strong></td>
                <td class="text-center text-small text-muted">${formatLastUpdated(book.abs_last_updated || book.last_fetched_at)}</td>
            </tr>`;
        });
    }

    html += `
                    </tbody>
                </table>
            </div>

            <!-- Pagination -->
            <div class="pagination">
                <button class="btn btn-secondary" onclick="previousPage()" ${pagination.page <= 1 ? 'disabled' : ''}>← Previous</button>
                <span class="page-info">Page ${pagination.page} of ${pagination.total_pages} (${pagination.total} total)</span>
                <button class="btn btn-secondary" onclick="nextPage()" ${pagination.page >= pagination.total_pages ? 'disabled' : ''}>Next →</button>
            </div>
        </div>`;

    document.getElementById('library-inner').innerHTML = html;
}

// Get status badge HTML
function getStatusBadge(book) {
    if (book.abs_id === null || book.hc_book_id === null) {
        return `<span class="sync-badge unmapped">Unmapped</span>`;
    }
    if (!book.sync_enabled) {
        return `<span class="sync-badge disabled">Disabled</span>`;
    }
    const diff = Math.abs(book.progress_diff);
    if (diff < 0.01) {
        return `<span class="sync-badge synced">✓ Synced</span>`;
    }
    return `<span class="sync-badge needs-sync">⚠ Sync</span>`;
}

// Load and render sync summary statistics
async function loadSyncSummary() {
    if (!libraryState.profileId) return;

    try {
        const response = await fetch(`/api/profiles/${libraryState.profileId}/sync-summary`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        
        const data = await response.json();
        if (data.success) {
            renderSyncSummary(data.data);
        }
    } catch (error) {
        console.error('Error loading sync summary:', error);
    }
}

// Render summary statistics cards
function renderSyncSummary(stats) {
    const html = `
        <div class="stat-card success">
            <div class="label">Total ABS Books</div>
            <div class="value">${stats.total_abs_books || 0}</div>
        </div>
        <div class="stat-card info">
            <div class="label">Mapped to HC</div>
            <div class="value">${stats.mapped_books || 0}</div>
        </div>
        <div class="stat-card success">
            <div class="label">In Sync</div>
            <div class="value">${stats.in_sync_books || 0}</div>
        </div>
        <div class="stat-card warning">
            <div class="label">Needs Sync</div>
            <div class="value">${stats.needs_sync_books || 0}</div>
        </div>
        <div class="stat-card muted">
            <div class="label">Unmapped</div>
            <div class="value">${stats.unmapped_books || 0}</div>
        </div>`;

    const statsDiv = document.getElementById('library-stats');
    if (statsDiv) {
        statsDiv.innerHTML = html;
    }
}

// Open book detail modal
async function openBookDetail(absBookId) {
    if (!libraryState.profileId) return;

    try {
        const response = await fetch(`/api/profiles/${libraryState.profileId}/books/${absBookId}`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        
        const data = await response.json();
        if (data.success) {
            renderBookDetail(data.data);
            document.getElementById('book-detail-modal').style.display = 'block';
        }
    } catch (error) {
        console.error('Error loading book detail:', error);
        app.showToast('Failed to load book details', 'error');
    }
}

// Render book detail modal content
function renderBookDetail(book) {
    const absProgress = (book.abs_progress * 100).toFixed(1);
    const hcProgress = (book.hardcover_progress * 100).toFixed(1);

    let html = `
        <div class="modal-overlay" id="book-detail-modal">
            <div class="modal-content">
                <div class="modal-header">
                    <h3>${escapeHtml(book.abs_title)}</h3>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').style.display='none'">×</button>
                </div>

                <div class="mb-2">
                    <p><strong>Author:</strong> ${escapeHtml(book.abs_author || 'Unknown')}</p>
                    <p><strong>ASIN:</strong> <code>${escapeHtml(book.asin || 'N/A')}</code></p>
                    <p><strong>ISBN:</strong> <code>${escapeHtml(book.isbn || 'N/A')}</code></p>
                </div>

                <div class="mb-2">
                    <h4>Progress Comparison</h4>
                    <div class="mb-1">
                        <div class="progress-label">
                            <span>AudiobookShelf</span>
                            <strong>${absProgress}%</strong>
                        </div>
                        <div class="progress-bar-large">
                            <div class="fill" style="width: ${absProgress}%; background: var(--success);"></div>
                        </div>
                    </div>
                    <div>
                        <div class="progress-label">
                            <span>Hardcover</span>
                            <strong>${hcProgress}%</strong>
                        </div>
                        <div class="progress-bar-large">
                            <div class="fill" style="width: ${hcProgress}%; background: var(--info);"></div>
                        </div>
                    </div>
                </div>

                <div class="mb-2">
                    <h4>Sync Status</h4>
                    <p>${getStatusBadge(book)}</p>
                </div>

                <div class="mb-2">
                    <h4>Progress History (Last 10)</h4>
                    <div class="detail-section">
                        <table>
                            <thead>
                                <tr>
                                    <th>Date</th>
                                    <th>Source</th>
                                    <th class="text-right">Progress</th>
                                </tr>
                            </thead>
                            <tbody>`;
                            
    if (book.progress_history && book.progress_history.length > 0) {
        book.progress_history.forEach(entry => {
            const date = new Date(entry.updated_at).toLocaleString();
            html += `<tr>
                <td>${date}</td>
                <td>${entry.source || 'unknown'}</td>
                <td class="text-right"><strong>${(entry.progress * 100).toFixed(1)}%</strong></td>
            </tr>`;
        });
    } else {
        html += '<tr><td colspan="3" class="empty-state">No history available</td></tr>';
    }

    html += `
                            </tbody>
                        </table>
                    </div>
                </div>

                <div class="mb-2">
                    <h4>Sync Events (Last 5)</h4>
                    <div class="detail-section">`;
                        
    if (book.sync_events && book.sync_events.length > 0) {
        book.sync_events.forEach(event => {
            const date = new Date(event.created_at).toLocaleString();
            html += `<div class="history-item">
                <div class="text-small text-muted">${date}</div>
                <div><strong>${event.event_type}</strong></div>
                <div class="text-small">${event.details || ''}</div>
            </div>`;
        });
    } else {
        html += '<div class="empty-state">No events recorded</div>';
    }

    html += `
                    </div>
                </div>

                <div class="modal-actions">
                    <button class="btn btn-primary" onclick="syncSingleBookFromDetail('${book.abs_id}')">🔄 Sync Now</button>
                    <button class="btn btn-secondary" onclick="openBookConfigModal('${book.abs_id}')">⚙️ Settings</button>
                    <button class="btn btn-secondary" onclick="openBookMappingModal('${book.abs_id}')">🔗 Mapping</button>
                    <button class="btn btn-secondary" onclick="this.closest('.modal-overlay').style.display='none'">Close</button>
                </div>
            </div>
        </div>`;

    document.body.insertAdjacentHTML('beforeend', html);
}

// Sync single book
async function syncSingleBook(absBookId) {
    if (!libraryState.profileId) return;

    if (!confirm('Sync this book?')) return;

    try {
        app.showToast('Syncing...', 'info');
        const response = await fetch(`/api/profiles/${libraryState.profileId}/books/${absBookId}/sync`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ dry_run: false })
        });

        const data = await response.json();
        if (data.success) {
            app.showToast('Book synced successfully!', 'success');
            loadLibraryBooks();
        } else {
            app.showToast(data.error || 'Sync failed', 'error');
        }
    } catch (error) {
        console.error('Sync error:', error);
        app.showToast('Sync failed', 'error');
    }
}

// Sync single book from detail modal
async function syncSingleBookFromDetail(absBookId) {
    await syncSingleBook(absBookId);
    document.querySelectorAll('[id="book-detail-modal"]').forEach(el => el.style.display = 'none');
}

// Edition selector state
let editionSelectorState = {
    absBookId: null,
    hcBookId: null,
    hcSlug: null,
    editions: []
};

// Open edition selector modal
async function openEditionSelector(absBookId, hcBookId, hcSlug) {
    if (!libraryState.profileId || !hcBookId) {
        app.showToast('Book must be mapped to Hardcover first', 'error');
        return;
    }

    editionSelectorState = { absBookId, hcBookId, hcSlug, editions: [] };

    // Show loading modal
    const modalHtml = `
        <div id="edition-selector-modal" class="modal-overlay">
            <div class="modal-content">
                <button class="modal-close" onclick="closeEditionSelector()" title="Close">&times;</button>
                <h3 style="margin-top: 0; padding-right: 2rem;">Select Edition</h3>
                <div id="edition-list" class="empty-state">
                    <div class="loading-spinner"></div>
                    <p>Loading editions...</p>
                </div>
            </div>
        </div>
    `;
    document.body.insertAdjacentHTML('beforeend', modalHtml);

    // Add click-outside-to-close handler
    const modal = document.getElementById('edition-selector-modal');
    modal.addEventListener('click', (e) => {
        if (e.target === modal) closeEditionSelector();
    });

    // Fetch editions
    try {
        const response = await fetch(`/api/profiles/${libraryState.profileId}/editions?book_id=${hcBookId}`);
        const data = await response.json();

        if (data.success && data.data) {
            editionSelectorState.editions = data.data;
            renderEditionList(data.data);
        } else {
            document.getElementById('edition-list').innerHTML = `
                <p class="text-danger">Failed to load editions: ${data.error || 'Unknown error'}</p>
            `;
        }
    } catch (error) {
        console.error('Failed to fetch editions:', error);
        document.getElementById('edition-list').innerHTML = `
            <p class="text-danger">Failed to load editions</p>
        `;
    }
}

// Render edition list
function renderEditionList(editions) {
    if (!editions || editions.length === 0) {
        document.getElementById('edition-list').innerHTML = `
            <p class="text-muted">No editions found for this book</p>
        `;
        return;
    }

    // Reading format names
    const formatNames = {
        1: '📖 Physical',
        2: '🎧 Audiobook',
        3: '📱 eBook',
        4: '📚 Other'
    };

    let html = `
        <div class="edition-table-container">
        <table class="data-table">
            <thead>
                <tr>
                    <th>Format</th>
                    <th>ASIN</th>
                    <th>ISBN-13</th>
                    <th>Publisher</th>
                    <th class="text-center">Duration</th>
                    <th class="text-center">Action</th>
                </tr>
            </thead>
            <tbody>
    `;

    editions.forEach(ed => {
        const format = formatNames[ed.reading_format_id] || `Format ${ed.reading_format_id}`;
        const duration = ed.audio_seconds ? `${Math.floor(ed.audio_seconds / 3600)}h ${Math.floor((ed.audio_seconds % 3600) / 60)}m` : '-';
        const isAudiobook = ed.reading_format_id === 2;

        html += `
            <tr class="${isAudiobook ? 'highlight-row' : ''}">
                <td>${format}</td>
                <td class="mono text-small">${ed.asin || '-'}</td>
                <td class="mono text-small">${ed.isbn_13 || '-'}</td>
                <td>${escapeHtml(ed.publisher || '-')}</td>
                <td class="text-center">${duration}</td>
                <td class="text-center">
                    <button class="btn btn-primary btn-sm" onclick="selectEdition(${ed.id})">
                        Select
                    </button>
                </td>
            </tr>
        `;
    });

    html += '</tbody></table></div>';
    
    // Add "Create New Edition" button
    html += `
        <div class="text-center mt-1 mb-1">
            <button class="btn btn-secondary" onclick="openCreateEditionModal()">
                <i class="fas fa-plus"></i> Create New Audiobook Edition
            </button>
        </div>
    `;
    
    // Add link to view on Hardcover
    if (editionSelectorState.hcSlug) {
        html += `
            <p class="text-center mt-1">
                <a href="https://hardcover.app/books/${editionSelectorState.hcSlug}/editions" target="_blank" rel="noopener noreferrer" class="link-info">
                    View all editions on Hardcover ↗
                </a>
            </p>
        `;
    }

    document.getElementById('edition-list').innerHTML = html;
}

// Select an edition
async function selectEdition(editionId) {
    if (!libraryState.profileId || !editionSelectorState.absBookId) return;

    try {
        app.showToast('Updating edition...', 'info');

        const response = await fetch(`/api/profiles/${libraryState.profileId}/books/${editionSelectorState.absBookId}/edition`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ edition_id: editionId })
        });

        const data = await response.json();
        if (data.success) {
            app.showToast('Edition updated successfully!', 'success');
            closeEditionSelector();
            loadLibraryBooks(); // Refresh the library view
        } else {
            app.showToast(data.error || 'Failed to update edition', 'error');
        }
    } catch (error) {
        console.error('Failed to update edition:', error);
        app.showToast('Failed to update edition', 'error');
    }
}

// Close edition selector modal
function closeEditionSelector() {
    const modal = document.getElementById('edition-selector-modal');
    if (modal) modal.remove();
    editionSelectorState = { absBookId: null, hcBookId: null, hcSlug: null, editions: [] };
}

// Create Edition state
let createEditionState = {
    bookId: null,
    prepopulated: null
};

// Open create edition modal
async function openCreateEditionModal() {
    if (!libraryState.profileId || !editionSelectorState.hcBookId) {
        app.showToast('Book information not available', 'error');
        return;
    }

    createEditionState = { bookId: editionSelectorState.hcBookId, prepopulated: null };

    // Show loading modal
    const modalHtml = `
        <div id="create-edition-modal" class="modal-overlay">
            <div class="modal-content modal-large">
                <button class="modal-close" onclick="closeCreateEditionModal()" title="Close">&times;</button>
                <h3 style="margin-top: 0; padding-right: 2rem;">Create New Audiobook Edition</h3>
                <div id="create-edition-form">
                    <div class="loading-spinner"></div>
                    <p>Loading book data...</p>
                </div>
            </div>
        </div>
    `;
    document.body.insertAdjacentHTML('beforeend', modalHtml);

    // Add click-outside-to-close handler
    const modal = document.getElementById('create-edition-modal');
    modal.addEventListener('click', (e) => {
        if (e.target === modal) closeCreateEditionModal();
    });

    // Fetch prepopulated data
    try {
        const response = await fetch(`/api/profiles/${libraryState.profileId}/editions/prepopulate?book_id=${createEditionState.bookId}`);
        const data = await response.json();

        if (data.success && data.data) {
            createEditionState.prepopulated = data.data;
            renderCreateEditionForm(data.data);
        } else {
            document.getElementById('create-edition-form').innerHTML = `
                <p class="text-danger">Failed to load book data: ${data.error || 'Unknown error'}</p>
            `;
        }
    } catch (error) {
        console.error('Failed to fetch prepopulated data:', error);
        document.getElementById('create-edition-form').innerHTML = `
            <p class="text-danger">Failed to load book data</p>
        `;
    }
}

// Render create edition form
function renderCreateEditionForm(prepopulated) {
    const html = `
        <form id="edition-form" onsubmit="submitCreateEdition(event)">
            <div class="form-group">
                <label for="edition-title">Title *</label>
                <input type="text" id="edition-title" class="form-input" value="${escapeHtml(prepopulated.title || '')}" required>
            </div>
            
            <div class="form-group">
                <label for="edition-subtitle">Subtitle</label>
                <input type="text" id="edition-subtitle" class="form-input" value="${escapeHtml(prepopulated.subtitle || '')}">
            </div>
            
            <div class="form-group">
                <label for="edition-asin">ASIN</label>
                <input type="text" id="edition-asin" class="form-input" value="${escapeHtml(prepopulated.asin || '')}">
                <small class="text-muted">Amazon Standard Identification Number</small>
            </div>
            
            <div class="form-group">
                <label for="edition-isbn13">ISBN-13</label>
                <input type="text" id="edition-isbn13" class="form-input" value="${escapeHtml(prepopulated.isbn_13 || '')}">
            </div>
            
            <div class="form-group">
                <label for="edition-isbn10">ISBN-10</label>
                <input type="text" id="edition-isbn10" class="form-input" value="${escapeHtml(prepopulated.isbn_10 || '')}">
            </div>
            
            <div class="form-group">
                <label for="edition-image-url">Cover Image URL</label>
                <input type="url" id="edition-image-url" class="form-input" value="${escapeHtml(prepopulated.image_url || '')}">
            </div>
            
            <div class="form-group">
                <label for="edition-publisher-search">Publisher</label>
                <div class="search-box">
                    <input type="text" id="edition-publisher-search" class="form-input" 
                        placeholder="Search for publisher..." 
                        onkeyup="searchPublishers(this.value)">
                    <input type="hidden" id="edition-publisher-id" value="${prepopulated.publisher_id || ''}">
                </div>
                <div id="publisher-results" class="search-results"></div>
            </div>
            
            <div class="form-group">
                <label for="edition-audio-length">Audio Length (seconds)</label>
                <input type="number" id="edition-audio-length" class="form-input" value="${prepopulated.audio_seconds || ''}">
                <small class="text-muted">Total duration in seconds</small>
            </div>
            
            <div class="form-group">
                <label for="edition-release-date">Release Date</label>
                <input type="date" id="edition-release-date" class="form-input" value="${prepopulated.release_date || ''}">
            </div>
            
            <div class="form-group">
                <label for="edition-format">Edition Format</label>
                <input type="text" id="edition-format" class="form-input" value="${prepopulated.edition_format || 'Audible Audio'}" 
                    placeholder="e.g., Audible Audio, MP3 CD">
            </div>
            
            <div class="form-group">
                <label for="edition-info">Edition Information</label>
                <textarea id="edition-info" class="form-input" rows="3" 
                    placeholder="Any additional information about this edition...">${escapeHtml(prepopulated.edition_information || '')}</textarea>
            </div>
            
            <div class="form-actions mt-1">
                <button type="button" class="btn btn-secondary" onclick="closeCreateEditionModal()">Cancel</button>
                <button type="submit" class="btn btn-primary">Create Edition</button>
            </div>
        </form>
    `;
    
    document.getElementById('create-edition-form').innerHTML = html;
}

// Search publishers with debouncing
let publisherSearchTimeout;
async function searchPublishers(query) {
    clearTimeout(publisherSearchTimeout);
    
    if (!query || query.length < 2) {
        document.getElementById('publisher-results').innerHTML = '';
        return;
    }
    
    publisherSearchTimeout = setTimeout(async () => {
        try {
            const response = await fetch(`/api/profiles/${libraryState.profileId}/search-publishers?name=${encodeURIComponent(query)}&limit=10`);
            const data = await response.json();
            
            if (data.success && data.data && data.data.length > 0) {
                let html = '<div class="dropdown-list">';
                data.data.forEach(pub => {
                    html += `
                        <div class="dropdown-item" onclick="selectPublisher('${pub.id}', '${escapeHtml(pub.name)}')">
                            ${escapeHtml(pub.name)}
                        </div>
                    `;
                });
                html += '</div>';
                document.getElementById('publisher-results').innerHTML = html;
            } else {
                document.getElementById('publisher-results').innerHTML = '<p class="text-muted text-small">No publishers found</p>';
            }
        } catch (error) {
            console.error('Failed to search publishers:', error);
        }
    }, 300);
}

// Select publisher from search results
function selectPublisher(id, name) {
    document.getElementById('edition-publisher-id').value = id;
    document.getElementById('edition-publisher-search').value = name;
    document.getElementById('publisher-results').innerHTML = '';
}

// Submit create edition form
async function submitCreateEdition(event) {
    event.preventDefault();
    
    if (!libraryState.profileId || !createEditionState.bookId) {
        app.showToast('Missing required data', 'error');
        return;
    }
    
    const formData = {
        book_id: createEditionState.bookId,
        title: document.getElementById('edition-title').value,
        subtitle: document.getElementById('edition-subtitle').value || undefined,
        asin: document.getElementById('edition-asin').value || undefined,
        isbn_13: document.getElementById('edition-isbn13').value || undefined,
        isbn_10: document.getElementById('edition-isbn10').value || undefined,
        image_url: document.getElementById('edition-image-url').value || undefined,
        publisher_id: parseInt(document.getElementById('edition-publisher-id').value) || undefined,
        audio_seconds: parseInt(document.getElementById('edition-audio-length').value) || undefined,
        release_date: document.getElementById('edition-release-date').value || undefined,
        edition_format: document.getElementById('edition-format').value || undefined,
        edition_information: document.getElementById('edition-info').value || undefined,
        language_id: 1, // Default to English
        author_ids: createEditionState.prepopulated?.author_ids || []
    };
    
    try {
        app.showToast('Creating edition...', 'info');
        
        const response = await fetch(`/api/profiles/${libraryState.profileId}/editions`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(formData)
        });
        
        const data = await response.json();
        if (data.success) {
            app.showToast('Edition created successfully!', 'success');
            closeCreateEditionModal();
            
            // Refresh the editions list in the edition selector
            if (editionSelectorState.hcBookId) {
                const editionsResponse = await fetch(`/api/profiles/${libraryState.profileId}/editions?book_id=${editionSelectorState.hcBookId}`);
                const editionsData = await editionsResponse.json();
                if (editionsData.success && editionsData.data) {
                    editionSelectorState.editions = editionsData.data;
                    renderEditionList(editionsData.data);
                }
            }
        } else {
            app.showToast(data.error || 'Failed to create edition', 'error');
        }
    } catch (error) {
        console.error('Failed to create edition:', error);
        app.showToast('Failed to create edition', 'error');
    }
}

// Close create edition modal
function closeCreateEditionModal() {
    const modal = document.getElementById('create-edition-modal');
    if (modal) modal.remove();
    createEditionState = { bookId: null, prepopulated: null };
}

// Add to Hardcover state
let addToHardcoverState = {
    absBookId: null,
    absTitle: null,
    searchQuery: null,
    searchResults: [],
    selectedBook: null,
    editions: []
};

// Open "Add to Hardcover" modal for unmapped books
async function openAddToHardcover(absBookId, absTitle) {
    if (!libraryState.profileId) {
        app.showToast('No profile selected', 'error');
        return;
    }

    addToHardcoverState = { absBookId, absTitle, searchQuery: absTitle, searchResults: [], selectedBook: null, editions: [] };

    // Show loading modal
    const modalHtml = `
        <div id="add-to-hardcover-modal" class="modal-overlay">
            <div class="modal-content modal-large">
                <button class="modal-close" onclick="closeAddToHardcover()" title="Close">&times;</button>
                <h3 style="margin-top: 0; padding-right: 2rem;">Add to Hardcover</h3>
                <div class="search-box mb-1">
                    <input type="text" id="hardcover-search-input" class="form-input" 
                        value="${escapeHtml(absTitle)}" 
                        placeholder="Search Hardcover..." 
                        onkeydown="if(event.key==='Enter') searchHardcoverManual()">
                    <button class="btn btn-primary" onclick="searchHardcoverManual()">🔍 Search</button>
                </div>
                <div id="add-to-hardcover-content" class="empty-state">
                    <div class="loading-spinner"></div>
                    <p>Searching Hardcover...</p>
                </div>
                <div class="modal-actions" style="justify-content: flex-end; margin-top: 1rem;">
                    <button class="btn btn-secondary" onclick="closeAddToHardcover()">Cancel</button>
                </div>
            </div>
        </div>
    `;
    document.body.insertAdjacentHTML('beforeend', modalHtml);
    
    // Add click-outside-to-close handler
    const modal = document.getElementById('add-to-hardcover-modal');
    modal.addEventListener('click', (e) => {
        if (e.target === modal) closeAddToHardcover();
    });

    // Perform initial search
    await searchHardcoverByQuery(absTitle);
}

// Search Hardcover with a specific query
async function searchHardcoverByQuery(query) {
    addToHardcoverState.searchQuery = query;
    
    document.getElementById('add-to-hardcover-content').innerHTML = `
        <div class="empty-state">
            <div class="loading-spinner"></div>
            <p>Searching Hardcover...</p>
        </div>
    `;

    try {
        const response = await fetch(`/api/profiles/${libraryState.profileId}/search-hardcover?q=${encodeURIComponent(query)}`);
        const data = await response.json();

        if (data.success && data.data && data.data.length > 0) {
            addToHardcoverState.searchResults = data.data;
            renderSearchResults(data.data);
        } else {
            document.getElementById('add-to-hardcover-content').innerHTML = `
                <p class="text-danger">No matching books found on Hardcover.</p>
                <p class="text-secondary text-small">Try a different search term or search manually on <a href="https://hardcover.app/search?q=${encodeURIComponent(query)}" target="_blank" rel="noopener noreferrer" class="link-info">hardcover.app</a></p>
            `;
        }
    } catch (error) {
        console.error('Failed to search Hardcover:', error);
        document.getElementById('add-to-hardcover-content').innerHTML = `
            <p class="text-danger">Failed to search Hardcover</p>
        `;
    }
}

// Manual search from input field
function searchHardcoverManual() {
    const input = document.getElementById('hardcover-search-input');
    if (input && input.value.trim()) {
        searchHardcoverByQuery(input.value.trim());
    }
}

// Render search results
function renderSearchResults(results) {
    if (!results || results.length === 0) {
        document.getElementById('add-to-hardcover-content').innerHTML = `
            <p class="text-muted">No books found</p>
        `;
        return;
    }

    let html = `
        <p class="mb-1 text-secondary">Found ${results.length} potential match${results.length > 1 ? 'es' : ''} on Hardcover:</p>
        <div class="search-results">
    `;

    results.forEach((book, idx) => {
        html += `
            <div class="search-result-item ${idx === 0 ? 'highlight' : ''}">
                <div class="result-info">
                    <strong>${escapeHtml(book.title)}</strong>
                    ${book.slug ? `<br><small class="text-secondary">hardcover.app/books/${book.slug}</small>` : ''}
                </div>
                <div class="result-actions">
                    ${book.slug ? `<a href="https://hardcover.app/books/${book.slug}" target="_blank" rel="noopener noreferrer" class="link-info text-small">View ↗</a>` : ''}
                    <button class="btn btn-primary" onclick="selectHardcoverBook(${book.book_id}, '${escapeHtml(book.slug || '')}')">
                        Select
                    </button>
                </div>
            </div>
        `;
    });

    html += '</div>';
    document.getElementById('add-to-hardcover-content').innerHTML = html;
}

// Select a Hardcover book and load its editions
async function selectHardcoverBook(bookId, slug) {
    addToHardcoverState.selectedBook = { book_id: bookId, slug };

    document.getElementById('add-to-hardcover-content').innerHTML = `
        <div class="empty-state">
            <div class="loading-spinner"></div>
            <p>Loading editions...</p>
        </div>
    `;

    // Fetch editions for this book
    try {
        const response = await fetch(`/api/profiles/${libraryState.profileId}/editions?book_id=${bookId}`);
        const data = await response.json();

        if (data.success && data.data && data.data.length > 0) {
            addToHardcoverState.editions = data.data;
            renderAddToHardcoverEditions(data.data, bookId, slug);
        } else {
            document.getElementById('add-to-hardcover-content').innerHTML = `
                <p class="text-danger">No editions found for this book</p>
                <button class="btn btn-secondary" onclick="renderSearchResults(addToHardcoverState.searchResults)">← Back to search results</button>
            `;
        }
    } catch (error) {
        console.error('Failed to fetch editions:', error);
        document.getElementById('add-to-hardcover-content').innerHTML = `
            <p class="text-danger">Failed to load editions</p>
            <button class="btn btn-secondary" onclick="renderSearchResults(addToHardcoverState.searchResults)">← Back to search results</button>
        `;
    }
}

// Render editions for adding to Hardcover
function renderAddToHardcoverEditions(editions, bookId, slug) {
    const formatNames = {
        1: '📖 Physical',
        2: '🎧 Audiobook',
        3: '📱 eBook',
        4: '📚 Other'
    };

    let html = `
        <div class="mb-1">
            <button class="btn btn-secondary btn-sm" onclick="renderSearchResults(addToHardcoverState.searchResults)">← Back to search</button>
        </div>
        <p class="mb-1 text-secondary">Select an edition to add as "Want to Read":</p>
        <table class="data-table">
            <thead>
                <tr>
                    <th>Format</th>
                    <th>ASIN</th>
                    <th>ISBN-13</th>
                    <th>Publisher</th>
                    <th class="text-center">Duration</th>
                    <th class="text-center">Action</th>
                </tr>
            </thead>
            <tbody>
    `;

    editions.forEach(ed => {
        const format = formatNames[ed.reading_format_id] || `Format ${ed.reading_format_id}`;
        const duration = ed.audio_seconds ? `${Math.floor(ed.audio_seconds / 3600)}h ${Math.floor((ed.audio_seconds % 3600) / 60)}m` : '-';
        const isAudiobook = ed.reading_format_id === 2;

        html += `
            <tr class="${isAudiobook ? 'highlight-row' : ''}">
                <td>${format}</td>
                <td class="mono text-small">${ed.asin || '-'}</td>
                <td class="mono text-small">${ed.isbn_13 || '-'}</td>
                <td>${escapeHtml(ed.publisher || '-')}</td>
                <td class="text-center">${duration}</td>
                <td class="text-center">
                    <button class="btn btn-primary btn-sm" onclick="addBookToWantToRead(${ed.id}, ${bookId})">
                        Add
                    </button>
                </td>
            </tr>
        `;
    });

    html += '</tbody></table>';
    
    // Add link to view on Hardcover
    if (slug) {
        html += `
            <p class="text-center mt-1">
                <a href="https://hardcover.app/books/${slug}/editions" target="_blank" rel="noopener noreferrer" class="link-info">
                    View all editions on Hardcover ↗
                </a>
            </p>
        `;
    }

    document.getElementById('add-to-hardcover-content').innerHTML = html;
}

// Add book to "Want to Read" on Hardcover
async function addBookToWantToRead(editionId, bookId) {
    if (!libraryState.profileId || !addToHardcoverState.absBookId) return;

    try {
        app.showToast('Adding to Hardcover...', 'info');

        const response = await fetch(`/api/profiles/${libraryState.profileId}/books/${addToHardcoverState.absBookId}/add-to-hardcover`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ edition_id: editionId, book_id: bookId })
        });

        const data = await response.json();
        if (data.success) {
            app.showToast('Book added to "Want to Read"!', 'success');
            closeAddToHardcover();
            
            // Trigger a sync for this book since it may have progress
            await syncSingleBook(addToHardcoverState.absBookId);
            
            // Refresh the library view
            loadLibraryBooks();
        } else {
            app.showToast(data.error || 'Failed to add book', 'error');
        }
    } catch (error) {
        console.error('Failed to add book to Hardcover:', error);
        app.showToast('Failed to add book to Hardcover', 'error');
    }
}

// Close add to Hardcover modal
function closeAddToHardcover() {
    const modal = document.getElementById('add-to-hardcover-modal');
    if (modal) modal.remove();
    addToHardcoverState = { absBookId: null, absTitle: null, searchResults: [], selectedBook: null, editions: [] };
}

// Collect from ABS
async function collectABSBooks() {
    if (!libraryState.profileId) return;

    if (!confirm('Collect books from AudiobookShelf?')) return;

    try {
        app.showToast('Collecting from ABS...', 'info');
        const response = await fetch(`/api/profiles/${libraryState.profileId}/collect/abs`, {
            method: 'POST'
        });

        const data = await response.json();
        if (data.success) {
            app.showToast(`Collected ${data.data.collected_count} books from ABS!`, 'success');
            loadLibraryBooks();
        } else {
            app.showToast(data.error || 'Collection failed', 'error');
        }
    } catch (error) {
        console.error('Collection error:', error);
        app.showToast('Collection failed', 'error');
    }
}

// Collect from Hardcover
async function collectHardcoverBooks() {
    if (!libraryState.profileId) return;

    if (!confirm('Collect books from Hardcover?')) return;

    try {
        app.showToast('Collecting from Hardcover...', 'info');
        const response = await fetch(`/api/profiles/${libraryState.profileId}/collect/hardcover`, {
            method: 'POST'
        });

        const data = await response.json();
        if (data.success) {
            app.showToast(`Collected ${data.data.collected_count} books from Hardcover!`, 'success');
            loadLibraryBooks();
        } else {
            app.showToast(data.error || 'Collection failed', 'error');
        }
    } catch (error) {
        console.error('Collection error:', error);
        app.showToast('Collection failed', 'error');
    }
}

// Auto-match ABS books to Hardcover
async function autoMatchBooks() {
    if (!libraryState.profileId) return;

    try {
        app.showToast('Auto-matching books...', 'info');
        const response = await fetch(`/api/profiles/${libraryState.profileId}/auto-match`, {
            method: 'POST'
        });

        const data = await response.json();
        if (data.success) {
            app.showToast('Auto-matching completed! Refreshing...', 'success');
            loadLibraryBooks();
        } else {
            app.showToast(data.error || 'Auto-match failed', 'error');
        }
    } catch (error) {
        console.error('Auto-match error:', error);
        app.showToast('Auto-match failed', 'error');
    }
}

// Pagination functions
function previousPage() {
    if (libraryState.currentPage > 1) {
        libraryState.currentPage--;
        loadLibraryBooks();
    }
}

function nextPage() {
    libraryState.currentPage++;
    loadLibraryBooks();
}

// Global function for sync mode change in edit modal
function handleSyncModeChange() {
    if (app) {
        app.handleSyncModeChange();
    }
}

// Global function for library filter mode change in edit modal
function handleLibraryFilterModeChange() {
    if (app) {
        app.handleLibraryFilterModeChange();
    }
}

// Global function for process unread books change in edit modal
function handleProcessUnreadChange() {
    const processUnreadEl = document.getElementById('edit-process-unread-books');
    const syncWantToReadEl = document.getElementById('edit-sync-want-to-read');
    const syncWantToReadGroup = document.getElementById('sync-want-to-read-group');
    
    if (!processUnreadEl || !syncWantToReadEl) return;
    
    if (processUnreadEl.checked) {
        syncWantToReadEl.disabled = false;
        if (syncWantToReadGroup) {
            syncWantToReadGroup.classList.remove('disabled-group');
        }
    } else {
        syncWantToReadEl.disabled = true;
        syncWantToReadEl.checked = false;
        if (syncWantToReadGroup) {
            syncWantToReadGroup.classList.add('disabled-group');
        }
    }
}

// Apply library filter (search)
async function applyLibraryFilter() {
    // This would be called on search input - for now just reload
    loadLibraryBooks();
}

// Open book config modal
function openBookConfigModal(absBookId) {
    document.getElementById('book-config-modal').style.display = 'block';
    document.getElementById('book-config-abs-id').value = absBookId;
    document.getElementById('book-config-profile-id').value = libraryState.profileId;
    
    // Load book title
    if (libraryState.currentBookDetail) {
        document.getElementById('book-config-title').textContent = escapeHtml(libraryState.currentBookDetail.abs_title);
    }
}

// Close book config modal
function closeBookConfigModal() {
    document.getElementById('book-config-modal').style.display = 'none';
}

// Open book mapping modal
function openBookMappingModal(absBookId) {
    document.getElementById('book-mapping-modal').style.display = 'block';
    document.getElementById('mapping-abs-id').value = absBookId;
    document.getElementById('mapping-profile-id').value = libraryState.profileId;
    
    if (libraryState.currentBookDetail) {
        document.getElementById('mapping-book-title').textContent = `AudiobookShelf Book: ${escapeHtml(libraryState.currentBookDetail.abs_title)}`;
    }
}

// Close book mapping modal
function closeBookMappingModal() {
    document.getElementById('book-mapping-modal').style.display = 'none';
}

// HTML escape helper
function escapeHtml(text) {
    if (!text) return '';
    const map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#039;'
    };
    return text.replace(/[&<>"']/g, m => map[m]);
}

// Load profiles into profile selector when library tab is shown
function loadLibraryProfiles() {
    updateLibraryProfileSelect();
}

// Switch to Library tab for a specific profile
function viewProfileLibrary(profileId) {
    libraryState.profileId = profileId;
    libraryState.currentPage = 1;
    showTab('library');
    
    // Set the profile selector to the right profile
    const select = document.getElementById('library-profile-select');
    if (select) {
        select.value = profileId;
        setTimeout(() => loadLibraryBooks(), 100);
    }
}

// Update showTab to handle library tab
const originalShowTab = SyncProfileApp.prototype.showTab;
SyncProfileApp.prototype.showTab = function(tabName) {
    originalShowTab.call(this, tabName);
    if (tabName === 'library') {
        loadLibraryProfiles();
    }
};