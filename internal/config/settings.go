// Package config handles persistent user preferences for µm.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings holds the values that persist across sessions.
type Settings struct {
	Volume  float64 `json:"volume"`
	LastDir string  `json:"last_dir"`
}

// Load reads settings from disk. Returns defaults if the file is missing or corrupt.
func Load() Settings {
	s := Settings{Volume: 0.7}
	data, err := os.ReadFile(path())
	if err != nil {
		return s
	}
	json.Unmarshal(data, &s) //nolint:errcheck — corrupt file falls back to defaults
	return s
}

// Save writes settings to disk. Errors are silently ignored (non-critical).
func Save(s Settings) {
	p := path()
	os.MkdirAll(filepath.Dir(p), 0o755)
	data, _ := json.Marshal(s)
	os.WriteFile(p, data, 0o644) //nolint:errcheck
}

func path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "um", "settings.json")
}
