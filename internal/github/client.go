package github

import (
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
}

// Config holds GitHub client configuration
type Config struct {
	Token string // GitHub personal access token (optional for public repos)
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
	Number    int            `json:"number"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	State     string         `json:"state"`
	User      User           `json:"user"`
	Labels    []Label        `json:"labels"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	ClosedAt  string         `json:"closed_at"`
	Comments  []IssueComment `json:"comments,omitempty"`
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
	References     []GitHubReference `json:"references"`
	PullRequests   []PullRequest     `json:"pullRequests"`
	Issues         []Issue           `json:"issues"`
	UserActivity   []UserActivity    `json:"userActivity"`
	GitHubUsername string            `json:"githubUsername"`
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

// makeGitHubRequest makes an HTTP request to GitHub API with retry logic for public repos
func (c *Client) makeGitHubRequest(url string, target interface{}) (interface{}, error) {
	// First try with authentication if token provided
	if c.token != "" {
		result, err := c.doGitHubRequest(url, true, target)
		if err != nil {
			// If we get 401 (unauthorized) with a token, retry without auth for public repos
			if isUnauthorizedError(err) {
				fmt.Printf("⚠ GitHub auth failed, retrying without token for public repo access...\n")
				return c.doGitHubRequest(url, false, target)
			}
			return nil, err
		}
		return result, nil
	}

	// No token provided, try without auth
	return c.doGitHubRequest(url, false, target)
}

// doGitHubRequest performs the actual HTTP request
func (c *Client) doGitHubRequest(url string, useAuth bool, target interface{}) (interface{}, error) {
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
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return nil, err
	}

	return target, nil
}

// isUnauthorizedError checks if an error is a 401 unauthorized error
func isUnauthorizedError(err error) bool {
	return err != nil && err.Error() == "GitHub API returned status 401"
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

// TestConnection tests GitHub API connectivity
func (c *Client) TestConnection() error {
	url := c.baseURL + "/rate_limit"

	// For rate limit endpoint, we can use a simple map
	var result map[string]interface{}
	_, err := c.makeGitHubRequest(url, &result)
	if err != nil {
		return fmt.Errorf("failed to connect to GitHub API: %w", err)
	}

	return nil
}

// SearchUserByEmail searches for a GitHub user by email address
func (c *Client) SearchUserByEmail(email string) (string, error) {
	// GitHub search API endpoint for users
	url := fmt.Sprintf("%s/search/users?q=%s+in:email", c.baseURL, email)

	result, err := c.makeGitHubRequest(url, &UserSearchResult{})
	if err != nil {
		return "", err
	}

	searchResult := result.(*UserSearchResult)
	if len(searchResult.Items) == 0 {
		return "", fmt.Errorf("no GitHub user found with email %s (email may be private or user may use different email for GitHub)", email)
	}

	// Return the first match (most relevant)
	return searchResult.Items[0].Login, nil
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

	return &enhancedPR, nil
}

// fetchEnhancedIssue retrieves detailed issue information including comments
func (c *Client) fetchEnhancedIssue(owner, repo, number string) (*Issue, error) {
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
	defer resp.Body.Close()

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
