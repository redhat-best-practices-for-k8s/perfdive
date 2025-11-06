package github

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cache handles caching of GitHub activity data
type Cache struct {
	cacheDir string
	ttl      time.Duration
}

// CacheEntry represents a cached item with expiration
type CacheEntry struct {
	Data      *ComprehensiveUserActivity `json:"data"`
	Timestamp time.Time                  `json:"timestamp"`
	Username  string                     `json:"username"`
	StartDate string                     `json:"start_date"`
	EndDate   string                     `json:"end_date"`
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

	return &Cache{
		cacheDir: cacheDir,
		ttl:      1 * time.Hour, // Default 1 hour cache
	}, nil
}

// getCacheKey generates a cache key based on username and date range
func (c *Cache) getCacheKey(username, startDate, endDate string) string {
	key := fmt.Sprintf("%s_%s_%s", username, startDate, endDate)
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x.json", hash[:8])
}

// Get retrieves cached data if it exists and is not expired
func (c *Cache) Get(username, startDate, endDate string) (*ComprehensiveUserActivity, bool) {
	cacheFile := filepath.Join(c.cacheDir, c.getCacheKey(username, startDate, endDate))

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Check if cache is expired
	if time.Since(entry.Timestamp) > c.ttl {
		// Expired - clean up
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

	cacheFile := filepath.Join(c.cacheDir, c.getCacheKey(username, startDate, endDate))
	return os.WriteFile(cacheFile, jsonData, 0644)
}

// Clear removes all cached entries
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

	return nil
}

// CleanExpired removes expired cache entries
func (c *Cache) CleanExpired() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(c.cacheDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var cacheEntry CacheEntry
		if err := json.Unmarshal(data, &cacheEntry); err != nil {
			continue
		}

		if time.Since(cacheEntry.Timestamp) > c.ttl {
			_ = os.Remove(filePath)
		}
	}

	return nil
}

