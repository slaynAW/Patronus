package eventlog

import (
	"testing"
	"time"
)

// Lecture réelle du journal « System » (CI Windows) : démarrages du noyau (Kernel-General 12).
func TestWindowsQueryRealLog(t *testing.T) {
	events, err := Query("System", XPath([]Selector{{Provider: "Microsoft-Windows-Kernel-General", IDs: []int{12, 13}}}, time.Now().AddDate(-1, 0, 0)), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("aucun démarrage dans le journal System")
	}
	for i, e := range events {
		t.Logf("%s %d %v %v", e.Provider, e.ID, e.Time, e.Data)
		if e.Provider != "Microsoft-Windows-Kernel-General" || e.Time.IsZero() {
			t.Errorf("événement mal lu : %+v", e)
		}
		if i > 0 && e.Time.After(events[i-1].Time) {
			t.Error("ordre : du plus récent au plus ancien attendu")
		}
	}
	// Requête sans résultat : pas d'erreur.
	if events, err := Query("System", XPath([]Selector{{Provider: "Fournisseur-Inexistant", IDs: []int{1}}}, time.Now()), 5); err != nil || len(events) != 0 {
		t.Errorf("requête vide : %v %v", events, err)
	}
}
