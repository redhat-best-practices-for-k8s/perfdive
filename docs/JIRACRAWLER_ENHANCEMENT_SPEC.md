# jiracrawler Enhancement Specification

## Overview

This document specifies enhancements to the `jiracrawler` library to add comprehensive enhanced context fetching for Jira issues. Currently, perfdive duplicates functionality that should live in jiracrawler.

## Background

### Current State
- jiracrawler uses **Bearer token authentication** (correct for Red Hat Jira)
- jiracrawler fetches basic issue data only
- perfdive has duplicate code for fetching enhanced context (comments, history, fields)
- perfdive's initial implementation used **Basic Auth** which failed with Red Hat Jira

### Problem
1. **Code Duplication**: Enhanced context logic exists in perfdive but should be in jiracrawler
2. **Authentication Mismatch**: Perfdive's direct API calls initially used wrong auth method
3. **Maintainability**: Changes need to be made in multiple places

### Solution
Add enhanced context fetching functions to jiracrawler using the existing Bearer token authentication pattern.

## Required Features

### 1. Enhanced Issue Structure

Add optional fields to the existing `Issue` struct:

```go
type Issue struct {
    Key          string
    Summary      string
    Description  string
    Status       StatusType
    IssueType    IssueTypeType
    Assignee     *jira.User
    Created      string
    Updated      string
    
    // NEW: Enhanced context fields (populated on demand)
    Comments     []Comment        `json:"comments,omitempty"`
    History      []HistoryItem    `json:"history,omitempty"`
    Labels       []string         `json:"labels,omitempty"`
    Components   []string         `json:"components,omitempty"`
    Priority     string           `json:"priority,omitempty"`
    TimeTracking *TimeTracking    `json:"time_tracking,omitempty"`
    CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}
```

### 2. New Data Structures

```go
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

// IssuePermissions represents what the authenticated user can access
type IssuePermissions struct {
    CanViewComments bool
    CanViewHistory  bool
    CanViewIssue    bool
}
```

### 3. New Functions to Add

#### 3.1 FetchIssueComments
```go
// FetchIssueComments retrieves all comments for a specific issue
// Uses Bearer token authentication from existing jira.Client
func FetchIssueComments(client *jira.Client, issueKey string) ([]Comment, error)
```

**Implementation Notes:**
- Endpoint: `GET /rest/api/2/issue/{issueKey}/comment`
- Use existing client's Bearer token authentication
- Parse response and convert to Comment structs
- Handle 403/401 gracefully (return empty slice, log warning)
- Handle 429 rate limiting (return error with retry suggestion)

**Response Structure to Parse:**
```json
{
  "comments": [
    {
      "id": "12345",
      "body": "Comment text",
      "author": {
        "displayName": "User Name"
      },
      "created": "2025-01-01T12:00:00.000-0700",
      "updated": "2025-01-01T12:00:00.000-0700"
    }
  ]
}
```

#### 3.2 FetchIssueHistory
```go
// FetchIssueHistory retrieves the change history for a specific issue
func FetchIssueHistory(client *jira.Client, issueKey string) ([]HistoryItem, error)
```

**Implementation Notes:**
- Endpoint: `GET /rest/api/2/issue/{issueKey}?expand=changelog`
- Use existing client's Bearer token authentication
- Parse changelog from response
- Handle permissions gracefully

**Response Structure to Parse:**
```json
{
  "changelog": {
    "histories": [
      {
        "id": "123",
        "author": {
          "displayName": "User Name"
        },
        "created": "2025-01-01T12:00:00.000-0700",
        "items": [
          {
            "field": "status",
            "fieldtype": "jira",
            "fromString": "Open",
            "toString": "In Progress"
          }
        ]
      }
    ]
  }
}
```

#### 3.3 FetchEnhancedFields
```go
// FetchEnhancedFields retrieves additional field data for an issue
func FetchEnhancedFields(client *jira.Client, issueKey string) (*EnhancedFields, error)

type EnhancedFields struct {
    Labels       []string
    Components   []string
    Priority     string
    IssueType    string
    TimeTracking *TimeTracking
    CustomFields map[string]interface{}
}
```

**Implementation Notes:**
- Endpoint: `GET /rest/api/2/issue/{issueKey}?fields=labels,components,priority,issuetype,timetracking`
- Parse specific fields from response

#### 3.4 CheckIssuePermissions
```go
// CheckIssuePermissions verifies what the user can access for an issue
func CheckIssuePermissions(client *jira.Client, issueKey string) (IssuePermissions, error)
```

**Implementation Notes:**
- Endpoint: `GET /rest/api/2/issue/{issueKey}?fields=id`
- Lightweight check (minimal data transfer)
- Returns boolean flags for what user can access
- Use this before attempting to fetch comments/history

