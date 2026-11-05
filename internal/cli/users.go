package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List users",
		Long:  `Read users from the OpenProject server.`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all users",
		Long:  `List all users on the OpenProject server.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			page, pageSize, err := pageFlags(cmd)
			if err != nil {
				return err
			}

			body, err := getOK(c, "/api/v3/users?"+pageParams(page, pageSize).Encode(), "users")
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
						ID        int    `json:"id"`
						Name      string `json:"name"`
						FirstName string `json:"firstName"`
						LastName  string `json:"lastName"`
						Email     string `json:"email"`
						Admin     bool   `json:"admin"`
						Login     string `json:"login"`
					} `json:"elements"`
				} `json:"_embedded"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Users (%d total, page %d/%d):\n",
				result.Total, page, pageCount(result.Total, pageSize))
			header := []string{"ID", "Name", "Email", "Login", "Admin"}
			var rows [][]string
			for _, u := range result.Embedded.Elements {
				admin := "no"
				if u.Admin {
					admin = "yes"
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", u.ID),
					u.Name,
					u.Email,
					u.Login,
					admin,
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
