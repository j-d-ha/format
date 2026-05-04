package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := GlobalConfigPath()
	if err != nil {
		t.Fatalf("GlobalConfigPath() error = %v, want nil", err)
	}

	want := filepath.Join(home, ".format", "format.json")
	if got != want {
		t.Fatalf("GlobalConfigPath() = %q, want %q", got, want)
	}
}

func TestLoadConfigForPathReportsMissingDefaultConfigs(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	withWorkingDirectory(t, cwd)

	_, _, err := LoadConfigForPath("")
	if err == nil {
		t.Fatal("LoadConfigForPath(\"\") error = nil, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadConfigForPath(\"\") error = %v, want os.ErrNotExist", err)
	}

	wantParts := []string{
		"no format config found",
		`looked for "format.json" relative to current working directory`,
		cwd,
		filepath.Join(home, ".format", "format.json"),
		"pass --config",
	}
	for _, want := range wantParts {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("LoadConfigForPath(\"\") error = %q, want substring %q", err.Error(), want)
		}
	}
}

func withWorkingDirectory(t *testing.T, path string) {
	t.Helper()

	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v, want nil", err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatalf("os.Chdir(%q) error = %v, want nil", path, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore working directory %q: %v", old, err)
		}
	})
}
