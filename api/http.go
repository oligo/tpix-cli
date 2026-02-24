package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/oligo/tpix-cli/config"
	"github.com/oligo/tpix-cli/utils"
)

// SearchPackages fetches packages matching a query from the TPIX server.
func SearchPackages(query, namespace string, limit int) (*SearchResponse, error) {
	url := fmt.Sprintf("/api/v1/search?q=%s", query)
	if namespace != "" {
		url += "&namespace=" + namespace
	}
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}

	resp, err := makeRequest("GET", url, nil, "")
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

// DownloadPackage downloads a package, extracts it to the cache directory,
// and optionally saves the archive to output path.
func DownloadPackage(namespace, name, version string) error {
	url := fmt.Sprintf("/api/v1/download/%s/%s/%s", namespace, name, version)

	resp, err := makeRequest("GET", url, nil, "")
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
	if err := utils.ExtractTarGz(tmpPath, extractDir); err != nil {
		return fmt.Errorf("failed to extract package: %w", err)
	}

	return nil
}

// FetchPackage fetches package details from the TPIX server.
func FetchPackage(namespace, name string) (*PackageResponse, error) {
	url := fmt.Sprintf("/api/v1/packages/%s/%s", namespace, name)
	resp, err := makeRequest("GET", url, nil, "")
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

// FetchPackageVersions fetches all versions for a package.
func fetchPackageVersions(namespace, name string) ([]PackageVersionInfo, error) {
	url := fmt.Sprintf("/api/v1/packages/%s/%s/versions", namespace, name)
	resp, err := makeRequest("GET", url, nil, "")
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

// UploadPackage uploads a package to the TPIX server.
func UploadPackage(packagePath, namespace string) (*UploadResponse, error) {
	file, err := os.Open(packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open package file: %w", err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field
	part, err := writer.CreateFormFile("file", fileInfo.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	if err := writer.WriteField("namespace", namespace); err != nil {
		return nil, fmt.Errorf("failed to write namespace field: %w", err)
	}

	writer.Close()

	// Create request
	url := "/api/v1/packages/upload"
	resp, err := makeRequest("POST", url, &buf, writer.FormDataContentType())
	if err != nil {
		return nil, fmt.Errorf("failed to upload package: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var uploadResp UploadResponse
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &uploadResp, nil
}
