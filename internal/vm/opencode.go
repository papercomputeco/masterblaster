package vm

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/paper-compute-co/masterblaster/internal/ui"
)

const (
	// openCodeURL is the download URL for the OpenCode linux-arm64 binary.
	openCodeURL = "https://github.com/anomalyco/opencode/releases/latest/download/opencode-linux-arm64.tar.gz"
)

// DownloadOpenCode fetches the OpenCode binary for linux-arm64 and returns
// the raw binary contents. It downloads the tar.gz archive and extracts the
// "opencode" binary from it.
func DownloadOpenCode() ([]byte, error) {
	ui.Status("Downloading OpenCode for linux-arm64...")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(openCodeURL)
	if err != nil {
		return nil, fmt.Errorf("downloading OpenCode: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading OpenCode: HTTP %d", resp.StatusCode)
	}

	// Extract the "opencode" binary from the tar.gz
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decompressing OpenCode archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading OpenCode archive: %w", err)
		}
		if hdr.Name == "opencode" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("extracting OpenCode binary: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("\"opencode\" binary not found in archive")
}

// CacheOpenCode downloads OpenCode if not already cached, and returns the
// binary contents. The cache file lives at <configDir>/cache/opencode-linux-arm64.
func CacheOpenCode(configDir string) ([]byte, error) {
	cacheDir := configDir + "/cache"
	cachePath := cacheDir + "/opencode-linux-arm64"

	// Check cache first
	if data, err := os.ReadFile(cachePath); err == nil {
		ui.Status("Using cached OpenCode binary")
		return data, nil
	}

	data, err := DownloadOpenCode()
	if err != nil {
		return nil, err
	}

	// Cache for next time
	if err := os.MkdirAll(cacheDir, 0755); err == nil {
		_ = os.WriteFile(cachePath, data, 0755)
	}

	return data, nil
}
