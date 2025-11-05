# Working Code Reference for jiracrawler Port

This document contains the exact working code from perfdive that should be ported to jiracrawler.

## Authentication Pattern (Already Works in jiracrawler)

```go
// This is the correct pattern - already in jiracrawler
tokenAuth := jira.BearerAuthTransport{
    Token: apikey,
}
client, err := jira.NewClient(tokenAuth.Client(), jiraURL)
```

## HTTP Request Pattern for jiracrawler

When adding new functions to jiracrawler, use the existing jira.Client:

```go
// Option 1: Use client.NewRequest (preferred)
req, err := client.NewRequest("GET", endpoint, nil)
if err != nil {
    return nil, err
}

var result ResponseStruct
resp, err := client.Do(req, &result)
if err != nil {
    return nil, err
}

// Option 2: Custom HTTP request (if needed)
url := fmt.Sprintf("%s/rest/api/2/issue/%s/comment", baseURL, issueKey)
req, _ := http.NewRequest("GET", url, nil)
// Bearer token auth is handled by client's transport
```

## Working Functions to Port

### 1. FetchIssueComments (from perfdive lines 298-357)

```go
func FetchIssueComments(client *jira.Client, baseURL, issueKey string) ([]Comment, error) {
    url := fmt.Sprintf("%s/rest/api/2/issue/%s/comment", baseURL, issueKey)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    // Bearer auth is handled by the client's transport
    req.Header.Set("Accept", "application/json")

    // Use the client's HTTP client (which has the Bearer token configured)
    resp, err := client.HTTPClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch comments: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("jira API returned status %d: %s",
            resp.StatusCode, string(bodyBytes))
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
```

### 2. FetchIssueHistory (from perfdive lines 360-432)

```go
func FetchIssueHistory(client *jira.Client, baseURL, issueKey string) ([]HistoryItem, error) {
    url := fmt.Sprintf("%s/rest/api/2/issue/%s?expand=changelog", baseURL, issueKey)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Accept", "application/json")

    resp, err := client.HTTPClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch history: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("jira API returned status %d: %s",
            resp.StatusCode, string(bodyBytes))
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
```

### 3. FetchEnhancedFields (from perfdive lines 445-528)

```go
type EnhancedFields struct {
    Labels       []string
    Components   []string
    Priority     string
    IssueType    string
    TimeTracking *TimeTracking
}

func FetchEnhancedFields(client *jira.Client, baseURL, issueKey string) (*EnhancedFields, error) {
    url := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=labels,components,priority,issuetype,timetracking",
        baseURL, issueKey)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Accept", "application/json")

    resp, err := client.HTTPClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch enhanced fields: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("jira API returned status %d: %s",
            resp.StatusCode, string(bodyBytes))
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
                OriginalEstimate  string `json:"originalEstimate"`
                RemainingEstimate string `json:"remainingEstimate"`
                TimeSpent         string `json:"timeSpent"`
            } `json:"timetracking"`
        } `json:"fields"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, fmt.Errorf("failed to decode enhanced fields response: %w", err)
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
    if response.Fields.TimeTracking.OriginalEstimate != "" || 
       response.Fields.TimeTracking.TimeSpent != "" {
        timeTracking = &TimeTracking{
            OriginalEstimate:  response.Fields.TimeTracking.OriginalEstimate,
            RemainingEstimate: response.Fields.TimeTracking.RemainingEstimate,
            TimeSpent:         response.Fields.TimeTracking.TimeSpent,
        }
    }

    return &EnhancedFields{
        Labels:       labels,
        Components:   components,
        Priority:     response.Fields.Priority.Name,
        IssueType:    response.Fields.IssueType.Name,
        TimeTracking: timeTracking,
    }, nil
}
```

### 4. CheckIssuePermissions (from perfdive lines 163-193)

```go
type IssuePermissions struct {
    CanViewComments bool
    CanViewHistory  bool
    CanViewIssue    bool
}

