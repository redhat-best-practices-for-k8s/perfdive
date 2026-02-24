package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/redhat-best-practices-for-k8s/perfdive/internal/github"
	"github.com/redhat-best-practices-for-k8s/perfdive/internal/jira"
)

// Format represents the output format type
type Format string

const (
	FormatText     Format = "text"
	FormatJSON     Format = "json"
	FormatMarkdown Format = "markdown"
	FormatHTML     Format = "html"
	FormatCSV      Format = "csv"
)

// ParseFormat parses a format string into a Format type
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "text", "":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	case "markdown", "md":
		return FormatMarkdown, nil
	case "html":
		return FormatHTML, nil
	case "csv":
		return FormatCSV, nil
	default:
		return FormatText, fmt.Errorf("unknown format '%s': supported formats are text, json, markdown, html, csv", s)
	}
}

// HighlightData contains data for highlight output
type HighlightData struct {
	Email       string
	DisplayName string
	StartDate   time.Time
	EndDate     time.Time
	Days        int

	// Stats
	PRsCreated    int
	PRsMerged     int
	PRsOpen       int
	JiraCreated   int
	JiraUpdated   int

	// Accomplishments
	Accomplishments []string
	BiggestAccomplishment string
	Why                   string

	// Raw data for detailed formats
	PullRequests []github.UserPullRequest
	Issues       []jira.Issue
}

// FormatHighlight formats highlight data according to the specified format
func FormatHighlight(data HighlightData, format Format) (string, error) {
	switch format {
	case FormatText:
		return formatHighlightText(data), nil
	case FormatJSON:
		return formatHighlightJSON(data)
	case FormatMarkdown:
		return formatHighlightMarkdown(data), nil
	case FormatHTML:
		return formatHighlightHTML(data), nil
	case FormatCSV:
		return formatHighlightCSV(data), nil
	default:
		return formatHighlightText(data), nil
	}
}

func formatHighlightText(data HighlightData) string {
	var sb strings.Builder

	sb.WriteString("\n")
	if data.PRsCreated > 0 {
		fmt.Fprintf(&sb, "- Created %d PRs in the last %d days (%d merged, %d open)\n",
			data.PRsCreated, data.Days, data.PRsMerged, data.PRsOpen)
	}
	fmt.Fprintf(&sb, "- Created %d Jira stories and updated Jira %d times\n",
		data.JiraCreated, data.JiraUpdated)

	if len(data.Accomplishments) > 0 {
		fmt.Fprintf(&sb, "- Top %d accomplishments:\n", len(data.Accomplishments))
		for i, acc := range data.Accomplishments {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, acc)
		}
	} else if data.BiggestAccomplishment != "" {
		fmt.Fprintf(&sb, "- Biggest accomplishment: %s\n", data.BiggestAccomplishment)
	}
	sb.WriteString("\n")

	return sb.String()
}

func formatHighlightJSON(data HighlightData) (string, error) {
	jsonData := map[string]interface{}{
		"email":       data.Email,
		"displayName": data.DisplayName,
		"startDate":   data.StartDate.Format("2006-01-02"),
		"endDate":     data.EndDate.Format("2006-01-02"),
		"days":        data.Days,
		"stats": map[string]int{
			"prsCreated":  data.PRsCreated,
			"prsMerged":   data.PRsMerged,
			"prsOpen":     data.PRsOpen,
			"jiraCreated": data.JiraCreated,
			"jiraUpdated": data.JiraUpdated,
		},
		"accomplishments":       data.Accomplishments,
		"biggestAccomplishment": data.BiggestAccomplishment,
		"why":                   data.Why,
	}

	bytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func formatHighlightMarkdown(data HighlightData) string {
	var sb strings.Builder

	name := data.DisplayName
	if name == "" {
		name = data.Email
	}

	fmt.Fprintf(&sb, "# Activity Summary: %s\n\n", name)
	fmt.Fprintf(&sb, "**Period:** %s to %s (%d days)\n\n",
		data.StartDate.Format("January 2, 2006"),
		data.EndDate.Format("January 2, 2006"),
		data.Days)

	sb.WriteString("## Statistics\n\n")
	sb.WriteString("| Metric | Count |\n")
	sb.WriteString("|--------|-------|\n")
	fmt.Fprintf(&sb, "| Pull Requests Created | %d |\n", data.PRsCreated)
	fmt.Fprintf(&sb, "| PRs Merged | %d |\n", data.PRsMerged)
	fmt.Fprintf(&sb, "| PRs Open | %d |\n", data.PRsOpen)
	fmt.Fprintf(&sb, "| Jira Issues Created | %d |\n", data.JiraCreated)
	fmt.Fprintf(&sb, "| Jira Issues Updated | %d |\n", data.JiraUpdated)
	sb.WriteString("\n")

	if len(data.Accomplishments) > 0 {
		sb.WriteString("## Top Accomplishments\n\n")
		for i, acc := range data.Accomplishments {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, acc)
		}
		sb.WriteString("\n")
	} else if data.BiggestAccomplishment != "" {
		sb.WriteString("## Biggest Accomplishment\n\n")
		fmt.Fprintf(&sb, "**%s**\n\n", data.BiggestAccomplishment)
		if data.Why != "" {
			fmt.Fprintf(&sb, "*%s*\n\n", data.Why)
		}
	}

	return sb.String()
}

