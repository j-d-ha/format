package app

import (
	"log/slog"
	"path/filepath"
	"testing"
)

func TestGeneratedLogFileNameUsesProjectRunnerAndSession(t *testing.T) {
	t.Setenv("FORMAT_LOG_DIR", "/tmp/format-logs")

	got := GeneratedLogFileName(LogMetadata{Project: "my api", Runner: "pi", SessionID: "pi/session"})
	want := filepath.Join("/tmp/format-logs", "my-api", "pi", "format-pi-session.log")
	if got != want {
		t.Fatalf("GeneratedLogFileName() = %q, want %q", got, want)
	}
}

func TestResolveLogMetadataUsesExplicitValues(t *testing.T) {
	t.Setenv("FORMAT_PROJECT", "env-project")
	t.Setenv("FORMAT_RUNNER", "env-runner")
	t.Setenv("FORMAT_SESSION_ID", "env-session")

	got := ResolveLogMetadata("my/project", "pi", "session one")
	if got.Project != "my-project" {
		t.Fatalf("Project = %q, want %q", got.Project, "my-project")
	}
	if got.Runner != "pi" {
		t.Fatalf("Runner = %q, want %q", got.Runner, "pi")
	}
	if got.SessionID != "session-one" {
		t.Fatalf("SessionID = %q, want %q", got.SessionID, "session-one")
	}
}

func TestResolveLogMetadataUsesEnvironmentFallbacks(t *testing.T) {
	t.Setenv("FORMAT_PROJECT", "env project")
	t.Setenv("FORMAT_RUNNER", "env runner")
	t.Setenv("FORMAT_SESSION_ID", "env/session")

	got := ResolveLogMetadata("", "", "")
	if got.Project != "env-project" {
		t.Fatalf("Project = %q, want %q", got.Project, "env-project")
	}
	if got.Runner != "env-runner" {
		t.Fatalf("Runner = %q, want %q", got.Runner, "env-runner")
	}
	if got.SessionID != "env-session" {
		t.Fatalf("SessionID = %q, want %q", got.SessionID, "env-session")
	}
}

func TestResolveLogLevel(t *testing.T) {
	tests := map[string]struct {
		env       string
		flagValue string
		flagSet   bool
		want      slog.Level
	}{
		"defaults to warn": {
			want: slog.LevelWarn,
		},
		"uses environment when flag unset": {
			env:  "debug",
			want: slog.LevelDebug,
		},
		"flag overrides environment": {
			env:       "debug",
			flagValue: "error",
			flagSet:   true,
			want:      slog.LevelError,
		},
	}

	for name, tc := range tests {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			t.Setenv("FORMAT_LOG_LEVEL", tc.env)

			got, err := ResolveLogLevel(tc.flagValue, tc.flagSet)
			if err != nil {
				t.Fatalf("ResolveLogLevel() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Fatalf("ResolveLogLevel() = %v, want %v", got, tc.want)
			}
		})
	}
}
