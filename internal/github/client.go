package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Client wraps GitHub API functionality
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	rateLimitRemaining int
	rateLimitReset     time.Time
}

// Config holds GitHub client configuration
type Config struct {
	Token string // GitHub personal access token (optional for public repos)
}

// GitHubErrorResponse represents an error response from GitHub API
type GitHubErrorResponse struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

// RateLimitResponse represents GitHub's rate limit status
type RateLimitResponse struct {
	Resources struct {
		Core struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"core"`
		Search struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"search"`
	} `json:"resources"`
}

// GitHubReference represents a parsed GitHub URL
type GitHubReference struct {
	Owner  string
	Repo   string
	Type   string // "pull" or "issues"
	Number string
	URL    string
}

// PullRequest represents GitHub PR information
type PullRequest struct {
	Number              int             `json:"number"`
	Title               string          `json:"title"`
	Body                string          `json:"body"`
	State               string          `json:"state"`
	User                User            `json:"user"`
	CreatedAt           string          `json:"created_at"`
	UpdatedAt           string          `json:"updated_at"`
	MergedAt            string          `json:"merged_at"`
	Commits             int             `json:"commits"`
	Additions           int             `json:"additions"`
	Deletions           int             `json:"deletions"`
	ChangedFiles        int             `json:"changed_files"`
	ReviewCommentsCount int             `json:"review_comments"` // This is a count from GitHub API
	ReviewComments      []ReviewComment `json:"-"`               // Populated separately if enhanced context is enabled
	FilesChanged        []FileChange    `json:"-"`               // Populated separately if enhanced context is enabled
	CodeDiff            string          `json:"-"`               // Populated separately if enhanced context is enabled
}

// Issue represents GitHub issue information
type Issue struct {
	Number        int            `json:"number"`
	Title         string         `json:"title"`
	Body          string         `json:"body"`
	State         string         `json:"state"`
	User          User           `json:"user"`
	Labels        []Label        `json:"labels"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	ClosedAt      string         `json:"closed_at"`
	CommentsCount int            `json:"comments"`           // Number of comments from basic API
	Comments      []IssueComment `json:"-"`                  // Populated separately if enhanced context is enabled
}

// User represents a GitHub user
type User struct {
	Login string `json:"login"`
	ID    int    `json:"id"`
}

// Label represents a GitHub label
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// JiraIssue represents basic Jira issue information needed for GitHub parsing
type JiraIssue struct {
	Key         string
	Summary     string
	Description string
}

// UserActivity represents a GitHub user's activity
type UserActivity struct {
	Type      string  `json:"type"`
	CreatedAt string  `json:"created_at"`
	Repo      Repo    `json:"repo"`
	Payload   Payload `json:"payload"`
}

// Repo represents a GitHub repository
type Repo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Payload represents the payload of a GitHub event
type Payload struct {
	Action  string   `json:"action"`
	Number  int      `json:"number"`
	Ref     string   `json:"ref"`
	Size    int      `json:"size"`
	Commits []Commit `json:"commits"`
}

// Commit represents a GitHub commit
type Commit struct {
	Message string `json:"message"`
	URL     string `json:"url"`
	SHA     string `json:"sha"`
}

// UserSearchResult represents GitHub user search results
type UserSearchResult struct {
	Items []GitHubUser `json:"items"`
}

// GitHubUser represents a GitHub user
type GitHubUser struct {
	Login     string `json:"login"`
	ID        int    `json:"id"`
	AvatarURL string `json:"avatar_url"`
	URL       string `json:"url"`
}

// GitHubContext holds all GitHub information related to a Jira issue
type GitHubContext struct {
	References            []GitHubReference          `json:"references"`
	PullRequests          []PullRequest              `json:"pullRequests"`
	Issues                []Issue                    `json:"issues"`
	UserActivity          []UserActivity             `json:"userActivity"` // Legacy events API activity
	GitHubUsername        string                     `json:"githubUsername"`
	ComprehensiveActivity *ComprehensiveUserActivity `json:"comprehensiveActivity,omitempty"` // Enhanced activity from multiple sources
}

// ReviewComment represents a GitHub PR review comment
type ReviewComment struct {
	ID        int    `json:"id"`
	User      User   `json:"user"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Position  int    `json:"position"`
	Line      int    `json:"line"`
	CreatedAt string `json:"created_at"`
}

