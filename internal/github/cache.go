package github

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache handles caching of GitHub activity data
type Cache struct {
	cacheDir     string
	ttl          time.Duration
	metadata     *CacheMetadata
	metadataPath string
	mu           sync.RWMutex
}

// CacheEntry represents a cached item with expiration
type CacheEntry struct {
	Data      *ComprehensiveUserActivity `json:"data"`
	Timestamp time.Time                  `json:"timestamp"`
	Username  string                     `json:"username"`
	StartDate string                     `json:"start_date"`
	EndDate   string                     `json:"end_date"`
}

// PRCacheEntry represents a cached Pull Request
type PRCacheEntry struct {
	Data      *PullRequest `json:"data"`
	Timestamp time.Time    `json:"timestamp"`
	Owner     string       `json:"owner"`
	Repo      string       `json:"repo"`
	Number    string       `json:"number"`
}

// IssueCacheEntry represents a cached Issue
type IssueCacheEntry struct {
	Data      *Issue    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
	Owner     string    `json:"owner"`
	Repo      string    `json:"repo"`
	Number    string    `json:"number"`
}

// CacheMetadata tracks all cache entries with their expiration
type CacheMetadata struct {
	Entries map[string]CacheMetadataEntry `json:"entries"`
}

// CacheMetadataEntry represents metadata for a single cache entry
type CacheMetadataEntry struct {
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	Type    string    `json:"type"` // "activity", "pr", "issue"
	Key     string    `json:"key"`  // Identifier (e.g., "owner/repo#123")
}

// NewCache creates a new cache with default TTL of 1 hour
func NewCache() (*Cache, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	cacheDir := filepath.Join(homeDir, ".perfdive", "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	// Create subdirectories for different cache types
	activityDir := filepath.Join(cacheDir, "activity")
	prsDir := filepath.Join(cacheDir, "prs")
	issuesDir := filepath.Join(cacheDir, "issues")
	
	for _, dir := range []string{activityDir, prsDir, issuesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	metadataPath := filepath.Join(cacheDir, "metadata.json")
	
	cache := &Cache{
		cacheDir:     cacheDir,
		ttl:          1 * time.Hour, // Default 1 hour cache for activity
		metadataPath: metadataPath,
		metadata:     &CacheMetadata{Entries: make(map[string]CacheMetadataEntry)},
	}

	// Load existing metadata if it exists
	if err := cache.loadMetadata(); err != nil {
		// If metadata doesn't exist or is corrupted, start fresh
		cache.metadata = &CacheMetadata{Entries: make(map[string]CacheMetadataEntry)}
	}

	return cache, nil
}

// getCacheKey generates a cache key based on username and date range
func (c *Cache) getCacheKey(username, startDate, endDate string) string {
	key := fmt.Sprintf("%s_%s_%s", username, startDate, endDate)
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x.json", hash[:8])
}

// loadMetadata loads cache metadata from disk
func (c *Cache) loadMetadata() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.metadataPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, c.metadata)
}

// saveMetadata saves cache metadata to disk
func (c *Cache) saveMetadata() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.MarshalIndent(c.metadata, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.metadataPath, data, 0644)
}

// updateMetadata adds or updates a metadata entry
func (c *Cache) updateMetadata(path, entryType, key string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.metadata.Entries[path] = CacheMetadataEntry{
		Created: now,
		Expires: now.Add(ttl),
		Type:    entryType,
		Key:     key,
	}
}

// isExpired checks if a cache entry is expired based on metadata
func (c *Cache) isExpired(path string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.metadata.Entries[path]
	if !exists {
		return true
	}

	return time.Now().After(entry.Expires)
}

