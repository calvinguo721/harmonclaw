// Package updater provides auto-update with rollback (GitHub Releases).
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultCheckInterval = 6 * time.Hour
	GitHubReleasesURL    = "https://api.github.com/repos/harmonclaw/harmonclaw/releases/latest"
)

// Updater checks GitHub Releases, downloads, replaces binary, restarts. Rollback on failure.
type Updater struct {
	Repo         string
	CurrentPath  string
	BackupPath   string
	CheckInterval time.Duration
	Client       *http.Client
	OnUpdate     func(newPath string) error
}

// NewUpdater creates an updater. Client must use governor.SecureClient() in production.
func NewUpdater(repo, currentPath string, client *http.Client) *Updater {
	if client == nil {
		client = http.DefaultClient
	}
	backup := currentPath + ".bak"
	return &Updater{
		Repo:          repo,
		CurrentPath:   currentPath,
		BackupPath:    backup,
		CheckInterval: DefaultCheckInterval,
		Client:        client,
	}
}

// LatestRelease returns tag and download URL for current GOOS/GOARCH.
func (u *Updater) LatestRelease(ctx context.Context) (tag, url string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.releasesURL(), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := u.Client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github api %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", err
	}
	suffix := fmt.Sprintf("%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, suffix) || strings.Contains(a.Name, "harmonclaw-") {
			return rel.TagName, a.BrowserDownloadURL, nil
		}
	}
	return rel.TagName, "", nil
}

func (u *Updater) releasesURL() string {
	if u.Repo != "" {
		return "https://api.github.com/repos/" + u.Repo + "/releases/latest"
	}
	return GitHubReleasesURL
}

// Download fetches url to dest.
func (u *Updater) Download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// Replace backs up current, moves new into place. Caller restarts.
func (u *Updater) Replace(newPath string) error {
	if err := os.Rename(u.CurrentPath, u.BackupPath); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if err := os.Rename(newPath, u.CurrentPath); err != nil {
		os.Rename(u.BackupPath, u.CurrentPath)
		return fmt.Errorf("replace: %w", err)
	}
	return nil
}

// Rollback restores from backup.
func (u *Updater) Rollback() error {
	return os.Rename(u.BackupPath, u.CurrentPath)
}
