package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	ghclient "github.com/redhat-best-practices-for-k8s/perfdive/internal/github"
	"github.com/redhat-best-practices-for-k8s/perfdive/internal/jira"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage perfdive cache",
	Long:  `View cache statistics, clear cache, or perform other cache management operations.`,
}

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Display cache statistics",
	Long:  `Show detailed statistics about the perfdive cache including hit rates, sizes, and expiration times.`,
	Run:   runCacheStats,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all cached data",
	Long:  `Remove all cached data including GitHub activity, PRs, issues, and Jira issues.`,
	Run:   runCacheClear,
}

var cacheCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove expired cache entries",
	Long:  `Remove only expired cache entries, keeping valid cached data.`,
	Run:   runCacheClean,
}

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheStatsCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheCleanCmd)
}

func runCacheStats(cmd *cobra.Command, args []string) {
	fmt.Println("Cache Statistics")
	fmt.Println("================")
	fmt.Println()

	// Get cache directory path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to get home directory: %v\n", err)
		os.Exit(1)
	}
	cacheDir := filepath.Join(homeDir, ".perfdive", "cache")

	// GitHub cache stats
	fmt.Println("GitHub Cache:")
	fmt.Println("-------------")
	ghCache, err := ghclient.NewCache()
	if err != nil {
		fmt.Printf("  Error loading GitHub cache: %v\n", err)
	} else {
		ghStats := ghCache.GetCacheStats()
		fmt.Printf("  Total entries:     %d\n", ghStats["total"])
		fmt.Printf("  Activity entries:  %d (TTL: 1 hour)\n", ghStats["activity"])
		fmt.Printf("  PR entries:        %d (TTL: 24 hours)\n", ghStats["prs"])
		fmt.Printf("  Issue entries:     %d (TTL: 24 hours)\n", ghStats["issues"])

		// Get detailed info from metadata
		ghMetadata := ghCache.GetDetailedStats()
		if ghMetadata != nil {
			fmt.Printf("  Oldest entry:      %s\n", formatTimeAgo(ghMetadata.OldestEntry))
			fmt.Printf("  Newest entry:      %s\n", formatTimeAgo(ghMetadata.NewestEntry))
			fmt.Printf("  Expired entries:   %d\n", ghMetadata.ExpiredCount)
		}
	}
	fmt.Println()

	// Jira cache stats
	fmt.Println("Jira Cache:")
	fmt.Println("-----------")
	jiraCache, err := jira.NewCache()
	if err != nil {
		fmt.Printf("  Error loading Jira cache: %v\n", err)
	} else {
		jiraStats := jiraCache.GetCacheStats()
		fmt.Printf("  Total entries:     %v\n", jiraStats["total"])
		fmt.Printf("  TTL:               %v\n", jiraStats["ttl"])

		// Get detailed info
		jiraMetadata := jiraCache.GetDetailedStats()
		if jiraMetadata != nil {
			fmt.Printf("  Oldest entry:      %s\n", formatTimeAgo(jiraMetadata.OldestEntry))
			fmt.Printf("  Newest entry:      %s\n", formatTimeAgo(jiraMetadata.NewestEntry))
			fmt.Printf("  Expired entries:   %d\n", jiraMetadata.ExpiredCount)
		}
	}
	fmt.Println()

	// Cache directory size
	size, count, err := getCacheDirStats(cacheDir)
	if err != nil {
		fmt.Printf("Cache directory: %s (error reading: %v)\n", cacheDir, err)
	} else {
		fmt.Printf("Cache Directory: %s\n", cacheDir)
		fmt.Printf("  Total files:       %d\n", count)
		fmt.Printf("  Total size:        %s\n", formatBytes(size))
	}
}

func runCacheClear(cmd *cobra.Command, args []string) {
	fmt.Println("Clearing all cache...")
	fmt.Println()

	// Clear GitHub cache
	fmt.Print("Clearing GitHub cache... ")
	ghCache, err := ghclient.NewCache()
	if err != nil {
		fmt.Printf("failed: %v\n", err)
	} else {
		if err := ghCache.Clear(); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			fmt.Println("done")
		}
	}

	// Clear Jira cache
	fmt.Print("Clearing Jira cache... ")
	jiraCache, err := jira.NewCache()
	if err != nil {
		fmt.Printf("failed: %v\n", err)
	} else {
		if err := jiraCache.Clear(); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			fmt.Println("done")
		}
	}

	fmt.Println()
	fmt.Println("Cache cleared successfully.")
}

func runCacheClean(cmd *cobra.Command, args []string) {
	fmt.Println("Cleaning expired cache entries...")
	fmt.Println()

	var totalCleaned int

	// Clean GitHub cache
	fmt.Print("Cleaning GitHub cache... ")
	ghCache, err := ghclient.NewCache()
	if err != nil {
		fmt.Printf("failed: %v\n", err)
	} else {
		before := ghCache.GetCacheStats()["total"]
		if err := ghCache.CleanExpired(); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			after := ghCache.GetCacheStats()["total"]
			cleaned := before - after
			totalCleaned += cleaned
			fmt.Printf("removed %d expired entries\n", cleaned)
		}
	}

	// Clean Jira cache
	fmt.Print("Cleaning Jira cache... ")
	jiraCache, err := jira.NewCache()
	if err != nil {
		fmt.Printf("failed: %v\n", err)
	} else {
		before := jiraCache.GetCacheStats()["total"].(int)
		if err := jiraCache.CleanExpired(); err != nil {
			fmt.Printf("failed: %v\n", err)
		} else {
			after := jiraCache.GetCacheStats()["total"].(int)
			cleaned := before - after
			totalCleaned += cleaned
			fmt.Printf("removed %d expired entries\n", cleaned)
		}
	}

	fmt.Println()
	fmt.Printf("Cleaned %d expired entries total.\n", totalCleaned)
}

// formatTimeAgo formats a time as a human-readable "time ago" string
func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}

	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// formatBytes formats a byte count as a human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// getCacheDirStats returns the total size and file count of the cache directory
func getCacheDirStats(path string) (int64, int, error) {
	var size int64
	var count int

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
			count++
		}
		return nil
	})

	return size, count, err
}
