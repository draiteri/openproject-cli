package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List users",
		Long:  `Read users from the OpenProject server.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all users",
		Long:  `List all users on the OpenProject server.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := getClient()
			if c == nil {
				return fmt.Errorf("not authenticated")
			}

			resp, err := c.Get("/api/v3/users")
			if err != nil {
				return fmt.Errorf("fetch users: %w", err)
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

			fmt.Fprintf(cmd.OutOrStdout(), "Users (%d total):\n", result.Total)
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
	})

	return cmd
}
