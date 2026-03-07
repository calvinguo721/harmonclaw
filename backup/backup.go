// Package backup provides Viking+configs+Ledger backup and restore.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Create writes a tar.gz of dataDir (Viking base), configsDir, ledgerDir.
func Create(dataDir, configsDir, ledgerDir, outPath string) error {
	if outPath == "" {
		outPath = fmt.Sprintf("harmonclaw-backup-%s.tar.gz", time.Now().Format("20060102150405"))
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	addDir := func(dir, prefix string) error {
		return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			name := filepath.Join(prefix, rel)
			h, _ := tar.FileInfoHeader(info, "")
			h.Name = name
			if err := tw.WriteHeader(h); err != nil {
				return err
			}
			r, _ := os.Open(path)
			if r != nil {
				io.Copy(tw, r)
				r.Close()
			}
			return nil
		})
	}
	if dataDir != "" {
		if err := addDir(dataDir, "viking"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if configsDir != "" {
		if err := addDir(configsDir, "configs"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if ledgerDir != "" {
		if err := addDir(ledgerDir, "ledger"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Restore extracts tar.gz to destDir.
func Restore(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		path := filepath.Join(destDir, h.Name)
		if h.Typeflag == tar.TypeDir {
			os.MkdirAll(path, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0755)
		w, err := os.Create(path)
		if err != nil {
			return err
		}
		io.Copy(w, tr)
		w.Close()
	}
	return nil
}
