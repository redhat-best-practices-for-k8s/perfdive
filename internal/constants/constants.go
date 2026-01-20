package constants

import "time"

// Cache TTLs
const (
	// DefaultActivityCacheTTL is the TTL for GitHub user activity cache
	DefaultActivityCacheTTL = 1 * time.Hour

	// DefaultIssueCacheTTL is the TTL for Jira issues and GitHub PRs/issues
	DefaultIssueCacheTTL = 24 * time.Hour
)

// Date formats
const (
	// DateFormatMMDDYYYY is the primary date format (MM-DD-YYYY)
	DateFormatMMDDYYYY = "01-02-2006"

	// DateFormatISO is the ISO 8601 date format (YYYY-MM-DD)
	DateFormatISO = "2006-01-02"

	// DateFormatHuman is a human-readable date format
	DateFormatHuman = "January 2, 2006"
)

// Default values
const (
	// DefaultOllamaModel is the default LLM model to use
	DefaultOllamaModel = "llama3.2:latest"

	// DefaultOllamaURL is the default Ollama API endpoint
	DefaultOllamaURL = "http://localhost:11434"

	// DefaultJiraURL is the default Jira instance URL
	DefaultJiraURL = "https://issues.redhat.com"

	// DefaultHighlightDays is the default number of days to look back for highlights
	DefaultHighlightDays = 7
)

// API limits
const (
	// DefaultReviewCommentsLimit is the max number of PR review comments to fetch
	DefaultReviewCommentsLimit = 20

	// DefaultIssueCommentsLimit is the max number of issue comments to fetch
	DefaultIssueCommentsLimit = 10

	// DefaultDiffSizeLimit is the max size of diff to fetch (5KB)
	DefaultDiffSizeLimit = 5000

	// DefaultPatchSizeLimit is the max size of patch per file
	DefaultPatchSizeLimit = 2000

	// DefaultRateLimitDelay is the delay between Jira API requests in milliseconds
	DefaultRateLimitDelay = 500
)

// Timeouts
const (
	// OllamaTimeout is the timeout for Ollama API requests
	OllamaTimeout = 5 * time.Minute

	// GitHubTimeout is the timeout for GitHub API requests
	GitHubTimeout = 30 * time.Second

	// OllamaTestTimeout is the timeout for Ollama connection test
	OllamaTestTimeout = 30 * time.Second
)

// Cache directories
const (
	// CacheBaseDir is the base directory name for cache
	CacheBaseDir = ".perfdive"

	// CacheSubDir is the subdirectory for cache files
	CacheSubDir = "cache"
)
