package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	ghclient "github.com/redhat-best-practices-for-k8s/perfdive/internal/github"
	"github.com/redhat-best-practices-for-k8s/perfdive/internal/jira"
	"github.com/redhat-best-practices-for-k8s/perfdive/internal/ollama"
)

var highlightCmd = &cobra.Command{
	Use:   "highlight [email]",
	Short: "Quick highlight summary of recent work",
	Long: `Generate a quick bullet-point summary of your recent work activity.
Default to the last 7 days if no date range is specified.

Example:
  perfdive highlight bpalm@redhat.com
  perfdive highlight bpalm@redhat.com --days 14
  perfdive highlight bpalm@redhat.com --list 5

Note: If github.gist_url is configured, highlights will be automatically appended to your journal.`,
	Args: cobra.ExactArgs(1),
	Run:  runHighlight,
}

func init() {
	rootCmd.AddCommand(highlightCmd)
	
	// Add highlight-specific flags
	highlightCmd.Flags().IntP("days", "d", 7, "Number of days to look back (default 7)")
	highlightCmd.Flags().BoolP("verbose", "v", false, "Show detailed progress information")
	highlightCmd.Flags().Bool("clear-cache", false, "Clear GitHub activity cache before running")
	highlightCmd.Flags().IntP("list", "l", 0, "List top N accomplishments instead of just the biggest (e.g., --list 5)")
}

