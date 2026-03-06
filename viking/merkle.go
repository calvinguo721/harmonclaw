package viking

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func vikingDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".harmonclaw", "viking"), nil
}

func ComputeRoot() (string, error) {
	dir, err := vikingDir()
	if err != nil {
		return "", err
	}
	var leaves [][]byte
	err = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		if filepath.Ext(p) != ".txt" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		h := sha256.Sum256(data)
		leaves = append(leaves, h[:])
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(leaves) == 0 {
		h := sha256.Sum256(nil)
		return hex.EncodeToString(h[:]), nil
	}
	sort.Slice(leaves, func(i, j int) bool {
		return string(leaves[i]) < string(leaves[j])
	})
	for len(leaves) > 1 {
		var next [][]byte
		for i := 0; i < len(leaves); i += 2 {
			var combined []byte
			combined = append(combined, leaves[i]...)
			if i+1 < len(leaves) {
				combined = append(combined, leaves[i+1]...)
			} else {
				combined = append(combined, leaves[i]...)
			}
			h := sha256.Sum256(combined)
			next = append(next, h[:])
		}
		leaves = next
	}
	return hex.EncodeToString(leaves[0]), nil
}

func AppendAuditRoot(root string) error {
	dir, err := vikingDir()
	if err != nil {
		return err
	}
	fpath := filepath.Join(dir, "audit_root.jsonl")
	var data []byte
	if b, err := os.ReadFile(fpath); err == nil {
		data = b
	}
	line := fmt.Sprintf(`{"root":"%s","ts":"%s"}`+"\n", root, time.Now().Format(time.RFC3339))
	data = append(data, line...)
	_, err = SafeWrite(fpath, data, ClassInternal)
	return err
}