// IssueComment represents a GitHub issue comment
type IssueComment struct {
	ID        int    `json:"id"`
	User      User   `json:"user"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// FileChange represents a file that was changed in a PR
type FileChange struct {
	Filename   string `json:"filename"`
	Status     string `json:"status"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	Changes    int    `json:"changes"`
	Patch      string `json:"patch,omitempty"`
	FileType   string `json:"file_type,omitempty"`
	IsTestFile bool   `json:"is_test_file,omitempty"`
	IsDocFile  bool   `json:"is_doc_file,omitempty"`
}

// NewClient creates a new GitHub API client
func NewClient(config Config) *Client {
	return &Client{
		baseURL: "https://api.github.com",
		token:   config.Token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ExtractGitHubReferences finds all GitHub URLs in text and parses them
func (c *Client) ExtractGitHubReferences(text string) []GitHubReference {
	// Regular expression to match GitHub URLs
	// Matches: https://github.com/owner/repo/pull/123 or https://github.com/owner/repo/issues/456
	githubRegex := regexp.MustCompile(`https://github\.com/([^/]+)/([^/]+)/(pull|issues)/(\d+)`)

	matches := githubRegex.FindAllStringSubmatch(text, -1)
	var references []GitHubReference

	for _, match := range matches {
		if len(match) == 5 {
			references = append(references, GitHubReference{
				Owner:  match[1],
				Repo:   match[2],
				Type:   match[3],
				Number: match[4],
				URL:    match[0],
			})
		}
	}

	return references
}

// FetchGitHubContextFromJiraIssues retrieves GitHub context for all references found in Jira issues
func (c *Client) FetchGitHubContextFromJiraIssues(jiraIssues []JiraIssue) (*GitHubContext, error) {
	context := &GitHubContext{
		References:   []GitHubReference{},
		PullRequests: []PullRequest{},
		Issues:       []Issue{},
	}

	// Extract all GitHub references from Jira issue content
	for _, issue := range jiraIssues {
		// Search in summary and description
		refs := c.ExtractGitHubReferences(issue.Summary + " " + issue.Description)
		context.References = append(context.References, refs...)
	}

	// Remove duplicates
	context.References = c.deduplicateReferences(context.References)

	// Fetch details for each reference with enhanced context
	for _, ref := range context.References {
		if ref.Type == "pull" {
			pr, err := c.fetchEnhancedPullRequest(ref.Owner, ref.Repo, ref.Number)
			if err != nil {
				fmt.Printf("Warning: failed to fetch PR %s: %v\n", ref.URL, err)
				continue
			}
			context.PullRequests = append(context.PullRequests, *pr)
		} else if ref.Type == "issues" {
			issue, err := c.fetchEnhancedIssue(ref.Owner, ref.Repo, ref.Number)
			if err != nil {
				fmt.Printf("Warning: failed to fetch issue %s: %v\n", ref.URL, err)
				continue
			}
			context.Issues = append(context.Issues, *issue)
		}
	}

	return context, nil
}

// makeGitHubRequest makes an HTTP request to GitHub API with retry logic for rate limits and public repos
func (c *Client) makeGitHubRequest(url string, target interface{}) (interface{}, error) {
	maxRetries := 3
	baseDelay := 2 * time.Second
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Add delay for retries with exponential backoff
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<uint(attempt-1)) // 2s, 4s, 8s
			fmt.Printf("  Retrying in %v (attempt %d/%d)...\n", delay, attempt+1, maxRetries)
			time.Sleep(delay)
		}

		// First try with authentication if token provided
		if c.token != "" {
			result, err := c.doGitHubRequest(url, true, target)
			if err != nil {
				// If we get 401 (unauthorized) with a token, retry without auth for public repos
				if isUnauthorizedError(err) {
					fmt.Printf("⚠ GitHub auth failed, retrying without token for public repo access...\n")
					return c.doGitHubRequest(url, false, target)
				}
				
				// Check if it's a rate limit error - retry if not last attempt
				if isRateLimitError(err) && attempt < maxRetries-1 {
					fmt.Printf("⚠ %v\n", err)
					continue
				}
				
				// Check if it's a secondary rate limit (abuse detection) - longer wait
				if isSecondaryRateLimitError(err) && attempt < maxRetries-1 {
					fmt.Printf("⚠ %v\n", err)
					fmt.Printf("  Waiting 60s for secondary rate limit reset...\n")
					time.Sleep(60 * time.Second)
					continue
				}
				
				return nil, err
			}
			return result, nil
		}

		// No token provided, try without auth
		result, err := c.doGitHubRequest(url, false, target)
		if err != nil {
			// Retry on rate limit errors
			if (isRateLimitError(err) || isSecondaryRateLimitError(err)) && attempt < maxRetries-1 {
				fmt.Printf("⚠ %v\n", err)
				continue
			}
			return nil, err
		}
		return result, nil
	}
	
	return nil, fmt.Errorf("GitHub API request failed after %d retries", maxRetries)
}

