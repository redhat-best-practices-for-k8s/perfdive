# GitHub Rate Limiting Fix - Summary

## Problem

Users were encountering HTTP 403 errors from GitHub API even on the first run of the day:

```
Warning: failed to fetch user pull requests: GitHub API returned status 403
Warning: failed to fetch user issues: GitHub API returned status 403
```

## Root Cause

1. **No rate limit handling in GitHub client**:
   - The client didn't check rate limit status before making requests
   - No parsing of rate limit headers from responses
   - No retry logic for rate limit errors
   - Generic error messages didn't distinguish between rate limits and other 403 errors

2. **GitHub's rate limits are strict**:
   - **Unauthenticated requests**: 60 requests/hour for Core API, 10 requests/minute for Search API
   - **Authenticated requests**: 5,000 requests/hour for Core API, 30 requests/minute for Search API
   - **Secondary rate limits**: Abuse detection triggers on rapid consecutive requests

3. **Search API is particularly limited**:
   - `FetchUserPullRequests` and `FetchUserIssues` use the Search API
   - Only 10-30 requests per minute depending on authentication
   - These are the endpoints that were failing

## Solution

### 1. Added Rate Limit Tracking

Enhanced the `Client` struct to track rate limit state:

```go
type Client struct {
    baseURL            string
    token              string
    httpClient         *http.Client
    rateLimitRemaining int        // Tracks remaining requests
    rateLimitReset     time.Time  // When rate limit resets
}
```

### 2. Implemented Rate Limit Header Parsing

Added `updateRateLimitFromHeaders` function to extract rate limit information from every GitHub API response:

```go
func (c *Client) updateRateLimitFromHeaders(resp *http.Response) {
    // Parses X-RateLimit-Remaining and X-RateLimit-Reset headers
    // Updates client state to track rate limit status
}
```

### 3. Added Proactive Rate Limit Checking

Modified `doGitHubRequest` to check rate limits before making requests:

```go
// Check if we need to wait for rate limit reset
if !c.rateLimitReset.IsZero() && c.rateLimitRemaining <= 1 && time.Now().Before(c.rateLimitReset) {
    waitTime := time.Until(c.rateLimitReset)
    fmt.Printf("⚠ Rate limit exceeded. Waiting %v until reset...\n", waitTime.Round(time.Second))
    time.Sleep(waitTime + time.Second)
}
```

### 4. Implemented Intelligent Error Handling

Added `handleErrorResponse` function to parse GitHub's JSON error responses:

```go
func (c *Client) handleErrorResponse(resp *http.Response) error {
    var errorResp GitHubErrorResponse
    // Decode error response to get detailed message
    
    // Distinguish between:
    // - Primary rate limit errors
    // - Secondary rate limit errors (abuse detection)
    // - Authentication/permission errors
    // - Other 403 errors
}
```

### 5. Added Retry Logic with Exponential Backoff

Enhanced `makeGitHubRequest` with retry logic:

```go
maxRetries := 3
baseDelay := 2 * time.Second

for attempt := 0; attempt < maxRetries; attempt++ {
    if attempt > 0 {
        delay := baseDelay * time.Duration(1<<uint(attempt-1)) // 2s, 4s, 8s
        time.Sleep(delay)
    }
    
    // Make request with retry on rate limit errors
    // Special handling for secondary rate limits (60s wait)
}
```

### 6. Added Rate Limit Status Checking

Implemented `GetRateLimitStatus` and enhanced `TestConnection` to display rate limit information at startup:

```go
func (c *Client) TestConnection() error {
    rateLimit, err := c.GetRateLimitStatus()
    // Displays:
    // - Core API rate limit (remaining/total)
    // - Search API rate limit (remaining/total)
    // - Reset times
    // - Warnings if limits are low
}
```

## Results

### What Users Will See

**On successful connection:**
```
✓ GitHub API connection OK (authenticated)
  Core API: 4987/5000 remaining (resets at 14:32:15)
  Search API: 28/30 remaining (resets at 13:35:00)
```

**When rate limit is low:**
```
⚠ Warning: Search API rate limit is low (3 remaining)
```

**When rate limited:**
```
⚠ GitHub API rate limit exceeded: API rate limit exceeded for user ID 12345
  Retrying in 2s (attempt 2/3)...
```

**For secondary rate limits:**
```
⚠ GitHub API abuse detection triggered (secondary rate limit): You have exceeded a secondary rate limit
  Waiting 60s for secondary rate limit reset...
```

### Specific Improvements

| Feature | Before | After |
|---------|--------|-------|
| Rate limit awareness | ❌ None | ✅ Tracks and displays status |
| Proactive checking | ❌ Requests fail | ✅ Waits before request if needed |
| Error messages | Generic "status 403" | Detailed message with reason |
| Retry logic | ❌ None | ✅ 3 retries with exponential backoff |
| Secondary rate limits | ❌ Not handled | ✅ Special 60s wait |
| Rate limit visibility | ❌ Unknown until failure | ✅ Shown at startup |

## Understanding GitHub Rate Limits

### Primary Rate Limits

**Unauthenticated:**
- Core API: 60 requests/hour
- Search API: 10 requests/minute

**Authenticated (with token):**
- Core API: 5,000 requests/hour
- Search API: 30 requests/minute

### Secondary Rate Limits (Abuse Detection)

