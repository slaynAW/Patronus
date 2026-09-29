package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// Le fichier de l'agent vérifié reste verrouillé : ni modifiable, ni renommable, ni effaçable tant
// que l'installation n'est pas terminée ; une empreinte différente est refusée.
func TestLockVerified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wol-agent-windows-amd64.exe")
	content := []byte("agent vérifié")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if _, err := lockVerified(path, hex.EncodeToString(make([]byte, 32))); err == nil {
		t.Fatal("empreinte différente acceptée")
	}
	f, err := lockVerified(path, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent piégé"), 0o600); err == nil {
		t.Error("fichier modifiable pendant le verrou")
	}
	if err := os.Rename(path, path+".autre"); err == nil {
		t.Error("fichier renommable pendant le verrou")
	}
	if err := os.Remove(path); err == nil {
		t.Error("fichier effaçable pendant le verrou")
	}
	// Lecture toujours possible (l'installateur lit et copie son propre fichier).
	if data, err := os.ReadFile(path); err != nil || string(data) != string(content) {
		t.Errorf("lecture pendant le verrou : %q %v", data, err)
	}
	f.Close()
	if err := os.Remove(path); err != nil {
		t.Errorf("fichier non libéré après le verrou : %v", err)
	}
}

// SHELLEXECUTEINFOW fait 112 octets sur x64 et ARM64.
func TestShellExecuteInfoLayout(t *testing.T) {
	if size := unsafe.Sizeof(shellExecuteInfo{}); size != 112 {
		t.Errorf("taille de SHELLEXECUTEINFOW : %d", size)
	}
	if off := unsafe.Offsetof(shellExecuteInfo{}.process); off != 104 {
		t.Errorf("position de hProcess : %d", off)
	}
}
