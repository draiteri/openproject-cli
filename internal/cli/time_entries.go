package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

// timeEntryFilters builds the list filters. OpenProject has no per-project or
// per-work-package time entry routes, so both go through the filters parameter.
func timeEntryFilters(projectID, wpID string) []map[string]interface{} {
	equals := func(name, value string) map[string]interface{} {
		return map[string]interface{}{
			name: map[string]interface{}{"operator": "=", "values": []string{value}},
		}
	}

	var filters []map[string]interface{}
	if projectID != "" {
		filters = append(filters, equals("project_id", projectID))
	}
	if wpID != "" {
		filters = append(filters, equals("entity_type", "WorkPackage"), equals("entity_id", wpID))
	}
	return filters
}

func newTimeEntriesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "time-entries",
		Short: "Manage time entries",
		Long:  `List and create time entries.`,
	}

	// list subcommand
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List time entries",
		Long:  `List time entries, optionally filtered by project and work package.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			projectID, _ := cmd.Flags().GetString("project")
			wpID, _ := cmd.Flags().GetString("work-package")

			page, pageSize, err := pageFlags(cmd)
			if err != nil {
				return err
			}

			params := pageParams(page, pageSize)
			if filters := timeEntryFilters(projectID, wpID); len(filters) > 0 {
				filterJSON, err := json.Marshal(filters)
				if err != nil {
					return fmt.Errorf("marshal filters: %w", err)
				}
				params.Set("filters", string(filterJSON))
			}

			body, err := getOK(c, "/api/v3/time_entries?"+params.Encode(), "time entries")
			if err != nil {
				return err
			}

			if outputFlag(cmd) == "json" {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return nil
			}

			var result struct {
				Total int `json:"total"`
				Count int `json:"count"`
				Embedded struct {
					Elements []struct {
						ID        int     `json:"id"`
						Hours     float64 `json:"hours"`
						Comment   string  `json:"comment"`
						SpentOn   string  `json:"spentOn"`
						CreatedAt string  `json:"createdAt"`
						Embedded  struct {
							WorkPackage struct {
								Subject string `json:"subject"`
								ID      int    `json:"id"`
							} `json:"workPackage"`
							Project struct {
								Name string `json:"name"`
							} `json:"project"`
							User struct {
								Name string `json:"name"`
							} `json:"user"`
						} `json:"_embedded"`
					} `json:"elements"`
				} `json:"_embedded"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Time Entries (%d total, page %d/%d):\n",
				result.Total, page, pageCount(result.Total, pageSize))
			header := []string{"ID", "Date", "Hours", "Comment", "Work Package", "Project", "User"}
			var rows [][]string
			for _, te := range result.Embedded.Elements {
				comment := te.Comment
				if len(comment) > 50 {
					comment = comment[:50] + "..."
				}
				date := te.SpentOn
				if date == "" {
					date = te.CreatedAt[:10]
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", te.ID),
					date,
					fmt.Sprintf("%.2f", te.Hours),
					comment,
					fmt.Sprintf("#%d: %s", te.Embedded.WorkPackage.ID, te.Embedded.WorkPackage.Subject),
					te.Embedded.Project.Name,
					te.Embedded.User.Name,
				})
			}
			printTable(cmd.OutOrStdout(), header, rows)
			return nil
		},
	}
	listCmd.Flags().StringP("project", "p", "", "Filter by project ID")
	listCmd.Flags().StringP("work-package", "W", "", "Filter by work package ID")
	addPageFlags(listCmd, listAllPageSize)
	cmd.AddCommand(listCmd)

	// create subcommand
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a time entry",
		Long:  `Create a new time entry for a work package.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			wpID, _ := cmd.Flags().GetString("work-package")
			hoursStr, _ := cmd.Flags().GetString("hours")
			comment, _ := cmd.Flags().GetString("comment")
			spentOn, _ := cmd.Flags().GetString("spent-on")

			if wpID == "" {
				return fmt.Errorf("--work-package is required")
			}
			if hoursStr == "" {
				return fmt.Errorf("--hours is required")
			}

			// Convert hours to ISO 8601 duration format (e.g., "2.5" -> "PT2H30M")
			hoursISO, err := toISODuration(hoursStr)
			if err != nil {
				return fmt.Errorf("invalid hours: %w", err)
			}

			bodyMap := map[string]interface{}{
				"hours": hoursISO,
				"_links": map[string]interface{}{
					"workPackage": map[string]interface{}{
						"href": fmt.Sprintf("/api/v3/work_packages/%s", wpID),
					},
				},
			}
			if comment != "" {
				bodyMap["comment"] = map[string]interface{}{
					"format": "plain",
					"raw":    comment,
				}
			}
			if spentOn != "" {
				bodyMap["spentOn"] = spentOn
			}

			bodyJSON, err := json.Marshal(bodyMap)
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}

			resp, err := c.Post("/api/v3/time_entries", "application/json", bytes.NewReader(bodyJSON))
			if err != nil {
				return fmt.Errorf("create time entry: %w", err)
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
				ID    int     `json:"id"`
				Hours float64 `json:"hours"`
			}
			if err := json.Unmarshal(body, &created); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created time entry #%d (%.2f hours)\n", created.ID, created.Hours)
			return nil
		},
	}
	createCmd.Flags().StringP("work-package", "w", "", "Work package ID (required)")
	createCmd.Flags().StringP("hours", "H", "", "Number of hours (required, e.g. '2.5' for 2h30m)")
	createCmd.Flags().StringP("comment", "c", "", "Comment")
	createCmd.Flags().StringP("spent-on", "d", "", "Date (YYYY-MM-DD), defaults to today")
	cmd.AddCommand(createCmd)

	return cmd
}

// toISODuration converts a decimal hours string to ISO 8601 duration format.
// Examples: "2.5" -> "PT2H30M", "1" -> "PT1H", "0.25" -> "PT15M".
func toISODuration(hoursStr string) (string, error) {
	var hours float64
	if _, err := fmt.Sscanf(hoursStr, "%f", &hours); err != nil {
		return "", fmt.Errorf("could not parse '%s' as a number", hoursStr)
	}
	if hours <= 0 {
		return "", fmt.Errorf("hours must be positive, got %f", hours)
	}

	totalMinutes := int(hours * 60)
	h := totalMinutes / 60
	m := totalMinutes % 60

	if m == 0 {
		return fmt.Sprintf("PT%dH", h), nil
	}
	if h == 0 {
		return fmt.Sprintf("PT%dM", m), nil
	}
	return fmt.Sprintf("PT%dH%dM", h, m), nil
}