// doGitHubRequest performs the actual HTTP request with rate limit handling
func (c *Client) doGitHubRequest(url string, useAuth bool, target interface{}) (interface{}, error) {
	// Check if we need to wait for rate limit reset
	if !c.rateLimitReset.IsZero() && c.rateLimitRemaining <= 1 && time.Now().Before(c.rateLimitReset) {
		waitTime := time.Until(c.rateLimitReset)
		fmt.Printf("⚠ Rate limit exceeded. Waiting %v until reset...\n", waitTime.Round(time.Second))
		time.Sleep(waitTime + time.Second) // Add 1 second buffer
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if useAuth && c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Update rate limit information from headers
	c.updateRateLimitFromHeaders(resp)

	if resp.StatusCode != 200 {
		return nil, c.handleErrorResponse(resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return nil, err
	}

	return target, nil
}

// updateRateLimitFromHeaders updates the client's rate limit state from response headers
func (c *Client) updateRateLimitFromHeaders(resp *http.Response) {
	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
		var remainingVal int
		if _, err := fmt.Sscanf(remaining, "%d", &remainingVal); err == nil {
			c.rateLimitRemaining = remainingVal
		}
	}
	
	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		var resetTimestamp int64
		if _, err := fmt.Sscanf(reset, "%d", &resetTimestamp); err == nil {
			c.rateLimitReset = time.Unix(resetTimestamp, 0)
		}
	}
}

// handleErrorResponse parses GitHub error responses and returns a detailed error
func (c *Client) handleErrorResponse(resp *http.Response) error {
	var errorResp GitHubErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil && errorResp.Message != "" {
		// Check for specific error types
		if resp.StatusCode == 403 {
			// Could be rate limit or authentication issue
			if strings.Contains(strings.ToLower(errorResp.Message), "rate limit") ||
			   strings.Contains(strings.ToLower(errorResp.Message), "api rate limit") {
				return fmt.Errorf("GitHub API rate limit exceeded: %s", errorResp.Message)
			}
			if strings.Contains(strings.ToLower(errorResp.Message), "abuse") {
				return fmt.Errorf("GitHub API abuse detection triggered (secondary rate limit): %s", errorResp.Message)
			}
			return fmt.Errorf("GitHub API access forbidden: %s", errorResp.Message)
		}
		return fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, errorResp.Message)
	}

	// Fallback to generic error if we can't parse the response
	return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
}

// isUnauthorizedError checks if an error is a 401 unauthorized error
func isUnauthorizedError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "GitHub API returned status 401")
}

// isRateLimitError checks if an error is a rate limit error
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "rate limit exceeded") || 
	       strings.Contains(errMsg, "api rate limit")
}

// isSecondaryRateLimitError checks if an error is a secondary rate limit (abuse detection) error
func isSecondaryRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "abuse detection")
}

// fetchPullRequest retrieves PR details from GitHub API
func (c *Client) fetchPullRequest(owner, repo, number string) (*PullRequest, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%s", c.baseURL, owner, repo, number)

	// Try with authentication first (if token provided), retry without auth on 401
	result, err := c.makeGitHubRequest(url, &PullRequest{})
	if err != nil {
		return nil, err
	}

	return result.(*PullRequest), nil
}

