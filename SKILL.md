---
name: openproject-cli
description: "Interact with OpenProject (projects, work packages, users, time entries) via the API. Trigger phrases: `openproject`, `work packages`, `projects`, `tasks`, `create task`, `create work package`, `list projects`, `list work packages`, `show work package`, `update work package`, `delete work package`, `time entries`, `applications`, `vacancies`, `rejected`."
author: "Daniele Raiteri"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - openproject-cli
---

# OpenProject CLI

## Prerequisites

This skill ships a **standalone, self-contained binary** at `bin/openproject-cli`
inside the skill directory. There is nothing to install and nothing to put on
`$PATH` — the skill folder is fully relocatable.

**Invoke the binary by its path inside the skill folder**, e.g.:

```bash
"<skill-dir>/bin/openproject-cli" --version
```

where `<skill-dir>` is the directory this `SKILL.md` lives in. All command
examples below are written as `openproject-cli ...` for brevity — read that as
the bundled `<skill-dir>/bin/openproject-cli`.

If `--version` fails (e.g. the prebuilt binary was built for a different
architecture), run the bundled `install.sh` from the skill folder to rebuild
(`go` required only in that fallback case).

## When to Use This CLI

Reach for this CLI when an agent needs to read or write OpenProject data
programmatically — list projects, find and manage work packages, create tasks,
track time, or look up users.

## Setup

### Credentials

Set these environment variables (or create a `.env` file in the skill directory
from `.env.example`):

```bash
export OPENPROJECT_USERNAME="your-email@example.com"
export OPENPROJECT_PASSWORD="your-password"
# Optional, defaults to the instance below:
export OPENPROJECT_URL="https://openproject.danieleraiteri.duckdns.org"
```

### Verify Authentication

```bash
openproject-cli auth status
# → Logged in as Daniele Raiteri (ID 5)
```

## Commands

### Authentication

```bash
openproject-cli auth login      # Login and verify the session
openproject-cli auth status     # Check if the session is valid
```

### Projects

```bash
openproject-cli projects list                         # List all projects (table)
openproject-cli projects list --output json           # List all projects (JSON)
```

### Users

```bash
openproject-cli users list                            # List all users
openproject-cli users list --output json              # List all users (JSON)
```

### Work Packages

**List** — by default shows 20 open (non-closed) work packages, with Status, Type, and Project columns.

```bash
openproject-cli work-packages list                                   # First 20 open (page 1)
openproject-cli work-packages list --all                             # All statuses (including closed/rejected)
openproject-cli work-packages list --project 22                      # Filter by project ID
openproject-cli work-packages list --status Rejected                 # Filter by status name
openproject-cli work-packages list --type Vacancy                    # Filter by type name
openproject-cli work-packages list --type Vacancy --status Rejected  # Combined filters
openproject-cli work-packages list --all --page 2 --page-size 50     # Pagination: page 2, 50 per page
openproject-cli work-packages list --output json                     # JSON output
```

**Available status names:** New, In Progress, Waiting, On Hold, Summary, Plan, Attend, Applied, Interview, Negotiation, To Buy, Ordered, Cancelled, Rejected, Closed, Check Again Later

**Available type names:** Task, Milestone, Summary task, Event, Shopping, Vacancy

**Show / Create / Update / Delete:**

```bash
openproject-cli work-packages show 60                                # Show full details
openproject-cli work-packages show 60 --output json                  # Show as JSON
openproject-cli work-packages create --project 22 --subject "Do X"   # Create a task
openproject-cli work-packages create --project 22 --subject "Do X" \
  --description "Details here" --type "Vacancy"                      # Create with type name
openproject-cli work-packages create --project 11 --subject "Bug" \
  --type "Task"                                                      # Create with type name
openproject-cli work-packages update 60 --status "Rejected"          # Update status by name
openproject-cli work-packages update 60 --status "14"                # Update status by ID also works
openproject-cli work-packages update 60 --assignee 5                 # Assign to user
openproject-cli work-packages update 60 --subject "New title"        # Rename
openproject-cli work-packages delete 60                              # Delete
```

### Time Entries

```bash
openproject-cli time-entries list                                          # List all
openproject-cli time-entries list --project 22                             # Filter by project
openproject-cli time-entries list --work-package 60                        # Filter by work package
openproject-cli time-entries create --work-package 60 --hours 2.5 \        # Log 2h30m
  --comment "Worked on X"
```

### Global Flags

| Flag | Shorthand | Description |
|---|---|---|
| `--output` | `-o` | Output format: `table` (default) or `json` |
| `--help` | `-h` | Show help |
| `--version` | `-v` | Show version |

## Unique Capabilities

- **Session-based auth** — Authenticates via web login, handles CSRF automatically
- **Full CRUD** — Create, read, update, and delete work packages
- **Filter by name** — `--status Rejected` and `--type Vacancy` use human-readable names, not IDs
- **See everything** — `--all` bypasses the default "open only" filter to show closed/rejected items
- **Pagination** — `--page` and `--page-size` control result size
- **JSON output** — All list/show commands support `--output json` for programmatic use
- **No dependencies** — Single self-contained binary in the skill directory
- **Agent-friendly tables** — Default output shows ID, Subject, Status, Type, and Project columns

## Common Agent Use Cases

### "Show me all rejected job applications"

```bash
openproject-cli work-packages list --type Vacancy --status Rejected --all
```

### "How many open vacancies are there?"

```bash
openproject-cli work-packages list --type Vacancy --status New
openproject-cli work-packages list --type Vacancy --status "In Progress"
```

### "What work packages are in the Travel project?"

```bash
openproject-cli work-packages list --project 22
```

### "Create a new task in IT project"

```bash
openproject-cli work-packages create --project 11 --subject "Fix server SSL" \
  --type "Task" --description "Renew the Let's Encrypt certificate"
```

### "Show me the full details of work package 109"

```bash
openproject-cli work-packages show 109
```

### "Log 3 hours on work package 109"

```bash
openproject-cli time-entries create --work-package 109 --hours 3 \
  --comment "Investigated 2FA issue"
```

### "Get all work packages as JSON"

```bash
openproject-cli work-packages list --all --page-size 200 --output json
```
