# OpenProject CLI

A standalone CLI tool for interacting with the [OpenProject](https://www.openproject.org/) API v3.
Designed to be used as a skill for AI coding agents (Claude/pi).

## Features

- **Projects** — List and view projects
- **Work Packages** — List, show, create, update, comment on, attach files to, and delete work packages (tasks, bugs, features, etc.)
- **Users** — List users
- **Time Entries** — List and create time entries
- **API key auth** — Authenticates with an OpenProject API token (HTTP Basic `apikey:<token>`), with a legacy session-login fallback
- **Agent-friendly output** — Tabular and JSON output modes for easy parsing

## Quick Start

```bash
# Set credentials (My Account > Access tokens > API in OpenProject)
export OPENPROJECT_API_KEY="your-api-token"

# Run the CLI
./bin/openproject-cli --help
```

## Commands

```
openproject-cli auth login          — Login and store session
openproject-cli auth status         — Check authentication status
openproject-cli projects list       — List all projects
openproject-cli users list          — List all users
openproject-cli work-packages list  — List work packages (optionally --project <id>)
openproject-cli work-packages show  — Show work package details
openproject-cli work-packages create — Create a work package
openproject-cli work-packages update — Update a work package
openproject-cli work-packages comment — Add a comment to a work package
openproject-cli work-packages attach — Attach a file to a work package
openproject-cli work-packages delete — Delete a work package
openproject-cli time-entries list   — List time entries
openproject-cli time-entries create — Create a time entry
```

## Configuration

The CLI reads these environment variables:

| Variable | Description |
|---|---|
| `OPENPROJECT_API_KEY` | API token from *My Account > Access tokens > API*. Preferred. |
| `OPENPROJECT_USERNAME` | OpenProject login email. Fallback, only used if no API key is set. |
| `OPENPROJECT_PASSWORD` | OpenProject password. Fallback, only used if no API key is set. |
| `OPENPROJECT_URL` | Base URL (default: `https://openproject.danieleraiteri.duckdns.org`) |

A `.env` file in the skill directory is also supported (see `.env.example`).

## Development

```bash
go build -o bin/openproject-cli ./cmd/openproject-cli
```

## License

Apache-2.0
