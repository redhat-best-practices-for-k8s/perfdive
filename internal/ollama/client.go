package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redhat-best-practices-for-k8s/perfdive/internal/github"
	"github.com/redhat-best-practices-for-k8s/perfdive/internal/jira"
)

// extractProjectFromKey extracts the project prefix from a Jira issue key
// e.g., "CNF-18498" -> "CNF", "OCPBUGS-45703" -> "OCPBUGS"
func extractProjectFromKey(key string) string {
	parts := strings.Split(key, "-")
	if len(parts) > 0 {
		return parts[0]
	}
	return "UNKNOWN"
}

// Client wraps the Ollama API client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Config holds the configuration for Ollama client
type Config struct {
	URL string
}

// GenerateRequest represents the request structure for Ollama
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// GenerateResponse represents the response structure from Ollama
type GenerateResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response"`
	Done      bool      `json:"done"`
}

// SummaryRequest contains the parameters for generating a summary
type SummaryRequest struct {
	Email         string
	DisplayName   string // User's display name from Jira (optional)
	StartDate     string
	EndDate       string
	Model         string
	Issues        []jira.Issue
	Format        string                // "text" or "json"
	GitHubContext *github.GitHubContext // Optional GitHub context
}

// NewClient creates a new Ollama client
func NewClient(config Config) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(config.URL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Allow time for model processing
		},
	}
}

// GenerateSummary generates a summary of Jira issues using Ollama
func (c *Client) GenerateSummary(req SummaryRequest) (string, error) {
	prompt := c.buildPrompt(req)

	ollamaReq := GenerateRequest{
		Model:  req.Model,
		Prompt: prompt,
		Stream: false,
	}

	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/generate", c.baseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var ollamaResp GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return ollamaResp.Response, nil
}

