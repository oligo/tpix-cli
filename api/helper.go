package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/typstify/tpix-cli/config"
)

const (
	TpixServer          = "https://tpix.typstify.com"
	TpixClientUserAgent = "tpix-client/v1.0.0"
)

// refreshMu prevents concurrent refresh attempts
var refreshMu sync.Mutex

// makeRequest creates an HTTP request with Bearer token.
// On 401 responses, it transparently attempts to refresh the access token
// and retries the request once.
func makeRequest(method, url string, body io.Reader, contentType string) (*http.Response, error) {
	// Buffer the body so we can replay it on retry
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	resp, err := doRequest(method, url, bodyBytes, contentType, cfg.AccessToken)
	if err != nil {
		return nil, err
	}

	// If 401 and we have a refresh token, try to refresh and retry
	if resp.StatusCode == http.StatusUnauthorized && cfg.RefreshToken != "" {
		resp.Body.Close()
		if refreshErr := refreshAccessToken(cfg); refreshErr == nil {
			// reload config
			cfg, err := config.Load()
			if err != nil {
				return nil, err
			}

			return doRequest(method, url, bodyBytes, contentType, cfg.AccessToken)
		}
	}

	return resp, nil
}

// doRequest executes a single HTTP request without retry logic.
func doRequest(method, url string, bodyBytes []byte, contentType string, accessToken string) (*http.Response, error) {
	apiUrl := fmt.Sprintf("%s%s", TpixServer, url)

	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, apiUrl, bodyReader)
	if err != nil {
		return nil, err
	}

	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	req.Header.Set("User-Agent", TpixClientUserAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return http.DefaultClient.Do(req)
}

// refreshAccessToken uses the stored refresh token to obtain a new access token.
// On success, it updates the config with both new tokens and persists them.
func refreshAccessToken(cfg config.Config) error {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	reqBody, _ := json.Marshal(map[string]string{
		"refresh_token": cfg.RefreshToken,
	})

	resp, err := doRequest("POST", "/auth/token/refresh", reqBody, "application/json", "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Refresh failed — clear refresh token so we don't keep retrying
		cfg.RefreshToken = ""
		config.Save(cfg)
		return fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	cfg.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		cfg.RefreshToken = tokenResp.RefreshToken
	}
	return config.Save(cfg)
}
