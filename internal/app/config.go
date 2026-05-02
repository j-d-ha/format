package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/go-playground/validator/v10"
)

// DefaultConfigPath is the config file path used when no explicit path is
// provided by the caller.
const DefaultConfigPath = "app.json"

// ErrInvalidConfig is returned when a app configuration is structurally
// valid JSON but missing required formatter settings.
var ErrInvalidConfig = errors.New("invalid app config")

// configValidator validates decoded app configuration values using the
// validation tags on the configuration structs.
var configValidator = validator.New(validator.WithRequiredStructEnabled())

// Config describes the top-level app.json configuration file.
type Config struct {
	// Version is the configuration schema version.
	Version int `json:"version" validate:"gt=0"`

	// MatchPolicy controls how formatter matches are applied. Common values are
	// "all" to run every matching formatter and "first" to run only the first
	// matching formatter.
	MatchPolicy string `json:"matchPolicy" validate:"omitempty,oneof=all first"`

	// Exclude contains global glob patterns that should be ignored before any
	// formatter-specific matching is performed.
	Exclude []string `json:"exclude,omitempty"`

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

	// Command is the executable and argv list used to invoke the formatter.
	// Placeholder arguments such as "$file" and "$files" are expanded by the
	// formatter runner.
	Command []string `json:"command" validate:"required,min=1,dive,required"`
}

// LoadDefaultConfig loads the default app configuration from app.json.
func LoadDefaultConfig() (*Config, error) {
	cfg, err := LoadConfig(DefaultConfigPath)
	if err != nil {
		return nil, fmt.Errorf("[in app.LoadDefaultConfig] load default config so the app command can start: %w", err)
	}

	return cfg, nil
}

// LoadConfig loads and validates a app configuration from path.
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

// DecodeConfig decodes and validates a app configuration from r.
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
