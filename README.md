# format

`format` is a small command-line tool that routes files to formatter commands based on glob patterns in a JSON configuration file.

It is useful when a project contains multiple languages and you want one command to run the right formatter for each file you pass in.

## Usage

```text
NAME:
   format - Format source code

USAGE:
   format [global options]

GLOBAL OPTIONS:
   --config string, -c string  path to a config file; defaults to ./format.json, then the user config directory
   --log-level string          minimum log level to write (debug, info, warn, error) (default: "warn")
   --log-session-id string     session identifier to include in generated log file names
   --help, -h                  show help
   --log-to-file               write logs to a generated log file
   --log-file string           write logs to the specified file path
```

Pass files as positional arguments:

```sh
format main.go README.md package.json
```

Invalid file inputs are warned about and skipped, so one bad path does not stop the whole run.

## Configuration

By default, `format` searches for configuration in this order:

1. `./format.json`
2. the user config directory at `format/format.json`

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
  "matchPolicy": "all",
  "exclude": [
    ".git/**",
    "node_modules/**",
    "dist/**",
    "build/**"
  ],
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
- `matchPolicy`: controls formatter matching behavior:
  - `all`: run every formatter whose patterns match a file.
  - `first`: run only the first matching formatter for each file.
- `exclude`: global glob patterns to skip before formatter matching.
- `workingDirectory`: default process working directory for formatter commands. If omitted, the current working directory used to launch `format` is used. Relative paths are resolved from that same current working directory.
- `formatters`: ordered formatter definitions.

Each formatter supports:

- `name`: human-readable formatter name used in logs.
- `patterns`: glob patterns matched against input files.
- `exclude`: formatter-specific glob patterns to skip.
- `workingDirectory`: optional formatter-specific process working directory. Overrides the top-level `workingDirectory`.
- `filesDelimiter`: optional delimiter used to join matched files when expanding `$FILES`. Defaults to a single space.
- `command`: formatter command and arguments. It must include the `$FILES` placeholder.

### Command expansion and working directory

Formatter commands are configured as an argv array; each JSON string becomes one process argument unless it contains one of the supported placeholders below.

| Placeholder | Required | Expands to | Notes |
| --- | --- | --- | --- |
| `$FILES` | Yes | One argument containing file paths joined by the formatter's `filesDelimiter`. | File paths are absolute, so they continue to work when `workingDirectory` changes the formatter process directory. `filesDelimiter` defaults to a single space and can be set to values such as `,`, `, `, or `;`. Embedded placeholders are supported, so `--include=$FILES` becomes one `--include=<joined-files>` argument. |
| `$WORKING_DIRECTORY` | No | The resolved process working directory as one argument. | Uses the formatter-level `workingDirectory` when present, otherwise the top-level `workingDirectory`, otherwise the directory where `format` was launched. Embedded placeholders are supported. |
| `$FILE` | No | Nothing. | Unsupported; commands using it are rejected. Use `$FILES` instead. |

For example, with the default delimiter, `"$FILES"` expands to `"/repo/a.go /repo/b.go"`. With `"filesDelimiter": ","`, `"--files=$FILES"` expands to `"--files=/repo/a.go,/repo/b.go"`.

### JSON Schema

This repository includes `format.schema.json` for editor completion and validation of `format.json` files.

To enable schema support in a config file, add a `$schema` property that points to the schema published from this repository:

```json
{
  "$schema": "https://raw.githubusercontent.com/j-d-ha/format/main/format.schema.json",
  "version": 1,
  "matchPolicy": "all",
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
```

Use a custom config file:

```sh
format -c ./format.json ./cmd/cli/main.go
```

Run with debug logging:

```sh
format --log-level debug main.go
```

Write logs to a generated log file:

```sh
format --log-to-file --log-session-id local-run main.go README.md
```

Write logs to a specific file:

```sh
format --log-file ./logs/format.log main.go README.md
```

## Logging

The default log level is `warn`.

Supported log levels are:

- `debug`
- `info`
- `warn`
- `error`

Use `--log-level info` to see high-level progress: config loaded, file matching summary, selected formatters, formatter completion, and total duration. Use `--log-level debug` when troubleshooting matching or command invocation; debug logs include config search paths, original CLI arguments, per-file match decisions, full formatter argv, and captured formatter stdout/stderr.

Use `--log-to-file` to write logs to a generated log file, or `--log-file` to choose the exact path. These two flags are mutually exclusive.

## Notes

- Formatter commands must be installed and available on your `PATH`.
- Matching uses slash-separated glob patterns, so patterns such as `**/*.go` work across platforms.
- Directories passed as inputs are skipped with a warning.
- Missing or invalid file paths are skipped with a warning.
