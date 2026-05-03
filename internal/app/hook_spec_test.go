package app

import "testing"

func TestHookSpecs(t *testing.T) {
	specs := HookSpecs()
	if len(specs) == 0 {
		t.Fatalf("HookSpecs() returned no specs")
	}

	seen := map[string]struct{}{}
	for _, spec := range specs {
		if spec.Name == "" {
			t.Fatalf("HookSpecs() contains spec with empty name: %#v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("HookSpecs() contains spec %q with empty usage", spec.Name)
		}
		if spec.Parser == nil {
			t.Fatalf("HookSpecs() contains spec %q with nil parser", spec.Name)
		}
		if _, ok := seen[spec.Name]; ok {
			t.Fatalf("HookSpecs() contains duplicate spec name %q", spec.Name)
		}
		seen[spec.Name] = struct{}{}
	}

	for _, name := range []string{"codex", "claude", "apply-patch"} {
		if _, ok := seen[name]; !ok {
			t.Fatalf("HookSpecs() missing spec %q", name)
		}
	}
}