// Get retrieves cached data if it exists and is not expired
func (c *Cache) Get(username, startDate, endDate string) (*ComprehensiveUserActivity, bool) {
	cacheFile := filepath.Join(c.cacheDir, "activity", c.getCacheKey(username, startDate, endDate))
	relativePath := filepath.Join("activity", c.getCacheKey(username, startDate, endDate))

	// Check metadata first
	if c.isExpired(relativePath) {
		_ = os.Remove(cacheFile)
		return nil, false
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Double-check with embedded timestamp
	if time.Since(entry.Timestamp) > c.ttl {
		_ = os.Remove(cacheFile)
		return nil, false
	}

	// Verify it's the right data
	if entry.Username != username || entry.StartDate != startDate || entry.EndDate != endDate {
		return nil, false
	}

	return entry.Data, true
}

// Set stores data in the cache
func (c *Cache) Set(username, startDate, endDate string, data *ComprehensiveUserActivity) error {
	entry := CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		Username:  username,
		StartDate: startDate,
		EndDate:   endDate,
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	cacheFile := filepath.Join(c.cacheDir, "activity", c.getCacheKey(username, startDate, endDate))
	relativePath := filepath.Join("activity", c.getCacheKey(username, startDate, endDate))
	
	if err := os.WriteFile(cacheFile, jsonData, 0644); err != nil {
		return err
	}

	// Update metadata
	key := fmt.Sprintf("%s_%s_%s", username, startDate, endDate)
	c.updateMetadata(relativePath, "activity", key, c.ttl)
	return c.saveMetadata()
}

// GetPR retrieves a cached Pull Request if it exists and is not expired (24-hour TTL)
func (c *Cache) GetPR(owner, repo, number string) (*PullRequest, bool) {
	filename := fmt.Sprintf("%s_%s_%s.json", owner, repo, number)
	cacheFile := filepath.Join(c.cacheDir, "prs", filename)
	relativePath := filepath.Join("prs", filename)

	// Check metadata first
	if c.isExpired(relativePath) {
		_ = os.Remove(cacheFile)
		return nil, false
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}

	var entry PRCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Double-check with embedded timestamp (24-hour TTL)
	if time.Since(entry.Timestamp) > 24*time.Hour {
		_ = os.Remove(cacheFile)
		return nil, false
	}

	return entry.Data, true
}

// SetPR stores a Pull Request in the cache with 24-hour TTL
func (c *Cache) SetPR(owner, repo, number string, data *PullRequest) error {
	entry := PRCacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		Owner:     owner,
		Repo:      repo,
		Number:    number,
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s_%s_%s.json", owner, repo, number)
	cacheFile := filepath.Join(c.cacheDir, "prs", filename)
	relativePath := filepath.Join("prs", filename)
	
	if err := os.WriteFile(cacheFile, jsonData, 0644); err != nil {
		return err
	}

	// Update metadata with 24-hour TTL
	key := fmt.Sprintf("%s/%s#%s", owner, repo, number)
	c.updateMetadata(relativePath, "pr", key, 24*time.Hour)
	return c.saveMetadata()
}

// GetIssue retrieves a cached Issue if it exists and is not expired (24-hour TTL)
func (c *Cache) GetIssue(owner, repo, number string) (*Issue, bool) {
	filename := fmt.Sprintf("%s_%s_%s.json", owner, repo, number)
	cacheFile := filepath.Join(c.cacheDir, "issues", filename)
	relativePath := filepath.Join("issues", filename)

	// Check metadata first
	if c.isExpired(relativePath) {
		_ = os.Remove(cacheFile)
		return nil, false
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}

	var entry IssueCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Double-check with embedded timestamp (24-hour TTL)
	if time.Since(entry.Timestamp) > 24*time.Hour {
		_ = os.Remove(cacheFile)
		return nil, false
	}

	return entry.Data, true
}

// SetIssue stores an Issue in the cache with 24-hour TTL
func (c *Cache) SetIssue(owner, repo, number string, data *Issue) error {
	entry := IssueCacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		Owner:     owner,
		Repo:      repo,
		Number:    number,
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s_%s_%s.json", owner, repo, number)
	cacheFile := filepath.Join(c.cacheDir, "issues", filename)
	relativePath := filepath.Join("issues", filename)
	
	if err := os.WriteFile(cacheFile, jsonData, 0644); err != nil {
		return err
	}

	// Update metadata with 24-hour TTL
	key := fmt.Sprintf("%s/%s#%s", owner, repo, number)
	c.updateMetadata(relativePath, "issue", key, 24*time.Hour)
	return c.saveMetadata()
}

// Clear removes all cached entries
func (c *Cache) Clear() error {
	// Clear all subdirectories
	for _, subdir := range []string{"activity", "prs", "issues"} {
		dirPath := filepath.Join(c.cacheDir, subdir)
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				_ = os.Remove(filepath.Join(dirPath, entry.Name()))
			}
		}
	}

	// Clear metadata
	c.mu.Lock()
	c.metadata.Entries = make(map[string]CacheMetadataEntry)
	c.mu.Unlock()
	
	return c.saveMetadata()
}

// CleanExpired removes expired cache entries based on metadata
func (c *Cache) CleanExpired() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	toDelete := []string{}

	// Find all expired entries in metadata
	for path, entry := range c.metadata.Entries {
		if now.After(entry.Expires) {
			toDelete = append(toDelete, path)
			
			// Delete the actual cache file
			filePath := filepath.Join(c.cacheDir, path)
			_ = os.Remove(filePath)
		}
	}

	// Remove from metadata
	for _, path := range toDelete {
		delete(c.metadata.Entries, path)
	}

	// Save updated metadata if any entries were deleted
	if len(toDelete) > 0 {
		return c.saveMetadata()
	}

	return nil
}

// GetCacheStats returns statistics about the cache
func (c *Cache) GetCacheStats() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := map[string]int{
		"activity": 0,
		"prs":      0,
		"issues":   0,
		"total":    len(c.metadata.Entries),
	}

	for _, entry := range c.metadata.Entries {
		if count, ok := stats[entry.Type]; ok {
			stats[entry.Type] = count + 1
		}
	}

	return stats
}

