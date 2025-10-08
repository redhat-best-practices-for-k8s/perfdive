package jira

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sebrandon1/jiracrawler/lib"
)

// Client wraps the jiracrawler functionality
type Client struct {
	config Config
}

// Config holds the configuration for Jira client
type Config struct {
	URL      string
	Username string
	Token    string
}

// Issue represents a simplified Jira issue for our purposes
type Issue struct {
	Key          string
	Summary      string
	Description  string
	Status       string
	IssueType    string `json:"issue_type,omitempty"`
	Assignee     string
	Created      time.Time
	Updated      time.Time
	Comments     []Comment              `json:"comments,omitempty"`
	History      []HistoryItem          `json:"history,omitempty"`
	Labels       []string               `json:"labels,omitempty"`
	Components   []string               `json:"components,omitempty"`
	Priority     string                 `json:"priority,omitempty"`
	TimeTracking *TimeTracking          `json:"time_tracking,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

// Comment represents a Jira issue comment
type Comment struct {
	ID      string    `json:"id"`
	Author  string    `json:"author"`
	Body    string    `json:"body"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// HistoryItem represents a Jira issue history entry
type HistoryItem struct {
	ID      string          `json:"id"`
	Author  string          `json:"author"`
	Created time.Time       `json:"created"`
	Items   []HistoryChange `json:"items"`
}

// HistoryChange represents a specific field change in history
type HistoryChange struct {
	Field      string `json:"field"`
	FieldType  string `json:"field_type"`
	From       string `json:"from"`
	FromString string `json:"from_string"`
	To         string `json:"to"`
	ToString   string `json:"to_string"`
}

// TimeTracking represents time tracking information
type TimeTracking struct {
	OriginalEstimate  string `json:"original_estimate,omitempty"`
	RemainingEstimate string `json:"remaining_estimate,omitempty"`
	TimeSpent         string `json:"time_spent,omitempty"`
	WorklogTotal      string `json:"worklog_total,omitempty"`
}

// NewClient creates a new Jira client with authentication
func NewClient(config Config) (*Client, error) {
	if config.URL == "" || config.Username == "" || config.Token == "" {
		return nil, fmt.Errorf("jira URL, username, and token are required")
	}

	return &Client{
		config: config,
	}, nil
}

// GetUserIssuesInDateRange retrieves issues assigned to a user within a date range
func (c *Client) GetUserIssuesInDateRange(email, startDate, endDate string) ([]Issue, error) {
	return c.GetUserIssuesInDateRangeWithContext(email, startDate, endDate, false, false)
}

// GetUserIssuesInDateRangeWithContext retrieves issues with optional enhanced context
func (c *Client) GetUserIssuesInDateRangeWithContext(email, startDate, endDate string, enhancedContext, verbose bool) ([]Issue, error) {
	// Convert date format from MM-DD-YYYY to YYYY-MM-DD
	start, err := time.Parse("01-02-2006", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start date format (expected MM-DD-YYYY): %w", err)
	}

	end, err := time.Parse("01-02-2006", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end date format (expected MM-DD-YYYY): %w", err)
	}

	// Convert to YYYY-MM-DD format for jiracrawler
	startDateFormatted := start.Format("2006-01-02")
	endDateFormatted := end.Format("2006-01-02")

	// Use jiracrawler to fetch issues - now returns a single UserUpdatesResult struct
	result := lib.FetchUserIssuesInDateRange(
		c.config.URL,
		c.config.Username,
		c.config.Token,
		email,
		startDateFormatted,
		endDateFormatted,
	)

	if result == nil {
		return nil, fmt.Errorf("failed to fetch issues from Jira")
	}

	// Convert jiracrawler Issues to our Issue struct format
	var issues []Issue
	for _, jiraIssue := range result.Issues {
		issue, err := convertJiraCrawlerIssue(jiraIssue)
		if err != nil {
			fmt.Printf("Warning: failed to convert issue %s: %v\n", jiraIssue.Key, err)
			continue
		}

		// Enhance with additional context if requested
		if enhancedContext {
			enhancedIssue, err := c.enhanceIssueWithContext(issue, verbose)
			if err != nil {
				if verbose {
					fmt.Printf("Warning: failed to enhance issue %s with additional context: %v\n", issue.Key, err)
				}
				// Still include the basic issue even if enhancement fails
				issues = append(issues, issue)
			} else {
				issues = append(issues, enhancedIssue)
			}
		} else {
			issues = append(issues, issue)
		}
	}

	return issues, nil
}

// IssuePermissions represents permissions for an issue
type IssuePermissions struct {
	CanViewComments bool
	CanViewHistory  bool
	CanViewIssue    bool
}

// checkIssuePermissions verifies what permissions the user has for an issue
func (c *Client) checkIssuePermissions(jiraClient *JiraAPIClient, issueKey string) (IssuePermissions, error) {
	url := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=id", jiraClient.baseURL, issueKey)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return IssuePermissions{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(jiraClient.username, jiraClient.token)
	req.Header.Set("Accept", "application/json")

	resp, err := jiraClient.httpClient.Do(req)
	if err != nil {
		return IssuePermissions{}, fmt.Errorf("failed to check permissions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	permissions := IssuePermissions{
		CanViewIssue:    resp.StatusCode == http.StatusOK,
		CanViewComments: resp.StatusCode == http.StatusOK,
		CanViewHistory:  resp.StatusCode == http.StatusOK,
	}

	// If we can't view the basic issue, we definitely can't view comments or history
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		permissions.CanViewComments = false
		permissions.CanViewHistory = false
	}

	return permissions, nil
}

// enhanceIssueWithContext fetches additional context for an issue (comments, history, etc.)
func (c *Client) enhanceIssueWithContext(issue Issue, verbose bool) (Issue, error) {
	// Create a direct Jira client to fetch additional data
	jiraClient, err := c.createJiraAPIClient()
	if err != nil {
		return issue, fmt.Errorf("failed to create Jira API client: %w", err)
	}

	if verbose {
		fmt.Printf("Fetching enhanced context for issue %s...\n", issue.Key)
	}

	// Check permissions first
	permissions, err := c.checkIssuePermissions(jiraClient, issue.Key)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: failed to check permissions for %s: %v\n", issue.Key, err)
		}
		// Continue anyway, we'll get errors on individual fetches if needed
	} else if !permissions.CanViewIssue {
		fmt.Printf("Warning: insufficient permissions to view issue %s - skipping enhanced context\n", issue.Key)
		return issue, nil
	}

	// Fetch comments
	if permissions.CanViewComments {
		comments, err := c.fetchIssueComments(jiraClient, issue.Key)
		if err != nil {
			fmt.Printf("Warning: failed to fetch comments for %s: %v\n", issue.Key, err)
		} else {
			issue.Comments = comments
			if verbose {
				fmt.Printf("  ✓ Fetched %d comments for %s\n", len(comments), issue.Key)
			}
		}
	} else if verbose {
		fmt.Printf("  ⊘ Skipping comments for %s (insufficient permissions)\n", issue.Key)
	}

	// Fetch history
	if permissions.CanViewHistory {
		history, err := c.fetchIssueHistory(jiraClient, issue.Key)
		if err != nil {
			fmt.Printf("Warning: failed to fetch history for %s: %v\n", issue.Key, err)
		} else {
			issue.History = history
			if verbose {
				fmt.Printf("  ✓ Fetched %d history entries for %s\n", len(history), issue.Key)
			}
		}
	} else if verbose {
		fmt.Printf("  ⊘ Skipping history for %s (insufficient permissions)\n", issue.Key)
	}

	// Fetch additional fields that might be available
	enhancedFields, err := c.fetchEnhancedFields(jiraClient, issue.Key)
	if err != nil {
		fmt.Printf("Warning: failed to fetch enhanced fields for %s: %v\n", issue.Key, err)
	} else {
		if verbose {
			fmt.Printf("  ✓ Fetched enhanced fields for %s\n", issue.Key)
		}
		if enhancedFields.Labels != nil {
			issue.Labels = enhancedFields.Labels
		}
		if enhancedFields.Components != nil {
			issue.Components = enhancedFields.Components
		}
		if enhancedFields.Priority != "" {
			issue.Priority = enhancedFields.Priority
		}
		if enhancedFields.IssueType != "" {
			issue.IssueType = enhancedFields.IssueType
		}
		if enhancedFields.TimeTracking != nil {
			issue.TimeTracking = enhancedFields.TimeTracking
		}
		if enhancedFields.CustomFields != nil {
			issue.CustomFields = enhancedFields.CustomFields
		}
	}

	return issue, nil
}

