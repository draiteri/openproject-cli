package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/daniele-raiteri/openproject-cli/internal/client"
	"github.com/spf13/cobra"
)

const emptyCollection = `{"total":0,"count":0,"_embedded":{"elements":[]}}`

// fakeAPI answers every request with one status and body, and records the URLs
// it was asked for.
type fakeAPI struct {
	status   int
	body     string
	requests []*url.URL
}

// run executes a command against the fake API and returns its stdout. It sets
// the package's global client, so these tests must not run in parallel.
func (f *fakeAPI) run(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, r.URL)
		w.WriteHeader(f.status)
		w.Write([]byte(f.body))
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(srv.URL, "test-key", "", "")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	previous := globalClient
	globalClient = c
	t.Cleanup(func() { globalClient = previous })

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.PersistentFlags().StringP("output", "o", "table", "")
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), err
}

var listCommands = []struct {
	name            string
	build           func() *cobra.Command
	path            string
	defaultPageSize string
}{
	{"projects", newProjectsCmd, "/api/v3/projects", "200"},
	{"users", newUsersCmd, "/api/v3/users", "200"},
	{"time-entries", newTimeEntriesCmd, "/api/v3/time_entries", "200"},
	{"work-packages", newWorkPackagesCmd, "/api/v3/work_packages", "20"},
}

func TestListDefaults(t *testing.T) {
	for _, tt := range listCommands {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{status: http.StatusOK, body: emptyCollection}
			if _, err := api.run(t, tt.build(), "list"); err != nil {
				t.Fatalf("list: %v", err)
			}
			got := api.requests[0]
			if got.Path != tt.path {
				t.Errorf("path = %q, want %q", got.Path, tt.path)
			}
			if o := got.Query().Get("offset"); o != "1" {
				t.Errorf("offset = %q, want 1", o)
			}
			if s := got.Query().Get("pageSize"); s != tt.defaultPageSize {
				t.Errorf("pageSize = %q, want %q", s, tt.defaultPageSize)
			}
		})
	}
}

func TestListSendsRequestedPage(t *testing.T) {
	for _, tt := range listCommands {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{status: http.StatusOK, body: emptyCollection}
			if _, err := api.run(t, tt.build(), "list", "--page", "3", "--page-size", "50"); err != nil {
				t.Fatalf("list: %v", err)
			}
			q := api.requests[0].Query()
			if o := q.Get("offset"); o != "3" {
				t.Errorf("offset = %q, want 3", o)
			}
			if s := q.Get("pageSize"); s != "50" {
				t.Errorf("pageSize = %q, want 50", s)
			}
		})
	}
}

func TestListRejectsOutOfRangePaging(t *testing.T) {
	cases := []struct {
		name, flag, value, wantErr string
	}{
		{"size above max", "--page-size", "201", "--page-size must be between 1 and 200, got 201"},
		{"size zero", "--page-size", "0", "--page-size must be between 1 and 200, got 0"},
		{"page zero", "--page", "0", "--page must be 1 or greater, got 0"},
	}
	for _, tt := range listCommands {
		for _, tc := range cases {
			t.Run(tt.name+"/"+tc.name, func(t *testing.T) {
				api := &fakeAPI{status: http.StatusOK, body: emptyCollection}
				_, err := api.run(t, tt.build(), "list", tc.flag, tc.value)
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				if len(api.requests) != 0 {
					t.Errorf("made %d requests, want none for invalid flags", len(api.requests))
				}
			})
		}
	}
}

// TestListReportsAPIErrors covers both output modes: an API failure must not
// read as an empty result, and must exit non-zero.
func TestListReportsAPIErrors(t *testing.T) {
	const denied = `{"_type":"Error","message":"denied"}`
	for _, tt := range listCommands {
		for _, args := range [][]string{{"list"}, {"list", "--output", "json"}} {
			t.Run(tt.name+"/"+strings.Join(args, " "), func(t *testing.T) {
				api := &fakeAPI{status: http.StatusForbidden, body: denied}
				out, err := api.run(t, tt.build(), args...)
				if err == nil {
					t.Fatalf("no error for HTTP 403, stdout = %q", out)
				}
				if !strings.Contains(err.Error(), "HTTP 403") || !strings.Contains(err.Error(), "denied") {
					t.Errorf("err = %v, want the status and the API message", err)
				}
				if out != "" {
					t.Errorf("stdout = %q, want nothing", out)
				}
			})
		}
	}
}

func TestShowReportsAPIErrors(t *testing.T) {
	api := &fakeAPI{status: http.StatusNotFound, body: `{"_type":"Error"}`}
	if _, err := api.run(t, newWorkPackagesCmd(), "show", "99999"); err == nil ||
		!strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err = %v, want HTTP 404", err)
	}
}

func TestListHeaderShowsPosition(t *testing.T) {
	api := &fakeAPI{status: http.StatusOK, body: `{"total":450,"count":0,"_embedded":{"elements":[]}}`}
	out, err := api.run(t, newProjectsCmd(), "list", "--page", "2")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if want := "Projects (450 total, page 2/3):"; !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want it to contain %q", out, want)
	}
}

func TestPageCount(t *testing.T) {
	tests := []struct{ total, pageSize, want int }{
		{0, 20, 1},
		{1, 20, 1},
		{20, 20, 1},
		{21, 20, 2},
		{774, 200, 4},
	}
	for _, tt := range tests {
		if got := pageCount(tt.total, tt.pageSize); got != tt.want {
			t.Errorf("pageCount(%d, %d) = %d, want %d", tt.total, tt.pageSize, got, tt.want)
		}
	}
}

func TestTimeEntriesFilters(t *testing.T) {
	type filter map[string]struct {
		Operator string   `json:"operator"`
		Values   []string `json:"values"`
	}
	tests := []struct {
		name string
		args []string
		want map[string]string
	}{
		{"none", nil, nil},
		{"project", []string{"--project", "6"}, map[string]string{"project_id": "6"}},
		{"work package", []string{"--work-package", "810"},
			map[string]string{"entity_type": "WorkPackage", "entity_id": "810"}},
		{"both", []string{"--project", "6", "--work-package", "810"},
			map[string]string{"project_id": "6", "entity_type": "WorkPackage", "entity_id": "810"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{status: http.StatusOK, body: emptyCollection}
			if _, err := api.run(t, newTimeEntriesCmd(), append([]string{"list"}, tt.args...)...); err != nil {
				t.Fatalf("list: %v", err)
			}
			req := api.requests[0]
			if req.Path != "/api/v3/time_entries" {
				t.Errorf("path = %q, want /api/v3/time_entries", req.Path)
			}

			raw := req.Query().Get("filters")
			if tt.want == nil {
				if raw != "" {
					t.Errorf("filters = %q, want none", raw)
				}
				return
			}
			var filters []filter
			if err := json.Unmarshal([]byte(raw), &filters); err != nil {
				t.Fatalf("filters %q: %v", raw, err)
			}
			got := map[string]string{}
			for _, f := range filters {
				for name, cond := range f {
					if cond.Operator != "=" || len(cond.Values) != 1 {
						t.Errorf("filter %s = %+v, want one value with operator =", name, cond)
						continue
					}
					got[name] = cond.Values[0]
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("filters = %v, want %v", got, tt.want)
			}
			for name, value := range tt.want {
				if got[name] != value {
					t.Errorf("filter %s = %q, want %q", name, got[name], value)
				}
			}
		})
	}
}
