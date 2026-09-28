package main

import (
	"strings"
	"testing"
)

// La page affichée par WebView2 doit être autonome : feuille de style, polices et script intégrés.
func TestInlinedPage(t *testing.T) {
	page, err := inlinedPage()
	if err != nil {
		t.Fatal(err)
	}
	for _, leftover := range []string{`href="app.css"`, `src="app.js"`, "url(fonts/"} {
		if strings.Contains(page, leftover) {
			t.Errorf("référence externe restante : %s", leftover)
		}
	}
	for _, want := range []string{"<style>", "--surface:", "url(data:font/woff2;base64,d09GMg", "font-src 'self' data:",
		"<script>", "window.__goResolve", "Ajouter un PC", "Arrêt inattendu"} {
		if !strings.Contains(page, want) {
			t.Errorf("contenu manquant : %s", want)
		}
	}
	if n := strings.Count(page, "url(data:font/woff2;base64,"); n != 5 {
		t.Errorf("polices intégrées : %d (5 attendues)", n)
	}
	// WebView2 (NavigateToString) refuse les pages de plus de 2 Mo.
	if len(page) > 1500*1024 {
		t.Errorf("page trop volumineuse : %d octets", len(page))
	}
}