// JiraAPIClient wraps HTTP client for direct Jira API calls
type JiraAPIClient struct {
	baseURL    string
	username   string
	token      string
	httpClient *http.Client
}

// createJiraAPIClient creates a direct HTTP client for Jira API calls
func (c *Client) createJiraAPIClient() (*JiraAPIClient, error) {
	return &JiraAPIClient{
		baseURL:    c.config.URL,
		username:   c.config.Username,
		token:      c.config.Token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// fetchIssueComments retrieves comments for a specific issue
func (c *Client) fetchIssueComments(jiraClient *JiraAPIClient, issueKey string) ([]Comment, error) {
	url := fmt.Sprintf("%s/rest/api/2/issue/%s/comment", jiraClient.baseURL, issueKey)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(jiraClient.username, jiraClient.token)
	req.Header.Set("Accept", "application/json")

	resp, err := jiraClient.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Read response body for more details
		var bodyBytes []byte
		bodyBytes, _ = io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)

		return nil, fmt.Errorf("jira API returned status %d for URL %s (user: %s): %s",
			resp.StatusCode, url, jiraClient.username, bodyStr)
	}

	var response struct {
		Comments []struct {
			ID     string `json:"id"`
			Body   string `json:"body"`
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Created string `json:"created"`
			Updated string `json:"updated"`
		} `json:"comments"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode comments response: %w", err)
	}

	var comments []Comment
	for _, c := range response.Comments {
		created, _ := time.Parse("2006-01-02T15:04:05.000-0700", c.Created)
		updated, _ := time.Parse("2006-01-02T15:04:05.000-0700", c.Updated)

		comments = append(comments, Comment{
			ID:      c.ID,
			Body:    c.Body,
			Author:  c.Author.DisplayName,
			Created: created,
			Updated: updated,
		})
	}

	return comments, nil
}

// fetchIssueHistory retrieves change history for a specific issue
func (c *Client) fetchIssueHistory(jiraClient *JiraAPIClient, issueKey string) ([]HistoryItem, error) {
	url := fmt.Sprintf("%s/rest/api/2/issue/%s?expand=changelog", jiraClient.baseURL, issueKey)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(jiraClient.username, jiraClient.token)
	req.Header.Set("Accept", "application/json")

	resp, err := jiraClient.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch history: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Read response body for more details
		var bodyBytes []byte
		bodyBytes, _ = io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)

		return nil, fmt.Errorf("jira API returned status %d for URL %s (user: %s): %s",
			resp.StatusCode, url, jiraClient.username, bodyStr)
	}

	var response struct {
		Changelog struct {
			Histories []struct {
				ID     string `json:"id"`
				Author struct {
					DisplayName string `json:"displayName"`
				} `json:"author"`
				Created string `json:"created"`
				Items   []struct {
					Field      string `json:"field"`
					FieldType  string `json:"fieldtype"`
					FromString string `json:"fromString"`
					ToString   string `json:"toString"`
				} `json:"items"`
			} `json:"histories"`
		} `json:"changelog"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode history response: %w", err)
	}

	var history []HistoryItem
	for _, h := range response.Changelog.Histories {
		created, _ := time.Parse("2006-01-02T15:04:05.000-0700", h.Created)

		var items []HistoryChange
		for _, item := range h.Items {
			items = append(items, HistoryChange{
				Field:      item.Field,
				FieldType:  item.FieldType,
				FromString: item.FromString,
				ToString:   item.ToString,
			})
		}

		history = append(history, HistoryItem{
			ID:      h.ID,
			Author:  h.Author.DisplayName,
			Created: created,
			Items:   items,
		})
	}

	return history, nil
}