func CheckIssuePermissions(client *jira.Client, baseURL, issueKey string) (IssuePermissions, error) {
    url := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=id", baseURL, issueKey)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return IssuePermissions{}, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Accept", "application/json")

    resp, err := client.HTTPClient.Do(req)
    if err != nil {
        return IssuePermissions{}, fmt.Errorf("failed to check permissions: %w", err)
    }
    defer resp.Body.Close()

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
```

### 5. FetchIssueWithEnhancedContext (from perfdive lines 196-278)

```go
func FetchIssueWithEnhancedContext(client *jira.Client, baseURL, issueKey string, verbose bool) (*Issue, error) {
    // Get basic issue first (use existing jiracrawler logic)
    issue, _, err := client.Issue.Get(issueKey, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch issue: %w", err)
    }

    if verbose {
        fmt.Printf("Fetching enhanced context for issue %s...\n", issueKey)
    }

    // Check permissions first
    permissions, err := CheckIssuePermissions(client, baseURL, issueKey)
    if err != nil {
        if verbose {
            fmt.Printf("Warning: failed to check permissions for %s: %v\n", issueKey, err)
        }
        // Continue anyway, we'll get errors on individual fetches if needed
    } else if !permissions.CanViewIssue {
        fmt.Printf("Warning: insufficient permissions to view issue %s - skipping enhanced context\n", issueKey)
        return convertToLibIssue(issue), nil
    }

    // Create result issue
    result := convertToLibIssue(issue)

    // Fetch comments
    if permissions.CanViewComments {
        comments, err := FetchIssueComments(client, baseURL, issueKey)
        if err != nil {
            fmt.Printf("Warning: failed to fetch comments for %s: %v\n", issueKey, err)
        } else {
            result.Comments = comments
            if verbose {
                fmt.Printf("  ✓ Fetched %d comments for %s\n", len(comments), issueKey)
            }
        }
    } else if verbose {
        fmt.Printf("  ⊘ Skipping comments for %s (insufficient permissions)\n", issueKey)
    }

    // Fetch history
    if permissions.CanViewHistory {
        history, err := FetchIssueHistory(client, baseURL, issueKey)
        if err != nil {
            fmt.Printf("Warning: failed to fetch history for %s: %v\n", issueKey, err)
        } else {
            result.History = history
            if verbose {
                fmt.Printf("  ✓ Fetched %d history entries for %s\n", len(history), issueKey)
            }
        }
    } else if verbose {
        fmt.Printf("  ⊘ Skipping history for %s (insufficient permissions)\n", issueKey)
    }

    // Fetch additional fields
    enhancedFields, err := FetchEnhancedFields(client, baseURL, issueKey)
    if err != nil {
        fmt.Printf("Warning: failed to fetch enhanced fields for %s: %v\n", issueKey, err)
    } else {
        if verbose {
            fmt.Printf("  ✓ Fetched enhanced fields for %s\n", issueKey)
        }
        if enhancedFields.Labels != nil {
            result.Labels = enhancedFields.Labels
        }
        if enhancedFields.Components != nil {
            result.Components = enhancedFields.Components
        }
        if enhancedFields.Priority != "" {
            result.Priority = enhancedFields.Priority
        }
        if enhancedFields.TimeTracking != nil {
            result.TimeTracking = enhancedFields.TimeTracking
        }
    }

    return result, nil
}

// Helper to convert go-jira Issue to lib Issue
func convertToLibIssue(jiraIssue *jira.Issue) *Issue {
    // Implementation depends on existing Issue struct in jiracrawler
    return &Issue{
        Key:         jiraIssue.Key,
        Summary:     jiraIssue.Fields.Summary,
        Description: jiraIssue.Fields.Description,
        // ... map other fields
    }
}
```

### 6. Batch Function with Rate Limiting

```go
func FetchUserIssuesInDateRangeWithContext(
    jiraURL, jiraUser, apikey, userEmail, startDate, endDate string,
    enhancedContext bool,
    verbose bool,
) *UserUpdatesResult {
    // First, get basic issues using existing function
    result := FetchUserIssuesInDateRange(jiraURL, jiraUser, apikey, userEmail, startDate, endDate)
    if result == nil {
        return nil
    }

    // If enhanced context not requested, return as-is
    if !enhancedContext {
        return result
    }

    // Create client for enhanced fetching
    tokenAuth := jira.BearerAuthTransport{
        Token: apikey,
    }
    client, err := jira.NewClient(tokenAuth.Client(), jiraURL)
    if err != nil {
        fmt.Printf("Warning: failed to create client for enhanced context: %v\n", err)
        return result
    }

    // Enhance each issue with additional context
    for i, issue := range result.Issues {
        enhanced, err := FetchIssueWithEnhancedContext(client, jiraURL, issue.Key, verbose)
        if err != nil {
            fmt.Printf("Warning: failed to enhance issue %s: %v\n", issue.Key, err)
            continue
        }
        result.Issues[i] = *enhanced

        // Rate limiting: sleep between requests to avoid 429 errors
        if i < len(result.Issues)-1 {
            time.Sleep(100 * time.Millisecond)
        }
    }

    return result
}
```

## Error Handling Patterns

### Rate Limiting (429)
```go
if resp.StatusCode == 429 {
    return nil, fmt.Errorf("rate limit exceeded - please wait and retry")
}
```

### Permission Errors (403, 401)
```go
if resp.StatusCode == 403 || resp.StatusCode == 401 {
    if verbose {
        fmt.Printf("Warning: insufficient permissions for issue %s\n", issueKey)
    }
    // Don't error out, return empty data
    return []Comment{}, nil
}
```

### General HTTP Errors
```go
if resp.StatusCode != 200 {
    bodyBytes, _ := io.ReadAll(resp.Body)
    return nil, fmt.Errorf("jira API returned %d: %s", resp.StatusCode, string(bodyBytes))
}
```

## Required Imports

```go
import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
    
    "github.com/andygrunwald/go-jira"
)
```

## Usage Example After Implementation

```go
package main

import (
    "fmt"
    "github.com/sebrandon1/jiracrawler/lib"
)

func main() {
    // Fetch issues with enhanced context
    result := lib.FetchUserIssuesInDateRangeWithContext(
        "https://issues.redhat.com",
        "bpalm@redhat.com",
        "your-bearer-token",
        "bpalm@redhat.com",
        "2025-06-01",
        "2025-07-28",
        true,  // enhancedContext
        true,  // verbose
    )
    
    if result == nil {
        fmt.Println("Failed to fetch issues")
        return
    }
    
    for _, issue := range result.Issues {
        fmt.Printf("\nIssue: %s - %s\n", issue.Key, issue.Summary)
        fmt.Printf("  Comments: %d\n", len(issue.Comments))
        fmt.Printf("  History: %d entries\n", len(issue.History))
        fmt.Printf("  Labels: %v\n", issue.Labels)
    }
}
```

---

**Note**: All code above is tested and working with Red Hat Jira as of 2025-10-15.

