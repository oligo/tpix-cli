package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/oligo/tpix-cli/utils"
)

const (
	pollInterval = 5 * time.Second
)

func DeviceLogin() (string, error) {
	// Initiate device flow
	resp, err := makeRequest("POST", "/auth/device/code", nil, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var deviceResp DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return "", err
	}

	// Display instructions to user
	fmt.Printf("=== Device Authorization ===\n")
	fmt.Printf("Visit: %s\n", deviceResp.VerificationURI)
	fmt.Printf("Enter code: %s\n", deviceResp.UserCode)
	fmt.Printf("Code expires in %d seconds\n", deviceResp.ExpiresIn)
	fmt.Printf("If the browser does not open, please open the above URL manually.")

	// open the url for user
	utils.OpenURL(deviceResp.VerificationURI)

	// Poll for token
	timeout := time.After(time.Duration(deviceResp.ExpiresIn) * time.Second)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	hostname, _ := os.Hostname()

	for {
		select {
		case <-timeout:
			return "", fmt.Errorf("device code expired, please try again.")
		case <-ticker.C:
			token, pending, err := pollForToken(deviceResp.DeviceCode, hostname)
			if err != nil {
				return "", err
			}
			if !pending {
				return token, nil
			}
			fmt.Print(".")
		}
	}
}

func pollForToken(deviceCode string, hostname string) (string, bool, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"device_code": deviceCode,
		"hostname":    hostname,
	})

	resp, err := makeRequest("POST", "/auth/device/token", bytes.NewBuffer(reqBody), "application/json")
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var tokenResp TokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return "", false, err
		}
		return tokenResp.AccessToken, false, nil
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return "", false, err
	}

	switch errResp.Error {
	case "authorization_pending":
		return "", true, nil // Keep polling
	case "access_denied":
		return "", false, fmt.Errorf("authorization denied by user")
	case "expired_token":
		return "", false, fmt.Errorf("device code expired")
	default:
		return "", false, fmt.Errorf("error: %s", errResp.Description)
	}
}
