package debug

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// Config defines configuration for the debug exporter.
type Config struct {
	exporterhelper.TimeoutSettings `mapstructure:",squash"` // squash ensures fields are correctly decoded in embedded struct
	exporterhelper.QueueSettings   `mapstructure:"sending_queue"`
	exporterhelper.RetrySettings   `mapstructure:"retry_on_failure"`

	// Verbosity controls the level of detail in the debug output
	Verbosity string `mapstructure:"verbosity"`
}

// Validate checks if the exporter configuration is valid
func (cfg *Config) Validate() error {
	return nil
}

// CreateDefaultConfig creates the default configuration for the debug exporter.
func CreateDefaultConfig() component.Config {
	return &Config{
		TimeoutSettings: exporterhelper.NewDefaultTimeoutSettings(),
		QueueSettings:   exporterhelper.NewDefaultQueueSettings(),
		RetrySettings:   exporterhelper.NewDefaultRetrySettings(),
		Verbosity:       "normal", // normal, detailed, basic
	}
}