func runHighlight(cmd *cobra.Command, args []string) {
	email := args[0]
	days, _ := cmd.Flags().GetInt("days")
	verbose, _ := cmd.Flags().GetBool("verbose")
	clearCache, _ := cmd.Flags().GetBool("clear-cache")
	listCount, _ := cmd.Flags().GetInt("list")

	// Clear cache if requested
	if clearCache {
		cache, err := ghclient.NewCache()
		if err == nil {
			if err := cache.Clear(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to clear cache: %v\n", err)
			} else {
				if verbose {
					fmt.Println("✓ Cache cleared")
				}
			}
		}
	}

	// Calculate date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)
	
	startDateStr := startDate.Format("01-02-2006")
	endDateStr := endDate.Format("01-02-2006")

	// Get configuration values
	jiraURL := viper.GetString("jira.url")
	jiraUsername := viper.GetString("jira.username")
	jiraToken := viper.GetString("jira.token")
	ollamaURL := viper.GetString("ollama.url")
	githubToken := viper.GetString("github.token")
	githubUsername := viper.GetString("github.username")
	gistURL := viper.GetString("github.gist_url")
	
	// Validate required configuration
	if jiraURL == "" || jiraUsername == "" || jiraToken == "" {
		fmt.Fprintf(os.Stderr, "Error: Jira credentials required. Set via config file or flags.\n")
		os.Exit(1)
	}

	// Check if journaling is configured (will be used automatically if gist_url is set)
	if gistURL != "" && githubToken == "" {
		fmt.Fprintf(os.Stderr, "Error: github.gist_url is configured but github.token is missing. Both are required for journaling.\n")
		os.Exit(1)
	}

	err := generateHighlight(email, startDateStr, endDateStr, jiraURL, jiraUsername, jiraToken, ollamaURL, githubToken, githubUsername, gistURL, verbose, listCount)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func generateHighlight(email, startDate, endDate, jiraURL, jiraUsername, jiraToken, ollamaURL, githubToken, githubUsername, gistURL string, verbose bool, listCount int) error {
	// Calculate days for output
	start, _ := time.Parse("01-02-2006", startDate)
	end, _ := time.Parse("01-02-2006", endDate)
	days := int(end.Sub(start).Hours() / 24)
	
	if verbose {
		fmt.Printf("Generating highlight for %s (%s to %s)\n", email, startDate, endDate)
		fmt.Printf("Date range: %d days\n\n", days)
	}
	
	// Create clients
	if verbose {
		fmt.Println("→ Creating Jira client...")
	}
	jiraClient, err := jira.NewClient(jira.Config{
		URL:      jiraURL,
		Username: jiraUsername,
		Token:    jiraToken,
	})
	if err != nil {
		return fmt.Errorf("failed to create Jira client: %w", err)
	}
	if verbose {
		fmt.Printf("  ✓ Connected to %s\n", jiraURL)
	}

	if verbose {
		fmt.Println("→ Creating GitHub client...")
	}
	githubClient := ghclient.NewClient(ghclient.Config{Token: githubToken})
	if verbose {
		if githubToken != "" {
			fmt.Println("  ✓ GitHub token configured")
		} else {
			fmt.Println("  ℹ No GitHub token (public repo access only)")
		}
	}

	// Fetch data in parallel
	type jiraResult struct {
		issues []jira.Issue
		err    error
	}
	
	type githubResult struct {
		activity *ghclient.ComprehensiveUserActivity
		username string
		err      error
	}
	
	jiraChan := make(chan jiraResult, 1)
	githubChan := make(chan githubResult, 1)

	// Fetch Jira data
	if verbose {
		fmt.Printf("\n→ Fetching Jira issues for %s...\n", email)
	}
	go func() {
		issues, err := jiraClient.GetUserIssuesInDateRangeWithContext(email, startDate, endDate, false, false)
		jiraChan <- jiraResult{issues: issues, err: err}
	}()

	// Fetch GitHub data
	if verbose {
		fmt.Println("→ Fetching GitHub activity...")
	}
	go func() {
		if githubToken == "" {
			githubChan <- githubResult{err: fmt.Errorf("no GitHub token provided")}
			return
		}

		var username string
		var activity *ghclient.ComprehensiveUserActivity
		var err error

		if githubUsername != "" {
			username = githubUsername
		} else {
			username, err = githubClient.SearchUserByEmail(email)
			if err != nil {
				githubChan <- githubResult{err: err}
				return
			}
		}

		// Convert date format for GitHub API
		start, _ := time.Parse("01-02-2006", startDate)
		end, _ := time.Parse("01-02-2006", endDate)
		startDateFormatted := start.Format("2006-01-02")
		endDateFormatted := end.Format("2006-01-02")

		activity, err = githubClient.FetchComprehensiveUserActivityWithCache(username, startDateFormatted, endDateFormatted, verbose)
		githubChan <- githubResult{activity: activity, username: username, err: err}
	}()

	// Wait for results
	jiraRes := <-jiraChan
	if verbose {
		if jiraRes.err == nil {
			fmt.Printf("  ✓ Found %d Jira issues\n", len(jiraRes.issues))
		} else {
			fmt.Printf("  ✗ Error: %v\n", jiraRes.err)
		}
	}
	
	githubRes := <-githubChan
	if verbose {
		if githubRes.err == nil && githubRes.activity != nil {
			fmt.Printf("  ✓ Found GitHub user '%s' with %d PRs, %d issues\n", 
				githubRes.username, 
				len(githubRes.activity.PullRequests),
				len(githubRes.activity.Issues))
		} else if githubRes.err != nil {
			fmt.Printf("  ℹ GitHub activity not available: %v\n", githubRes.err)
		}
	}

	if jiraRes.err != nil {
		return fmt.Errorf("failed to fetch Jira data: %w", jiraRes.err)
	}

	// Build highlight output
	var output strings.Builder
	output.WriteString("\n")
	
	// GitHub stats
	if githubRes.err == nil && githubRes.activity != nil {
		activity := githubRes.activity
		
		// Count PR stats
		created := len(activity.PullRequests)
		merged := 0
		open := 0
		
		for _, pr := range activity.PullRequests {
			switch pr.State {
			case "open":
				open++
			case "closed":
				merged++
			}
		}
		
		line := fmt.Sprintf("- Created %d PRs in the last %d days (%d merged, %d open)\n", created, days, merged, open)
		output.WriteString(line)
	}

	// Jira stats
	if len(jiraRes.issues) > 0 {
		// Count created vs updated
		created := 0
		updated := 0
		
		startTime, _ := time.Parse("01-02-2006", startDate)
		
		for _, issue := range jiraRes.issues {
			createdTime, err := time.Parse("2006-01-02T15:04:05.999-0700", issue.Created)
			if err != nil {
				// Try alternate format
				createdTime, _ = time.Parse(time.RFC3339, issue.Created)
			}
			
			if createdTime.After(startTime) {
				created++
			} else {
				updated++
			}
		}
		
		line := fmt.Sprintf("- Created %d Jira stories and updated Jira %d times\n", created, updated)
		output.WriteString(line)
	} else {
		output.WriteString("- Created 0 Jira stories and updated Jira 0 times\n")
	}

	// AI-generated accomplishment(s)
	if ollamaURL != "" {
		model := "llama3.2:latest"
		if verbose {
			fmt.Printf("\n→ Generating AI summary using Ollama...\n")
			fmt.Printf("  Model: %s\n", model)
			fmt.Printf("  Endpoint: %s\n", ollamaURL)
		}
		ollamaClient := ollama.NewClient(ollama.Config{URL: ollamaURL})
		
		if listCount > 0 {
			// Generate list of top N accomplishments
			accomplishments, err := generateAccomplishmentsList(ollamaClient, jiraRes.issues, githubRes.activity, email, verbose, model, listCount)
			if err == nil {
				if verbose {
					fmt.Printf("  ✓ AI summary generated (top %d accomplishments)\n", listCount)
				}
				output.WriteString(fmt.Sprintf("- Top %d accomplishments:\n", listCount))
				for i, acc := range accomplishments {
					output.WriteString(fmt.Sprintf("  %d. %s\n", i+1, acc))
				}
			} else {
				if verbose {
					fmt.Printf("  ✗ Failed to generate AI summary: %v\n", err)
				}
				output.WriteString(fmt.Sprintf("- Top %d accomplishments: (Unable to generate: %v)\n", listCount, err))
			}
		} else {
			// Generate single biggest accomplishment
			accomplishment, why, err := generateAccomplishmentSummary(ollamaClient, jiraRes.issues, githubRes.activity, email, verbose, model)
			if err == nil {
				if verbose {
					fmt.Println("  ✓ AI summary generated")
					if why != "" {
						fmt.Printf("\n  💡 Why this is the biggest accomplishment:\n")
						fmt.Printf("     %s\n", why)
					}
				}
				line := fmt.Sprintf("- Biggest accomplishment: %s\n", accomplishment)
				output.WriteString(line)
				
				// Add the why to journal entries (when gist_url is configured)
				if gistURL != "" && why != "" {
					output.WriteString(fmt.Sprintf("  - Why: %s\n", why))
				}
			} else {
				if verbose {
					fmt.Printf("  ✗ Failed to generate AI summary: %v\n", err)
				}
				line := fmt.Sprintf("- Biggest accomplishment: (Unable to generate: %v)\n", err)
				output.WriteString(line)
			}
		}
	}
	
	output.WriteString("\n")
	
	// Print output to console
	if verbose {
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("HIGHLIGHT SUMMARY")
		fmt.Println(strings.Repeat("=", 60))
	}
	fmt.Print(output.String())
	
	// Append to journal if gist_url is configured
	if gistURL != "" && githubToken != "" {
		if verbose {
			fmt.Printf("\n→ Updating GitHub Gist journal...\n")
		}
		err := appendToJournal(githubClient, gistURL, startDate, endDate, output.String(), verbose)
		if err != nil {
			return fmt.Errorf("failed to update journal: %w", err)
		}
		fmt.Printf("✓ Journal updated: %s\n\n", gistURL)
	}
	
	return nil
}

func generateAccomplishmentSummary(client *ollama.Client, issues []jira.Issue, activity *ghclient.ComprehensiveUserActivity, email string, verbose bool, model string) (string, string, error) {
	var prompt string
	
	// Always ask for the why, but only display it in verbose mode or journal
	prompt = "You are analyzing work activity for a Red Hat engineer to identify the biggest accomplishment.\n\n"
	prompt += "Step 1: Review the work below and identify the single most significant accomplishment.\n"
	prompt += "Step 2: Explain why THAT EXACT accomplishment matters for Red Hat, its partners, customers, and the open source community. Your explanation must directly reference and explain the specific work you identified.\n\n"
	prompt += "Format your response EXACTLY as:\n"
	prompt += "ACCOMPLISHMENT: [one concise sentence, max 15 words]\n"
	prompt += "WHY: [Reference the exact accomplishment] is significant because [explain its specific impact]. Consider the impact on: Red Hat's business objectives, partner integrations, customer deployments, and/or open source community contributions. [Add 1-2 more sentences about the concrete benefits].\n\n"
	prompt += "Example of GOOD format:\n"
	prompt += "ACCOMPLISHMENT: Migrated authentication service to OAuth 2.0\n"
	prompt += "WHY: Migrating the authentication service to OAuth 2.0 is significant because it addresses critical security vulnerabilities affecting Red Hat's enterprise customers. This modernization enables Red Hat's partners to integrate more easily with their IAM solutions, reduces security risks for customer deployments, and aligns with industry-standard open source authentication frameworks used across the community.\n\n"
	prompt += "CRITICAL: Your WHY must begin by referencing the exact accomplishment you identified. Tie the impact to Red Hat's ecosystem (company, partners, customers, or open source community). Do NOT talk about different work.\n\n"
	
	// Add Jira context
	if len(issues) > 0 {
		prompt += "JIRA WORK:\n"
		for i, issue := range issues {
			if i >= 5 {
				break // Limit to top 5
			}
			prompt += fmt.Sprintf("- %s: %s [%s]\n", issue.Key, issue.Summary, issue.Status.Name)
		}
		prompt += "\n"
	}
	
	// Add GitHub context
	if activity != nil && len(activity.PullRequests) > 0 {
		prompt += "GITHUB WORK:\n"
		for i, pr := range activity.PullRequests {
			if i >= 5 {
				break // Limit to top 5
			}
			prompt += fmt.Sprintf("- PR: %s [%s]\n", pr.Title, pr.State)
		}
		prompt += "\n"
	}
	
	// Use the exported CallOllama method for simple prompts
	response, err := client.CallOllama(model, prompt)
	if err != nil {
		return "", "", err
	}
	
	// Parse out the accomplishment and why
	accomplishment, why := parseAccomplishmentResponse(response)
	return accomplishment, why, nil
}

func generateAccomplishmentsList(client *ollama.Client, issues []jira.Issue, activity *ghclient.ComprehensiveUserActivity, email string, verbose bool, model string, count int) ([]string, error) {
	var prompt string
	
	prompt = fmt.Sprintf("You are analyzing work activity for a Red Hat engineer to identify the top %d accomplishments.\n\n", count)
	prompt += fmt.Sprintf("Review the work below and list the %d most significant accomplishments in priority order (most important first).\n\n", count)
	prompt += "Format your response as a numbered list with concise descriptions (max 15 words each):\n"
	prompt += "1. [first accomplishment]\n"
	prompt += "2. [second accomplishment]\n"
	prompt += fmt.Sprintf("%d. [last accomplishment]\n\n", count)
	prompt += "Focus on technical achievements, feature implementations, bug fixes, and contributions that have measurable impact.\n\n"
	
	// Add Jira context
	if len(issues) > 0 {
		prompt += "JIRA WORK:\n"
		for i, issue := range issues {
			if i >= 10 {
				break // Limit to top 10
			}
			prompt += fmt.Sprintf("- %s: %s [%s]\n", issue.Key, issue.Summary, issue.Status.Name)
		}
		prompt += "\n"
	}
	
	// Add GitHub context
	if activity != nil && len(activity.PullRequests) > 0 {
		prompt += "GITHUB WORK:\n"
		for i, pr := range activity.PullRequests {
			if i >= 10 {
				break // Limit to top 10
			}
			prompt += fmt.Sprintf("- PR: %s [%s]\n", pr.Title, pr.State)
		}
		prompt += "\n"
	}
	
	// Use the exported CallOllama method
	response, err := client.CallOllama(model, prompt)
	if err != nil {
		return nil, err
	}
	
	// Parse the numbered list
	accomplishments := parseAccomplishmentsList(response, count)
	return accomplishments, nil
}

func parseAccomplishmentsList(response string, expectedCount int) []string {
	lines := strings.Split(response, "\n")
	accomplishments := []string{}
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Look for numbered lines: "1. ", "2. ", etc.
		// Also handle variations like "1) " or "1 - "
		if len(line) > 3 {
			// Check if line starts with a number followed by period, paren, or dash
			if (line[0] >= '0' && line[0] <= '9') {
				if line[1] == '.' || line[1] == ')' || (line[1] == ' ' && line[2] == '-') {
					// Extract the accomplishment text after the number
					text := ""
					switch line[1] {
					case '.', ')':
						text = strings.TrimSpace(line[2:])
					case ' ':
						text = strings.TrimSpace(line[3:])
					}
					
					if text != "" {
						accomplishments = append(accomplishments, text)
					}
				}
			}
		}
	}
	
	// If we didn't find enough numbered items, try to split by lines
	if len(accomplishments) < expectedCount {
		accomplishments = []string{}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(strings.ToLower(line), "here are") && 
			   !strings.HasPrefix(strings.ToLower(line), "the top") {
				accomplishments = append(accomplishments, line)
				if len(accomplishments) >= expectedCount {
					break
				}
			}
		}
	}
	
	// Limit to expected count
	if len(accomplishments) > expectedCount {
		accomplishments = accomplishments[:expectedCount]
	}
	
	return accomplishments
}

