package disks

import "strings"

// mount est un système de fichiers monté.
type mount struct {
	Device, Mount, FS string
}

// ignoredFS : systèmes de fichiers sans disque à suivre (images, mémoire, réseau, conteneurs).
var ignoredFS = map[string]bool{
	"squashfs": true, "iso9660": true, "tmpfs": true, "overlay": true, "devtmpfs": true, "ramfs": true,
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "fuse.sshfs": true, "udf": true,
}

// parseMounts garde, une fois par disque, les systèmes de fichiers montés depuis un périphérique
// (« /dev/… », sauf les images « loop »).
func parseMounts(text string) []mount {
	var out []mount
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		device, path, fs := f[0], unescape(f[1]), f[2]
		if !strings.HasPrefix(device, "/dev/") || strings.HasPrefix(device, "/dev/loop") || ignoredFS[fs] || seen[device] {
			continue
		}
		seen[device] = true
		out = append(out, mount{Device: device, Mount: path, FS: fs})
	}
	return out
}

// unescape décode les espaces (« \040 ») des chemins de /proc/mounts.
func unescape(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\134`, `\`).Replace(s)
}

// physicalBlock indique un disque physique de /sys/block (pas une image, la mémoire ou un volume logique).
func physicalBlock(name string) bool {
	for _, p := range []string{"loop", "ram", "zram", "dm-", "md", "sr", "fd", "nbd"} {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}
