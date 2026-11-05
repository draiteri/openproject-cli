// Package cli contains the Cobra command tree for the OpenProject CLI.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/daniele-raiteri/openproject-cli/internal/client"
	"github.com/daniele-raiteri/openproject-cli/internal/config"
	"github.com/spf13/cobra"
)

const version = "0.5.0"

// Run executes the root command and returns an exit code.
func Run() int {
	rootCmd := &cobra.Command{
		Use:     "openproject-cli",
		Short:   "CLI for the OpenProject API",
		Version: version,
		Long: `A standalone CLI tool for interacting with the OpenProject API v3.

Requires OPENPROJECT_API_KEY to be set (or a .env file in the skill directory).
OPENPROJECT_USERNAME and OPENPROJECT_PASSWORD are still honoured as a fallback.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if skipAuth(cmd) {
				return nil
			}
			return ensureAuthenticated()
		},
	}

	rootCmd.SetVersionTemplate("openproject-cli {{.Version}}\n")

	// Add persistent flags
	rootCmd.PersistentFlags().StringP("output", "o", "table", "Output format: table or json")

	// Register subcommands
	rootCmd.AddCommand(newAuthCmd())
	rootCmd.AddCommand(newProjectsCmd())
	rootCmd.AddCommand(newUsersCmd())
	rootCmd.AddCommand(newWorkPackagesCmd())
	rootCmd.AddCommand(newTimeEntriesCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// skipAuth returns true for commands that don't need authentication.
func skipAuth(cmd *cobra.Command) bool {
	if cmd.Name() == "openproject-cli" || cmd.Name() == "help" {
		return true
	}
	// auth subcommands (login, status) don't need pre-auth
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "auth" {
			return true
		}
	}
	return false
}

// ensureAuthenticated ensures the client is logged in.
func ensureAuthenticated() error {
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
	// Store client globally for commands to use
	globalClient = c
	return nil
}

// globalClient is set by ensureAuthenticated and used by commands.
var globalClient *client.Client

// getClient returns the authenticated global client.
func getClient() *client.Client {
	return globalClient
}

// outputFlag returns the value of the --output flag.
func outputFlag(cmd *cobra.Command) string {
	val, _ := cmd.Flags().GetString("output")
	if val == "" {
		return "table"
	}
	return val
}

// printTable prints a table with aligned columns.
func printTable(w io.Writer, header []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	tw.Flush()
}
