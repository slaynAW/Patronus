package netprofile

import "testing"

// Lecture réelle sur le PC de test (CI Windows) : réseaux connectés et pare-feu.
func TestReadWindowsProfilesAndFirewall(t *testing.T) {
	profiles, err := Profiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		t.Logf("réseau « %s » (%s, carte %d) : %s", p.Name, p.Interface, p.Index, p.Category)
		if p.Category < Public || p.Category > Domain || p.Index <= 0 {
			t.Errorf("réseau mal lu : %+v", p)
		}
	}
	fw, err := ReadFirewall("Règle inexistante de test")
	if err != nil {
		t.Fatal(err)
	}
	if fw.RuleFound {
		t.Error("règle inexistante trouvée")
	}
	t.Logf("pare-feu (réseaux Publics) actif : %t", fw.PublicEnabled)
}
