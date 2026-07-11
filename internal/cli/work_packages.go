package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/daniele-raiteri/openproject-cli/internal/client"
	"github.com/spf13/cobra"
)

func newWorkPackagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work-packages",
		Short: "Manage work packages",
		Long:  `List, show, create, update, and delete work packages.`,
	}

	// --- list ---
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List work packages",
		Long: `List work packages with optional filters.

By default only open (non-closed) work packages are shown. Use --all to see
every status, or --status to filter by a specific status name.

Status names: New, In Progress, Waiting, On Hold, Summary, Plan, Attend, 
              Applied, Interview, Negotiation, To Buy, Ordered, Cancelled,
              Rejected, Closed, Check Again Later
Type names: Task, Milestone, Summary task, Event, Shopping, Vacancy`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			projectID, _ := cmd.Flags().GetString("project")
			statusFilter, _ := cmd.Flags().GetString("status")
			typeFilter, _ := cmd.Flags().GetString("type")
			showAll, _ := cmd.Flags().GetBool("all")
			page, _ := cmd.Flags().GetInt("page")
			pageSize, _ := cmd.Flags().GetInt("page-size")

			if page < 1 {
				page = 1
			}
			if pageSize < 1 || pageSize > 200 {
				pageSize = 20
			}

			// Build filters
			var filters []map[string]interface{}

			if showAll {
				// Show all statuses (bypass default open filter)
				filters = append(filters, map[string]interface{}{
					"status": map[string]interface{}{
						"operator": "*",
						"values":   []interface{}{},
					},
				})
			}

			if statusFilter != "" {
				// Look up status ID by name
				statusID, err := lookupStatusID(c, statusFilter)
				if err != nil {
					return fmt.Errorf("lookup status: %w", err)
				}
				filters = append(filters, map[string]interface{}{
					"status": map[string]interface{}{
						"operator": "=",
						"values":   []interface{}{strconv.Itoa(statusID)},
					},
				})
			}

			if typeFilter != "" {
				// Look up type ID by name
				typeID, err := lookupTypeID(c, typeFilter)
				if err != nil {
					return fmt.Errorf("lookup type: %w", err)
				}
				filters = append(filters, map[string]interface{}{
					"type": map[string]interface{}{
						"operator": "=",
						"values":   []interface{}{strconv.Itoa(typeID)},
					},
				})
			}

			// Build URL
			path := "/api/v3/work_packages"

			params := url.Values{}
			params.Set("offset", strconv.Itoa((page-1)*pageSize+1))
			params.Set("pageSize", strconv.Itoa(pageSize))

			if len(filters) > 0 {
				filterJSON, err := json.Marshal(filters)
				if err != nil {
					return fmt.Errorf("marshal filters: %w", err)
				}
				params.Set("filters", string(filterJSON))
			}

			// If project is specified, use project-specific endpoint
			if projectID != "" {
				path = fmt.Sprintf("/api/v3/projects/%s/work_packages", projectID)
			}

			path = path + "?" + params.Encode()

			resp, err := c.Get(path)
			if err != nil {
				return fmt.Errorf("fetch work packages: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if outputFlag(cmd) == "json" {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return nil
			}

			var result struct {
				Total  int `json:"total"`
				Count  int `json:"count"`
				Offset int `json:"offset"`
				Embedded struct {
					Elements []struct {
						ID        int    `json:"id"`
						Subject   string `json:"subject"`
						StartDate string `json:"startDate"`
						DueDate   string `json:"dueDate"`
						Links     struct {
							Status  *struct{ Title string } `json:"status"`
							Type    *struct{ Title string } `json:"type"`
							Project *struct{ Title string } `json:"project"`
						} `json:"_links"`
					} `json:"elements"`
				} `json:"_embedded"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			// Calculate total pages
			totalPages := (result.Total + pageSize - 1) / pageSize

			fmt.Fprintf(cmd.OutOrStdout(), "Work Packages (%d total, page %d/%d):\n",
				result.Total, page, totalPages)
			header := []string{"ID", "Subject", "Status", "Type", "Project"}
			var rows [][]string
			for _, wp := range result.Embedded.Elements {
				status := ""
				if wp.Links.Status != nil {
					status = wp.Links.Status.Title
				}
				wpType := ""
				if wp.Links.Type != nil {
					wpType = wp.Links.Type.Title
				}
				project := ""
				if wp.Links.Project != nil {
					project = wp.Links.Project.Title
				}

				rows = append(rows, []string{
					fmt.Sprintf("%d", wp.ID),
					wp.Subject,
					status,
					wpType,
					project,
				})
			}
			// Sort by ID for consistent output
			sort.Slice(rows, func(i, j int) bool {
				idI, _ := strconv.Atoi(rows[i][0])
				idJ, _ := strconv.Atoi(rows[j][0])
				return idI < idJ
			})
			printTable(cmd.OutOrStdout(), header, rows)
			return nil
		},
	}
	listCmd.Flags().StringP("project", "p", "", "Filter by project ID")
	listCmd.Flags().StringP("status", "s", "", "Filter by status name (e.g. 'Rejected', 'New')")
	listCmd.Flags().StringP("type", "t", "", "Filter by type name (e.g. 'Vacancy', 'Task')")
	listCmd.Flags().BoolP("all", "a", false, "Show all work packages (including closed)")
	listCmd.Flags().IntP("page", "", 1, "Page number")
	listCmd.Flags().IntP("page-size", "", 20, "Items per page (max 200)")
	cmd.AddCommand(listCmd)

	// --- show ---
	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show work package details",
		Long:  `Display full details of a single work package.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			resp, err := c.Get("/api/v3/work_packages/" + args[0])
			if err != nil {
				return fmt.Errorf("fetch work package: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if outputFlag(cmd) == "json" {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return nil
			}

			var wp struct {
				ID          int    `json:"id"`
				Subject     string `json:"subject"`
				Description struct {
					Raw string `json:"raw"`
				} `json:"description"`
				StartDate   string `json:"startDate"`
				DueDate     string `json:"dueDate"`
				LockVersion int    `json:"lockVersion"`
				Embedded    struct {
					Project struct {
						Name string `json:"name"`
						ID   int    `json:"id"`
					} `json:"project"`
					Type struct {
						Name string `json:"name"`
					} `json:"type"`
					Status struct {
						Name string `json:"name"`
					} `json:"status"`
					Priority struct {
						Name string `json:"name"`
					} `json:"priority"`
					Assignee *struct {
						Name string `json:"name"`
					} `json:"assignee,omitempty"`
				} `json:"_embedded"`
			}
			if err := json.Unmarshal(body, &wp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Work Package #%d\n", wp.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "  Subject:  %s\n", wp.Subject)
			fmt.Fprintf(cmd.OutOrStdout(), "  Project:  %s\n", wp.Embedded.Project.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  Type:     %s\n", wp.Embedded.Type.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  Status:   %s\n", wp.Embedded.Status.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "  Priority: %s\n", wp.Embedded.Priority.Name)
			if wp.Embedded.Assignee != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  Assignee: %s\n", wp.Embedded.Assignee.Name)
			}
			if wp.StartDate != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Start:    %s\n", wp.StartDate)
			}
			if wp.DueDate != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Due:      %s\n", wp.DueDate)
			}
			if wp.Description.Raw != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Description:\n%s\n", wp.Description.Raw)
			}
			return nil
		},
	}
	cmd.AddCommand(showCmd)

	// --- create ---
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a work package",
		Long:  `Create a new work package in a project. Requires --project and --subject.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			projectID, _ := cmd.Flags().GetString("project")
			subject, _ := cmd.Flags().GetString("subject")
			wpType, _ := cmd.Flags().GetString("type")
			description, _ := cmd.Flags().GetString("description")

			if projectID == "" {
				return fmt.Errorf("--project is required")
			}
			if subject == "" {
				return fmt.Errorf("--subject is required")
			}

			bodyMap := map[string]interface{}{
				"_type":       "WorkPackage",
				"subject":     subject,
				"lockVersion": 0,
			}
			if wpType != "" {
				// Try to resolve type name to ID
				typeID, err := lookupTypeID(c, wpType)
				if err != nil {
					// Maybe it's already an ID
					if _, err := strconv.Atoi(wpType); err == nil {
						typeID, _ = strconv.Atoi(wpType)
					} else {
						return fmt.Errorf("resolve type name: %w", err)
					}
				}
				bodyMap["_links"] = map[string]interface{}{
					"type": map[string]interface{}{
						"href": fmt.Sprintf("/api/v3/types/%d", typeID),
					},
				}
			}
			if description != "" {
				bodyMap["description"] = map[string]interface{}{
					"format": "markdown",
					"raw":    description,
				}
			}

			bodyJSON, err := json.Marshal(bodyMap)
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}

			path := fmt.Sprintf("/api/v3/projects/%s/work_packages", projectID)
			resp, err := c.Post(path, "application/json", bytes.NewReader(bodyJSON))
			if err != nil {
				return fmt.Errorf("create work package: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if resp.StatusCode != http.StatusCreated {
				return fmt.Errorf("create failed (HTTP %d): %s", resp.StatusCode, string(body))
			}

			var created struct {
				ID      int    `json:"id"`
				Subject string `json:"subject"`
			}
			if err := json.Unmarshal(body, &created); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created work package #%d: %s\n", created.ID, created.Subject)
			return nil
		},
	}
	createCmd.Flags().StringP("project", "p", "", "Project ID (required)")
	createCmd.Flags().StringP("subject", "s", "", "Subject (required)")
	createCmd.Flags().StringP("type", "t", "", "Type name or ID (e.g. 'Task', 'Vacancy', '1')")
	createCmd.Flags().StringP("description", "d", "", "Description in markdown")
	cmd.AddCommand(createCmd)

	// --- update ---
	updateCmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a work package",
		Long:  `Update an existing work package's subject, status, or assignee.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			wpID := args[0]
			subject, _ := cmd.Flags().GetString("subject")
			statusName, _ := cmd.Flags().GetString("status")
			assigneeID, _ := cmd.Flags().GetString("assignee")

			if subject == "" && statusName == "" && assigneeID == "" {
				return fmt.Errorf("at least one of --subject, --status, --assignee is required")
			}

			// Fetch current lockVersion
			resp, err := c.Get("/api/v3/work_packages/" + wpID)
			if err != nil {
				return fmt.Errorf("fetch work package: %w", err)
			}
			var current struct {
				LockVersion int `json:"lockVersion"`
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err := json.Unmarshal(body, &current); err != nil {
				return fmt.Errorf("parse current work package: %w", err)
			}

			bodyMap := map[string]interface{}{
				"lockVersion": current.LockVersion,
			}
			if subject != "" {
				bodyMap["subject"] = subject
			}
			if statusName != "" || assigneeID != "" {
				links := make(map[string]interface{})
				if statusName != "" {
					// Resolve status name to ID
					statusID, err := lookupStatusID(c, statusName)
					if err != nil {
						// Maybe it's already an ID
						if _, err := strconv.Atoi(statusName); err == nil {
							statusID, _ = strconv.Atoi(statusName)
						} else {
							return fmt.Errorf("resolve status name: %w", err)
						}
					}
					links["status"] = map[string]interface{}{
						"href": fmt.Sprintf("/api/v3/statuses/%d", statusID),
					}
				}
				if assigneeID != "" {
					links["assignee"] = map[string]interface{}{
						"href": fmt.Sprintf("/api/v3/users/%s", assigneeID),
					}
				}
				bodyMap["_links"] = links
			}

			bodyJSON, err := json.Marshal(bodyMap)
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}

			patchResp, err := c.Patch("/api/v3/work_packages/"+wpID, "application/json", bytes.NewReader(bodyJSON))
			if err != nil {
				return fmt.Errorf("update work package: %w", err)
			}
			defer patchResp.Body.Close()

			patchBody, err := io.ReadAll(patchResp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if patchResp.StatusCode != http.StatusOK {
				return fmt.Errorf("update failed (HTTP %d): %s", patchResp.StatusCode, string(patchBody))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Updated work package #%s\n", wpID)
			return nil
		},
	}
	updateCmd.Flags().StringP("subject", "s", "", "New subject")
	updateCmd.Flags().StringP("status", "t", "", "New status name or ID (e.g. 'Rejected', '14')")
	updateCmd.Flags().StringP("assignee", "a", "", "New assignee ID")
	cmd.AddCommand(updateCmd)

	// --- delete ---
	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a work package",
		Long:  `Delete a work package by ID. This cannot be undone.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			resp, err := c.Delete("/api/v3/work_packages/" + args[0])
			if err != nil {
				return fmt.Errorf("delete work package: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNoContent {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("delete failed (HTTP %d): %s", resp.StatusCode, string(body))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted work package #%s\n", args[0])
			return nil
		},
	}
	cmd.AddCommand(deleteCmd)

	return cmd
}

// lookupStatusID fetches the statuses from the API and finds the ID for a given name.
// If the name matches multiple statuses, returns an error.
func lookupStatusID(c *client.Client, name string) (int, error) {
	resp, err := c.Get("/api/v3/statuses")
	if err != nil {
		return 0, fmt.Errorf("fetch statuses: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read statuses: %w", err)
	}

	var result struct {
		Embedded struct {
			Elements []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"elements"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parse statuses: %w", err)
	}

	var matchID int
	matchCount := 0
	for _, s := range result.Embedded.Elements {
		// Case-insensitive comparison
		if equalFold(s.Name, name) {
			matchID = s.ID
			matchCount++
		}
	}

	if matchCount == 0 {
		return 0, fmt.Errorf("no status found matching '%s'", name)
	}
	if matchCount > 1 {
		return 0, fmt.Errorf("multiple statuses match '%s'", name)
	}
	return matchID, nil
}

// lookupTypeID fetches the types from the API and finds the ID for a given name.
func lookupTypeID(c *client.Client, name string) (int, error) {
	resp, err := c.Get("/api/v3/types")
	if err != nil {
		return 0, fmt.Errorf("fetch types: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read types: %w", err)
	}

	var result struct {
		Embedded struct {
			Elements []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"elements"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parse types: %w", err)
	}

	var matchID int
	matchCount := 0
	for _, t := range result.Embedded.Elements {
		if equalFold(t.Name, name) {
			matchID = t.ID
			matchCount++
		}
	}

	if matchCount == 0 {
		return 0, fmt.Errorf("no type found matching '%s'", name)
	}
	if matchCount > 1 {
		return 0, fmt.Errorf("multiple types match '%s'", name)
	}
	return matchID, nil
}

// equalFold is like strings.EqualFold but handles simple ASCII case folding
// without requiring the strings package import for this file.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca := a[i]
		cb := b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
