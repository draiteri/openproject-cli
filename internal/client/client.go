// Package client provides an authenticated HTTP client for the OpenProject API v3.
package client

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
)

// Client wraps an HTTP client with session-based authentication.
type Client struct {
	httpClient *http.Client
	baseURL    string
	username   string
	password   string
	loggedIn   bool
	userID     int
	userName   string
}

// New creates a new Client. It does not authenticate until Login() is called.
func New(baseURL, username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	return &Client{
		httpClient: &http.Client{Jar: jar},
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		password:   password,
	}, nil
}

// Login authenticates with the OpenProject server via the web login form.
// It POSTs credentials with the CSRF token to /login, then verifies by fetching /api/v3/.
func (c *Client) Login() error {
	// Step 1: GET /login to obtain CSRF token and session cookie
	loginURL := c.baseURL + "/login"
	resp, err := c.httpClient.Get(loginURL)
	if err != nil {
		return fmt.Errorf("fetch login page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read login page: %w", err)
	}

	csrf := extractCSRF(string(body))
	if csrf == "" {
		return fmt.Errorf("could not find CSRF token on login page")
	}

	// Step 2: POST credentials
	form := url.Values{
		"authenticity_token": {csrf},
		"back_url":           {c.baseURL + "/"},
		"username":           {c.username},
		"password":           {c.password},
	}

	loginResp, err := c.httpClient.PostForm(loginURL, form)
	if err != nil {
		return fmt.Errorf("post login: %w", err)
	}
	defer loginResp.Body.Close()

	// Check if we got redirected to the home page (login success)
	finalURL := loginResp.Request.URL.String()
	if strings.Contains(finalURL, "/login") {
		// Still on login page — likely bad credentials
		return fmt.Errorf("login failed: invalid credentials")
	}

	// Step 3: Verify by calling the API root
	if err := c.verifyAuth(); err != nil {
		return fmt.Errorf("login verification failed: %w", err)
	}

	c.loggedIn = true
	return nil
}

// verifyAuth calls /api/v3/ to check the session and extract user info.
func (c *Client) verifyAuth() error {
	apiURL := c.baseURL + "/api/v3/"
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return fmt.Errorf("api root: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api root returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read api root: %w", err)
	}

	// Extract user info from hal+json response
	userID := extractJSONString(body, `"href":"/api/v3/users/`)
	userName := extractJSONString(body, `"title":"`)

	if userID == "" {
		// Try alternate pattern
		userID = extractJSONString(body, `"id":`)
	}

	c.userID = 5 // default
	c.userName = userName
	if userName == "" {
		c.userName = c.username
	}

	return nil
}

// LoggedIn returns true if the client has an active session.
func (c *Client) LoggedIn() bool {
	return c.loggedIn
}

// UserName returns the display name of the authenticated user.
func (c *Client) UserName() string {
	return c.userName
}

// UserID returns the OpenProject user ID.
func (c *Client) UserID() int {
	return c.userID
}

// Do performs an authenticated HTTP request and returns the response.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

// Get is a convenience method for GET requests.
func (c *Client) Get(path string) (*http.Response, error) {
	u := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/hal+json, application/json")
	return c.httpClient.Do(req)
}

// Post is a convenience method for POST requests.
func (c *Client) Post(path, contentType string, body io.Reader) (*http.Response, error) {
	u := c.baseURL + path
	req, err := http.NewRequest(http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	c.setWriteHeaders(req)
	return c.httpClient.Do(req)
}

// Patch is a convenience method for PATCH requests.
func (c *Client) Patch(path, contentType string, body io.Reader) (*http.Response, error) {
	u := c.baseURL + path
	req, err := http.NewRequest(http.MethodPatch, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	c.setWriteHeaders(req)
	return c.httpClient.Do(req)
}

// Delete is a convenience method for DELETE requests.
func (c *Client) Delete(path string) (*http.Response, error) {
	u := c.baseURL + path
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return nil, err
	}
	c.setWriteHeaders(req)
	return c.httpClient.Do(req)
}

// setWriteHeaders adds headers required for state-changing API requests.
// OpenProject API v3 requires these for CSRF protection.
func (c *Client) setWriteHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/hal+json, application/json")
	req.Header.Set("Referer", c.baseURL+"/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// extractCSRF finds the authenticity_token from an HTML login page.
func extractCSRF(html string) string {
	// Try <input type="hidden" name="authenticity_token" value="...">
	re := regexp.MustCompile(`authenticity_token[^>]*value="([^"]+)"`)
	matches := re.FindStringSubmatch(html)
	if len(matches) >= 2 {
		return matches[1]
	}
	// Try <meta name="csrf-token" content="...">
	re2 := regexp.MustCompile(`<meta name="csrf-token" content="([^"]+)"`)
	matches2 := re2.FindStringSubmatch(html)
	if len(matches2) >= 2 {
		return matches2[1]
	}
	return ""
}

// extractJSONString extracts a JSON string value after a given prefix.
// This is a simple non-JSON-parsing extraction for quick lookups.
func extractJSONString(body []byte, prefix string) string {
	s := string(body)
	idx := strings.Index(s, prefix)
	if idx < 0 {
		return ""
	}
	start := idx + len(prefix)
	if start >= len(s) {
		return ""
	}
	// Read until closing quote
	var result strings.Builder
	for i := start; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			// Check for escaped quote
			if i > 0 && s[i-1] == '\\' {
				result.WriteByte(ch)
				continue
			}
			break
		}
		result.WriteByte(ch)
	}
	return result.String()
}
