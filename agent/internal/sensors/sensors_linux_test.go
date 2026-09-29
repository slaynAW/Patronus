package sensors

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHwmon(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		full := filepath.Join(root, path)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("hwmon0/name", "k10temp\n")
	write("hwmon0/temp1_input", "68250\n")
	write("hwmon0/temp1_label", "Tctl\n")
	write("hwmon0/temp3_input", "61000\n")
	write("hwmon0/temp3_label", "Tccd1\n")
	write("hwmon1/name", "amdgpu\n")
	write("hwmon1/temp1_input", "54000\n")
	write("hwmon1/temp1_label", "edge\n")
	write("hwmon2/name", "acpitz\n")
	write("hwmon2/temp1_input", "27800\n")

	cpu, gpu := fromHwmon(readHwmon(root))
	if value(cpu) != 68.3 || value(gpu) != 54 {
		t.Errorf("hwmon : %v, %v", value(cpu), value(gpu))
	}
}
