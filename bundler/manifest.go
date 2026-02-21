package bundler

import (
	"bytes"
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

// Manifest represents a Typst package manifest
// Based on official Typst package specification:
// https://github.com/typst/packages/blob/main/docs/manifest.md
type Manifest struct {
	Package  *Package  `toml:"package" json:"package"`
	Template *Template `toml:"template,omitempty" json:"template,omitempty"`
}

type Package struct {
	Name       string `toml:"name" json:"name"`
	Version    string `toml:"version" json:"version"`
	Entrypoint string `toml:"entrypoint" json:"entrypoint"`

	// Required for repository submission
	Authors     []string `toml:"authors" json:"authors"`
	License     string   `toml:"license" json:"license"`
	Description string   `toml:"description" json:"description"`

	// Optional fields
	Homepage   string   `toml:"homepage" json:"homepage"`
	Repository string   `toml:"repository" json:"repository"`
	Keywords   []string `toml:"keywords" json:"keywords"`

	// Discovery metadata
	Categories  []string `toml:"categories" json:"categories"`
	Disciplines []string `toml:"disciplines" json:"disciplines"`

	// Technical metadata
	Compiler string   `toml:"compiler" json:"compiler"`
	Exclude  []string `toml:"exclude" json:"exclude"`
}

// Template represents template-specific configuration
type Template struct {
	Entrypoint string `toml:"entrypoint" json:"entrypoint"`
	Path       string `toml:"path" json:"path"`
	Thumbnail  string `toml:"thumbnail" json:"thumbnail"`
}

func DecodeBytes(manifestData []byte, val any) error {
	decoder := toml.NewDecoder(bytes.NewReader(manifestData))
	if err := decoder.Decode(val); err != nil {
		return fmt.Errorf("failed to parse TOML: %w", err)
	}

	return nil
}
