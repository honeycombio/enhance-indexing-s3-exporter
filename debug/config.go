package debug

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
)

// Config defines configuration for the debug exporter.
type Config struct {
	// Verbosity controls the level of detail in the debug output
	Verbosity configtelemetry.Level `mapstructure:"verbosity,omitempty"`

	// SamplingInitial sets the number of messages to log at the start
	SamplingInitial int `mapstructure:"sampling_initial"`

	// SamplingThereafter sets the number of messages to log after the initial set
	SamplingThereafter int `mapstructure:"sampling_thereafter"`

	// UseInternalLogger controls whether to use the internal logger
	UseInternalLogger bool `mapstructure:"use_internal_logger"`
}

// Validate checks if the exporter configuration is valid
func (cfg *Config) Validate() error {
	return nil
}

// CreateDefaultConfig creates the default configuration for the debug exporter.
func CreateDefaultConfig() component.Config {
	return &Config{
		Verbosity:          configtelemetry.LevelBasic, // basic is the default per OpenTelemetry spec
		SamplingInitial:    2,
		SamplingThereafter: 1,
		UseInternalLogger:  true,
	}
}
