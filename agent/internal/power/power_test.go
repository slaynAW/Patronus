package power

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, cmd := range []string{"shutdown", "reboot", "sleep"} {
		if a, ok := Parse(cmd); !ok || string(a) != cmd {
			t.Fatalf("%s non reconnu", cmd)
		}
	}
	if _, ok := Parse("status"); ok {
		t.Fatal("status n'est pas une action d'alimentation")
	}
}

func TestDryRun(t *testing.T) {
	var buf bytes.Buffer
	if err := (DryRun{Logger: log.New(&buf, "", 0)}).Do(Shutdown, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "simulation") {
		t.Fatalf("journal inattendu : %q", buf.String())
	}
}
