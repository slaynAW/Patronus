package config

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Dir est le dossier de l'agent : %ProgramData%\WolAgent.
func Dir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "WolAgent")
}

// DefaultPath est l'emplacement du fichier de configuration.
func DefaultPath() string { return filepath.Join(Dir(), "config.json") }

// restrictPermissions limite l'accès au fichier à SYSTEM et aux Administrateurs.
// Les SID sont utilisés plutôt que les noms, qui dépendent de la langue de Windows.
func restrictPermissions(path string) error {
	icacls := filepath.Join(os.Getenv("SystemRoot"), "System32", "icacls.exe")
	return exec.Command(icacls, path, "/inheritance:r",
		"/grant:r", "*S-1-5-18:F", // SYSTEM
		"/grant:r", "*S-1-5-32-544:F", // Administrateurs
	).Run()
}
