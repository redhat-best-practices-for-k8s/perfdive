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

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "perfdive [email] [start-date] [end-date] [model]",
	Short: "Generate a summary of Jira activity using Ollama",
	Long: `perfdive fetches Jira issues assigned to a user within a date range
and generates a summary using Ollama with the specified model.

Enhanced context is enabled by default, fetching Jira comments, history,
GitHub PR diffs, reviews, and detailed file analysis.

Model defaults to llama3.2:latest if not specified.

Example:
  perfdive bpalm@redhat.com 06-01-2025 06-31-2025
  perfdive bpalm@redhat.com 06-01-2025 06-31-2025 llama3.2:latest
  perfdive --github-username sebrandon1 bpalm@redhat.com 06-01-2025 06-31-2025
  perfdive --github-activity bpalm@redhat.com 06-01-2025 06-31-2025
  perfdive --verbose bpalm@redhat.com 06-01-2025 06-31-2025`,
	Args: cobra.RangeArgs(3, 4),
	Run:  runPerfdive,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.perfdive.yaml)")

	// Local flags
	rootCmd.Flags().StringP("jira-url", "j", "https://issues.redhat.com", "Jira base URL")
	rootCmd.Flags().StringP("jira-username", "u", "", "Jira username")
	rootCmd.Flags().StringP("jira-token", "t", "", "Jira API token")
	rootCmd.Flags().StringP("ollama-url", "o", "http://localhost:11434", "Ollama API URL")
	rootCmd.Flags().StringP("output", "f", "text", "Output format (text or json)")
	rootCmd.Flags().StringP("github-token", "g", "", "GitHub API token (optional, for private repos)")
	rootCmd.Flags().StringP("github-username", "", "", "Explicit GitHub username (overrides email-based search)")
	rootCmd.Flags().BoolP("github-activity", "a", false, "Fetch user's GitHub activity via email search (auto-enabled if --github-username provided)")
	rootCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output including warnings and debug information")

	// Bind flags to viper
	_ = viper.BindPFlag("jira.url", rootCmd.Flags().Lookup("jira-url"))
	_ = viper.BindPFlag("jira.username", rootCmd.Flags().Lookup("jira-username"))
	_ = viper.BindPFlag("jira.token", rootCmd.Flags().Lookup("jira-token"))
	_ = viper.BindPFlag("ollama.url", rootCmd.Flags().Lookup("ollama-url"))
	_ = viper.BindPFlag("output.format", rootCmd.Flags().Lookup("output"))
	_ = viper.BindPFlag("github.token", rootCmd.Flags().Lookup("github-token"))
	_ = viper.BindPFlag("github.username", rootCmd.Flags().Lookup("github-username"))
	_ = viper.BindPFlag("github.activity", rootCmd.Flags().Lookup("github-activity"))
	_ = viper.BindPFlag("verbose", rootCmd.Flags().Lookup("verbose"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".perfdive" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".perfdive")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

func runPerfdive(cmd *cobra.Command, args []string) {
	email := args[0]
	startDate := args[1]
	endDate := args[2]

	// Model is optional, default to llama3.2:latest
	model := "llama3.2:latest"
	if len(args) >= 4 {
		model = args[3]
	}

	fmt.Printf("Processing Jira issues for %s from %s to %s using model %s\n",
		email, startDate, endDate, model)

	// Get configuration values
	jiraURL := viper.GetString("jira.url")
	jiraUsername := viper.GetString("jira.username")
	jiraToken := viper.GetString("jira.token")
	ollamaURL := viper.GetString("ollama.url")
	outputFormat := viper.GetString("output.format")
	githubToken := viper.GetString("github.token")
	githubUsername := viper.GetString("github.username")
	fetchGitHubActivity := viper.GetBool("github.activity")
	verbose := viper.GetBool("verbose")

	// Validate required configuration
	if jiraURL == "" {
		fmt.Fprintf(os.Stderr, "Error: Jira URL is required. Set via --jira-url flag or config file\n")
		os.Exit(1)
	}
	if jiraUsername == "" {
		fmt.Fprintf(os.Stderr, "Error: Jira username is required. Set via --jira-username flag or config file\n")
		os.Exit(1)
	}
	if jiraToken == "" {
		fmt.Fprintf(os.Stderr, "Error: Jira token is required. Set via --jira-token flag or config file\n")
		os.Exit(1)
	}

	err := processUserActivity(email, startDate, endDate, model, jiraURL, jiraUsername, jiraToken, ollamaURL, outputFormat, githubToken, githubUsername, fetchGitHubActivity, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// processUserActivity handles the core logic of fetching Jira issues and generating summaries
func processUserActivity(email, startDate, endDate, model, jiraURL, jiraUsername, jiraToken, ollamaURL, outputFormat, githubToken, githubUsername string, fetchGitHubActivity, verbose bool) error {
	// Create Jira client
	jiraClient, err := jira.NewClient(jira.Config{
		URL:      jiraURL,
		Username: jiraUsername,
		Token:    jiraToken,
	})
	if err != nil {
		return fmt.Errorf("failed to create Jira client: %w", err)
	}

	// Test Jira connection
	fmt.Println("Testing Jira connection...")
	if err := jiraClient.TestConnection(); err != nil {
		return fmt.Errorf("failed to connect to Jira: %w", err)
	}
	fmt.Println("✓ Jira connection successful")

	// Create Ollama client
	ollamaClient := ollama.NewClient(ollama.Config{
		URL: ollamaURL,
	})

	// Test Ollama connection
	fmt.Printf("Testing Ollama connection with model %s...\n", model)
	if err := ollamaClient.TestConnection(model); err != nil {
		return fmt.Errorf("failed to connect to Ollama: %w", err)
	}
	fmt.Println("✓ Ollama connection successful")

	// Fetch Jira issues
	fmt.Printf("Fetching Jira issues for %s from %s to %s...\n", email, startDate, endDate)
	issues, err := jiraClient.GetUserIssuesInDateRangeWithContext(email, startDate, endDate, true, verbose)
	if err != nil {
		return fmt.Errorf("failed to fetch Jira issues: %w", err)
	}

	fmt.Printf("Found %d issues\n", len(issues))

	// Always extract GitHub references to show count
	githubClient := ghclient.NewClient(ghclient.Config{Token: githubToken})

	// Convert jira issues to ghclient.JiraIssue format for GitHub parsing
	var jiraIssuesForGithub []ghclient.JiraIssue
	for _, issue := range issues {
		jiraIssuesForGithub = append(jiraIssuesForGithub, ghclient.JiraIssue{
			Key:         issue.Key,
			Summary:     issue.Summary,
			Description: issue.Description,
		})
	}

	// Fetch GitHub context from URLs found in Jira issues
	fmt.Println("Analyzing GitHub references in Jira issues...")
	githubContext, err := githubClient.FetchGitHubContextFromJiraIssues(jiraIssuesForGithub)
	if err != nil {
		fmt.Printf("Warning: failed to fetch GitHub context: %v\n", err)
		githubContext = &ghclient.GitHubContext{} // Create empty context to avoid nil pointer
	}

	// Show GitHub references found
	if len(githubContext.References) > 0 {
		fmt.Printf("Found %d GitHub references in Jira issues\n", len(githubContext.References))
		if githubToken == "" {
			fmt.Printf("ℹ Use --github-token to fetch detailed GitHub context\n")
		} else {
			fmt.Printf("✓ Enhanced GitHub context enabled (fetching PR diffs, reviews, file analysis)\n")
		}
	} else {
		fmt.Println("No GitHub references found in Jira issues")
	}

	// Enhanced context status for Jira
	fmt.Printf("✓ Enhanced Jira context enabled (fetching comments, history, time tracking)\n")

	// Fetch user's GitHub activity if requested or if GitHub username is provided
	if fetchGitHubActivity || githubUsername != "" {
		if githubToken == "" {
			fmt.Println("⚠ GitHub activity requires --github-token for user search")
		} else {
			// Convert date format for GitHub API
			start, _ := time.Parse("01-02-2006", startDate)
			end, _ := time.Parse("01-02-2006", endDate)
			startDateFormatted := start.Format("2006-01-02")
			endDateFormatted := end.Format("2006-01-02")

			var userActivity []ghclient.UserActivity
			var foundUsername string
			var err error

			if githubUsername != "" {
				// Use explicit GitHub username
				fmt.Printf("ℹ Using explicit GitHub username '%s' (overriding email-based search)\n", githubUsername)
				fmt.Printf("Fetching comprehensive GitHub activity for username: %s...\n", githubUsername)

				// Fetch comprehensive activity from multiple sources
				comprehensiveActivity, err := githubClient.FetchComprehensiveUserActivity(githubUsername, startDateFormatted, endDateFormatted)
				if err != nil {
					fmt.Printf("⚠ Could not fetch comprehensive GitHub activity for %s: %v\n", githubUsername, err)

					// Fallback to legacy activity fetching
					activities, err := githubClient.FetchUserActivity(githubUsername)
					if err != nil {
						fmt.Printf("⚠ Could not fetch GitHub user activity for %s: %v\n", githubUsername, err)
					} else {
						userActivity = githubClient.FilterActivityByDateRange(activities, startDateFormatted, endDateFormatted)
						foundUsername = githubUsername
					}
				} else {
					// Use comprehensive activity
					foundUsername = githubUsername
					if githubContext == nil {
						githubContext = &ghclient.GitHubContext{}
					}
					githubContext.ComprehensiveActivity = comprehensiveActivity
					githubContext.GitHubUsername = foundUsername

					totalActivity := len(comprehensiveActivity.Events) + len(comprehensiveActivity.PullRequests) + len(comprehensiveActivity.Issues)
					fmt.Printf("✓ Found GitHub user '%s' with %d total activities in date range\n", foundUsername, totalActivity)
					fmt.Printf("  - Events: %d, Pull Requests: %d, Issues: %d\n",
						len(comprehensiveActivity.Events),
						len(comprehensiveActivity.PullRequests),
						len(comprehensiveActivity.Issues))
				}
			} else {
				// Fall back to email-based search
				fmt.Printf("Searching for GitHub user with email %s...\n", email)
				userActivity, foundUsername, err = githubClient.FetchUserGitHubActivity(email, startDateFormatted, endDateFormatted)
				if err != nil {
					fmt.Printf("⚠ Could not fetch GitHub user activity: %v\n", err)
				}
			}

			// Handle legacy userActivity if we didn't use comprehensive fetching
			if err == nil && len(userActivity) >= 0 && (githubContext == nil || githubContext.ComprehensiveActivity == nil) {
				if githubContext == nil {
					githubContext = &ghclient.GitHubContext{}
				}
				githubContext.UserActivity = userActivity
				githubContext.GitHubUsername = foundUsername
				fmt.Printf("✓ Found GitHub user '%s' with %d activities in date range\n", foundUsername, len(userActivity))
			}
		}
	}

	// Extract user's display name from Jira issues
	var displayName string
	for _, issue := range issues {
		if issue.Assignee != "" {
			displayName = issue.Assignee
			break // Use the first non-empty assignee name we find
		}
	}

	// Generate summary using Ollama
	fmt.Printf("Generating summary using %s...\n", model)
	summary, err := ollamaClient.GenerateSummary(ollama.SummaryRequest{
		Email:         email,
		DisplayName:   displayName,
		StartDate:     startDate,
		EndDate:       endDate,
		Model:         model,
		Issues:        issues,
		Format:        outputFormat,
		GitHubContext: githubContext,
	})
	if err != nil {
		return fmt.Errorf("failed to generate summary: %w", err)
	}

	// Output the result
	fmt.Println("\n" + strings.Repeat("=", 60))
	if displayName != "" {
		fmt.Printf("SUMMARY FOR %s (%s) (%s to %s)\n", displayName, email, startDate, endDate)
	} else {
		fmt.Printf("SUMMARY FOR %s (%s to %s)\n", email, startDate, endDate)
	}
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(summary)

	// Add reference URLs section (only for text format)
	if outputFormat != "json" {
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("REFERENCE URLS")
		fmt.Println(strings.Repeat("=", 60))

		// List Jira URLs
		if len(issues) > 0 {
			fmt.Println("\nJira Issues:")
			for _, issue := range issues {
				jiraIssueURL := fmt.Sprintf("%s/browse/%s", jiraURL, issue.Key)
				fmt.Printf("- %s: %s\n", issue.Key, jiraIssueURL)
			}
		}

		// List GitHub URLs
		if githubContext != nil && len(githubContext.References) > 0 {
			fmt.Println("\nGitHub References from Jira:")
			for _, ref := range githubContext.References {
				fmt.Printf("- %s/%s #%s: %s\n", ref.Owner, ref.Repo, ref.Number, ref.URL)
			}
		}

		// Show if no references were found
		if len(issues) == 0 && (githubContext == nil || len(githubContext.References) == 0) {
			fmt.Println("\nNo Jira issues or GitHub references found for this period.")
		}
	}

	return nil
}
