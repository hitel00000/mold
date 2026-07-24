package runtime

import (
	"errors"

	"github.com/hitel00000/mold/view"
)

// Config holds configuration parameters required to bootstrap a Mold application runtime.
type Config struct {
	// ResourceDir is the directory path containing Resource YAML files. (Required)
	ResourceDir string

	// DBPath is the file path for the SQLite database. (Required)
	DBPath string

	// BlobDir is the root directory path for local fsblob storage. (Optional)
	// If empty, BlobStore support is not initialized.
	BlobDir string

	// Overrides holds custom HTML template overrides for views. (Optional)
	Overrides *view.TemplateOverrides
}

// Validate checks the configuration for required fields.
// Note: This validates Config struct validity during bootstrap, not record data.
func (c Config) Validate() error {
	if c.ResourceDir == "" {
		return errors.New("runtime: Config.ResourceDir is required and cannot be empty")
	}
	if c.DBPath == "" {
		return errors.New("runtime: Config.DBPath is required and cannot be empty")
	}
	return nil
}
