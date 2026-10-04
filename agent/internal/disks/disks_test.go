package disks

import (
	"errors"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func TestToDrive(t *testing.T) {
	// SSD NVMe en bonne santé, compteurs lus.
	d := toDrive(physicalDisk{Name: "Samsung SSD 980 1TB", MediaType: 4, BusType: 17, HealthStatus: 0, Size: 1_000_204_886_016},
		&reliability{Temperature: 41, TemperatureMax: 68, Wear: 3, PowerOnHours: 1234, ReadErrors: 0, WriteErrors: -1})
	if d.Media != protocol.DriveSSD || d.Bus != "NVMe" || d.Health != protocol.DriveHealthy || *d.Temp != 41 || *d.TempMax != 68 ||
		*d.Wear != 3 || *d.Hours != 1234 || *d.ReadErrors != 0 || d.WriteErrors != nil {
		t.Errorf("SSD : %+v", d)
	}
	// Disque dur à surveiller : pas d'usure (propre aux SSD), sonde non lue (0 °C).
	d = toDrive(physicalDisk{Name: "WDC WD20EZRZ", MediaType: 3, BusType: 11, HealthStatus: 1}, &reliability{Temperature: 0, Wear: 0, ReadErrors: 12, WriteErrors: 0, PowerOnHours: -1})
	if d.Media != protocol.DriveHDD || d.Bus != "SATA" || d.Health != protocol.DriveWarning || d.Temp != nil || d.Wear != nil || d.Hours != nil || *d.ReadErrors != 12 {
		t.Errorf("disque dur : %+v", d)
	}
	// Compteurs illisibles, état inconnu.
	d = toDrive(physicalDisk{Name: "Clé USB", MediaType: 0, BusType: 7, HealthStatus: 5}, nil)
	if d.Media != "" || d.Bus != "USB" || d.Health != "" || d.Temp != nil {
		t.Errorf("clé USB : %+v", d)
	}
	if !virtualBus(14) || !virtualBus(16) || virtualBus(17) {
		t.Error("disques virtuels")
	}
}

func TestParseMounts(t *testing.T) {
	text := `sysfs /sys sysfs rw 0 0
/dev/nvme0n1p2 / ext4 rw,relatime 0 0
/dev/nvme0n1p1 /boot/efi vfat rw 0 0
tmpfs /run tmpfs rw 0 0
/dev/loop3 /snap/core/1 squashfs ro 0 0
/dev/sda1 /mnt/Mes\040données ntfs3 rw 0 0
/dev/nvme0n1p2 /var/lib/docker ext4 rw 0 0
`
	got := parseMounts(text)
	want := []mount{{"/dev/nvme0n1p2", "/", "ext4"}, {"/dev/nvme0n1p1", "/boot/efi", "vfat"}, {"/dev/sda1", "/mnt/Mes données", "ntfs3"}}
	if len(got) != len(want) {
		t.Fatalf("montages : %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("montage %d : %+v, attendu %+v", i, got[i], want[i])
		}
	}
	for name, want := range map[string]bool{"nvme0n1": true, "sda": true, "loop0": false, "zram0": false, "dm-1": false, "sr0": false} {
		if physicalBlock(name) != want {
			t.Errorf("%s : %v", name, !want)
		}
	}
}

func TestReaderCachesAndPaces(t *testing.T) {
	now := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	calls := map[string]int{}
	fail := true
	r := &Reader{now: func() time.Time { return now }}
	r.readVolumes = func() ([]protocol.Volume, error) {
		calls["volumes"]++
		if fail {
			return nil, errors.New("illisible")
		}
		return []protocol.Volume{{Mount: "C:", Total: 100, Free: 10}}, nil
	}
	r.readDrives = func() ([]protocol.Drive, error) {
		calls["drives"]++
		return []protocol.Drive{{Name: "SSD"}}, nil
	}
	r.readErrors = func(since time.Time) (int, time.Time, error) {
		calls["errors"]++
		if !since.Equal(now.Add(-30 * 24 * time.Hour)) {
			t.Errorf("période des erreurs : %v", since)
		}
		return 3, now.Add(-time.Hour), nil
	}
	var reported []string
	r.OnError = func(what string, err error) { reported = append(reported, what) }
	if r.Read() != nil {
		t.Fatal("relevé avant la première lecture")
	}
	r.Refresh()
	d := r.Read()
	if d == nil || len(d.Volumes) != 0 || len(d.Drives) != 1 || d.Errors != 3 || d.LastError != now.Add(-time.Hour).Unix() || len(reported) != 1 {
		t.Fatalf("premier relevé : %+v %v", d, reported)
	}
	// 10 s plus tard : rien n'est dû ; 30 s : lecteurs ; 10 min : santé ; 30 min : erreurs.
	fail = false
	now = now.Add(10 * time.Second)
	r.Refresh()
	now = now.Add(20 * time.Second)
	r.Refresh()
	now = now.Add(10 * time.Minute)
	r.Refresh()
	if calls["volumes"] != 3 || calls["drives"] != 2 || calls["errors"] != 1 {
		t.Errorf("relevés : %v", calls)
	}
	if d := r.Read(); len(d.Volumes) != 1 || d.Volumes[0].Mount != "C:" {
		t.Errorf("lecteurs : %+v", d)
	}
}
