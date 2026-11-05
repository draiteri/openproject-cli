package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/daniele-raiteri/openproject-cli/internal/client"
	"github.com/spf13/cobra"
)

func newWorkPackagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work-packages",
		Short: "Manage work packages",
		Long:  `List, show, create, update, comment on, attach files to, and delete work packages.`,
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
			page, pageSize, err := pageFlags(cmd)
			if err != nil {
				return err
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

			params := pageParams(page, pageSize)

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

			body, err := getOK(c, path, "work packages")
			if err != nil {
				return err
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

			fmt.Fprintf(cmd.OutOrStdout(), "Work Packages (%d total, page %d/%d):\n",
				result.Total, page, pageCount(result.Total, pageSize))
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
	addPageFlags(listCmd, 20)
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

			body, err := getOK(c, "/api/v3/work_packages/"+args[0], "work package")
			if err != nil {
				return err
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
		Long: `Create a new work package in a project. Requires --project and --subject.

Use --parent to attach the new work package to an existing one at creation
time, rather than creating it orphaned and re-parenting it afterwards.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			projectID, _ := cmd.Flags().GetString("project")
			subject, _ := cmd.Flags().GetString("subject")
			wpType, _ := cmd.Flags().GetString("type")
			description, _ := cmd.Flags().GetString("description")
			parentID, _ := cmd.Flags().GetString("parent")

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

			links := make(map[string]interface{})
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
				links["type"] = map[string]interface{}{
					"href": fmt.Sprintf("/api/v3/types/%d", typeID),
				}
			}
			if parentID != "" {
				if _, err := strconv.Atoi(parentID); err != nil {
					return fmt.Errorf("--parent must be a work package ID, got %q", parentID)
				}
				links["parent"] = map[string]interface{}{
					"href": "/api/v3/work_packages/" + parentID,
				}
			}
			if len(links) > 0 {
				bodyMap["_links"] = links
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
				Links   struct {
					Parent *struct {
						Href  string `json:"href"`
						Title string `json:"title"`
					} `json:"parent"`
				} `json:"_links"`
			}
			if err := json.Unmarshal(body, &created); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created work package #%d: %s\n", created.ID, created.Subject)

			// Echo the parent the server actually recorded, so a silently
			// dropped --parent is visible rather than producing an orphan.
			if parentID != "" {
				if created.Links.Parent != nil && created.Links.Parent.Href != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Parent: #%s %s\n",
						parentID, created.Links.Parent.Title)
				} else {
					return fmt.Errorf("work package #%d was created but the server did not record parent #%s — it is orphaned",
						created.ID, parentID)
				}
			}
			return nil
		},
	}
	createCmd.Flags().StringP("project", "p", "", "Project ID (required)")
	createCmd.Flags().StringP("subject", "s", "", "Subject (required)")
	createCmd.Flags().StringP("type", "t", "", "Type name or ID (e.g. 'Task', 'Vacancy', '1')")
	createCmd.Flags().StringP("description", "d", "", "Description in markdown")
	createCmd.Flags().String("parent", "", "Parent work package ID")
	cmd.AddCommand(createCmd)

	// --- update ---
	updateCmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a work package",
		Long: `Update an existing work package's subject, description, status,
assignee, or parent.

--description replaces the description outright; it does not append. Use
"work-packages comment" for progress notes that should preserve history.

--parent re-parents the work package. Pass --parent none to detach it and
make it a top-level item.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			wpID := args[0]
			subject, _ := cmd.Flags().GetString("subject")
			statusName, _ := cmd.Flags().GetString("status")
			assigneeID, _ := cmd.Flags().GetString("assignee")
			description, _ := cmd.Flags().GetString("description")
			parentID, _ := cmd.Flags().GetString("parent")

			// Distinguish "flag absent" from "flag set to empty string", so an
			// explicitly blank description can clear the field.
			descriptionSet := cmd.Flags().Changed("description")

			if subject == "" && statusName == "" && assigneeID == "" &&
				!descriptionSet && parentID == "" {
				return fmt.Errorf("at least one of --subject, --description, --status, --assignee, --parent is required")
			}

			// Fetch current lockVersion
			body, err := getOK(c, "/api/v3/work_packages/"+wpID, "work package")
			if err != nil {
				return err
			}
			var current struct {
				LockVersion int `json:"lockVersion"`
			}
			if err := json.Unmarshal(body, &current); err != nil {
				return fmt.Errorf("parse current work package: %w", err)
			}

			bodyMap := map[string]interface{}{
				"lockVersion": current.LockVersion,
			}
			if subject != "" {
				bodyMap["subject"] = subject
			}
			if descriptionSet {
				bodyMap["description"] = map[string]interface{}{
					"format": "markdown",
					"raw":    description,
				}
			}
			if statusName != "" || assigneeID != "" || parentID != "" {
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
				if parentID != "" {
					// A null href detaches the work package from its parent.
					if equalFold(parentID, "none") || equalFold(parentID, "null") {
						links["parent"] = map[string]interface{}{"href": nil}
					} else {
						if _, err := strconv.Atoi(parentID); err != nil {
							return fmt.Errorf("--parent must be a work package ID or \"none\", got %q", parentID)
						}
						links["parent"] = map[string]interface{}{
							"href": "/api/v3/work_packages/" + parentID,
						}
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
	updateCmd.Flags().StringP("description", "d", "", "New description in markdown (replaces the existing one)")
	updateCmd.Flags().String("parent", "", "New parent work package ID, or \"none\" to detach")
	updateCmd.Flags().StringP("status", "t", "", "New status name or ID (e.g. 'Rejected', '14')")
	updateCmd.Flags().StringP("assignee", "a", "", "New assignee ID")
	cmd.AddCommand(updateCmd)

	// --- comment ---
	commentCmd := &cobra.Command{
		Use:   "comment <id> <text>",
		Short: "Add a comment to a work package",
		Long: `Add a comment to a work package's activity stream.

The comment body is interpreted as markdown. Use --no-notify to add the
comment without notifying watchers, the author, and the assignee.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			wpID, text := args[0], args[1]
			if text == "" {
				return fmt.Errorf("comment text must not be empty")
			}

			noNotify, _ := cmd.Flags().GetBool("no-notify")

			bodyJSON, err := json.Marshal(map[string]interface{}{
				"comment": map[string]interface{}{
					"raw": text,
				},
			})
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}

			path := fmt.Sprintf("/api/v3/work_packages/%s/activities", url.PathEscape(wpID))
			if noNotify {
				path += "?notify=false"
			}

			resp, err := c.Post(path, "application/json", bytes.NewReader(bodyJSON))
			if err != nil {
				return fmt.Errorf("add comment: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				return fmt.Errorf("comment failed (HTTP %d): %s", resp.StatusCode, string(body))
			}

			if outputFlag(cmd) == "json" {
				fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return nil
			}

			var created struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(body, &created); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added comment #%d to work package #%s\n", created.ID, wpID)
			return nil
		},
	}
	commentCmd.Flags().Bool("no-notify", false, "Do not notify watchers, author, and assignee")
	cmd.AddCommand(commentCmd)

	// --- attach ---
	attachCmd := &cobra.Command{
		Use:   "attach <id> <file>",
		Short: "Attach a file to a work package",
		Long: `Upload a local file and attach it to a work package.

By default the attachment keeps the local file's base name; use --name to
store it under a different name.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			wpID, filePath := args[0], args[1]
			description, _ := cmd.Flags().GetString("description")
			fileName, _ := cmd.Flags().GetString("name")
			if fileName == "" {
				fileName = filepath.Base(filePath)
			}

			f, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("open file: %w", err)
			}
			defer f.Close()

			info, err := f.Stat()
			if err != nil {
				return fmt.Errorf("stat file: %w", err)
			}
			if info.IsDir() {
				return fmt.Errorf("%s is a directory, not a file", filePath)
			}

			body, contentType, err := buildAttachmentBody(f, fileName, description)
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/api/v3/work_packages/%s/attachments", url.PathEscape(wpID))
			resp, err := c.Post(path, contentType, body)
			if err != nil {
				return fmt.Errorf("upload attachment: %w", err)
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				return fmt.Errorf("attach failed (HTTP %d): %s", resp.StatusCode, string(respBody))
			}

			if outputFlag(cmd) == "json" {
				fmt.Fprintln(cmd.OutOrStdout(), string(respBody))
				return nil
			}

			var created struct {
				ID       int    `json:"id"`
				FileName string `json:"fileName"`
				FileSize int64  `json:"fileSize"`
			}
			if err := json.Unmarshal(respBody, &created); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Attached %s (%d bytes) to work package #%s as attachment #%d\n",
				created.FileName, created.FileSize, wpID, created.ID)
			return nil
		},
	}
	attachCmd.Flags().StringP("description", "d", "", "Description of the attachment")
	attachCmd.Flags().StringP("name", "n", "", "Store under this file name instead of the local base name")
	cmd.AddCommand(attachCmd)

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

// buildAttachmentBody assembles the multipart/form-data body the attachments
// endpoint requires: exactly two parts, "metadata" (application/json, carrying
// at least fileName) and "file" (the raw bytes). OpenProject takes the stored
// name from the metadata part, not from the file part's Content-Disposition.
func buildAttachmentBody(file io.Reader, fileName, description string) (io.Reader, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	metadata := map[string]interface{}{
		"fileName": fileName,
	}
	if description != "" {
		metadata["description"] = map[string]interface{}{
			"format": "plain",
			"raw":    description,
		}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, "", fmt.Errorf("marshal attachment metadata: %w", err)
	}

	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metaHeader.Set("Content-Type", "application/json")
	metaPart, err := mw.CreatePart(metaHeader)
	if err != nil {
		return nil, "", fmt.Errorf("create metadata part: %w", err)
	}
	if _, err := metaPart.Write(metadataJSON); err != nil {
		return nil, "", fmt.Errorf("write metadata part: %w", err)
	}

	// CreateFormFile would hardcode application/octet-stream, so build the
	// part by hand to send a content type matching the file extension.
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename=%q`, fileName))
	fileHeader.Set("Content-Type", contentTypeFor(fileName))
	filePart, err := mw.CreatePart(fileHeader)
	if err != nil {
		return nil, "", fmt.Errorf("create file part: %w", err)
	}
	if _, err := io.Copy(filePart, file); err != nil {
		return nil, "", fmt.Errorf("write file part: %w", err)
	}

	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}

	return &buf, mw.FormDataContentType(), nil
}

// contentTypeFor guesses a MIME type from a file name's extension.
func contentTypeFor(fileName string) string {
	if ct := mime.TypeByExtension(filepath.Ext(fileName)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// lookupStatusID fetches the statuses from the API and finds the ID for a given name.
// If the name matches multiple statuses, returns an error.
func lookupStatusID(c *client.Client, name string) (int, error) {
	body, err := getOK(c, "/api/v3/statuses", "statuses")
	if err != nil {
		return 0, err
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
	body, err := getOK(c, "/api/v3/types", "types")
	if err != nil {
		return 0, err
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