#### 3.5 FetchIssueWithEnhancedContext (Main Entry Point)
```go
// FetchIssueWithEnhancedContext retrieves an issue with all available enhanced context
// This is the main function consumers should use
func FetchIssueWithEnhancedContext(client *jira.Client, issueKey string, verbose bool) (*Issue, error)
```

**Implementation Notes:**
- Fetches basic issue first
- Checks permissions
- Conditionally fetches comments, history, enhanced fields based on permissions
- Logs warnings for permission issues (only if verbose=true)
- Returns fully populated Issue struct
- Gracefully handles rate limiting

#### 3.6 Enhanced Batch Function
```go
// FetchUserIssuesInDateRangeWithContext - Enhanced version of existing function
// Adds optional parameter to fetch enhanced context for all issues
func FetchUserIssuesInDateRangeWithContext(
    jiraURL, jiraUser, apikey, userEmail, startDate, endDate string,
    enhancedContext bool,
    verbose bool,
) *UserUpdatesResult
```

**Implementation Notes:**
- Wrapper around existing `FetchUserIssuesInDateRange`
- If `enhancedContext=true`, call `FetchIssueWithEnhancedContext` for each issue
- Add rate limiting protection (sleep between requests)
- Return UserUpdatesResult with fully populated issues

## Implementation Pattern

### Authentication (Already Works in jiracrawler)

```go
// This pattern already exists in jiracrawler and works correctly
tokenAuth := jira.BearerAuthTransport{
    Token: apikey,
}
client, err := jira.NewClient(tokenAuth.Client(), jiraURL)
```

### HTTP Request Pattern for New Functions

```go
// Use the existing jira.Client's HTTP client (it has Bearer token)
req, err := http.NewRequest("GET", url, nil)
if err != nil {
    return nil, err
}
req.Header.Set("Accept", "application/json")

resp, err := client.HTTPClient.Do(req)
// ... handle response
```

**Note**: The jira.Client from `github.com/andygrunwald/go-jira` automatically handles Bearer authentication, so no need to manually set Authorization headers.

## Error Handling

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
    return []Comment{}, nil // Return empty, don't error
}
```

### General Errors
```go
if resp.StatusCode != 200 {
    bodyBytes, _ := io.ReadAll(resp.Body)
    return nil, fmt.Errorf("jira API returned %d: %s", resp.StatusCode, string(bodyBytes))
}
```

## Rate Limiting Protection

Add a configurable rate limiter to prevent 429 errors:

```go
// Add to lib package
var (
    requestDelay = 100 * time.Millisecond  // Default delay between requests
)

// SetRequestDelay allows consumers to configure rate limiting
func SetRequestDelay(delay time.Duration) {
    requestDelay = delay
}

// Use in batch operations
func FetchUserIssuesInDateRangeWithContext(...) {
    for _, issue := range issues {
        enhanced, err := FetchIssueWithEnhancedContext(client, issue.Key, verbose)
        // ... handle result
        time.Sleep(requestDelay)  // Rate limiting
    }
}
```

## Testing

### Unit Tests Required

1. **TestFetchIssueComments**
   - Mock Jira response
   - Verify Comment struct population
   - Test permission errors (403)
   - Test rate limiting (429)

2. **TestFetchIssueHistory**
   - Mock changelog response
   - Verify HistoryItem parsing
   - Test empty history

3. **TestFetchEnhancedFields**
   - Mock fields response
   - Verify all fields parsed correctly

4. **TestCheckIssuePermissions**
   - Test various HTTP status codes
   - Verify permission flags set correctly

5. **TestFetchIssueWithEnhancedContext**
   - End-to-end test with all enhancements
   - Test graceful degradation on permission errors

### Integration Tests

Create integration test that requires real Jira credentials:

```go
// TestIntegrationEnhancedContext - requires JIRA_URL, JIRA_USER, JIRA_TOKEN env vars
func TestIntegrationEnhancedContext(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    // Test against real Jira instance
}
```

## Migration Path for perfdive

### Phase 1: Update jiracrawler
1. Add new structures and functions to jiracrawler
2. Release new version of jiracrawler (e.g., v0.0.13)
3. Add tests

### Phase 2: Update perfdive  
1. Update `go.mod` to use new jiracrawler version
2. Remove `internal/jira/client.go` functions that are duplicated:
   - `fetchIssueComments`
   - `fetchIssueHistory`
   - `fetchEnhancedFields`
   - `checkIssuePermissions`
   - `enhanceIssueWithContext`
3. Call jiracrawler functions directly:
   ```go
   // Replace this perfdive code:
   issues, err := jiraClient.GetUserIssuesInDateRangeWithContext(...)
   
   // With this:
   result := lib.FetchUserIssuesInDateRangeWithContext(
       jiraURL, jiraUsername, jiraToken, 
       email, startDate, endDate,
       true,  // enhancedContext
       verbose,
   )
   issues := result.Issues
   ```

### Phase 3: Cleanup
1. Remove unused types from perfdive (they'll come from jiracrawler)
2. Simplify perfdive's jira client to just be a thin wrapper
3. Update README documentation

## API Examples

### Example 1: Fetch Issue with Enhanced Context
```go
import "github.com/sebrandon1/jiracrawler/lib"

