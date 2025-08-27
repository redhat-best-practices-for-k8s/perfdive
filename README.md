# perfdive

A Golang CLI application that fetches Jira issues assigned to a user within a date range and generates AI-powered summaries using Ollama.

## Features

- Fetches Jira issues assigned to a specific user within a date range
- **GitHub Integration**: Automatically detects and fetches context from GitHub URLs in Jira issues
- **GitHub User Activity**: Optional feature to fetch user's personal GitHub activity by matching their email
- Uses Ollama to generate intelligent summaries of user activity with enhanced GitHub context
- Supports both text and JSON output formats
- Configurable via command line flags or configuration file
- Built with Cobra CLI framework and Viper for configuration

## Installation

### Prerequisites

- Go 1.25.0 or later
- Access to a Jira instance with API access
- Ollama running locally or accessible via network

### Build from source

```bash
git clone https://github.com/redhat-best-practices-for-k8s/perfdive.git
cd perfdive
make build
```

Or alternatively using Go directly:

```bash
go build -o perfdive .
```

### Makefile Targets

The project includes a Makefile with the following targets:

- `make build` - Build the perfdive binary
- `make clean` - Remove build artifacts  
- `make test` - Run tests
- `make fmt` - Format Go code
- `make vet` - Run go vet
- `make check` - Run fmt, vet, and test
- `make help` - Show all available targets

## Configuration

### Option 1: Configuration File

Copy the example configuration file and customize it:

```bash
cp .perfdive.yaml.example ~/.perfdive.yaml
```

Edit `~/.perfdive.yaml`:

```yaml
jira:
  url: "https://your-company.atlassian.net"
  username: "your-email@company.com"
  token: "your-jira-api-token"

ollama:
  url: "http://localhost:11434"

github:
  token: "your-github-token"  # Optional: for private repos or higher rate limits

output:
  format: "text"  # "text" or "json"
```

### Option 2: Command Line Flags

You can specify all configuration via command line flags:

```bash
./perfdive \
  --jira-url "https://your-company.atlassian.net" \
  --jira-username "your-email@company.com" \
  --jira-token "your-jira-api-token" \
  --ollama-url "http://localhost:11434" \
  --output "text" \
  user@company.com 01-01-2025 01-31-2025 llama3.2:latest
```

### Jira API Token

To get a Jira API token:

1. Go to your Jira account settings
2. Navigate to Security → API tokens  
3. Create a new API token
4. Use this token in the configuration

**Note**: The application uses Bearer token authentication via the jiracrawler library. Ensure your API token has the necessary permissions to read issues and user information.

### GitHub Integration (Optional)

The application automatically detects GitHub URLs in Jira issue descriptions and summaries. When found, it can fetch additional context from GitHub:

- **Pull Request details**: title, description, author, status, commit counts, file changes
- **Issue details**: title, description, labels, status, creation/close dates
- **Enhanced AI summaries**: GitHub context is included in Ollama prompts for richer analysis
- **Reference counting**: Always shows count of GitHub URLs found in Jira issues

To enable GitHub integration:

1. Create a GitHub Personal Access Token:
   - Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
   - Create a new token with `public_repo` scope (or `repo` for private repositories)
   - Copy the token

2. Add the token to your configuration or use the `--github-token` flag

**Important**: Use a **Classic Personal Access Token**, not a fine-grained token. Fine-grained tokens are repository-scoped and won't work with perfdive's user search and cross-repository operations.

**Note**: GitHub integration is optional. Without a token, the application works with public repositories with rate limiting. With a token, you get higher rate limits and access to private repositories.

### GitHub Reference Status Messages

The application will show different messages based on what it finds:

