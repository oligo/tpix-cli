package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oligo/tpix-cli/config"
)

const (
	tpixServer          = "http://localhost:8082"
	TpixClientUserAgent = "tpix-client/v1.0.0"
)

// API response types

type SearchResponse struct {
	Query   string         `json:"query"`
	Count   int            `json:"count"`
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PackageResponse represents a package details response
type PackageResponse struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Namespace     string               `json:"namespace"`
	Description   string               `json:"description"`
	HomepageURL   string               `json:"homepage_url"`
	RepositoryURL string               `json:"repository_url"`
	License       string               `json:"license"`
	CreatedAt     *time.Time           `json:"created_at"`
	UpdatedAt     *time.Time           `json:"updated_at"`
	LatestVersion PackageVersionInfo   `json:"latest_version"`
	Versions      []PackageVersionInfo `json:"versions"`
}

// PackageVersionInfo represents package version information
type PackageVersionInfo struct {
	Version      string     `json:"version"`
	TypstVersion string     `json:"typst_version"`
	SHA256       string     `json:"sha256"`
	PublishedAt  *time.Time `json:"published_at"`
}

// PackageVersionsResponse represents the response from the versions endpoint
type PackageVersionsResponse struct {
	Versions []PackageVersionInfo `json:"versions"`
}

// searchPackages fetches packages matching a query from the TPIX server.
func searchPackages(query, namespace string, limit int) (*SearchResponse, error) {
	url := fmt.Sprintf("%s/api/v1/search?q=%s", tpixServer, query)
	if namespace != "" {
		url += "&namespace=" + namespace
	}
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}

	resp, err := makeRequest("GET", url)
	if err != nil {
		return nil, fmt.Errorf("failed to search packages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed: %s", string(body))
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// downloadPackage downloads a package, extracts it to the cache directory,
// and optionally saves the archive to output path.
func downloadPackage(namespace, name, version string) error {
	url := fmt.Sprintf("%s/api/v1/download/%s/%s/%s", tpixServer, namespace, name, version)

	resp, err := makeRequest("GET", url)
	if err != nil {
		return fmt.Errorf("failed to download package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed: %s", string(body))
	}

	// Create temp file for the archive
	tmpFile, err := os.CreateTemp("", "tpix-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Extract to cache directory
	cacheDir := config.AppConfig.TypstCachePkgPath
	if cacheDir == "" {
		return fmt.Errorf("typst cache directory not configured")
	}

	extractDir := filepath.Join(cacheDir, namespace, name, version)
	if err := extractTarGz(tmpPath, extractDir); err != nil {
		return fmt.Errorf("failed to extract package: %w", err)
	}

	return nil
}

// extractTarGz extracts a tar.gz archive to the specified directory.
func extractTarGz(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// fetchPackage fetches package details from the TPIX server.
func fetchPackage(namespace, name string) (*PackageResponse, error) {
	url := fmt.Sprintf("%s/api/v1/packages/%s/%s", tpixServer, namespace, name)
	resp, err := makeRequest("GET", url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get package: %s", string(body))
	}

	var pkg PackageResponse
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Fetch all versions
	versions, err := fetchPackageVersions(namespace, name)
	if err == nil && len(versions) > 0 {
		pkg.Versions = versions
	}

	return &pkg, nil
}

// fetchPackageVersions fetches all versions for a package.
func fetchPackageVersions(namespace, name string) ([]PackageVersionInfo, error) {
	url := fmt.Sprintf("%s/api/v1/packages/%s/%s/versions", tpixServer, namespace, name)
	resp, err := makeRequest("GET", url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get versions: %s", string(body))
	}

	var versionsResp PackageVersionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&versionsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return versionsResp.Versions, nil
}

// Helper function to create HTTP request with Bearer token
func makeRequest(method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	if config.AppConfig.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+config.AppConfig.AccessToken)
	}

	req.Header.Set("User-Agent", TpixClientUserAgent)

	return http.DefaultClient.Do(req)
}
