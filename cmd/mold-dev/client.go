package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/hitel00000/mold/auth"
)

// ReloadClient executes HTTP API calls to trigger resource reloads and handle session authentication.
type ReloadClient struct {
	baseURL    string
	httpClient *http.Client
	jar        *cookiejar.Jar
}

// ReloadResponse captures the result of a POST /_mold/reload HTTP call.
type ReloadResponse struct {
	StatusCode int
	Success    bool
	Message    string
}

// NewReloadClient creates a new client targeting the specified base URL.
func NewReloadClient(baseURL string) (*ReloadClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	return &ReloadClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: client,
		jar:        jar,
	}, nil
}

// SetSessionCookie directly sets the _mold_session cookie (Option B).
func (c *ReloadClient) SetSessionCookie(cookieValue string) error {
	if cookieValue == "" {
		return nil
	}
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	cookie := &http.Cookie{
		Name:  auth.SessionCookieName,
		Value: cookieValue,
		Path:  "/",
	}
	c.jar.SetCookies(u, []*http.Cookie{cookie})
	return nil
}

// Login performs form authentication via POST /login (Option A).
func (c *ReloadClient) Login(ctx context.Context, email, password string) error {
	if email == "" || password == "" {
		return fmt.Errorf("email and password must not be empty")
	}

	form := url.Values{}
	form.Set("username", email)
	form.Set("password", password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// TriggerReload sends POST /_mold/reload using stored HTTP session credentials.
func (c *ReloadClient) TriggerReload(ctx context.Context) (*ReloadResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/_mold/reload", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create reload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reload request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	result := &ReloadResponse{
		StatusCode: resp.StatusCode,
		Success:    resp.StatusCode == http.StatusOK,
	}

	if resp.StatusCode == http.StatusOK {
		result.Message = string(bodyBytes)
	} else {
		var errEnv struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(bodyBytes, &errEnv); jsonErr == nil && errEnv.Error.Message != "" {
			result.Message = fmt.Sprintf("[%s] %s", errEnv.Error.Code, errEnv.Error.Message)
		} else {
			result.Message = string(bodyBytes)
		}
	}

	return result, nil
}