GitHub has additional "abuse detection" mechanisms:
- Triggered by rapid consecutive requests
- Not documented with specific numbers
- Requires waiting ~60 seconds
- Independent of primary rate limits

### Which APIs Use Which Limits

**Core API (5,000/hour):**
- `GET /repos/{owner}/{repo}/pulls/{number}`
- `GET /repos/{owner}/{repo}/issues/{number}`
- `GET /users/{username}`
- Most other endpoints

**Search API (30/minute):**
- `GET /search/issues` (used by `FetchUserPullRequests`)
- `GET /search/issues` (used by `FetchUserIssues`)
- `GET /search/users`

## Best Practices

### 1. Always Use Authentication

```bash
export GITHUB_TOKEN="your_personal_access_token"
./perfdive user@company.com 06-01-2025 07-28-2025
```

**Benefits:**
- 5,000 requests/hour vs 60 requests/hour (83x more)
- 30 Search API requests/minute vs 10 requests/minute (3x more)

### 2. Monitor Rate Limits

The tool now shows rate limit status at startup. Watch for warnings:
- Core API < 10 remaining
- Search API < 5 remaining

### 3. Space Out Runs

If you need to run multiple times:
- Wait a few minutes between runs for Search API to reset
- Core API resets hourly
- Search API resets every minute

### 4. Use Caching

The tool has caching enabled by default (1-hour TTL):
- Repeated queries for same date range use cache
- Saves API quota
- Location: `~/.perfdive/cache/`

To clear cache:
```bash
rm -rf ~/.perfdive/cache/
```

## Code Changes

### Files Modified

1. **`internal/github/client.go`**:
   - Added rate limit tracking to `Client` struct
   - Added `GitHubErrorResponse` and `RateLimitResponse` types
   - Implemented `updateRateLimitFromHeaders()` function
   - Implemented `handleErrorResponse()` function for detailed errors
   - Enhanced `makeGitHubRequest()` with retry logic
   - Enhanced `doGitHubRequest()` with rate limit checking
   - Added `GetRateLimitStatus()` function
   - Enhanced `TestConnection()` to display rate limits
   - Added helper functions: `isRateLimitError()`, `isSecondaryRateLimitError()`

## Troubleshooting

### Still Seeing 403 Errors?

1. **Check if authenticated:**
   ```bash
   echo $GITHUB_TOKEN
   ```
   If empty, set your token.

2. **Check rate limit status:**
   The tool shows this at startup. Look for warnings.

3. **Check token permissions:**
   - Token needs `repo` scope for private repos
   - Token needs `user` scope for user search
   - Token needs `read:org` for organization data

4. **Try again later:**
   - Core API: Resets hourly
   - Search API: Resets per minute
   - Secondary limits: Wait 60+ seconds

### Different Error Messages

**"GitHub API rate limit exceeded"**
- Primary rate limit hit
- Wait until reset time shown in message
- Or wait for automatic retry

**"GitHub API abuse detection triggered"**
- Secondary rate limit (rapid requests)
- Tool waits 60s automatically
- Spread out your requests

**"GitHub API access forbidden"**
- Authentication or permission issue
- Check token is valid and has correct scopes
- May be trying to access private repo without permission

## Performance Impact

### Time Costs

With rate limiting and retries:

| Scenario | Time Impact |
|----------|-------------|
| Normal operation | No change |
| Near rate limit | Automatic wait (few seconds to 1 hour) |
| Rate limit hit | Auto-retry with backoff (2s, 4s, 8s) |
| Secondary rate limit | Auto-wait 60s then retry |

### Request Efficiency

Average perfdive run:
- Jira issues: ~10-20 API calls
- GitHub PR/issue details: ~5-10 API calls
- GitHub user search: 1-2 Search API calls
- GitHub user activity: 2-4 API calls

**Total: ~20-40 API calls per run**

With authenticated token:
- Core API: Can run ~125 times before hitting limit
- Search API: Need to space runs ~2 minutes apart

## Future Improvements

### Potential Enhancements

1. **Adaptive pacing:**
   - Automatically slow down as rate limit decreases
   - Pace requests to stay under limits

2. **Better Search API handling:**
   - Batch multiple searches where possible
   - Use GraphQL API instead (single rate limit)

3. **Rate limit prediction:**
   - Estimate if run will exceed limits before starting
   - Suggest waiting or using cache

4. **Per-endpoint limits:**
   - Track Core vs Search API limits separately
   - Prioritize requests based on which limit is tighter

5. **Longer cache TTL:**
   - Make cache TTL configurable
   - Default to longer cache for older data

## Summary

✅ **Rate limit tracking** - Monitors remaining requests  
✅ **Proactive checking** - Waits before making requests if needed  
✅ **Detailed error messages** - Distinguishes rate limits from other errors  
✅ **Automatic retry** - Exponential backoff on rate limit errors  
✅ **Secondary rate limit handling** - Special handling for abuse detection  
✅ **Rate limit visibility** - Displays status at startup  
✅ **No breaking changes** - Works automatically, no config needed  

The GitHub rate limiting fix makes perfdive more reliable and provides better visibility into GitHub API quota usage, with automatic handling of rate limits through waiting and retries.

---

**Completed**: November 10, 2025  
**Retry Attempts**: 3 with exponential backoff (2s, 4s, 8s)  
**Secondary Rate Limit Wait**: 60 seconds  
**Status**: ✅ Implemented and tested


