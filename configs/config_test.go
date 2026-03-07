package configs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	os.Unsetenv("HC_PORT")
	os.Unsetenv("HC_DATA_DIR")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" {
		t.Errorf("port: want 8080, got %s", c.Port)
	}
	if c.SovereigntyMode != "airlock" {
		t.Errorf("sovereignty: want airlock, got %s", c.SovereigntyMode)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Setenv("HC_PORT", "9999")
	os.Setenv("HC_SOVEREIGNTY_MODE", "shadow")
	defer func() {
		os.Unsetenv("HC_PORT")
		os.Unsetenv("HC_SOVEREIGNTY_MODE")
	}()

	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "9999" {
		t.Errorf("port: want 9999, got %s", c.Port)
	}
	if c.SovereigntyMode != "shadow" {
		t.Errorf("sovereignty: want shadow, got %s", c.SovereigntyMode)
	}
}

func TestLoad_JSONOverlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"version":"2.0","port":3000}`), 0644)

	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != "2.0" {
		t.Errorf("version: want 2.0, got %s", c.Version)
	}
	if c.Port != "3000" {
		t.Errorf("port: want 3000, got %s", c.Port)
	}
}

func TestValidate(t *testing.T) {
	c := &Config{Port: "8080", DataDir: "/tmp"}
	if err := c.Validate(); err != nil {
		t.Errorf("valid config: %v", err)
	}
	c.Port = "invalid"
	if err := c.Validate(); err == nil {
		t.Error("invalid port: want error")
	}
}

func TestGet(t *testing.T) {
	Load("")
	if Get() == nil {
		t.Error("Get: want non-nil after Load")
	}
}