// fetchIssue retrieves issue details from GitHub API
func (c *Client) fetchIssue(owner, repo, number string) (*Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%s", c.baseURL, owner, repo, number)

	// Try with authentication first (if token provided), retry without auth on 401
	result, err := c.makeGitHubRequest(url, &Issue{})
	if err != nil {
		return nil, err
	}

	return result.(*Issue), nil
}

// deduplicateReferences removes duplicate GitHub references
func (c *Client) deduplicateReferences(refs []GitHubReference) []GitHubReference {
	seen := make(map[string]bool)
	var unique []GitHubReference

	for _, ref := range refs {
		key := fmt.Sprintf("%s/%s/%s/%s", ref.Owner, ref.Repo, ref.Type, ref.Number)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, ref)
		}
	}

	return unique
}

// TestConnection tests GitHub API connectivity and displays rate limit status
func (c *Client) TestConnection() error {
	rateLimit, err := c.GetRateLimitStatus()
	if err != nil {
		return fmt.Errorf("failed to connect to GitHub API: %w", err)
	}

	// Display rate limit information
	if c.token != "" {
		fmt.Printf("✓ GitHub API connection OK (authenticated)\n")
		fmt.Printf("  Core API: %d/%d remaining (resets at %s)\n", 
			rateLimit.Resources.Core.Remaining, 
			rateLimit.Resources.Core.Limit,
			time.Unix(rateLimit.Resources.Core.Reset, 0).Format("15:04:05"))
		fmt.Printf("  Search API: %d/%d remaining (resets at %s)\n", 
			rateLimit.Resources.Search.Remaining, 
			rateLimit.Resources.Search.Limit,
			time.Unix(rateLimit.Resources.Search.Reset, 0).Format("15:04:05"))
	} else {
		fmt.Printf("✓ GitHub API connection OK (unauthenticated - limited to 60 requests/hour)\n")
	}

	// Warn if rate limits are low
	if rateLimit.Resources.Core.Remaining < 10 {
		fmt.Printf("⚠ Warning: Core API rate limit is low (%d remaining)\n", rateLimit.Resources.Core.Remaining)
	}
	if rateLimit.Resources.Search.Remaining < 5 {
		fmt.Printf("⚠ Warning: Search API rate limit is low (%d remaining)\n", rateLimit.Resources.Search.Remaining)
	}

	return nil
}

// GetRateLimitStatus retrieves the current rate limit status from GitHub
func (c *Client) GetRateLimitStatus() (*RateLimitResponse, error) {
	url := c.baseURL + "/rate_limit"

	var rateLimit RateLimitResponse
	_, err := c.makeGitHubRequest(url, &rateLimit)
	if err != nil {
		return nil, err
	}

	return &rateLimit, nil
}

// SearchUserByEmail searches for a GitHub user by email address
func (c *Client) SearchUserByEmail(email string) (string, error) {
	// GitHub search API endpoint for users
	url := fmt.Sprintf("%s/search/users?q=%s+in:email", c.baseURL, email)

	var searchResult UserSearchResult
	result, err := c.makeGitHubRequest(url, &searchResult)
	if err != nil {
		return "", err
	}

	userSearchResult := result.(*UserSearchResult)
	if len(userSearchResult.Items) == 0 {
		return "", fmt.Errorf("no GitHub user found with email %s (email may be private or user may use different email for GitHub)", email)
	}

	// Return the first match (most relevant)
	return userSearchResult.Items[0].Login, nil
}

// FetchUserActivity retrieves a user's recent GitHub activity
func (c *Client) FetchUserActivity(username string) ([]UserActivity, error) {
	url := fmt.Sprintf("%s/users/%s/events", c.baseURL, username)

	var activities []UserActivity
	result, err := c.makeGitHubRequest(url, &activities)
	if err != nil {
		return nil, err
	}

	return *result.(*[]UserActivity), nil
}

// PullRequestSearchResult represents the search result structure for PRs
type PullRequestSearchResult struct {
	Items []UserPullRequest `json:"items"`
}

