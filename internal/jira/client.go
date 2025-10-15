package jira

import (
	"fmt"
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

// Re-export jiracrawler types for convenience
type (
	Issue            = lib.Issue
	Comment          = lib.Comment
	HistoryItem      = lib.HistoryItem
	HistoryChange    = lib.HistoryChange
	TimeTracking     = lib.TimeTracking
	IssuePermissions = lib.IssuePermissions
	EnhancedFields   = lib.EnhancedFields
)

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
// This is now a simple wrapper around jiracrawler's enhanced function
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

	// Use jiracrawler's enhanced function
	result := lib.FetchUserIssuesInDateRangeWithContext(
		c.config.URL,
		c.config.Username,
		c.config.Token,
		email,
		startDateFormatted,
		endDateFormatted,
		enhancedContext,
		verbose,
	)

	if result == nil {
		return nil, fmt.Errorf("failed to fetch issues from Jira")
	}

	return result.Issues, nil
}

// UserInfo represents information about the authenticated user
type UserInfo struct {
	Username    string
	DisplayName string
	Email       string
	Active      bool
}

// VerifyAuthentication checks if the authentication is working and returns user info
// Now uses jiracrawler's function
func (c *Client) VerifyAuthentication() (*UserInfo, error) {
	// Use jiracrawler to verify by attempting to fetch issues for a minimal date range
	// jiracrawler handles authentication internally
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	result := lib.FetchUserIssuesInDateRange(
		c.config.URL,
		c.config.Username,
		c.config.Token,
		c.config.Username,
		yesterday,
		today,
	)

	if result == nil {
		return nil, fmt.Errorf("authentication failed - could not connect to Jira")
	}

	// If we got here, authentication worked
	// Return basic user info based on what we have
	return &UserInfo{
		Username:    c.config.Username,
		DisplayName: c.config.Username, // jiracrawler doesn't return display name from this call
		Email:       c.config.Username,
		Active:      true,
	}, nil
}

// TestConnection tests the Jira connection by attempting to fetch a minimal query
func (c *Client) TestConnection() error {
	// Try to verify authentication
	userInfo, err := c.VerifyAuthentication()
	if err != nil {
		fmt.Printf("  Warning: Could not verify user details (%v)\n", err)
		fmt.Printf("  Note: Enhanced context (comments, history) may be limited\n")
		return nil // Non-fatal - jiracrawler might still work
	}

	fmt.Printf("  Authenticated as: %s (%s)\n", userInfo.DisplayName, userInfo.Email)
	return nil
}