func formatHighlightHTML(data HighlightData) string {
	var sb strings.Builder

	name := html.EscapeString(data.DisplayName)
	if name == "" {
		name = html.EscapeString(data.Email)
	}

	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	fmt.Fprintf(&sb, "  <title>Activity Summary - %s</title>\n", name)
	sb.WriteString("  <style>\n")
	sb.WriteString("    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px; }\n")
	sb.WriteString("    h1 { color: #333; border-bottom: 2px solid #e74c3c; padding-bottom: 10px; }\n")
	sb.WriteString("    h2 { color: #555; }\n")
	sb.WriteString("    table { border-collapse: collapse; width: 100%; margin: 20px 0; }\n")
	sb.WriteString("    th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }\n")
	sb.WriteString("    th { background-color: #f4f4f4; font-weight: bold; }\n")
	sb.WriteString("    tr:nth-child(even) { background-color: #f9f9f9; }\n")
	sb.WriteString("    .accomplishment { background-color: #e8f5e9; padding: 15px; border-radius: 5px; margin: 10px 0; }\n")
	sb.WriteString("    .why { color: #666; font-style: italic; }\n")
	sb.WriteString("    .period { color: #888; font-size: 0.9em; }\n")
	sb.WriteString("    ol { padding-left: 20px; }\n")
	sb.WriteString("    li { margin: 8px 0; }\n")
	sb.WriteString("  </style>\n")
	sb.WriteString("</head>\n<body>\n")

	fmt.Fprintf(&sb, "  <h1>Activity Summary: %s</h1>\n", name)
	fmt.Fprintf(&sb, "  <p class=\"period\"><strong>Period:</strong> %s to %s (%d days)</p>\n",
		data.StartDate.Format("January 2, 2006"),
		data.EndDate.Format("January 2, 2006"),
		data.Days)

	sb.WriteString("  <h2>Statistics</h2>\n")
	sb.WriteString("  <table>\n")
	sb.WriteString("    <tr><th>Metric</th><th>Count</th></tr>\n")
	fmt.Fprintf(&sb, "    <tr><td>Pull Requests Created</td><td>%d</td></tr>\n", data.PRsCreated)
	fmt.Fprintf(&sb, "    <tr><td>PRs Merged</td><td>%d</td></tr>\n", data.PRsMerged)
	fmt.Fprintf(&sb, "    <tr><td>PRs Open</td><td>%d</td></tr>\n", data.PRsOpen)
	fmt.Fprintf(&sb, "    <tr><td>Jira Issues Created</td><td>%d</td></tr>\n", data.JiraCreated)
	fmt.Fprintf(&sb, "    <tr><td>Jira Issues Updated</td><td>%d</td></tr>\n", data.JiraUpdated)
	sb.WriteString("  </table>\n")

	if len(data.Accomplishments) > 0 {
		sb.WriteString("  <h2>Top Accomplishments</h2>\n")
		sb.WriteString("  <ol>\n")
		for _, acc := range data.Accomplishments {
			fmt.Fprintf(&sb, "    <li>%s</li>\n", html.EscapeString(acc))
		}
		sb.WriteString("  </ol>\n")
	} else if data.BiggestAccomplishment != "" {
		sb.WriteString("  <h2>Biggest Accomplishment</h2>\n")
		sb.WriteString("  <div class=\"accomplishment\">\n")
		fmt.Fprintf(&sb, "    <strong>%s</strong>\n", html.EscapeString(data.BiggestAccomplishment))
		if data.Why != "" {
			fmt.Fprintf(&sb, "    <p class=\"why\">%s</p>\n", html.EscapeString(data.Why))
		}
		sb.WriteString("  </div>\n")
	}

	sb.WriteString("</body>\n</html>\n")

	return sb.String()
}

func formatHighlightCSV(data HighlightData) string {
	var sb strings.Builder
	w := csv.NewWriter(&sb)

	// Write header
	header := []string{"Email", "Name", "Start Date", "End Date", "Days", "PRs Created", "PRs Merged", "PRs Open", "Jira Created", "Jira Updated", "Biggest Accomplishment"}
	_ = w.Write(header)

	// Write data row
	row := []string{
		data.Email,
		data.DisplayName,
		data.StartDate.Format("2006-01-02"),
		data.EndDate.Format("2006-01-02"),
		fmt.Sprintf("%d", data.Days),
		fmt.Sprintf("%d", data.PRsCreated),
		fmt.Sprintf("%d", data.PRsMerged),
		fmt.Sprintf("%d", data.PRsOpen),
		fmt.Sprintf("%d", data.JiraCreated),
		fmt.Sprintf("%d", data.JiraUpdated),
		data.BiggestAccomplishment,
	}
	_ = w.Write(row)

	w.Flush()
	return sb.String()
}