// FetchUserPullRequests retrieves pull requests created by a user across all repos with pagination
func (c *Client) FetchUserPullRequests(username string) ([]UserPullRequest, error) {
	var allPRs []UserPullRequest
	page := 1
	perPage := 100

	for {
		// Search for PRs created by the user with pagination
		url := fmt.Sprintf("%s/search/issues?q=type:pr+author:%s&sort=created&order=desc&per_page=%d&page=%d",
			c.baseURL, username, perPage, page)

		var searchResult PullRequestSearchResult

		result, err := c.makeGitHubRequest(url, &searchResult)
		if err != nil {
			return allPRs, err // Return what we have so far
		}

		prs := result.(*PullRequestSearchResult).Items
		if len(prs) == 0 {
			break // No more results
		}

		allPRs = append(allPRs, prs...)

		// GitHub Search API has a limit of 1000 results (10 pages of 100)
		// Also break if we got less than perPage results (indicates last page)
		if len(prs) < perPage || page >= 10 {
			break
		}

		page++
	}

	return allPRs, nil
}

// IssueSearchResult represents the search result structure for issues
type IssueSearchResult struct {
	Items []UserIssue `json:"items"`
}

// FetchUserIssues retrieves issues created by a user across all repos with pagination
func (c *Client) FetchUserIssues(username string) ([]UserIssue, error) {
	var allIssues []UserIssue
	page := 1
	perPage := 100

	for {
		// Search for issues created by the user with pagination
		url := fmt.Sprintf("%s/search/issues?q=type:issue+author:%s&sort=created&order=desc&per_page=%d&page=%d",
			c.baseURL, username, perPage, page)

		var searchResult IssueSearchResult

		result, err := c.makeGitHubRequest(url, &searchResult)
		if err != nil {
			return allIssues, err // Return what we have so far
		}

		issues := result.(*IssueSearchResult).Items
		if len(issues) == 0 {
			break // No more results
		}

		allIssues = append(allIssues, issues...)

		// GitHub Search API has a limit of 1000 results (10 pages of 100)
		// Also break if we got less than perPage results (indicates last page)
		if len(issues) < perPage || page >= 10 {
			break
		}

		page++
	}

	return allIssues, nil
}

// UserPullRequest represents a PR from search results
type UserPullRequest struct {
	Number        int     `json:"number"`
	Title         string  `json:"title"`
	Body          string  `json:"body"`
	State         string  `json:"state"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	HTMLURL       string  `json:"html_url"`
	RepositoryURL string  `json:"repository_url"`
	User          User    `json:"user"`
	Labels        []Label `json:"labels"`
}

// UserIssue represents an issue from search results
type UserIssue struct {
	Number        int     `json:"number"`
	Title         string  `json:"title"`
	Body          string  `json:"body"`
	State         string  `json:"state"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	HTMLURL       string  `json:"html_url"`
	RepositoryURL string  `json:"repository_url"`
	User          User    `json:"user"`
	Labels        []Label `json:"labels"`
}

// FilterActivityByDateRange filters user activity to a specific date range
func (c *Client) FilterActivityByDateRange(activities []UserActivity, startDate, endDate string) []UserActivity {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return activities // Return all if date parsing fails
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return activities // Return all if date parsing fails
	}

	var filtered []UserActivity
	for _, activity := range activities {
		activityTime, err := time.Parse(time.RFC3339, activity.CreatedAt)
		if err != nil {
			continue // Skip if we can't parse the date
		}

		if activityTime.After(start) && activityTime.Before(end.Add(24*time.Hour)) {
			filtered = append(filtered, activity)
		}
	}

	return filtered
}

// FetchUserGitHubActivity searches for a user by email and fetches their activity
func (c *Client) FetchUserGitHubActivity(email, startDate, endDate string) ([]UserActivity, string, error) {
	// First, try to find the GitHub user by email
	username, err := c.SearchUserByEmail(email)
	if err != nil {
		return nil, "", err
	}

	// Fetch their recent activity
	activities, err := c.FetchUserActivity(username)
	if err != nil {
		return nil, username, err
	}

	// Filter by date range
	filtered := c.FilterActivityByDateRange(activities, startDate, endDate)

	return filtered, username, nil
}

