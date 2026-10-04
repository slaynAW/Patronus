package disks

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

var errUnsupported = errors.New("non pris en charge")

// volumes lit les systèmes de fichiers montés depuis un disque (/proc/self/mounts).
func volumes() ([]protocol.Volume, error) {
	raw, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return nil, err
	}
	var out []protocol.Volume
	for _, m := range parseMounts(string(raw)) {
		var st unix.Statfs_t
		if unix.Statfs(m.Mount, &st) != nil || st.Blocks == 0 {
			continue
		}
		out = append(out, protocol.Volume{Mount: m.Mount, FS: m.FS, Total: st.Blocks * uint64(st.Bsize), Free: st.Bavail * uint64(st.Bsize)})
	}
	return out, nil
}

// drives lit les disques physiques (/sys/block) : modèle, type, taille, température si le noyau la donne.
func drives() ([]protocol.Drive, error) {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	var out []protocol.Drive
	for _, e := range entries {
		name := e.Name()
		if !physicalBlock(name) {
			continue
		}
		dir := filepath.Join("/sys/block", name)
		d := protocol.Drive{Name: strings.TrimSpace(readText(filepath.Join(dir, "device", "model")))}
		if d.Name == "" {
			d.Name = name
		}
		if sectors, err := strconv.ParseUint(strings.TrimSpace(readText(filepath.Join(dir, "size"))), 10, 64); err == nil {
			d.Size = sectors * 512
		}
		if d.Size == 0 {
			continue
		}
		switch strings.TrimSpace(readText(filepath.Join(dir, "queue", "rotational"))) {
		case "1":
			d.Media = protocol.DriveHDD
		case "0":
			d.Media = protocol.DriveSSD
		}
		target, _ := filepath.EvalSymlinks(dir)
		switch {
		case strings.HasPrefix(name, "nvme"):
			d.Bus = "NVMe"
		case strings.Contains(target, "/usb"):
			d.Bus = "USB"
		case strings.Contains(target, "/ata"):
			d.Bus = "SATA"
		}
		for _, pattern := range []string{"device/hwmon/hwmon*/temp1_input", "device/hwmon*/temp1_input", "device/device/hwmon/hwmon*/temp1_input"} {
			files, _ := filepath.Glob(filepath.Join(dir, pattern))
			if len(files) == 0 {
				continue
			}
			if milli, err := strconv.ParseFloat(strings.TrimSpace(readText(files[0])), 64); err == nil && milli > 0 {
				t := milli / 1000
				d.Temp = &t
				break
			}
		}
		out = append(out, d)
	}
	return out, nil
}

func diskErrors(time.Time) (int, time.Time, error) { return 0, time.Time{}, errUnsupported }

func readText(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}
