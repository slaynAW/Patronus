package main

import (
	"strings"
	"testing"
)

// La page affichée par WebView2 doit être autonome : feuille de style et script intégrés.
func TestInlinedPage(t *testing.T) {
	page, err := inlinedPage()
	if err != nil {
		t.Fatal(err)
	}
	for _, leftover := range []string{`href="app.css"`, `src="app.js"`} {
		if strings.Contains(page, leftover) {
			t.Errorf("référence externe restante : %s", leftover)
		}
	}
	for _, want := range []string{"<style>", "--status-online", "<script>", "window.__goResolve", "Ajouter un PC"} {
		if !strings.Contains(page, want) {
			t.Errorf("contenu manquant : %s", want)
		}
	}
}
