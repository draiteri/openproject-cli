// Package client provides an authenticated HTTP client for the OpenProject API v3.
package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
)

// Client wraps an HTTP client with OpenProject authentication.
//
// Two auth modes are supported. When apiKey is set the client uses HTTP Basic
// auth with the literal username "apikey" and the token as the password, which
// is OpenProject's documented API authentication scheme. Otherwise it falls
// back to a session login through the web login form.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	username   string
	password   string
	loggedIn   bool
	userID     int
	userName   string
}

// New creates a new Client. It does not authenticate until Login() is called.
// If apiKey is non-empty it is used and username/password are ignored.
func New(baseURL, apiKey, username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	return &Client{
		httpClient: &http.Client{Jar: jar},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		username:   username,
		password:   password,
	}, nil
}

// UsesAPIKey reports whether the client authenticates with an API key.
func (c *Client) UsesAPIKey() bool {
	return c.apiKey != ""
}

// Login authenticates with the OpenProject server.
//
// In API key mode this only verifies the token against /api/v3/users/me. In
// session mode it POSTs the credentials with the CSRF token to /login first.
func (c *Client) Login() error {
	if c.apiKey == "" {
		if err := c.sessionLogin(); err != nil {
			return err
		}
	}

	if err := c.verifyAuth(); err != nil {
		return fmt.Errorf("login verification failed: %w", err)
	}

	c.loggedIn = true
	return nil
}

// sessionLogin performs the legacy username/password login through the web form.
func (c *Client) sessionLogin() error {
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

	return nil
}

// verifyAuth calls /api/v3/users/me to check the credentials and read user info.
func (c *Client) verifyAuth() error {
	resp, err := c.Get("/api/v3/users/me")
	if err != nil {
		return fmt.Errorf("fetch current user: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read current user: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid credentials (HTTP 401)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("/api/v3/users/me returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var me struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return fmt.Errorf("parse current user: %w", err)
	}

	c.userID = me.ID
	c.userName = me.Name
	if c.userName == "" {
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
	c.applyAuth(req)
	return c.httpClient.Do(req)
}

// Get is a convenience method for GET requests.
func (c *Client) Get(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/hal+json, application/json")
	return c.Do(req)
}

// Post is a convenience method for POST requests.
func (c *Client) Post(path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	c.setWriteHeaders(req)
	return c.Do(req)
}

// Patch is a convenience method for PATCH requests.
func (c *Client) Patch(path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPatch, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	c.setWriteHeaders(req)
	return c.Do(req)
}

// Delete is a convenience method for DELETE requests.
func (c *Client) Delete(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	c.setWriteHeaders(req)
	return c.Do(req)
}

// applyAuth attaches the API key credentials to a request. In session mode the
// cookie jar carries the authentication instead, so nothing is added.
func (c *Client) applyAuth(req *http.Request) {
	if c.apiKey != "" {
		req.SetBasicAuth("apikey", c.apiKey)
	}
}

// setWriteHeaders adds headers required for state-changing API requests.
// OpenProject API v3 requires these for CSRF protection in session mode.
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