// fetchEnhancedPullRequest retrieves detailed PR information including reviews, files, and diffs
func (c *Client) fetchEnhancedPullRequest(owner, repo, number string) (*PullRequest, error) {
	// Try to get from cache first (24-hour TTL)
	cache, err := NewCache()
	if err == nil {
		if cachedPR, found := cache.GetPR(owner, repo, number); found {
			return cachedPR, nil
		}
	}

	// First fetch basic PR information
	basicPR, err := c.fetchPullRequest(owner, repo, number)
	if err != nil {
		return nil, err
	}

	// Enhance with additional context
	enhancedPR := *basicPR

	// Fetch review comments
	reviewComments, err := c.fetchPRReviewComments(owner, repo, number)
	if err != nil {
		fmt.Printf("Warning: failed to fetch review comments for PR %s/%s#%s: %v\n", owner, repo, number, err)
	} else {
		enhancedPR.ReviewComments = reviewComments
	}

	// Fetch files changed
	filesChanged, err := c.fetchPRFiles(owner, repo, number)
	if err != nil {
		fmt.Printf("Warning: failed to fetch files for PR %s/%s#%s: %v\n", owner, repo, number, err)
	} else {
		enhancedPR.FilesChanged = filesChanged
	}

	// Fetch diff (truncated for AI processing)
	diff, err := c.fetchPRDiff(owner, repo, number)
	if err != nil {
		fmt.Printf("Warning: failed to fetch diff for PR %s/%s#%s: %v\n", owner, repo, number, err)
	} else {
		enhancedPR.CodeDiff = diff
	}

	// Cache the enhanced PR (24-hour TTL)
	if cache != nil {
		_ = cache.SetPR(owner, repo, number, &enhancedPR)
	}

	return &enhancedPR, nil
}

// fetchEnhancedIssue retrieves detailed issue information including comments
func (c *Client) fetchEnhancedIssue(owner, repo, number string) (*Issue, error) {
	// Try to get from cache first (24-hour TTL)
	cache, err := NewCache()
	if err == nil {
		if cachedIssue, found := cache.GetIssue(owner, repo, number); found {
			return cachedIssue, nil
		}
	}

	// First fetch basic issue information
	basicIssue, err := c.fetchIssue(owner, repo, number)
	if err != nil {
		return nil, err
	}

	// Enhance with additional context
	enhancedIssue := *basicIssue

	// Fetch issue comments
	comments, err := c.fetchIssueComments(owner, repo, number)
	if err != nil {
		fmt.Printf("Warning: failed to fetch comments for issue %s/%s#%s: %v\n", owner, repo, number, err)
	} else {
		enhancedIssue.Comments = comments
	}

	// Cache the enhanced issue (24-hour TTL)
	if cache != nil {
		_ = cache.SetIssue(owner, repo, number, &enhancedIssue)
	}

	return &enhancedIssue, nil
}

// fetchPRReviewComments retrieves review comments for a PR
func (c *Client) fetchPRReviewComments(owner, repo, number string) ([]ReviewComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%s/comments", c.baseURL, owner, repo, number)

	var comments []ReviewComment
	result, err := c.makeGitHubRequest(url, &comments)
	if err != nil {
		return nil, err
	}

	reviewComments := *result.(*[]ReviewComment)

	// Limit to most recent/relevant comments to avoid overwhelming AI
	if len(reviewComments) > 20 {
		reviewComments = reviewComments[:20]
	}

	return reviewComments, nil
}

// fetchPRFiles retrieves files changed in a PR
func (c *Client) fetchPRFiles(owner, repo, number string) ([]FileChange, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%s/files", c.baseURL, owner, repo, number)

	var files []FileChange
	result, err := c.makeGitHubRequest(url, &files)
	if err != nil {
		return nil, err
	}

	fileChanges := *result.(*[]FileChange)

	// Analyze and categorize files
	for i := range fileChanges {
		fileChanges[i].FileType = c.categorizeFileType(fileChanges[i].Filename)
		fileChanges[i].IsTestFile = c.isTestFile(fileChanges[i].Filename)
		fileChanges[i].IsDocFile = c.isDocumentationFile(fileChanges[i].Filename)

		// Truncate large patches to avoid overwhelming AI
		if len(fileChanges[i].Patch) > 2000 {
			fileChanges[i].Patch = fileChanges[i].Patch[:2000] + "\n... (truncated)"
		}
	}

	return fileChanges, nil
}