// buildPrompt constructs the prompt for the LLM based on the issues and request parameters
func (c *Client) buildPrompt(req SummaryRequest) string {
	var builder strings.Builder

	// Determine which name to use for the prompt
	userName := req.Email
	if req.DisplayName != "" {
		userName = req.DisplayName
	}

	// Base instruction
	builder.WriteString(fmt.Sprintf(
		"Generate a summary in 2 to 4 paragraphs about what %s has done between %s and %s based on their assigned Jira issues.\n\n",
		userName, req.StartDate, req.EndDate,
	))

	builder.WriteString("Please organize the summary by Jira project (e.g., CNF, CNFCERT, OCPBUGS) to show the scope of work across different areas.\n\n")

	// Add format instruction
	if req.Format == "json" {
		builder.WriteString("Please format the response as JSON with the following structure:\n")
		builder.WriteString(`{
  "summary": "2-4 paragraph summary here",
  "period": "date range",
  "user": "user name or email",
  "total_issues": number,
  "projects": {
    "PROJECT_NAME": {
      "issue_count": number,
      "key_activities": ["activity1", "activity2"]
    }
  },
  "key_activities": ["overall activity1", "overall activity2", "overall activity3"]
}

`)
	} else {
		builder.WriteString("Please format the response as plain text.\n\n")
	}

	// Add issue data grouped by project
	builder.WriteString("Here are the Jira issues assigned to this user in the specified period, organized by project:\n\n")

	if len(req.Issues) == 0 {
		builder.WriteString("No issues were found for this user in the specified date range.\n")
	} else {
		// Group issues by project
		projectGroups := make(map[string][]jira.Issue)
		for _, issue := range req.Issues {
			project := extractProjectFromKey(issue.Key)
			projectGroups[project] = append(projectGroups[project], issue)
		}

		// Output issues grouped by project
		for project, issues := range projectGroups {
			builder.WriteString(fmt.Sprintf("=== %s PROJECT ===\n", project))
			for _, issue := range issues {
				builder.WriteString(fmt.Sprintf("%s:\n", issue.Key))
				builder.WriteString(fmt.Sprintf("- Summary: %s\n", issue.Summary))
				builder.WriteString(fmt.Sprintf("- Status: %s\n", issue.Status))
				builder.WriteString(fmt.Sprintf("- Created: %s\n", issue.Created.Format("2006-01-02")))
				builder.WriteString(fmt.Sprintf("- Updated: %s\n", issue.Updated.Format("2006-01-02")))
				if issue.Description != "" {
					// Truncate description if too long
					description := issue.Description
					if len(description) > 200 {
						description = description[:200] + "..."
					}
					builder.WriteString(fmt.Sprintf("- Description: %s\n", description))
				}

				// Add enhanced context
				if issue.Priority != "" {
					builder.WriteString(fmt.Sprintf("- Priority: %s\n", issue.Priority))
				}

				if len(issue.Labels) > 0 {
					builder.WriteString(fmt.Sprintf("- Labels: %s\n", strings.Join(issue.Labels, ", ")))
				}

				if len(issue.Components) > 0 {
					builder.WriteString(fmt.Sprintf("- Components: %s\n", strings.Join(issue.Components, ", ")))
				}

				if issue.TimeTracking != nil {
					builder.WriteString("- Time Tracking:\n")
					if issue.TimeTracking.OriginalEstimate != "" {
						builder.WriteString(fmt.Sprintf("  * Original Estimate: %s\n", issue.TimeTracking.OriginalEstimate))
					}
					if issue.TimeTracking.TimeSpent != "" {
						builder.WriteString(fmt.Sprintf("  * Time Spent: %s\n", issue.TimeTracking.TimeSpent))
					}
					if issue.TimeTracking.RemainingEstimate != "" {
						builder.WriteString(fmt.Sprintf("  * Remaining: %s\n", issue.TimeTracking.RemainingEstimate))
					}
				}

				// Add comments if available
				if len(issue.Comments) > 0 {
					builder.WriteString(fmt.Sprintf("- Comments (%d):\n", len(issue.Comments)))
					commentCount := 0
					for _, comment := range issue.Comments {
						if commentCount >= 5 { // Limit to 5 most recent/relevant comments
							break
						}
						commentBody := comment.Body
						if len(commentBody) > 200 {
							commentBody = commentBody[:200] + "..."
						}
						builder.WriteString(fmt.Sprintf("  * %s (%s): %s\n",
							comment.Author, comment.Created.Format("2006-01-02"), commentBody))
						commentCount++
					}
					if len(issue.Comments) > 5 {
						builder.WriteString(fmt.Sprintf("  ... and %d more comments\n", len(issue.Comments)-5))
					}
				}

				// Add history if available (show key transitions and changes)
				if len(issue.History) > 0 {
					builder.WriteString("- Key Changes:\n")
					historyCount := 0
					for _, historyItem := range issue.History {
						if historyCount >= 3 { // Limit to 3 most important history items
							break
						}
						for _, change := range historyItem.Items {
							// Focus on important field changes
							if change.Field == "status" || change.Field == "assignee" ||
								change.Field == "priority" || change.Field == "resolution" {
								builder.WriteString(fmt.Sprintf("  * %s (%s): %s changed from '%s' to '%s'\n",
									historyItem.Author, historyItem.Created.Format("2006-01-02"),
									change.Field, change.FromString, change.ToString))
								historyCount++
								break // Only show one change per history item to avoid clutter
							}
						}
					}
				}

				// Add custom fields if they contain relevant information
				if len(issue.CustomFields) > 0 {
					builder.WriteString("- Additional Context:\n")
					fieldCount := 0
					for fieldName, fieldValue := range issue.CustomFields {
						if fieldCount >= 3 { // Limit to 3 most relevant custom fields
							break
						}
						if fieldValue != nil && fieldValue != "" {
							builder.WriteString(fmt.Sprintf("  * %s: %v\n", fieldName, fieldValue))
							fieldCount++
						}
					}
				}

				builder.WriteString("\n")
			}
			builder.WriteString("\n")
		}
	}

	// Add GitHub context if available
	if req.GitHubContext != nil && (len(req.GitHubContext.PullRequests) > 0 || len(req.GitHubContext.Issues) > 0) {
		builder.WriteString("\n" + strings.Repeat("=", 50) + "\n")
		builder.WriteString("RELATED GITHUB ACTIVITY:\n")
		builder.WriteString(strings.Repeat("=", 50) + "\n\n")

		if len(req.GitHubContext.PullRequests) > 0 {
			builder.WriteString("GitHub Pull Requests referenced in Jira issues:\n\n")
			for _, pr := range req.GitHubContext.PullRequests {
				// Find the repository info from the references
				var repoName string
				for _, ref := range req.GitHubContext.References {
					if ref.Type == "pull" && ref.Number == fmt.Sprintf("%d", pr.Number) {
						repoName = fmt.Sprintf("%s/%s", ref.Owner, ref.Repo)
						break
					}
				}
				if repoName == "" {
					repoName = "unknown/repo"
				}

				builder.WriteString(fmt.Sprintf("%s #%d:\n", repoName, pr.Number))
				builder.WriteString(fmt.Sprintf("- Title: %s\n", pr.Title))
				builder.WriteString(fmt.Sprintf("- State: %s\n", pr.State))
				builder.WriteString(fmt.Sprintf("- Author: %s\n", pr.User.Login))
				builder.WriteString(fmt.Sprintf("- Created: %s\n", pr.CreatedAt))
				if pr.MergedAt != "" {
					builder.WriteString(fmt.Sprintf("- Merged: %s\n", pr.MergedAt))
				}
				if pr.Body != "" {
					// Truncate PR body if too long
					body := pr.Body
					if len(body) > 300 {
						body = body[:300] + "..."
					}
					builder.WriteString(fmt.Sprintf("- Description: %s\n", body))
				}
				builder.WriteString(fmt.Sprintf("- Changes: +%d/-%d lines across %d files\n", pr.Additions, pr.Deletions, pr.ChangedFiles))

				// Add file analysis
				if len(pr.FilesChanged) > 0 {
					builder.WriteString("- Files changed:\n")
					sourceFiles := 0
					testFiles := 0
					docFiles := 0
					configFiles := 0

					for _, file := range pr.FilesChanged {
						switch file.FileType {
						case "source_code":
							sourceFiles++
						case "test":
							testFiles++
						case "documentation":
							docFiles++
						case "configuration":
							configFiles++
						}

						if file.IsTestFile {
							testFiles++
						}
						if file.IsDocFile {
							docFiles++
						}
					}

					if sourceFiles > 0 {
						builder.WriteString(fmt.Sprintf("  * %d source code files\n", sourceFiles))
					}
					if testFiles > 0 {
						builder.WriteString(fmt.Sprintf("  * %d test files\n", testFiles))
					}
					if docFiles > 0 {
						builder.WriteString(fmt.Sprintf("  * %d documentation files\n", docFiles))
					}
					if configFiles > 0 {
						builder.WriteString(fmt.Sprintf("  * %d configuration files\n", configFiles))
					}

					// Include some key file details
					builder.WriteString("  * Key files:\n")
					fileCount := 0
					for _, file := range pr.FilesChanged {
						if fileCount >= 5 { // Limit to 5 key files
							break
						}
						builder.WriteString(fmt.Sprintf("    - %s (%s, +%d/-%d)\n",
							file.Filename, file.Status, file.Additions, file.Deletions))
						fileCount++
					}
				}

				// Add review comments if available
				if len(pr.ReviewComments) > 0 {
					builder.WriteString(fmt.Sprintf("- Review Comments (%d):\n", len(pr.ReviewComments)))
					commentCount := 0
					for _, comment := range pr.ReviewComments {
						if commentCount >= 3 { // Limit to 3 most relevant comments
							break
						}
						commentBody := comment.Body
						if len(commentBody) > 150 {
							commentBody = commentBody[:150] + "..."
						}
						builder.WriteString(fmt.Sprintf("  * %s: %s\n", comment.User.Login, commentBody))
						commentCount++
					}
				}

				// Add code diff summary if available
				if pr.CodeDiff != "" {
					builder.WriteString("- Code Changes Summary:\n")
					// Just include first few lines of diff for context
					diffLines := strings.Split(pr.CodeDiff, "\n")
					lineCount := 0
					for _, line := range diffLines {
						if lineCount >= 10 { // Limit to 10 lines of diff
							builder.WriteString("  ... (more changes)\n")
							break
						}
						if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "@@") {
							builder.WriteString(fmt.Sprintf("  %s\n", line))
							lineCount++
						}
					}
				}

				builder.WriteString("\n")
			}
		}

		if len(req.GitHubContext.Issues) > 0 {
			builder.WriteString("GitHub Issues referenced in Jira issues:\n\n")
			for _, issue := range req.GitHubContext.Issues {
				// Find the repository info from the references
				var repoName string
				for _, ref := range req.GitHubContext.References {
					if ref.Type == "issues" && ref.Number == fmt.Sprintf("%d", issue.Number) {
						repoName = fmt.Sprintf("%s/%s", ref.Owner, ref.Repo)
						break
					}
				}
				if repoName == "" {
					repoName = "unknown/repo"
				}

				builder.WriteString(fmt.Sprintf("%s #%d:\n", repoName, issue.Number))
				builder.WriteString(fmt.Sprintf("- Title: %s\n", issue.Title))
				builder.WriteString(fmt.Sprintf("- State: %s\n", issue.State))
				builder.WriteString(fmt.Sprintf("- Author: %s\n", issue.User.Login))
				builder.WriteString(fmt.Sprintf("- Created: %s\n", issue.CreatedAt))
				if issue.ClosedAt != "" {
					builder.WriteString(fmt.Sprintf("- Closed: %s\n", issue.ClosedAt))
				}
				if len(issue.Labels) > 0 {
					labels := make([]string, len(issue.Labels))
					for j, label := range issue.Labels {
						labels[j] = label.Name
					}
					builder.WriteString(fmt.Sprintf("- Labels: %s\n", strings.Join(labels, ", ")))
				}
				if issue.Body != "" {
					// Truncate issue body if too long
					body := issue.Body
					if len(body) > 300 {
						body = body[:300] + "..."
					}
					builder.WriteString(fmt.Sprintf("- Description: %s\n", body))
				}

				// Add comments if available
				if len(issue.Comments) > 0 {
					builder.WriteString(fmt.Sprintf("- Comments (%d):\n", len(issue.Comments)))
					commentCount := 0
					for _, comment := range issue.Comments {
						if commentCount >= 3 { // Limit to 3 most relevant comments
							break
						}
						commentBody := comment.Body
						if len(commentBody) > 150 {
							commentBody = commentBody[:150] + "..."
						}
						builder.WriteString(fmt.Sprintf("  * %s: %s\n", comment.User.Login, commentBody))
						commentCount++
					}
				}

				builder.WriteString("\n")
			}
		}
	}

	// Add GitHub user activity if available
	if req.GitHubContext != nil && len(req.GitHubContext.UserActivity) > 0 {
		builder.WriteString("\n" + strings.Repeat("=", 50) + "\n")
		builder.WriteString(fmt.Sprintf("GITHUB USER ACTIVITY (%s):\n", req.GitHubContext.GitHubUsername))
		builder.WriteString(strings.Repeat("=", 50) + "\n\n")

		activityTypes := make(map[string]int)

		builder.WriteString("GitHub activities during the specified period:\n\n")
		for i, activity := range req.GitHubContext.UserActivity {
			activityTypes[activity.Type]++

			builder.WriteString(fmt.Sprintf("Activity %d:\n", i+1))
			builder.WriteString(fmt.Sprintf("- Type: %s\n", activity.Type))
			builder.WriteString(fmt.Sprintf("- Date: %s\n", activity.CreatedAt))
			if activity.Repo.Name != "" {
				builder.WriteString(fmt.Sprintf("- Repository: %s\n", activity.Repo.Name))
			}

			// Add specific details based on activity type
			switch activity.Type {
			case "PushEvent":
				if len(activity.Payload.Commits) > 0 {
					builder.WriteString(fmt.Sprintf("- Commits: %d\n", len(activity.Payload.Commits)))
					if activity.Payload.Commits[0].Message != "" {
						message := activity.Payload.Commits[0].Message
						if len(message) > 100 {
							message = message[:100] + "..."
						}
						builder.WriteString(fmt.Sprintf("- Latest commit: %s\n", message))
					}
				}
			case "PullRequestEvent":
				if activity.Payload.Action != "" {
					builder.WriteString(fmt.Sprintf("- Action: %s\n", activity.Payload.Action))
				}
				if activity.Payload.Number > 0 {
					builder.WriteString(fmt.Sprintf("- PR Number: #%d\n", activity.Payload.Number))
				}
			case "IssuesEvent":
				if activity.Payload.Action != "" {
					builder.WriteString(fmt.Sprintf("- Action: %s\n", activity.Payload.Action))
				}
				if activity.Payload.Number > 0 {
					builder.WriteString(fmt.Sprintf("- Issue Number: #%d\n", activity.Payload.Number))
				}
			case "CreateEvent":
				if activity.Payload.Ref != "" {
					builder.WriteString(fmt.Sprintf("- Created: %s\n", activity.Payload.Ref))
				}
			}

			builder.WriteString("\n")
		}

		// Add activity summary
		builder.WriteString("Activity Summary:\n")
		for actType, count := range activityTypes {
			builder.WriteString(fmt.Sprintf("- %s: %d times\n", actType, count))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

// TestConnection tests the Ollama connection by making a simple request
func (c *Client) TestConnection(model string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testReq := GenerateRequest{
		Model:  model,
		Prompt: "Say hello",
		Stream: false,
	}

	reqBody, err := json.Marshal(testReq)
	if err != nil {
		return fmt.Errorf("failed to marshal test request: %w", err)
	}

	url := fmt.Sprintf("%s/api/generate", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to connect to Ollama: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama test request failed with status %d", resp.StatusCode)
	}

	return nil
}
