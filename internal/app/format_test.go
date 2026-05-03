package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExpandCommandArguments(t *testing.T) {
	workingDirectory := t.TempDir()
	writeTestFile(t, workingDirectory, "B.DotSettings")
	writeTestFile(t, workingDirectory, "A.DotSettings")
	writeTestFile(t, workingDirectory, "$FILES.DotSettings")
	writeTestFile(t, workingDirectory, "$FILE.DotSettings")
	writeTestFile(t, workingDirectory, "src", "Nested.DotSettings")

	tests := map[string]struct {
		command          []string
		files            []string
		workingDirectory string
		filesDelimiter   string
		want             []string
		wantErr          bool
	}{
		"expands files with default space delimiter": {
			command: []string{"tool", "--write", "$FILES"},
			files:   []string{"/repo/a.go", "/repo/b.go"},
			want:    []string{"tool", "--write", "/repo/a.go /repo/b.go"},
		},
		"expands working directory as one argument": {
			command:          []string{"tool", "--cwd", "$WORKING_DIRECTORY", "$FILES"},
			files:            []string{"/repo/a.go"},
			workingDirectory: "/repo",
			want:             []string{"tool", "--cwd", "/repo", "/repo/a.go"},
		},
		"expands files with custom comma delimiter": {
			command:        []string{"tool", "--files", "$FILES"},
			files:          []string{"/repo/a.go", "/repo/b.go"},
			filesDelimiter: ",",
			want:           []string{"tool", "--files", "/repo/a.go,/repo/b.go"},
		},
		"expands embedded files placeholder as delimited list": {
			command:        []string{"tool", "--files=$FILES"},
			files:          []string{"/repo/a.go", "/repo/b.go"},
			filesDelimiter: ";",
			want:           []string{"tool", "--files=/repo/a.go;/repo/b.go"},
		},
		"expands embedded working directory placeholder": {
			command:          []string{"tool", "--cwd=$WORKING_DIRECTORY", "$FILES"},
			files:            []string{"/repo/a.go"},
			workingDirectory: "/repo",
			want:             []string{"tool", "--cwd=/repo", "/repo/a.go"},
		},
		"expands first file basename placeholder as one argument": {
			command:          []string{"tool", "--settings", "$GLOB_FIRST_BASENAME([AB].DotSettings)", "$FILES"},
			files:            []string{"/repo/a.cs"},
			workingDirectory: workingDirectory,
			want:             []string{"tool", "--settings", "A.DotSettings", "/repo/a.cs"},
		},
		"expands embedded first file basename placeholder": {
			command:          []string{"tool", "--settings=$GLOB_FIRST_BASENAME([AB].DotSettings)", "$FILES"},
			files:            []string{"/repo/a.cs"},
			workingDirectory: workingDirectory,
			want:             []string{"tool", "--settings=A.DotSettings", "/repo/a.cs"},
		},
		"expands multiple first file basename placeholders": {
			command:          []string{"tool", "$GLOB_FIRST_BASENAME([AB].DotSettings):$GLOB_FIRST_BASENAME(src/*.DotSettings)", "$FILES"},
			files:            []string{"/repo/a.cs"},
			workingDirectory: workingDirectory,
			want:             []string{"tool", "A.DotSettings:Nested.DotSettings", "/repo/a.cs"},
		},
		"does not re-expand files placeholder from basename": {
			command:          []string{"tool", "--settings=$GLOB_FIRST_BASENAME($FILES.DotSettings)", "$FILES"},
			files:            []string{"/repo/a.cs"},
			workingDirectory: workingDirectory,
			want:             []string{"tool", "--settings=$FILES.DotSettings", "/repo/a.cs"},
		},
		"does not reject singular file placeholder from basename": {
			command:          []string{"tool", "--settings=$GLOB_FIRST_BASENAME($FILE.DotSettings)", "$FILES"},
			files:            []string{"/repo/a.cs"},
			workingDirectory: workingDirectory,
			want:             []string{"tool", "--settings=$FILE.DotSettings", "/repo/a.cs"},
		},
		"rejects invalid first file basename glob": {
			command:          []string{"tool", "$GLOB_FIRST_BASENAME([)", "$FILES"},
			files:            []string{"/repo/a.cs"},
			workingDirectory: workingDirectory,
			wantErr:          true,
		},
		"rejects unmatched first file basename glob": {
			command:          []string{"tool", "$GLOB_FIRST_BASENAME(*.missing)", "$FILES"},
			files:            []string{"/repo/a.cs"},
			workingDirectory: workingDirectory,
			wantErr:          true,
		},
		"rejects malformed first file basename placeholder": {
			command:          []string{"tool", "$GLOB_FIRST_BASENAME()", "$FILES"},
			files:            []string{"/repo/a.cs"},
			workingDirectory: workingDirectory,
			wantErr:          true,
		},
		"allows command without files placeholder": {
			command: []string{"tool"},
			files:   []string{"/repo/a.go"},
			want:    []string{"tool"},
		},
		"rejects singular file placeholder": {
			command: []string{"tool", "$FILE"},
			files:   []string{"/repo/a.go"},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			got, err := expandCommandArguments(tc.command, tc.files, tc.workingDirectory, tc.filesDelimiter)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expandCommandArguments() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("expandCommandArguments() error = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expandCommandArguments() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGroupFilesByFormatterRespectsFirstMatchOrder(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	file := filepath.Join(cwd, "site", "docs", "guide.md")
	writeTestFile(t, file)

	cfg := &Config{
		Formatters: []Formatter{
			{Name: "docs", Patterns: []string{"*/docs/*.md"}},
			{Name: "markdown", Patterns: []string{"**/*.md"}},
		},
	}

	groups, stats, err := groupFilesByFormatter(slog.Default(), cfg, []string{file})
	if err != nil {
		t.Fatalf("groupFilesByFormatter() error = %v, want nil", err)
	}
	if stats.matchedFileCount != 1 {
		t.Fatalf("matchedFileCount = %d, want 1", stats.matchedFileCount)
	}
	if got := groups[0].files; !reflect.DeepEqual(got, []string{file}) {
		t.Fatalf("first formatter files = %v, want %v", got, []string{file})
	}
	if got := groups[1].files; len(got) != 0 {
		t.Fatalf("second formatter files = %v, want empty", got)
	}
}

func writeTestFile(t *testing.T, pathElements ...string) {
	t.Helper()

	path := filepath.Join(pathElements...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v, want nil", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v, want nil", err)
	}
}

func TestEffectiveWorkingDirectory(t *testing.T) {
	tests := map[string]struct {
		cfg       *Config
		formatter Formatter
		want      string
	}{
		"uses top level default": {
			cfg:  &Config{WorkingDirectory: "repo"},
			want: "repo",
		},
		"formatter overrides top level default": {
			cfg:       &Config{WorkingDirectory: "repo"},
			formatter: Formatter{WorkingDirectory: "subdir"},
			want:      "subdir",
		},
	}

	for name, tc := range tests {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			got := effectiveWorkingDirectory(tc.cfg, tc.formatter)
			if got != tc.want {
				t.Fatalf("effectiveWorkingDirectory() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveWorkingDirectory(t *testing.T) {
	got, err := resolveWorkingDirectory(".")
	if err != nil {
		t.Fatalf("resolveWorkingDirectory() error = %v, want nil", err)
	}

	want, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v, want nil", err)
	}
	if got != want {
		t.Fatalf("resolveWorkingDirectory() = %q, want %q", got, want)
	}
}