func parseAccomplishmentResponse(response string) (accomplishment string, why string) {
	lines := strings.Split(response, "\n")
	
	for i, line := range lines {
		line = strings.TrimSpace(line)
		
		// Look for ACCOMPLISHMENT: line
		if strings.HasPrefix(line, "ACCOMPLISHMENT:") {
			accomplishment = strings.TrimSpace(strings.TrimPrefix(line, "ACCOMPLISHMENT:"))
		}
		
		// Look for WHY: line and gather all subsequent lines
		if strings.HasPrefix(line, "WHY:") {
			whyLines := []string{strings.TrimSpace(strings.TrimPrefix(line, "WHY:"))}
			// Gather remaining lines as part of the explanation
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" {
					whyLines = append(whyLines, strings.TrimSpace(lines[j]))
				}
			}
			why = strings.Join(whyLines, " ")
			break
		}
	}
	
	// Fallback if parsing fails
	if accomplishment == "" {
		accomplishment = strings.TrimSpace(response)
	}
	
	return accomplishment, why
}

// removeExistingEntry removes an existing journal entry for a given date header
func removeExistingEntry(content, dateHeader string) string {
	// Find the start of the entry
	startIdx := strings.Index(content, dateHeader)
	if startIdx == -1 {
		return content // Entry not found, return unchanged
	}
	
	// Find the next entry (look for next "## " or end of content)
	// We need to find where this entry ends
	endIdx := len(content)
	
	// Look for the next date header after this one
	nextHeaderIdx := strings.Index(content[startIdx+len(dateHeader):], "\n## ")
	if nextHeaderIdx != -1 {
		// Found next entry, calculate actual position
		endIdx = startIdx + len(dateHeader) + nextHeaderIdx + 1 // +1 to include the newline
	}
	
	// Remove the entry (from start to end, including the separator)
	// Also remove trailing "---" separator if present
	section := content[startIdx:endIdx]
	if strings.Contains(section, "\n---\n") {
		// Find and include the separator in the removal
		separatorIdx := strings.Index(content[startIdx:endIdx], "\n---\n")
		if separatorIdx != -1 {
			endIdx = startIdx + separatorIdx + 5 // +5 for "\n---\n"
		}
	}
	
	// Reconstruct content without the old entry
	return content[:startIdx] + content[endIdx:]
}

