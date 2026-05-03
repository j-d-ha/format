package app

import (
	"encoding/json"
	"fmt"
	"strings"
)

// HookInput contains files and metadata extracted from an agent harness hook
// payload.
type HookInput struct {
	Files     []string
	SessionID string
}

// ParseCodexHookInput extracts edited file paths and session metadata from a
// Codex hook JSON payload.
func ParseCodexHookInput(raw []byte) (HookInput, error) {
	var payload struct {
		SessionID string `json:"session_id"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}

	if err := json.Unmarshal(raw, &payload); err != nil {
		return HookInput{}, fmt.Errorf("[in app.ParseCodexHookInput] decode codex hook JSON before extracting edited files and session metadata: %w", err)
	}

	return HookInput{
		Files:     ParseApplyPatchEditedFiles(payload.ToolInput.Command),
		SessionID: strings.TrimSpace(payload.SessionID),
	}, nil
}

// ParseApplyPatchEditedFiles extracts file paths from apply_patch command text.
func ParseApplyPatchEditedFiles(command string) []string {
	files := make([]string, 0)
	seen := map[string]struct{}{}

	for _, line := range strings.Split(command, "\n") {
		for _, prefix := range []string{"*** Update File: ", "*** Add File: "} {
			if !strings.HasPrefix(line, prefix) {
				continue
			}

			file := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if file == "" {
				continue
			}
			if _, ok := seen[file]; ok {
				continue
			}

			seen[file] = struct{}{}
			files = append(files, file)
		}
	}

	return files
}