// fetchPRDiff retrieves the full diff for a PR (truncated for AI processing)
func (c *Client) fetchPRDiff(owner, repo, number string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%s", c.baseURL, owner, repo, number)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	// Read and truncate diff to manageable size for AI
	diff := make([]byte, 5000) // Limit to 5KB
	n, _ := resp.Body.Read(diff)
	diffStr := string(diff[:n])

	if n == 5000 {
		diffStr += "\n... (diff truncated for AI processing)"
	}

	return diffStr, nil
}

// fetchIssueComments retrieves comments for a GitHub issue
func (c *Client) fetchIssueComments(owner, repo, number string) ([]IssueComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%s/comments", c.baseURL, owner, repo, number)

	var comments []IssueComment
	result, err := c.makeGitHubRequest(url, &comments)
	if err != nil {
		return nil, err
	}

	issueComments := *result.(*[]IssueComment)

	// Limit to most recent comments to avoid overwhelming AI
	if len(issueComments) > 10 {
		issueComments = issueComments[:10]
	}

	return issueComments, nil
}

// categorizeFileType determines the type of file based on extension
func (c *Client) categorizeFileType(filename string) string {
	ext := strings.ToLower(filename[strings.LastIndex(filename, ".")+1:])

	switch ext {
	case "go", "java", "py", "js", "ts", "cpp", "c", "h", "rs", "rb", "php":
		return "source_code"
	case "md", "txt", "rst", "adoc":
		return "documentation"
	case "json", "yaml", "yml", "xml", "toml":
		return "configuration"
	case "sql":
		return "database"
	case "dockerfile", "makefile":
		return "build"
	case "test.go", "spec.js", "test.py":
		return "test"
	default:
		return "other"
	}
}

// isTestFile determines if a file is a test file
func (c *Client) isTestFile(filename string) bool {
	lowerName := strings.ToLower(filename)
	return strings.Contains(lowerName, "test") ||
		strings.Contains(lowerName, "spec") ||
		strings.Contains(lowerName, "_test.") ||
		strings.Contains(lowerName, ".test.")
}

// isDocumentationFile determines if a file is documentation
func (c *Client) isDocumentationFile(filename string) bool {
	lowerName := strings.ToLower(filename)
	return strings.HasSuffix(lowerName, ".md") ||
		strings.HasSuffix(lowerName, ".txt") ||
		strings.HasSuffix(lowerName, ".rst") ||
		strings.Contains(lowerName, "readme") ||
		strings.Contains(lowerName, "doc") ||
		strings.Contains(lowerName, "changelog")
}

// FetchComprehensiveUserActivity fetches user activity from multiple sources with caching
func (c *Client) FetchComprehensiveUserActivity(username, startDate, endDate string) (*ComprehensiveUserActivity, error) {
	return c.FetchComprehensiveUserActivityWithCache(username, startDate, endDate, false)
}

// FetchComprehensiveUserActivityWithCache fetches user activity with optional verbose cache logging
func (c *Client) FetchComprehensiveUserActivityWithCache(username, startDate, endDate string, verbose bool) (*ComprehensiveUserActivity, error) {
	// Try to get from cache first
	cache, err := NewCache()
	if err == nil {
		if cachedActivity, found := cache.Get(username, startDate, endDate); found {
			if verbose {
				fmt.Printf("  ✓ Using cached GitHub activity (saves API rate limit)\n")
			}
			return cachedActivity, nil
		}
	}

	activity := &ComprehensiveUserActivity{
		Username: username,
	}

	// Fetch traditional events
	events, err := c.FetchUserActivity(username)
	if err != nil {
		fmt.Printf("Warning: failed to fetch user events: %v\n", err)
	} else {
		activity.Events = c.FilterActivityByDateRange(events, startDate, endDate)
	}

	// Fetch PRs created by user
	prs, err := c.FetchUserPullRequests(username)
	if err != nil {
		fmt.Printf("Warning: failed to fetch user pull requests: %v\n", err)
	} else {
		activity.PullRequests = c.FilterPullRequestsByDateRange(prs, startDate, endDate)
	}

	// Fetch issues created by user
	issues, err := c.FetchUserIssues(username)
	if err != nil {
		fmt.Printf("Warning: failed to fetch user issues: %v\n", err)
	} else {
		activity.Issues = c.FilterIssuesByDateRange(issues, startDate, endDate)
	}

	// Cache the results
	if cache != nil {
		_ = cache.Set(username, startDate, endDate, activity)
	}

	return activity, nil
}

