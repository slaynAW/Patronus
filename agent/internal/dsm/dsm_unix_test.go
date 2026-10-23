//go:build !windows

package dsm

import (
	"os"
	"path/filepath"
	"testing"
)

// Le bit « exécutable » n'existe que hors de Windows.
func TestDetect(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "synopoweroff")
	saved := PowerOffTool
	t.Cleanup(func() { PowerOffTool = saved })
	PowerOffTool = tool

	if Detect() {
		t.Error("DSM reconnu sans son outil d'extinction")
	}
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Detect() {
		t.Error("DSM reconnu avec un outil non exécutable")
	}
	if err := os.Chmod(tool, 0o755); err != nil {
		t.Fatal(err)
	}
	if !Detect() {
		t.Error("DSM non reconnu")
	}
}
