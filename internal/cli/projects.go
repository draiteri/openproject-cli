package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newProjectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List and manage projects",
		Long:  `Read projects from the OpenProject server.`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long:  `List all projects visible to the authenticated user.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			page, pageSize, err := pageFlags(cmd)
			if err != nil {
				return err
			}

			body, err := getOK(c, "/api/v3/projects?"+pageParams(page, pageSize).Encode(), "projects")
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
						ID         int    `json:"id"`
						Name       string `json:"name"`
						Identifier string `json:"identifier"`
						Active     bool   `json:"active"`
					} `json:"elements"`
				} `json:"_embedded"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Projects (%d total, page %d/%d):\n",
				result.Total, page, pageCount(result.Total, pageSize))
			header := []string{"ID", "Name", "Identifier", "Active"}
			var rows [][]string
			for _, p := range result.Embedded.Elements {
				active := "yes"
				if !p.Active {
					active = "no"
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", p.ID),
					p.Name,
					p.Identifier,
					active,
				})
			}
			printTable(cmd.OutOrStdout(), header, rows)
			return nil
		},
	}
	addPageFlags(listCmd, listAllPageSize)
	cmd.AddCommand(listCmd)

	return cmd
}
