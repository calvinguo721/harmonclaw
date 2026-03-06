package viking

import (
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
)

// Classification 等保分级，Governor 可按此决定是否允许出网
const (
	ClassPublic    = "public"
	ClassInternal  = "internal"
	ClassSensitive = "sensitive"
	ClassSecret    = "secret"
)

func SafeWrite(path string, data []byte, classification string) (uint32, error) {
	if classification == "" {
		classification = ClassInternal
	}
	header := fmt.Sprintf("# classification=%s\n", classification)

	tmpPath := path + ".tmp"
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", tmpPath, err)
	}

	if _, err := f.WriteString(header); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return 0, fmt.Errorf("write header %s: %w", tmpPath, err)
	}
	_, err = f.Write(data)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return 0, fmt.Errorf("write %s: %w", tmpPath, err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return 0, fmt.Errorf("fsync %s: %w", tmpPath, err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("close %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}

	return crc32.ChecksumIEEE(data), nil
}
