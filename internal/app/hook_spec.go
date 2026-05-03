package app

// HookSpec describes an agent harness hook command.
type HookSpec struct {
	Name             string
	Usage            string
	Parser           HookInputParser
	DefaultLogToFile bool
}

// HookSpecs returns supported agent harness hook command definitions.
func HookSpecs() []HookSpec {
	return []HookSpec{
		{
			Name:             "codex",
			Usage:            "Read Codex hook JSON from stdin and format edited files; logs to file by default",
			Parser:           ParseCodexHookInput,
			DefaultLogToFile: true,
		},
		{
			Name:   "apply-patch",
			Usage:  "Read apply-patch text from stdin and format edited files",
			Parser: ParseApplyPatchHookInput,
		},
	}
}
