# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

perfdive is a Go CLI application for tracking and summarizing work activity. It fetches Jira issues and GitHub activity for a user within a date range, then uses Ollama (local LLM) to generate AI-powered summaries of accomplishments.

## Build and Development Commands

```bash
# Build
make build              # Build the perfdive binary
go build -o perfdive .  # Alternative direct build

# Install/Uninstall
make install            # Build and install to ~/.local/bin (PREFIX=/path to customize)
make uninstall          # Remove installed binary

# Testing and Quality
make test               # Run tests
make fmt                # Format code
make vet                # Run go vet
make lint               # Run golangci-lint
make check              # Run fmt, vet, lint, and test together

# Other
make deps               # Download dependencies
make tidy               # Run go mod tidy
make build-all          # Build for linux/darwin/windows
```

## Running the Application

```bash
# Quick highlight summary (most common usage)
./perfdive highlight user@email.com                    # Last 7 days
./perfdive highlight user@email.com --days 14          # Last 14 days
./perfdive highlight user@email.com --list 5           # Top 5 accomplishments
./perfdive highlight user@email.com --verbose          # Detailed progress output
./perfdive highlight user@email.com --clear-cache      # Force refresh

# Full analysis mode
./perfdive user@email.com 01-01-2025 01-31-2025 llama3.2:latest
./perfdive --github-activity user@email.com 01-01-2025 01-31-2025
```

## Architecture

The application follows a standard Go CLI structure using Cobra/Viper:

```
main.go                    # Entry point, calls cmd.Execute()
cmd/
  root.go                  # Root command + full analysis mode
  highlight.go             # Quick highlight subcommand
internal/
  jira/client.go           # Jira API wrapper using jiracrawler library
  jira/cache.go            # 24-hour Jira issue cache
  github/client.go         # GitHub API client (PRs, issues, gists, user activity)
  github/cache.go          # Caching for GitHub PRs/issues (24h) and user activity (1h)
  ollama/client.go         # Ollama LLM API client for summary generation
```

### Key Data Flow

1. **Jira Client** (`internal/jira/`) - Wraps the `sebrandon1/jiracrawler` library. Fetches issues assigned to a user within a date range. Supports enhanced context (comments, history).

2. **GitHub Client** (`internal/github/`) - Direct HTTP client to GitHub API. Two main functions:
   - Extract GitHub URLs from Jira issues and fetch PR/issue details
   - Fetch user's comprehensive GitHub activity (PRs created, issues filed, events)

3. **Ollama Client** (`internal/ollama/`) - Sends prompts to local Ollama instance. Generates separate Jira and GitHub summaries, then combines with quantitative metrics.

4. **Caching** - Both Jira and GitHub clients use file-based caching (`~/.perfdive/cache/`) to minimize API calls and handle rate limits.

### Configuration

Configuration via `~/.perfdive.yaml` or command line flags:

```yaml
jira:
  url: "https://issues.redhat.com"
  username: "your-email@company.com"
  token: "your-jira-api-token"
ollama:
  url: "http://localhost:11434"
github:
  token: "your-github-token"
  gist_url: "https://gist.github.com/user/id"  # For journal feature
```

## Key Dependencies

- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/sebrandon1/jiracrawler` - Jira API client library

## Go Version

This project uses Go 1.25.4.
