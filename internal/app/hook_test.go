package app

import (
	"reflect"
	"testing"
)

func TestParseApplyPatchEditedFiles(t *testing.T) {
	tests := map[string]struct {
		command string
		want    []string
	}{
		"extracts added and updated files": {
			command: "*** Begin Patch\n*** Update File: internal/app/format.go\n*** Add File: README.md\n*** End Patch",
			want:    []string{"internal/app/format.go", "README.md"},
		},
		"deduplicates files": {
			command: "*** Update File: main.go\n*** Update File: main.go",
			want:    []string{"main.go"},
		},
		"ignores unsupported patch lines": {
			command: "*** Delete File: old.go\nunchanged",
			want:    []string{},
		},
	}

	for name, tc := range tests {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			got := ParseApplyPatchEditedFiles(tc.command)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseApplyPatchEditedFiles() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseGenericPatchHookInput(t *testing.T) {
	raw := []byte("*** Update File: a.go\n*** Add File: b.md")
	got, err := ParseGenericPatchHookInput(raw)
	if err != nil {
		t.Fatalf("ParseGenericPatchHookInput() error = %v, want nil", err)
	}

	want := HookInput{Files: []string{"a.go", "b.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseGenericPatchHookInput() = %v, want %v", got, want)
	}
}

func TestParseCodexHookInput(t *testing.T) {
	tests := map[string]struct {
		raw     []byte
		want    HookInput
		wantErr bool
	}{
		"extracts session and files": {
			raw: []byte(`{"session_id":"abc-123","tool_input":{"command":"*** Begin Patch\n*** Update File: a.go\n*** Add File: b.md\n*** End Patch"}}`),
			want: HookInput{
				Files:     []string{"a.go", "b.md"},
				SessionID: "abc-123",
			},
		},
		"rejects invalid json": {
			raw:     []byte(`{`),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			got, err := ParseCodexHookInput(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseCodexHookInput() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCodexHookInput() error = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseCodexHookInput() = %v, want %v", got, tc.want)
			}
		})
	}
}
