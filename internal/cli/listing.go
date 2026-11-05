package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/daniele-raiteri/openproject-cli/internal/client"
	"github.com/spf13/cobra"
)

const (
	// maxPageSize is the largest --page-size the CLI accepts.
	maxPageSize = 200
	// listAllPageSize is the default for commands documented as listing
	// everything, which would otherwise stop at the server's default of 20.
	listAllPageSize = maxPageSize
)

// addPageFlags registers --page and --page-size on a list command.
func addPageFlags(cmd *cobra.Command, defaultPageSize int) {
	cmd.Flags().Int("page", 1, "Page number (1-based)")
	cmd.Flags().Int("page-size", defaultPageSize, fmt.Sprintf("Items per page (max %d)", maxPageSize))
}

// pageFlags reads --page and --page-size. Out-of-range values are errors: a
// silent correction would return a different slice than the one requested.
func pageFlags(cmd *cobra.Command) (page, pageSize int, err error) {
	page, _ = cmd.Flags().GetInt("page")
	pageSize, _ = cmd.Flags().GetInt("page-size")
	if page < 1 {
		return 0, 0, fmt.Errorf("--page must be 1 or greater, got %d", page)
	}
	if pageSize < 1 || pageSize > maxPageSize {
		return 0, 0, fmt.Errorf("--page-size must be between 1 and %d, got %d; use --page to read further",
			maxPageSize, pageSize)
	}
	return page, pageSize, nil
}

// pageParams builds the OpenProject pagination query. The API's offset is the
// 1-based page number, not a row index, so it takes the page unchanged.
func pageParams(page, pageSize int) url.Values {
	params := url.Values{}
	params.Set("offset", strconv.Itoa(page))
	params.Set("pageSize", strconv.Itoa(pageSize))
	return params
}

// pageCount returns how many pages hold total rows. An empty result is one
// page, so headers read "page 1/1" rather than "page 1/0".
func pageCount(total, pageSize int) int {
	if total <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

// getOK fetches path and returns its body. Any status other than 200 is an
// error, so an API failure cannot be mistaken for an empty result.
func getOK(c *client.Client, path, what string) ([]byte, error) {
	resp, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", what, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s failed (HTTP %d): %s", what, resp.StatusCode, string(body))
	}
	return body, nil
}
