# format

`format` is a small command-line tool that routes files to formatter commands based on glob patterns in a JSON configuration file.

It is useful when a project contains multiple languages and you want one command to run the right formatter for each file you pass in.

## Install

```sh
go install github.com/j-d-ha/format/cmd/format@latest
```

Make sure your Go bin directory is on `PATH`:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

Then add a project-local `format.json` or user config at `~/.format/format.json`.

## Claude Code hook setup

Enable auto-formatting after Claude Code file edits by adding a project-local `PostToolUse` hook.

Create or update `.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit|MultiEdit|NotebookEdit",
        "hooks": [
          {
            "type": "command",
            "command": "format hook claude"
          }
        ]
      }
    ]
  }
}
```

Restart Claude Code, or reload settings. Make sure `format` is installed and on `PATH` for Claude Code. If Claude Code cannot resolve shell profile changes, use an absolute command path such as `/Users/me/go/bin/format hook claude`.

## Codex hook setup

Enable Codex hooks, then add a project-local `PostToolUse` hook for `apply_patch` edits.

Create or update `.codex/config.toml`:

```toml
[features]
codex_hooks = true
```

Create or update `.codex/hooks.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "^apply_patch$",
        "hooks": [
          {
            "type": "command",
            "command": "format hook codex",
            "timeout": 30,
            "statusMessage": "Formatting edited files"
          }
        ]
      }
    ]
  }
}
```

Restart Codex after changing hook config. Trust project `.codex/` config when prompted. Make sure `format` is installed and on `PATH` for Codex.

## Usage

```text
NAME:
   format - Format source files

USAGE:
   format [global options] [command [command options]]

COMMANDS:
   files    Format explicit file arguments
   hook     Format files from agent harness hook input
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --config string, -c string  path to config file; defaults to ./format.json, then ~/.format/format.json
   --log-level string          minimum log level to write (debug, info, warn, error) (default: "warn")
   --log-project string        project name to include in generated log file paths; defaults to FORMAT_PROJECT, git root name, then cwd name
   --log-runner string         runner name to include in generated log file paths; defaults to FORMAT_RUNNER, then cli
   --log-session-id string     session identifier to include in generated log file names; defaults to FORMAT_SESSION_ID, then timestamp-pid
   --help, -h                  show help
   --log-to-file               write logs to generated log file
   --log-file string           write logs to specified file path
```

Pass files as positional arguments with either the root command or the explicit `files` subcommand:

```sh
format main.go README.md package.json
format files main.go README.md package.json
```

Invalid file inputs are warned about and skipped, so one bad path does not stop the whole run.

## Commands

### `format files`

Formats explicit file path arguments. This is the same behavior as the root command and is kept as a named command for scripts that prefer an explicit input source.

```sh
format files internal/app/format.go README.md
```

### `format hook`

Formats files extracted from agent harness hook input. Harness-specific parsers live under this namespace so formatter configuration stays harness-agnostic.

Currently supported:

```sh
format hook codex
format hook claude
format hook apply-patch
```

Place global flags before subcommands:

```sh
format --log-level debug hook codex
```

#### `format hook codex`

Reads Codex hook JSON from `stdin`, extracts:

- `session_id` for generated log file names
- `tool_input.command` for edited file paths

The Codex parser scans `tool_input.command` for apply-patch file headers:

```text
*** Update File: path/to/file
*** Add File: path/to/file
```

Then it formats those files through the same matcher and formatter engine used by `format files`.

Example hook command:

```sh
format hook codex
```

Codex hook logging defaults to generated file logs even when `--log-to-file` is not passed. If `session_id` is present, log path becomes:

```text
~/Library/Logs/format/<project>/codex/format-<session_id>.log
```

Overrides:

```sh
format --log-project my-api --log-session-id my-session hook codex
format --log-file ./.codex/logs/format.log hook codex
format --log-level debug hook codex
```

If `stdin` is empty or no edited files are found, command exits successfully without running formatters.

#### `format hook claude`

Reads Claude Code hook JSON from `stdin`, extracts:

- `session_id` for generated log file names
- `tool_input.file_path` for edited files from `Write`, `Edit`, and `MultiEdit`
- `tool_input.notebook_path` for edited notebooks from `NotebookEdit`

Example `.claude/settings.json` hook command:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit|MultiEdit|NotebookEdit",
        "hooks": [
          {
            "type": "command",
            "command": "format hook claude"
          }
        ]
      }
    ]
  }
}
```

Claude hook logging defaults to generated file logs even when `--log-to-file` is not passed. If `session_id` is present, log path becomes:

```text
~/Library/Logs/format/<project>/claude/format-<session_id>.log
```

#### `format hook apply-patch`

Reads raw apply-patch text from `stdin`, extracts edited files from the same patch headers, and formats those files.

```sh
format hook apply-patch
```

Unlike `format hook codex`, this command does not log to file by default because raw patch input has no harness session metadata. Use normal logging flags when needed:

```sh
format --log-to-file --log-session-id patch-run hook apply-patch
format --log-file ./logs/patch-format.log hook apply-patch
```

## Configuration

By default, `format` searches for configuration in this order:

1. `./format.json`
2. `~/.format/format.json`

You can also pass an explicit config file:

```sh
format --config ./my-format.json main.go README.md
# or
format -c ./my-format.json main.go README.md
```

A config file contains global excludes and an ordered list of formatter definitions:

```json
{
  "version": 1,
  "matchPolicy": "first",
  "exclude": [".git/**", "node_modules/**", "dist/**", "build/**"],
  "workingDirectory": ".",
  "formatters": [
    {
      "name": "prettier",
      "patterns": ["**/*.js", "**/*.ts", "**/*.json", "**/*.md"],
      "exclude": ["package-lock.json"],
      "command": ["prettier", "--write", "$FILES"]
    },
    {
      "name": "gofmt",
      "patterns": ["**/*.go"],
      "command": ["gofmt", "-w", "$FILES"]
    }
  ]
}
```

### Fields

- `version`: configuration schema version. Must be greater than `0`.
- `matchPolicy`: controls formatter matching behavior. Defaults to `first` when omitted:
  - `first`: run only the first matching formatter for each file. Formatter order in `format.json` matters. If `*/docs/*.md` appears before `**/*.md`, then `site/docs/guide.md` runs only the `*/docs/*.md` formatter.
  - `all`: run every formatter whose patterns match a file.
- `exclude`: global glob patterns to skip before formatter matching.
- `workingDirectory`: default process working directory for formatter commands. If omitted, the current working directory used to launch `format` is used. Relative paths are resolved from that same current working directory.
- `formatters`: ordered formatter definitions.

Each formatter supports:

- `name`: human-readable formatter name used in logs.
- `patterns`: glob patterns matched against input files.
- `exclude`: formatter-specific glob patterns to skip.
- `workingDirectory`: optional formatter-specific process working directory. Overrides the top-level `workingDirectory`.
- `filesDelimiter`: optional delimiter used to join matched files when expanding `$FILES`. Defaults to a single space.
- `command`: formatter command and arguments. Include `$FILES` when matched files should be passed to the formatter.

### Command expansion and working directory

Formatter commands are configured as an argv array; each JSON string becomes one process argument unless it contains one of the supported placeholders below.

| Placeholder                    | Required | Expands to                                                                     | Notes                                                                                                                                                                                                                                                                                                                          |
| ------------------------------ | -------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `$FILES`                       | No       | One argument containing file paths joined by the formatter's `filesDelimiter`. | File paths are absolute, so they continue to work when `workingDirectory` changes the formatter process directory. `filesDelimiter` defaults to a single space and can be set to values such as `,`, `, `, or `;`. Embedded placeholders are supported, so `--include=$FILES` becomes one `--include=<joined-files>` argument. |
| `$WORKING_DIRECTORY`           | No       | The resolved process working directory as one argument.                        | Uses the formatter-level `workingDirectory` when present, otherwise the top-level `workingDirectory`, otherwise the directory where `format` was launched. Embedded placeholders are supported.                                                                                                                                |
| `$GLOB_FIRST_BASENAME(<glob>)` | No       | Basename of the first deterministic match for `<glob>`.                        | Glob resolves relative to the formatter working directory. Matches are sorted before choosing the first. Embedded placeholders are supported, so `--settings=$GLOB_FIRST_BASENAME(*.DotSettings)` becomes `--settings=<file>.DotSettings`. Invalid or unmatched globs fail the command.                                        |
| `$FILE`                        | No       | Nothing.                                                                       | Unsupported; commands using it are rejected. Use `$FILES` instead.                                                                                                                                                                                                                                                             |

For example, with the default delimiter, `"$FILES"` expands to `"/repo/a.go /repo/b.go"`. With `"filesDelimiter": ","`, `"--files=$FILES"` expands to `"--files=/repo/a.go,/repo/b.go"`. `"--settings=$GLOB_FIRST_BASENAME(*.DotSettings)"` resolves the glob from the formatter working directory and expands to the basename of the first sorted match.

### JSON Schema

This repository includes `format.schema.json` for editor completion and validation of `format.json` files.

To enable schema support in a config file, add a `$schema` property that points to the schema published from this repository:

```json
{
  "$schema": "https://raw.githubusercontent.com/j-d-ha/format/main/format.schema.json",
  "version": 1,
  "matchPolicy": "first",
  "formatters": [
    {
      "name": "gofmt",
      "patterns": ["**/*.go"],
      "command": ["gofmt", "-w", "$FILES"]
    }
  ]
}
```

For local development of this repository, you can instead use a relative path such as `"$schema": "./format.schema.json"`.

## Examples

Format Go and Markdown files using the default `format.json`:

```sh
format internal/app/format.go README.md
format files internal/app/format.go README.md
```

Use a custom config file:

```sh
format -c ./format.json ./cmd/format/main.go
```

Run with debug logging:

```sh
format --log-level debug main.go
```

Write logs to a generated log file:

```sh
format --log-to-file --log-project my-api --log-session-id local-run main.go README.md
```

Generated log path shape:

```text
<log-base>/<project>/<runner>/format-<session-id>.log
```

Default log bases:

- macOS: `~/Library/Logs/format`
- Linux: `${XDG_STATE_HOME}/format/logs` or `~/.local/state/format/logs`
- Windows: `%LOCALAPPDATA%\\format\\Logs`

Generated log identity precedence:

- project: `--log-project`, `FORMAT_PROJECT`, Git root folder name, current directory name, `unknown`
- runner: `--log-runner`, `FORMAT_RUNNER`, hook default such as `codex`, then `cli`
- session: `--log-session-id`, `FORMAT_SESSION_ID`, hook payload session, then `YYYYMMDDTHHMMSSZ-pid`

Set `FORMAT_LOG_DIR` to override the generated log base directory.

Write logs to a specific file:

```sh
format --log-file ./logs/format.log main.go README.md
```

Format files from a Codex hook payload on stdin:

```sh
format --log-level debug hook codex
```

Format files from a Claude Code hook payload on stdin:

```sh
format --log-level debug hook claude
```

Format files from raw apply-patch text on stdin:

```sh
format hook apply-patch
```

## Logging

The default log level is `warn`.

Supported log levels are:

- `debug`
- `info`
- `warn`
- `error`

Use `--log-level info` to see high-level progress: config loaded, file matching summary, selected formatters, formatter completion, and total duration. Use `--log-level debug` when troubleshooting matching or command invocation; debug logs include config search paths, original CLI arguments, per-file match decisions, full formatter argv, and captured formatter stdout/stderr.

Use `--log-to-file` to write logs to a generated log file, or `--log-file` to choose the exact path. These two flags are mutually exclusive. Generated log records include `project`, `runner`, `sessionID`, `cwd`, and `gitRoot` when available.

## Notes

- Formatter commands must be installed and available on your `PATH`.
- Matching uses slash-separated glob patterns, so patterns such as `**/*.go` work across platforms.
- Directories passed as inputs are skipped with a warning.
- Missing or invalid file paths are skipped with a warning.
