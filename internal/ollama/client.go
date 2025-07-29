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

// GenerateSummary generates a combined summary with separate Jira and GitHub sections
func (c *Client) GenerateSummary(req SummaryRequest) (string, error) {
	var result strings.Builder

	// Generate Jira summary
	jiraSummary, err := c.generateJiraSummary(req)
	if err != nil {
		return "", fmt.Errorf("failed to generate Jira summary: %w", err)
	}

	// Generate GitHub summary
	githubSummary, err := c.generateGitHubSummary(req)
	if err != nil {
		return "", fmt.Errorf("failed to generate GitHub summary: %w", err)
	}

	// Combine the results
	result.WriteString("**JIRA PROJECT WORK SUMMARY**\n\n")
	result.WriteString(jiraSummary)
	result.WriteString("\n\n")

	result.WriteString("**GITHUB DEVELOPMENT SUMMARY**\n\n")
	result.WriteString(githubSummary)
	result.WriteString("\n\n")

	// Add quantitative summary
	result.WriteString("**PERFORMANCE METRICS**\n\n")
	result.WriteString(c.buildQuantitativeSummary(req))

	return result.String(), nil
}

// generateJiraSummary creates a focused summary of Jira work
func (c *Client) generateJiraSummary(req SummaryRequest) (string, error) {
	prompt := c.buildJiraPrompt(req)
	return c.callOllama(req.Model, prompt)
}

// generateGitHubSummary creates a focused summary of GitHub work
func (c *Client) generateGitHubSummary(req SummaryRequest) (string, error) {
	// Check if there's meaningful GitHub activity to analyze
	if !c.hasMeaningfulGitHubActivity(req) {
		userName := req.Email
		if req.DisplayName != "" {
			userName = req.DisplayName
		}

		return fmt.Sprintf("No meaningful GitHub development activity found for %s during the specified period (%s to %s).\n\nWhile some GitHub events may have been detected, there were no pull requests created or issues reported that would indicate active development contributions.",
			userName, req.StartDate, req.EndDate), nil
	}

	prompt := c.buildGitHubPrompt(req)
	return c.callOllama(req.Model, prompt)
}

// hasMeaningfulGitHubActivity checks if there are meaningful GitHub contributions (PRs or issues)
func (c *Client) hasMeaningfulGitHubActivity(req SummaryRequest) bool {
	if req.GitHubContext == nil || req.GitHubContext.ComprehensiveActivity == nil {
		return false
	}

	activity := req.GitHubContext.ComprehensiveActivity
	return len(activity.PullRequests) > 0 || len(activity.Issues) > 0
}