- `Found X GitHub references in Jira issues` - Always shows the count of unique GitHub URLs found
- `ℹ Use --github-token to fetch detailed GitHub context` - When references found but no token provided  
- `⚠ GitHub auth failed, retrying without token for public repo access...` - When invalid token provided, retrying for public repos
- `✓ Successfully fetched details for X GitHub PRs and Y issues` - When GitHub API calls succeed
- `⚠ Found GitHub references but couldn't fetch details (check GitHub token)` - When API calls fail even without auth
- `No GitHub references found in Jira issues` - When no GitHub URLs are detected

### Automatic Retry for Public Repositories

The application intelligently handles GitHub authentication:

- **With valid token**: Uses authenticated requests (higher rate limits, private repo access)
- **With invalid token**: Automatically retries without authentication for public repositories  
- **Without token**: Directly accesses public repositories (with rate limiting)

This means you don't need to worry about 401 errors when accessing public GitHub repositories, even if you provide an incorrect token.

### GitHub User Activity (Advanced Feature)

Beyond just fetching context from GitHub URLs found in Jira issues, perfdive can also fetch a user's personal GitHub activity:

**What it provides:**
- Recent GitHub events (commits, PRs, issues, repository creation)
- Activity correlation with the same date range as Jira analysis
- Comprehensive view of both ticket work (Jira) and actual development (GitHub)

**How to enable:**
```bash
./perfdive user@company.com 01-01-2025 01-31-2025 llama3.2:latest \
  --github-token "your-token" \
  --github-activity
```

**Requirements:**
- Requires `--github-token` (uses GitHub search API)
- User's email must be publicly associated with their GitHub account
- Some users keep emails private, which will prevent user matching

**Limitations:**
- Only works if the user's email is public in their GitHub profile
- GitHub API rate limits apply (higher with authentication)
- Limited to recent activities (GitHub API typically shows last 90 days)

**When it works vs doesn't:**
- ✅ **Works**: User has public email in GitHub profile matching Jira email
- ❌ **Doesn't work**: User has private email settings or uses different email for GitHub
- ❌ **Doesn't work**: User doesn't have a GitHub account with that email

## Usage

### Basic Usage

```bash
./perfdive [email] [start-date] [end-date] [model]
```

Example:
```bash
./perfdive bpalm@redhat.com 06-01-2025 06-31-2025 llama3.2:latest
```

### Parameters

- **email**: Email address of the user whose Jira issues you want to analyze
- **start-date**: Start date in MM-DD-YYYY format
- **end-date**: End date in MM-DD-YYYY format  
- **model**: Ollama model to use for generating summaries (e.g., llama3.2:latest, mistral, etc.)

### Command Line Flags