func appendToJournal(client *ghclient.Client, gistURL, startDate, endDate, content string, verbose bool) error {
	// Extract gist ID from URL
	gistID, err := ghclient.ExtractGistIDFromURL(gistURL)
	if err != nil {
		return fmt.Errorf("invalid gist URL: %w", err)
	}
	
	if verbose {
		fmt.Printf("  → Fetching gist %s...\n", gistID)
	}

	// Fetch existing gist
	gist, err := client.GetGist(gistID)
	if err != nil {
		return fmt.Errorf("failed to fetch gist: %w", err)
	}
	
	if verbose {
		fmt.Printf("  ✓ Gist found with %d file(s)\n", len(gist.Files))
	}

	// Find the journal file (or use the first file if there's only one)
	var filename string
	var existingContent string
	
	if len(gist.Files) == 0 {
		return fmt.Errorf("gist has no files")
	}
	
	// Use first file, or look for one named "journal" or similar
	for name, file := range gist.Files {
		filename = name
		existingContent = file.Content
		if strings.Contains(strings.ToLower(name), "journal") {
			break // Prefer files with "journal" in the name
		}
	}

	// Create date header
	start, _ := time.Parse("01-02-2006", startDate)
	end, _ := time.Parse("01-02-2006", endDate)
	dateHeader := fmt.Sprintf("## %s to %s\n", start.Format("January 2, 2006"), end.Format("January 2, 2006"))
	
	// Check if entry for this date range already exists and remove it
	if strings.Contains(existingContent, dateHeader) {
		if verbose {
			fmt.Println("  ℹ Entry for this date range already exists, replacing with updated version...")
		}
		existingContent = removeExistingEntry(existingContent, dateHeader)
	} else {
		if verbose {
			fmt.Printf("  → Appending new entry to '%s'...\n", filename)
		}
	}

	// Prepare new content (prepend so newest entries are at the top)
	var newContent strings.Builder
	newContent.WriteString(dateHeader)
	newContent.WriteString(content)
	newContent.WriteString("\n---\n\n")
	newContent.WriteString(existingContent)

	// Update gist
	update := ghclient.GistUpdate{
		Files: map[string]ghclient.GistFile{
			filename: {
				Content: newContent.String(),
			},
		},
	}

	_, err = client.UpdateGist(gistID, update)
	if err != nil {
		return fmt.Errorf("failed to update gist: %w", err)
	}
	
	if verbose {
		fmt.Println("  ✓ Gist updated successfully")
	}

	return nil
}

