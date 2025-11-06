# jiracrawler Enhancement - Quick Start Guide

## TL;DR

Move the enhanced Jira context fetching logic from perfdive into jiracrawler so it's reusable and maintained in one place.

## What to Add to jiracrawler

### 1. New Structs (add to `lib/types.go`)

```go
type Comment struct {
    ID      string
    Author  string
    Body    string
    Created time.Time
    Updated time.Time
}

type HistoryItem struct {
    ID      string
    Author  string
    Created time.Time
    Items   []HistoryChange
}

type HistoryChange struct {
    Field      string
    FieldType  string
    FromString string
    ToString   string
}

type TimeTracking struct {
    OriginalEstimate  string
    RemainingEstimate string
    TimeSpent         string
}
```

### 2. Main Function (add to new file `lib/enhanced_context.go`)

```go
// FetchIssueWithEnhancedContext gets issue with comments, history, etc.
func FetchIssueWithEnhancedContext(client *jira.Client, issueKey string, verbose bool) (*Issue, error) {
    // 1. Fetch basic issue
    // 2. Check permissions
    // 3. Fetch comments (if allowed)
    // 4. Fetch history (if allowed)
    // 5. Fetch enhanced fields
    // 6. Return populated Issue
}
```

### 3. Helper Functions (same file)

```go
func FetchIssueComments(client *jira.Client, issueKey string) ([]Comment, error)
func FetchIssueHistory(client *jira.Client, issueKey string) ([]HistoryItem, error)
func FetchEnhancedFields(client *jira.Client, issueKey string) (*EnhancedFields, error)
func CheckIssuePermissions(client *jira.Client, issueKey string) (IssuePermissions, error)
```

### 4. Enhanced Batch Function (add to `lib/fetch.go`)

```go
func FetchUserIssuesInDateRangeWithContext(
    jiraURL, jiraUser, apikey, userEmail, startDate, endDate string,
    enhancedContext bool, 
    verbose bool,
) *UserUpdatesResult {
    // Call existing FetchUserIssuesInDateRange
    // If enhancedContext=true, enhance each issue
    // Add rate limiting (100ms between requests)
}
```

## Copy These Working Functions from perfdive

All these functions in `perfdive/internal/jira/client.go` already work correctly:

1. **fetchIssueComments** (lines 298-357) → copy to jiracrawler  
2. **fetchIssueHistory** (lines 360-432) → copy to jiracrawler  
3. **fetchEnhancedFields** (lines 445-528) → copy to jiracrawler  
4. **checkIssuePermissions** (lines 163-193) → copy to jiracrawler  
5. **enhanceIssueWithContext** (lines 196-278) → becomes FetchIssueWithEnhancedContext

## Key Differences When Porting

### perfdive pattern:
```go
// Creates HTTP request manually
req.Header.Set("Authorization", "Bearer "+token)
resp, err := httpClient.Do(req)
```

### jiracrawler pattern:
```go
// Use existing jira.Client (already has Bearer auth)
// The client from andygrunwald/go-jira handles auth automatically
issue, resp, err := client.Issue.Get(issueKey, nil)

// OR for custom endpoints:
req, _ := client.NewRequest("GET", "rest/api/2/issue/"+issueKey+"/comment", nil)
resp, err := client.Do(req, &result)
```

## Testing Checklist

```bash
# Run tests
go test ./lib/... -v

# Run with real Jira (requires env vars)
JIRA_URL=https://issues.redhat.com \
JIRA_USER=bpalm@redhat.com \
JIRA_TOKEN=your_token \
go test ./lib/... -v

# Test rate limiting
go test -run TestRateLimiting -v
```

## How perfdive Will Use It (After jiracrawler is updated)

### Before (current perfdive code):
```go
// Lots of duplicate HTTP client code
jiraClient, _ := jira.NewClient(...)
issues, _ := jiraClient.GetUserIssuesInDateRangeWithContext(...)
```

### After (using enhanced jiracrawler):
```go
import "github.com/sebrandon1/jiracrawler/lib"

result := lib.FetchUserIssuesInDateRangeWithContext(
    jiraURL, jiraUsername, jiraToken,
    email, startDate, endDate,
    true,  // fetch enhanced context
    verbose,
)
issues := result.Issues  // Already has comments, history, etc.
```

## File Structure in jiracrawler

```
lib/
├── types.go              # Add Comment, HistoryItem, etc.
├── fetch.go              # Add FetchUserIssuesInDateRangeWithContext
├── enhanced_context.go   # NEW: All enhanced context functions
├── rate_limit.go         # NEW: Rate limiting helpers (optional)
└── enhanced_context_test.go  # NEW: Tests
```

## Priority Order

1. ✅ Add structs to types.go
2. ✅ Create enhanced_context.go with 5 functions
3. ✅ Update Issue struct to include optional enhanced fields
4. ✅ Add FetchUserIssuesInDateRangeWithContext to fetch.go
5. ✅ Add basic tests
6. ✅ Release new version (v0.0.13)
7. ✅ Update perfdive to use it
8. ✅ Remove duplicate code from perfdive

## Common Pitfalls to Avoid

❌ **Don't use Basic Auth** - Use Bearer tokens (like existing jiracrawler code)  
❌ **Don't fail on 403** - Return empty data with warning  
❌ **Don't hammer the API** - Add rate limiting (100ms between requests)  
❌ **Don't make breaking changes** - Keep existing functions working

## Success Indicators

- ✅ `go test ./...` passes
- ✅ Integration test with real Jira works
- ✅ No 403 errors (authentication works)
- ✅ Graceful handling of 429 (rate limits)
- ✅ perfdive can remove ~500 lines of duplicate code

## Get Started

1. Open jiracrawler repository
2. Create branch: `git checkout -b feature/enhanced-context`
3. Start with: `cp perfdive/internal/jira/client.go reference.go` (for reference)
4. Follow the spec in `JIRACRAWLER_ENHANCEMENT_SPEC.md`
5. Test frequently with: `go test ./... -v`

---

**Need Help?** See the full spec: `JIRACRAWLER_ENHANCEMENT_SPEC.md`