tokenAuth := jira.BearerAuthTransport{
    Token: apiToken,
}
client, _ := jira.NewClient(tokenAuth.Client(), jiraURL)

issue, err := lib.FetchIssueWithEnhancedContext(client, "CNFCERT-123", true)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Issue: %s\n", issue.Summary)
fmt.Printf("Comments: %d\n", len(issue.Comments))
fmt.Printf("History Entries: %d\n", len(issue.History))
```

### Example 2: Batch Fetch with Enhanced Context
```go
result := lib.FetchUserIssuesInDateRangeWithContext(
    "https://issues.redhat.com",
    "bpalm@redhat.com",
    "token123",
    "bpalm@redhat.com",
    "2025-06-01",
    "2025-07-28",
    true,   // fetch enhanced context
    true,   // verbose logging
)

for _, issue := range result.Issues {
    fmt.Printf("%s: %d comments\n", issue.Key, len(issue.Comments))
}
```

## Files to Modify in jiracrawler

1. **`lib/types.go`** (or create if doesn't exist)
   - Add Comment, HistoryItem, HistoryChange, TimeTracking structs
   - Add IssuePermissions struct
   - Add EnhancedFields struct
   - Update Issue struct with optional fields

2. **`lib/enhanced_context.go`** (new file)
   - FetchIssueComments
   - FetchIssueHistory
   - FetchEnhancedFields
   - CheckIssuePermissions
   - FetchIssueWithEnhancedContext

3. **`lib/fetch.go`** (existing file)
   - Add FetchUserIssuesInDateRangeWithContext function
   - Add rate limiting controls

4. **`lib/rate_limit.go`** (new file, optional)
   - Rate limiting configuration
   - Request delay management

5. **Tests**
   - `lib/enhanced_context_test.go`
   - `lib/integration_test.go`

## Success Criteria

✅ All new functions work with Bearer token authentication  
✅ Graceful handling of permission errors (403/401)  
✅ Rate limiting protection to avoid 429 errors  
✅ Comprehensive test coverage (>80%)  
✅ perfdive can remove its duplicate code  
✅ API is intuitive and well-documented  
✅ Backward compatible (existing functions still work)

## Reference Implementation

Current working code in perfdive (`internal/jira/client.go`) can be used as reference:
- Lines 298-357: `fetchIssueComments`
- Lines 360-432: `fetchIssueHistory`
- Lines 445-528: `fetchEnhancedFields`
- Lines 163-193: `checkIssuePermissions`
- Lines 196-278: `enhanceIssueWithContext`

**Key Pattern**: All use `req.Header.Set("Authorization", "Bearer "+token)` for authentication.

## Timeline Estimate

- **Week 1**: Add structures and core functions to jiracrawler
- **Week 1-2**: Add tests and documentation
- **Week 2**: Release new jiracrawler version
- **Week 2-3**: Update perfdive to use new jiracrawler functions
- **Week 3**: Remove duplicate code from perfdive
- **Week 3**: Final testing and documentation updates

## Questions to Resolve

1. Should rate limiting be configurable or use sensible defaults?
   - **Recommendation**: Provide `SetRequestDelay()` function with 100ms default

2. Should we fetch all enhanced context by default or make it opt-in?
   - **Recommendation**: Opt-in via function parameters (backward compatible)

3. How to handle CustomFields - they're instance-specific?
   - **Recommendation**: Return as `map[string]interface{}` for flexibility

4. Should we add caching to reduce API calls?
   - **Recommendation**: Not initially, let consumers implement caching if needed

## Additional Resources

- Red Hat Jira API: `https://issues.redhat.com/rest/api/2/`
- Jira REST API docs: https://docs.atlassian.com/jira-software/REST/latest/
- go-jira library: https://github.com/andygrunwald/go-jira
- Current jiracrawler: https://github.com/sebrandon1/jiracrawler

---

**Document Version**: 1.0  
**Last Updated**: 2025-10-15  
**Author**: Generated from perfdive authentication fix analysis

