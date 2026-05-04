package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-playground/validator/v10"
)

// DefaultConfigPath is the project-local config file path used when no
// explicit path is provided by the caller.
const DefaultConfigPath = "format.json"

// ConfigFlagName is the CLI flag name used to provide an explicit config path.
const ConfigFlagName = "config"

// GlobalConfigPath returns the per-user config file path searched after the
// project-local config path when no explicit config path is provided.
func GlobalConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("[in app.GlobalConfigPath] locate user home directory so global config can be searched in ~/.format: %w", err)
	}

	return filepath.Join(homeDir, ".format", DefaultConfigPath), nil
}

// ErrInvalidConfig is returned when a format configuration is structurally
// valid JSON but missing required formatter settings.
var ErrInvalidConfig = errors.New("invalid format config")

// configValidator validates decoded format configuration values using the
// validation tags on the configuration structs.
var configValidator = validator.New(validator.WithRequiredStructEnabled())

// Config describes the top-level format.json configuration file.
type Config struct {
	// Schema optionally identifies a JSON schema used by editors and tooling.
	Schema string `json:"$schema,omitempty"`

	// Version is the configuration schema version.
	Version int `json:"version" validate:"gt=0"`

	// MatchPolicy controls how formatter matches are applied. It defaults to
	// "first" so formatter order is respected when patterns overlap. Use "all"
	// to run every matching formatter.
	MatchPolicy string `json:"matchPolicy" validate:"omitempty,oneof=all first"`

	// Exclude contains global glob patterns that should be ignored before any
	// formatter-specific matching is performed.
	Exclude []string `json:"exclude,omitempty"`

	// WorkingDirectory is the default process working directory used when running
	// formatter commands. Relative values are resolved from the caller's current
	// working directory.
	WorkingDirectory string `json:"workingDirectory,omitempty"`

	// Formatters is the ordered list of formatter definitions.
	Formatters []Formatter `json:"formatters" validate:"required,min=1,dive"`
}

// Formatter maps a set of file patterns to a formatter command.
type Formatter struct {
	// Name is a human-readable identifier for the formatter.
	Name string `json:"name" validate:"required"`

	// Patterns contains glob patterns matched against candidate file paths.
	Patterns []string `json:"patterns" validate:"required,min=1,dive,required"`

	// Exclude contains formatter-specific glob patterns to ignore.
	Exclude []string `json:"exclude,omitempty"`

	// WorkingDirectory overrides the top-level process working directory for this
	// formatter. Relative values are resolved from the caller's current working
	// directory.
	WorkingDirectory string `json:"workingDirectory,omitempty"`

	// FilesDelimiter joins file paths when expanding the $FILES placeholder. It
	// defaults to a single space when omitted.
	FilesDelimiter string `json:"filesDelimiter,omitempty"`

	// Command is the executable and argv list used to invoke the formatter.
	// Placeholder arguments such as "$FILES" and "$WORKING_DIRECTORY" are
	// expanded by the formatter runner.
	Command []string `json:"command" validate:"required,min=1,dive,required"`
}

// LoadDefaultConfig loads the default format configuration from format.json.
func LoadDefaultConfig() (*Config, error) {
	cfg, _, err := LoadConfigForPath("")
	if err != nil {
		return nil, fmt.Errorf("[in app.LoadDefaultConfig] load default config so the app command can start: %w", err)
	}

	return cfg, nil
}

// LoadConfigForPath loads and validates the explicit config path when provided.
// Without an explicit path, it searches the project-local config path first and
// then the per-user global config path.
func LoadConfigForPath(path string) (*Config, string, error) {
	if path != "" {
		cfg, err := LoadConfig(path)
		if err != nil {
			return nil, "", fmt.Errorf("[in app.LoadConfigForPath] load explicit config %q because caller requested it: %w", path, err)
		}

		return cfg, path, nil
	}

	paths := []string{DefaultConfigPath}
	globalPath, err := GlobalConfigPath()
	if err != nil {
		return nil, "", fmt.Errorf("[in app.LoadConfigForPath] resolve global config path after project config was not explicitly requested: %w", err)
	}
	paths = append(paths, globalPath)

	var loadErrors []error
	for _, candidate := range paths {
		cfg, err := LoadConfig(candidate)
		if err == nil {
			return cfg, candidate, nil
		}

		if errors.Is(err, os.ErrNotExist) {
			loadErrors = append(loadErrors, err)
			continue
		}

		return nil, "", fmt.Errorf("[in app.LoadConfigForPath] load config %q selected by default search order: %w", candidate, err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("[in app.LoadConfigForPath] get current working directory after config lookup failed: %w", err)
	}

	return nil, "", fmt.Errorf("[in app.LoadConfigForPath] no format config found; looked for %q relative to current working directory %q, then global config %q; pass --config to use a specific config file: %w", DefaultConfigPath, wd, globalPath, errors.Join(loadErrors...))
}

// LoadConfig loads and validates a format configuration from path.
func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("[in app.LoadConfig] open config %q so formatter settings can be read: %w", path, err)
	}
	defer file.Close()

	cfg, err := DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("[in app.LoadConfig] decode config %q so formatter settings can be used: %w", path, err)
	}

	return cfg, nil
}

// DecodeConfig decodes and validates a format configuration from r.
func DecodeConfig(r io.Reader) (*Config, error) {
	var cfg Config

	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("[in app.DecodeConfig] decode JSON config so formatter settings can be loaded: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("[in app.DecodeConfig] validate config so formatter settings are complete: %w", err)
	}

	return &cfg, nil
}

// Validate reports whether cfg contains the fields required to run formatters.
func (cfg Config) Validate() error {
	if err := configValidator.Struct(cfg); err != nil {
		return fmt.Errorf("[in app.Validate] reject config because required formatter settings are missing or invalid: %w", errors.Join(ErrInvalidConfig, err))
	}

	return nil
}