// ComprehensiveUserActivity holds all types of user activity
type ComprehensiveUserActivity struct {
	Username     string            `json:"username"`
	Events       []UserActivity    `json:"events"`
	PullRequests []UserPullRequest `json:"pull_requests"`
	Issues       []UserIssue       `json:"issues"`
}

// FilterPullRequestsByDateRange filters PRs by date range
func (c *Client) FilterPullRequestsByDateRange(prs []UserPullRequest, startDate, endDate string) []UserPullRequest {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return prs
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return prs
	}

	var filtered []UserPullRequest
	for _, pr := range prs {
		createdTime, err := time.Parse(time.RFC3339, pr.CreatedAt)
		if err != nil {
			continue
		}

		if createdTime.After(start) && createdTime.Before(end.Add(24*time.Hour)) {
			filtered = append(filtered, pr)
		}
	}

	return filtered
}

// FilterIssuesByDateRange filters issues by date range
func (c *Client) FilterIssuesByDateRange(issues []UserIssue, startDate, endDate string) []UserIssue {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return issues
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return issues
	}

	var filtered []UserIssue
	for _, issue := range issues {
		createdTime, err := time.Parse(time.RFC3339, issue.CreatedAt)
		if err != nil {
			continue
		}

		if createdTime.After(start) && createdTime.Before(end.Add(24*time.Hour)) {
			filtered = append(filtered, issue)
		}
	}

	return filtered
}

// Gist represents a GitHub Gist
type Gist struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Public      bool                   `json:"public"`
	Files       map[string]GistFile    `json:"files"`
	HTMLURL     string                 `json:"html_url"`
	UpdatedAt   string                 `json:"updated_at"`
}

// GistFile represents a file in a Gist
type GistFile struct {
	Filename string `json:"filename"`
	Type     string `json:"type"`
	Language string `json:"language"`
	RawURL   string `json:"raw_url"`
	Size     int    `json:"size"`
	Content  string `json:"content,omitempty"`
}

// GistUpdate represents the structure for updating a Gist
type GistUpdate struct {
	Description string                 `json:"description,omitempty"`
	Files       map[string]GistFile    `json:"files"`
}

// GetGist retrieves a Gist by ID
func (c *Client) GetGist(gistID string) (*Gist, error) {
	url := fmt.Sprintf("%s/gists/%s", c.baseURL, gistID)

	var gist Gist
	result, err := c.makeGitHubRequest(url, &gist)
	if err != nil {
		return nil, err
	}

	return result.(*Gist), nil
}

// UpdateGist updates a Gist's content
func (c *Client) UpdateGist(gistID string, update GistUpdate) (*Gist, error) {
	url := fmt.Sprintf("%s/gists/%s", c.baseURL, gistID)

	reqBody, err := json.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal update: %w", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	if c.token == "" {
		return nil, fmt.Errorf("GitHub token required for updating gists")
	}

	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var gist Gist
	if err := json.NewDecoder(resp.Body).Decode(&gist); err != nil {
		return nil, err
	}

	return &gist, nil
}

// ExtractGistIDFromURL extracts the Gist ID from a GitHub Gist URL
func ExtractGistIDFromURL(gistURL string) (string, error) {
	// Handle various Gist URL formats:
	// https://gist.github.com/username/abc123
	// https://gist.github.com/abc123
	// abc123 (just the ID)
	
	if !strings.Contains(gistURL, "gist.github.com") && !strings.Contains(gistURL, "/") {
		// Assume it's just the ID
		return gistURL, nil
	}

	parts := strings.Split(gistURL, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid gist URL format")
	}

	// Get the last non-empty part
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i], nil
		}
	}

	return "", fmt.Errorf("could not extract gist ID from URL")
}
