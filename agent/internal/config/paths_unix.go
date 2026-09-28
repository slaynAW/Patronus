//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dir est le dossier de configuration de l'agent.
func Dir() string {
	if runtime.GOOS == "darwin" {
		return "/Library/Application Support/WolAgent"
	}
	return "/etc/wol-agent"
}

// DefaultPath est l'emplacement du fichier de configuration.
func DefaultPath() string { return filepath.Join(Dir(), "config.json") }

// restrictPermissions rend le fichier lisible uniquement par son propriétaire (root).
func restrictPermissions(path string) error { return os.Chmod(path, 0o600) }

// restrictDirPermissions réserve le dossier à son propriétaire (root).
func restrictDirPermissions(dir string) error { return os.Chmod(dir, 0o700) }
