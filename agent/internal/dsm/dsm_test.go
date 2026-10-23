package dsm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseVersion(t *testing.T) {
	for text, want := range map[string]string{
		// DSM 7.2 (extrait du fichier /etc.defaults/VERSION).
		"majorversion=\"7\"\nminorversion=\"2\"\nmajor=\"7\"\nminor=\"2\"\nmicro=\"2\"\nbuildphase=\"GM\"\nbuildnumber=\"72806\"\n" +
			"smallfixnumber=\"3\"\nnano=\"3\"\nbase=\"72806\"\nproductversion=\"7.2.2\"\nos_name=\"DSM\"\n": "DSM 7.2.2-72806 Update 3",
		"productversion=\"7.1.1\"\nbuildnumber=\"42962\"\nsmallfixnumber=\"0\"\n": "DSM 7.1.1-42962",
		"majorversion=\"6\"\nminorversion=\"2\"\nbuildnumber=\"25556\"\n":         "DSM 6.2-25556",
		"":                        "",
		"buildnumber=\"72806\"\n": "",
	} {
		if got := ParseVersion(text); got != want {
			t.Errorf("%q : %q, attendu %q", text, got, want)
		}
	}
}

func TestModel(t *testing.T) {
	dir := t.TempDir()
	saved := []string{modelFile, synoinfoFile}
	t.Cleanup(func() { modelFile, synoinfoFile = saved[0], saved[1] })
	modelFile, synoinfoFile = filepath.Join(dir, "absent"), filepath.Join(dir, "synoinfo.conf")

	if m := Model(); m != "" {
		t.Errorf("modèle sans fichier : %q", m)
	}
	if err := os.WriteFile(synoinfoFile, []byte("company_title=\"Synology\"\nupnpmodelname=\"DS218+\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := Model(); m != "DS218+" {
		t.Errorf("modèle (synoinfo.conf) : %q", m)
	}
	modelFile = filepath.Join(dir, "syno_hw_version")
	if err := os.WriteFile(modelFile, []byte("DS218+\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := Model(); m != "DS218+" {
		t.Errorf("modèle (noyau) : %q", m)
	}
}
