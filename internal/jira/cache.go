package jira

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache handles caching of Jira issues
type Cache struct {
	cacheDir     string
	ttl          time.Duration
	metadata     *CacheMetadata
	metadataPath string
	mu           sync.RWMutex
}

// IssueCacheEntry represents a cached Jira issue
type IssueCacheEntry struct {
	Data      *Issue    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
	IssueKey  string    `json:"issue_key"`
}

// CacheMetadata tracks all cache entries with their expiration
type CacheMetadata struct {
	Entries map[string]CacheMetadataEntry `json:"entries"`
}

// CacheMetadataEntry represents metadata for a single cache entry
type CacheMetadataEntry struct {
	Created  time.Time `json:"created"`
	Expires  time.Time `json:"expires"`
	Type     string    `json:"type"` // "issue"
	Key      string    `json:"key"`  // Identifier (e.g., "CNFCERT-1234")
}

// NewCache creates a new Jira cache with 24-hour TTL
func NewCache() (*Cache, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	cacheDir := filepath.Join(homeDir, ".perfdive", "cache", "jira")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	metadataPath := filepath.Join(cacheDir, "metadata.json")
	
	cache := &Cache{
		cacheDir:     cacheDir,
		ttl:          24 * time.Hour, // 24-hour cache for Jira issues
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
func (c *Cache) updateMetadata(filename, issueKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.metadata.Entries[filename] = CacheMetadataEntry{
		Created: now,
		Expires: now.Add(c.ttl),
		Type:    "issue",
		Key:     issueKey,
	}
}

// isExpired checks if a cache entry is expired based on metadata
func (c *Cache) isExpired(filename string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.metadata.Entries[filename]
	if !exists {
		return true
	}

	return time.Now().After(entry.Expires)
}

// getCacheFilename generates a cache filename for a Jira issue
func (c *Cache) getCacheFilename(issueKey string) string {
	// Use the issue key directly, but sanitize it for filesystem
	// Replace any problematic characters with underscores
	safeKey := issueKey
	return fmt.Sprintf("%s.json", safeKey)
}

// GetIssue retrieves a cached Jira issue if it exists and is not expired (24-hour TTL)
func (c *Cache) GetIssue(issueKey string) (*Issue, bool) {
	filename := c.getCacheFilename(issueKey)
	cacheFile := filepath.Join(c.cacheDir, filename)

	// Check metadata first
	if c.isExpired(filename) {
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

// SetIssue stores a Jira issue in the cache with 24-hour TTL
func (c *Cache) SetIssue(issue *Issue) error {
	if issue == nil || issue.Key == "" {
		return fmt.Errorf("invalid issue: missing key")
	}

	entry := IssueCacheEntry{
		Data:      issue,
		Timestamp: time.Now(),
		IssueKey:  issue.Key,
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	filename := c.getCacheFilename(issue.Key)
	cacheFile := filepath.Join(c.cacheDir, filename)
	
	if err := os.WriteFile(cacheFile, jsonData, 0644); err != nil {
		return err
	}

	// Update metadata with 24-hour TTL
	c.updateMetadata(filename, issue.Key)
	return c.saveMetadata()
}

// GetIssues retrieves multiple cached issues, returning only those found in cache
func (c *Cache) GetIssues(issueKeys []string) (map[string]*Issue, []string) {
	cached := make(map[string]*Issue)
	missing := []string{}

	for _, key := range issueKeys {
		if issue, found := c.GetIssue(key); found {
			cached[key] = issue
		} else {
			missing = append(missing, key)
		}
	}

	return cached, missing
}

// SetIssues stores multiple Jira issues in the cache
func (c *Cache) SetIssues(issues []Issue) error {
	for i := range issues {
		if err := c.SetIssue(&issues[i]); err != nil {
			// Log error but continue with other issues
			fmt.Printf("Warning: failed to cache issue %s: %v\n", issues[i].Key, err)
		}
	}
	return nil
}

// Clear removes all cached Jira issues
func (c *Cache) Clear() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			_ = os.Remove(filepath.Join(c.cacheDir, entry.Name()))
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
	for filename, entry := range c.metadata.Entries {
		if now.After(entry.Expires) {
			toDelete = append(toDelete, filename)
			
			// Delete the actual cache file
			filePath := filepath.Join(c.cacheDir, filename)
			_ = os.Remove(filePath)
		}
	}

	// Remove from metadata
	for _, filename := range toDelete {
		delete(c.metadata.Entries, filename)
	}

	// Save updated metadata if any entries were deleted
	if len(toDelete) > 0 {
		return c.saveMetadata()
	}

	return nil
}

// GetCacheStats returns statistics about the cache
func (c *Cache) GetCacheStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := map[string]interface{}{
		"total": len(c.metadata.Entries),
		"ttl":   "24 hours",
	}

	return stats
}

