//go:build darwin && arm64

package vm

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// resizeRawImageGo grows a raw disk image to at least sizeBytes using a
// sparse truncate. This does not require qemu-img and creates sparse files
// on APFS and HFS+. It will not shrink an image that is already larger.
func resizeRawImageGo(imagePath string, sizeBytes int64) error {
	f, err := os.OpenFile(imagePath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening image for resize: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat image for resize: %w", err)
	}
	if info.Size() >= sizeBytes {
		return nil // already large enough, nothing to do
	}
	if err := f.Truncate(sizeBytes); err != nil {
		return fmt.Errorf("truncating image to %d bytes: %w", sizeBytes, err)
	}
	return nil
}

// parseSizeBytes converts a human-friendly size string (as used in jcard.toml)
// to a byte count. Supported suffixes: GiB, MiB, KiB, GB, MB, KB (case-insensitive).
// A bare number is interpreted as bytes.
func parseSizeBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	suffixes := []struct {
		suffix string
		factor int64
	}{
		{"GiB", 1 << 30},
		{"MiB", 1 << 20},
		{"KiB", 1 << 10},
		{"GB", 1_000_000_000},
		{"MB", 1_000_000},
		{"KB", 1_000},
	}
	upper := strings.ToUpper(s)
	for _, e := range suffixes {
		if strings.HasSuffix(upper, strings.ToUpper(e.suffix)) {
			numStr := s[:len(s)-len(e.suffix)]
			n, err := strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q: %w", s, err)
			}
			return n * e.factor, nil
		}
	}
	// Bare number — bytes
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return n, nil
}
