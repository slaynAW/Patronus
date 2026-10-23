package service

import (
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/internal/dsm"
)

func TestUnitFile(t *testing.T) {
	unit := unitFile("/usr/local/bin/wol-agent", "/etc/wol-agent/config.json")
	for _, want := range []string{
		"ExecStart=/usr/local/bin/wol-agent run --config \"/etc/wol-agent/config.json\"\n",
		// /etc en lecture seule, sauf le dossier de l'agent : identité, appareils autorisés, journal, mesures.
		"ProtectSystem=full\n",
		"ReadWriteDirectories=/etc/wol-agent\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("ligne manquante : %q\n%s", want, unit)
		}
	}
}

func TestDSMUnits(t *testing.T) {
	units := dsmUnits()
	for name, exec := range map[string]string{dsm.PowerOffUnit: dsm.PowerOffTool, dsm.RebootUnit: dsm.PowerOffTool + " -r"} {
		unit := units[name]
		if !strings.Contains(unit, "Type=oneshot\n") || !strings.Contains(unit, "ExecStart="+exec+"\n") {
			t.Errorf("%s :\n%s", name, unit)
		}
		// Jamais au démarrage : seulement à la demande de l'agent.
		if strings.Contains(unit, "[Install]") {
			t.Errorf("%s activée au démarrage", name)
		}
	}
	if len(units) != 2 {
		t.Errorf("%d unités", len(units))
	}
}
