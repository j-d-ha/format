package app

import (
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