- `--jira-url` (`-j`): Jira base URL
- `--jira-username` (`-u`): Jira username
- `--jira-token` (`-t`): Jira API token
- `--ollama-url` (`-o`): Ollama API URL (default: http://localhost:11434)
- `--github-token` (`-g`): GitHub API token (optional, for private repos)
- `--github-activity` (`-a`): Fetch user's GitHub activity by matching email (requires GitHub token)
- `--output` (`-f`): Output format - "text" or "json" (default: text)
- `--config`: Path to config file (default: $HOME/.perfdive.yaml)

### Output Formats

#### Text Format (Default)
```
Processing Jira issues for user@company.com from 01-01-2025 to 01-31-2025 using model llama3.2:latest
Testing Jira connection...
✓ Jira connection successful
Testing Ollama connection with model llama3.2:latest...
✓ Ollama connection successful
Fetching Jira issues for user@company.com from 01-01-2025 to 01-31-2025...
Found 5 issues
Found 3 GitHub references in Jira issues
ℹ Use --github-token to fetch detailed GitHub context
Generating summary using llama3.2:latest...

============================================================
SUMMARY FOR user@company.com (01-01-2025 to 01-31-2025)
============================================================
During the specified period from 01-01-2025 to 01-31-2025, user@company.com was actively involved in...
```

#### JSON Format
```bash
./perfdive --output json user@company.com 01-01-2025 01-31-2025 llama3.2:latest
```

```json
{
  "summary": "During the specified period...",
  "period": "01-01-2025 to 01-31-2025", 
  "user": "user@company.com",
  "total_issues": 5,
  "key_activities": ["Bug fixes", "Feature development", "Code review"]
}
```

## Examples

### Generate a summary for a user's work in December 2024

```bash
./perfdive john.doe@company.com 12-01-2024 12-31-2024 llama3.2:latest
```

### Generate a JSON summary using a specific Ollama model

```bash
./perfdive --output json jane.smith@company.com 01-01-2025 01-15-2025 mistral:latest
```

### Use with custom Jira and Ollama endpoints

```bash
./perfdive \
  --jira-url "https://custom-jira.company.com" \
  --ollama-url "http://ollama-server:11434" \
  user@company.com 06-01-2025 06-30-2025 llama3.2:latest
```

### Use with GitHub integration for enhanced context

```bash
./perfdive \
  --jira-url "https://issues.redhat.com" \
  --jira-username "user@company.com" \
  --jira-token "your-jira-token" \
  --github-token "your-github-token" \
  user@company.com 06-01-2025 06-30-2025 llama3.2:latest
```

This will automatically detect GitHub URLs in Jira issues and include PR/issue details in the summary.

### Use with comprehensive GitHub activity analysis

```bash
./perfdive \
  --jira-url "https://issues.redhat.com" \
  --jira-username "user@company.com" \
  --jira-token "your-jira-token" \
  --github-token "your-github-token" \
  --github-activity \
  user@company.com 06-01-2025 06-30-2025 llama3.2:latest
```

This will fetch:
- Jira issues assigned to the user
- GitHub URLs referenced in those Jira issues  
- User's personal GitHub activity (commits, PRs, etc.) for the same time period
- Generate a comprehensive summary correlating all data sources

## Architecture

The application consists of several key components:

- **CLI Interface** (`cmd/root.go`): Built with Cobra, handles command parsing and configuration
- **Jira Client** (`internal/jira/client.go`): Wraps the sebrandon1/jiracrawler library for Jira API access
- **GitHub Client** (`internal/github/client.go`): Detects GitHub URLs and fetches PR/issue context via GitHub API
- **Ollama Client** (`internal/ollama/client.go`): Handles communication with Ollama for AI summary generation with enhanced context
- **Configuration**: Uses Viper for flexible configuration management

## Dependencies

- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [jiracrawler](https://github.com/sebrandon1/jiracrawler) - Jira API client library

## Troubleshooting

### Common Issues

1. **Jira Authentication Failed**
   - Verify your Jira URL, username, and API token
   - Ensure your API token has the necessary permissions

2. **Ollama Connection Failed**
   - Ensure Ollama is running and accessible
   - Verify the Ollama URL is correct
   - Check that the specified model is installed in Ollama

3. **GitHub 401 Errors (Fixed Automatically)**
   - The application automatically retries without authentication for public repos
   - You'll see: `⚠ GitHub auth failed, retrying without token for public repo access...`
   - This is normal behavior and usually resolves automatically

4. **No Issues Found**
   - Verify the user email is correct
   - Check that issues exist for the user in the specified date range
   - Ensure the date format is MM-DD-YYYY

5. **GitHub User Not Found (with --github-activity)**
   - Error: `no GitHub user found with email user@company.com`
   - **Cause**: User's email is private in GitHub settings or they use a different email
   - **Solutions**: 
     - Ask user to make their email public in GitHub profile settings
     - Try with the actual email they use for GitHub commits
     - Use without `--github-activity` flag for basic functionality

6. **Date Format Errors**
   - Use MM-DD-YYYY format (e.g., 06-01-2025, not 6-1-2025)

### Debug Mode

For additional debugging information, you can run with verbose output:

```bash
./perfdive --config ~/.perfdive.yaml user@company.com 01-01-2025 01-31-2025 llama3.2:latest
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.