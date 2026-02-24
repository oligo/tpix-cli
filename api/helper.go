package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/oligo/tpix-cli/config"
)

const (
	TpixServer          = "https://tpix.typstify.com"
	TpixClientUserAgent = "tpix-client/v1.0.0"
)

// Helper function to create HTTP request with Bearer token
func makeRequest(method, url string, body io.Reader, contentType string) (*http.Response, error) {
	apiUrl := fmt.Sprintf("%s%s", TpixServer, url)
	req, err := http.NewRequest(method, apiUrl, body)
	if err != nil {
		return nil, err
	}

	if config.AppConfig.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+config.AppConfig.AccessToken)
	}

	req.Header.Set("User-Agent", TpixClientUserAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return http.DefaultClient.Do(req)
}
