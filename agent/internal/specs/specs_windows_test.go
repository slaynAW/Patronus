package specs

import "testing"

// Lecture réelle de la table SMBIOS sous Windows (CI).
func TestWindowsFirmwareTable(t *testing.T) {
	table, err := firmwareTable()
	if err != nil {
		t.Fatal(err)
	}
	structures, err := parseSMBIOS(table)
	if err != nil {
		t.Errorf("table SMBIOS : %v", err)
	}
	if len(structures) == 0 {
		t.Fatal("aucune structure SMBIOS")
	}
	fw := readFirmware(structures)
	t.Logf("%d structures ; modèle %q, carte %+v, processeurs %+v, %d emplacements, barrettes %+v", len(structures), fw.model, fw.board, fw.cpus, fw.slots, fw.modules)
	t.Logf("cartes graphiques : %+v", gpus())
}