// EnhancedFields holds additional fields that can be fetched
type EnhancedFields struct {
	Labels       []string
	Components   []string
	Priority     string
	IssueType    string
	TimeTracking *TimeTracking
	CustomFields map[string]interface{}
}

// fetchEnhancedFields retrieves additional field data for an issue
func (c *Client) fetchEnhancedFields(jiraClient *JiraAPIClient, issueKey string) (EnhancedFields, error) {
	url := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=labels,components,priority,issuetype,timetracking", jiraClient.baseURL, issueKey)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return EnhancedFields{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(jiraClient.username, jiraClient.token)
	req.Header.Set("Accept", "application/json")

	resp, err := jiraClient.httpClient.Do(req)
	if err != nil {
		return EnhancedFields{}, fmt.Errorf("failed to fetch enhanced fields: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Read response body for more details
		var bodyBytes []byte
		bodyBytes, _ = io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)

		return EnhancedFields{}, fmt.Errorf("jira API returned status %d for URL %s (user: %s): %s",
			resp.StatusCode, url, jiraClient.username, bodyStr)
	}

	var response struct {
		Fields struct {
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
			Components []struct {
				Name string `json:"name"`
			} `json:"components"`
			Priority struct {
				Name string `json:"name"`
			} `json:"priority"`
			IssueType struct {
				Name string `json:"name"`
			} `json:"issuetype"`
			TimeTracking struct {
				OriginalEstimate         string `json:"originalEstimate"`
				RemainingEstimate        string `json:"remainingEstimate"`
				TimeSpent                string `json:"timeSpent"`
				OriginalEstimateSeconds  int    `json:"originalEstimateSeconds"`
				RemainingEstimateSeconds int    `json:"remainingEstimateSeconds"`
				TimeSpentSeconds         int    `json:"timeSpentSeconds"`
			} `json:"timetracking"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return EnhancedFields{}, fmt.Errorf("failed to decode enhanced fields response: %w", err)
	}

	var labels []string
	for _, label := range response.Fields.Labels {
		labels = append(labels, label.Name)
	}

	var components []string
	for _, component := range response.Fields.Components {
		components = append(components, component.Name)
	}

	var timeTracking *TimeTracking
	if response.Fields.TimeTracking.OriginalEstimate != "" || response.Fields.TimeTracking.TimeSpent != "" {
		timeTracking = &TimeTracking{
			OriginalEstimate:  response.Fields.TimeTracking.OriginalEstimate,
			RemainingEstimate: response.Fields.TimeTracking.RemainingEstimate,
			TimeSpent:         response.Fields.TimeTracking.TimeSpent,
		}
	}

	return EnhancedFields{
		Labels:       labels,
		Components:   components,
		Priority:     response.Fields.Priority.Name,
		IssueType:    response.Fields.IssueType.Name,
		TimeTracking: timeTracking,
		CustomFields: make(map[string]interface{}), // For future extension
	}, nil
}

// convertJiraCrawlerIssue converts a jiracrawler Issue struct to our Issue struct
func convertJiraCrawlerIssue(jcIssue lib.Issue) (Issue, error) {
	var issue Issue

	// Basic fields
	issue.Key = jcIssue.Key
	issue.Summary = jcIssue.Summary
	issue.Description = jcIssue.Description
	issue.Status = jcIssue.Status.Name

	// Issue type
	issue.IssueType = jcIssue.IssueType.Name

	// Assignee (can be nil)
	if jcIssue.Assignee != nil {
		issue.Assignee = jcIssue.Assignee.DisplayName
	}

	// Parse created date
	if jcIssue.Created != "" {
		if created, err := time.Parse(time.RFC3339, jcIssue.Created); err == nil {
			issue.Created = created
		} else {
			return issue, fmt.Errorf("failed to parse created date: %w", err)
		}
	}

	// Parse updated date
	if jcIssue.Updated != "" {
		if updated, err := time.Parse(time.RFC3339, jcIssue.Updated); err == nil {
			issue.Updated = updated
		} else {
			return issue, fmt.Errorf("failed to parse updated date: %w", err)
		}
	}

	return issue, nil
}

// UserInfo represents information about the authenticated user
type UserInfo struct {
	Username    string
	DisplayName string
	Email       string
	Active      bool
}

// VerifyAuthentication checks if the authentication is working and returns user info
func (c *Client) VerifyAuthentication() (*UserInfo, error) {
	jiraClient, err := c.createJiraAPIClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira API client: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/2/myself", jiraClient.baseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(jiraClient.username, jiraClient.token)
	req.Header.Set("Accept", "application/json")

	resp, err := jiraClient.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to verify authentication: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		var bodyBytes []byte
		bodyBytes, _ = io.ReadAll(resp.Body)
		return nil, fmt.Errorf("authentication failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		var bodyBytes []byte
		bodyBytes, _ = io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Name         string `json:"name"`
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
		Active       bool   `json:"active"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode user info response: %w", err)
	}

	return &UserInfo{
		Username:    response.Name,
		DisplayName: response.DisplayName,
		Email:       response.EmailAddress,
		Active:      response.Active,
	}, nil
}

// TestConnection tests the Jira connection by attempting to fetch a minimal query
func (c *Client) TestConnection() error {
	// Try to verify authentication (non-fatal)
	userInfo, err := c.VerifyAuthentication()
	if err != nil {
		fmt.Printf("  Warning: Could not verify user details (%v)\n", err)
		fmt.Printf("  Note: Enhanced context (comments, history) may be limited\n")
	} else {
		fmt.Printf("  Authenticated as: %s (%s)\n", userInfo.DisplayName, userInfo.Email)
	}

	// Use jiracrawler to test connection by fetching a small date range
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	// Try to fetch issues for the current user (using username as email approximation)
	result := lib.FetchUserIssuesInDateRange(
		c.config.URL,
		c.config.Username,
		c.config.Token,
		c.config.Username,
		yesterday,
		today,
	)

	if result == nil {
		return fmt.Errorf("failed to connect to Jira - check URL, username, and token")
	}

	return nil
}
