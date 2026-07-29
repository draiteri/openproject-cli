package cli

import (
	"fmt"

	"github.com/daniele-raiteri/openproject-cli/internal/client"
	"github.com/daniele-raiteri/openproject-cli/internal/config"
	"github.com/spf13/cobra"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
		Long:  `Login, check status, or logout from the OpenProject server.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "login",
		Short: "Authenticate with the OpenProject server",
		Long: `Authenticate with the OpenProject server using the API token from the
OPENPROJECT_API_KEY environment variable, falling back to OPENPROJECT_USERNAME
and OPENPROJECT_PASSWORD if no token is set.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("configuration error: %w", err)
			}

			c, err := client.New(cfg.BaseURL, cfg.APIKey, cfg.Username, cfg.Password)
			if err != nil {
				return fmt.Errorf("client error: %w", err)
			}

			if err := c.Login(); err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			globalClient = c
			fmt.Fprintf(cmd.OutOrStdout(), "Successfully logged in as %s (ID %d) via %s\n",
				c.UserName(), c.UserID(), authMode(c))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check authentication status",
		Long:  `Check whether the current session is authenticated.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Not configured: %v\n", err)
				return nil
			}

			c, err := client.New(cfg.BaseURL, cfg.APIKey, cfg.Username, cfg.Password)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Client error: %v\n", err)
				return nil
			}

			if err := c.Login(); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Not logged in: %v\n", err)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s (ID %d) via %s\n",
				c.UserName(), c.UserID(), authMode(c))
			globalClient = c
			return nil
		},
	})

	return cmd
}

// authMode describes which authentication scheme the client is using.
func authMode(c *client.Client) string {
	if c.UsesAPIKey() {
		return "API key"
	}
	return "session login"
}