// callOllama makes the actual API call to Ollama
func (c *Client) callOllama(model, prompt string) (string, error) {
	ollamaReq := GenerateRequest{
		Model:  model,
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

// buildJiraPrompt creates a focused prompt for analyzing Jira work
func (c *Client) buildJiraPrompt(req SummaryRequest) string {
	var builder strings.Builder

	userName := req.Email
	if req.DisplayName != "" {
		userName = req.DisplayName
	}

	builder.WriteString(fmt.Sprintf(
		"Analyze %s's Jira project work from %s to %s. Write a professional summary of their project management and problem-solving contributions.\n\n",
		userName, req.StartDate, req.EndDate,
	))

	builder.WriteString("Focus on:\n")
	builder.WriteString("- Issues resolved and business impact\n")
	builder.WriteString("- Project contributions across different areas\n")
	builder.WriteString("- Technical problem-solving achievements\n")
	builder.WriteString("- Collaboration and stakeholder engagement\n\n")
	builder.WriteString("IMPORTANT: Do NOT include any numerical ratings, scores, or grades. Focus on qualitative analysis only.\n\n")

	// Add Jira issues data
	c.addJiraData(&builder, req)

	return builder.String()
}

// buildGitHubPrompt creates a focused prompt for analyzing GitHub work
func (c *Client) buildGitHubPrompt(req SummaryRequest) string {
	var builder strings.Builder

	userName := req.Email
	if req.DisplayName != "" {
		userName = req.DisplayName
	}

	builder.WriteString(fmt.Sprintf(
		"Analyze %s's GitHub development contributions from %s to %s. Write a professional summary of their technical contributions and development productivity.\n\n",
		userName, req.StartDate, req.EndDate,
	))

	builder.WriteString("Focus on:\n")
	builder.WriteString("- Code contributions and technical improvements\n")
	builder.WriteString("- Repository impact and collaboration\n")
	builder.WriteString("- Development quality and productivity\n")
	builder.WriteString("- Open source community engagement\n\n")
	builder.WriteString("IMPORTANT: Do NOT include any numerical ratings, scores, or grades. Focus on qualitative analysis only.\n\n")

	// Add GitHub data
	c.addGitHubData(&builder, req)

	return builder.String()
}

// buildQuantitativeSummary creates the metrics section
func (c *Client) buildQuantitativeSummary(req SummaryRequest) string {
	var builder strings.Builder

	// Jira metrics
	builder.WriteString(fmt.Sprintf("**Jira Issues:** %d total\n", len(req.Issues)))
	if len(req.Issues) > 0 {
		projectGroups := make(map[string]int)
		for _, issue := range req.Issues {
			project := extractProjectFromKey(issue.Key)
			projectGroups[project]++
		}
		for project, count := range projectGroups {
			builder.WriteString(fmt.Sprintf("- %s: %d issues\n", project, count))
		}
	}

	// GitHub metrics
	if req.GitHubContext != nil && req.GitHubContext.ComprehensiveActivity != nil {
		activity := req.GitHubContext.ComprehensiveActivity
		totalActivity := len(activity.PullRequests) + len(activity.Issues) + len(activity.Events)
		builder.WriteString(fmt.Sprintf("\n**GitHub Contributions:** %d total\n", totalActivity))
		builder.WriteString(fmt.Sprintf("- Pull Requests: %d\n", len(activity.PullRequests)))
		builder.WriteString(fmt.Sprintf("- Issues: %d\n", len(activity.Issues)))
		builder.WriteString(fmt.Sprintf("- Other Activities: %d\n", len(activity.Events)))
	}

	return builder.String()
}

// addJiraData adds Jira issues data to the prompt builder
func (c *Client) addJiraData(builder *strings.Builder, req SummaryRequest) {
	builder.WriteString("JIRA ISSUES DATA:\n")

	if len(req.Issues) == 0 {
		builder.WriteString("No Jira issues found for this period.\n")
		return
	}

	// Group issues by project
	projectGroups := make(map[string][]jira.Issue)
	for _, issue := range req.Issues {
		project := extractProjectFromKey(issue.Key)
		projectGroups[project] = append(projectGroups[project], issue)
	}

	// Output issues grouped by project
	for project, issues := range projectGroups {
		fmt.Fprintf(builder, "\n%s PROJECT (%d issues):\n", project, len(issues))
		for _, issue := range issues {
			issueTypeDisplay := ""
			if issue.IssueType != "" {
				issueTypeDisplay = fmt.Sprintf(" (%s)", issue.IssueType)
			}
			fmt.Fprintf(builder, "- %s%s: %s [%s]\n", issue.Key, issueTypeDisplay, issue.Summary, issue.Status)
			if issue.Description != "" && len(issue.Description) > 0 {
				desc := issue.Description
				if len(desc) > 150 {
					desc = desc[:150] + "..."
				}
				fmt.Fprintf(builder, "  Context: %s\n", desc)
			}
		}
	}
}

// addGitHubData adds GitHub activity data to the prompt builder
func (c *Client) addGitHubData(builder *strings.Builder, req SummaryRequest) {
	builder.WriteString("GITHUB ACTIVITY DATA:\n")

	if req.GitHubContext == nil || req.GitHubContext.ComprehensiveActivity == nil {
		builder.WriteString("No GitHub activity data available.\n")
		return
	}

	activity := req.GitHubContext.ComprehensiveActivity

	// Summarize PRs by repository
	if len(activity.PullRequests) > 0 {
		fmt.Fprintf(builder, "\nPull Requests (%d total):\n", len(activity.PullRequests))

		repoGroups := make(map[string][]github.UserPullRequest)
		for _, pr := range activity.PullRequests {
			repoName := "unknown/repo"
			if pr.RepositoryURL != "" {
				parts := strings.Split(pr.RepositoryURL, "/")
				if len(parts) >= 2 {
					owner := parts[len(parts)-2]
					repo := parts[len(parts)-1]
					repoName = fmt.Sprintf("%s/%s", owner, repo)
				}
			}
			repoGroups[repoName] = append(repoGroups[repoName], pr)
		}

		for repoName, prs := range repoGroups {
			openCount := 0
			closedCount := 0
			for _, pr := range prs {
				if pr.State == "open" {
					openCount++
				} else {
					closedCount++
				}
			}
			fmt.Fprintf(builder, "- %s: %d PRs (%d open, %d closed/merged)\n",
				repoName, len(prs), openCount, closedCount)
		}
	}

	// Add issues if any
	if len(activity.Issues) > 0 {
		fmt.Fprintf(builder, "\nIssues Reported (%d total):\n", len(activity.Issues))
		for _, issue := range activity.Issues {
			fmt.Fprintf(builder, "- %s: %s [%s]\n", issue.HTMLURL, issue.Title, issue.State)
		}
	}
}

// TestConnection tests the Ollama connection by making a simple request
func (c *Client) TestConnection(model string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testReq := GenerateRequest{
		Model:  model,
		Prompt: "test",
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
