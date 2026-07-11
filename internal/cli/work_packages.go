package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
		Long:  `List work packages, optionally filtered by project.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			projectID, _ := cmd.Flags().GetString("project")
			path := "/api/v3/work_packages"
			if projectID != "" {
				path = fmt.Sprintf("/api/v3/projects/%s/work_packages", projectID)
			}

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
				Total int `json:"total"`
				Count int `json:"count"`
				Embedded struct {
					Elements []struct {
						ID          int    `json:"id"`
						Subject     string `json:"subject"`
						LockVersion int    `json:"lockVersion"`
					} `json:"elements"`
				} `json:"_embedded"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Work Packages (%d total):\n", result.Total)
			header := []string{"ID", "Subject"}
			var rows [][]string
			for _, wp := range result.Embedded.Elements {
				rows = append(rows, []string{
					fmt.Sprintf("%d", wp.ID),
					wp.Subject,
				})
			}
			printTable(cmd.OutOrStdout(), header, rows)
			return nil
		},
	}
	listCmd.Flags().StringP("project", "p", "", "Filter by project ID")
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
				"_type":   "WorkPackage",
				"subject": subject,
				"lockVersion": 0,
			}
			if wpType != "" {
				bodyMap["_links"] = map[string]interface{}{
					"type": map[string]interface{}{
						"href": fmt.Sprintf("/api/v3/types/%s", wpType),
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
	createCmd.Flags().StringP("type", "t", "", "Type ID (default: first type of the project)")
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
			statusID, _ := cmd.Flags().GetString("status")
			assigneeID, _ := cmd.Flags().GetString("assignee")

			if subject == "" && statusID == "" && assigneeID == "" {
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
			if statusID != "" || assigneeID != "" {
				links := make(map[string]interface{})
				if statusID != "" {
					links["status"] = map[string]interface{}{
						"href": fmt.Sprintf("/api/v3/statuses/%s", statusID),
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
	updateCmd.Flags().StringP("status", "t", "", "New status ID")
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
