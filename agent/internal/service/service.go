// Package service installe l'agent comme service système (démarrage automatique, relance en cas d'erreur).
package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Options d'installation.
type Options struct {
	Binary string // emplacement définitif de l'exécutable
	Config string // fichier de configuration
	Port   int
	// FirewallPublic ouvre aussi le port sur les réseaux « publics » (Windows). Déconseillé.
	FirewallPublic bool
	// NoFirewall ne touche pas au pare-feu.
	NoFirewall bool
}

// CopySelf copie l'exécutable courant vers dest (installation ou mise à jour).
func CopySelf(dest string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(src); err == nil {
		src = resolved
	}
	if same(src, dest) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dest + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("copie de l'agent vers %s impossible : %w", dest, err)
	}
	return nil
}

func same(a, b string) bool {
	ia, err1 := os.Stat(a)
	ib, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ia, ib)
}
