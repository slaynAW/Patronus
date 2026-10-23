// Package dsm reconnaît DSM, le système des NAS Synology, et lit sa version et le modèle du NAS
// (docs/NAS-SYNOLOGY.md).
package dsm

import (
	"os"
	"strings"
)

// Emplacements de DSM (remplaçables pour les tests).
var (
	// PowerOffTool est l'outil d'extinction de DSM, celui de son interface : il arrête proprement les
	// paquets et les volumes, et refuse pendant une opération critique (mise à jour, réparation d'un volume…).
	PowerOffTool = "/usr/syno/sbin/synopoweroff"
	versionFile  = "/etc.defaults/VERSION"
	modelFile    = "/proc/sys/kernel/syno_hw_version"
	synoinfoFile = "/etc.defaults/synoinfo.conf"
)

// Unités systemd ponctuelles écrites par « wol-agent install » sur un NAS Synology : l'agent les démarre
// pour éteindre ou redémarrer le NAS, et synopoweroff s'exécute ainsi hors de son bac à sable.
const (
	PowerOffUnit = "wol-agent-poweroff.service"
	RebootUnit   = "wol-agent-reboot.service"
)

// Detect indique un NAS Synology sous DSM : son outil d'extinction est présent.
func Detect() bool {
	info, err := os.Stat(PowerOffTool)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// Version renvoie la version de DSM (« DSM 7.2.2-72806 Update 3 »), vide si elle est illisible.
func Version() string {
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	return ParseVersion(string(data))
}

// ParseVersion lit le fichier VERSION de DSM (clé="valeur" par ligne) : productversion, buildnumber et
// smallfixnumber, comme DSM l'affiche.
func ParseVersion(text string) string {
	values := keyValues(text)
	product := values["productversion"]
	if product == "" && values["majorversion"] != "" {
		product = values["majorversion"] + "." + values["minorversion"]
	}
	if product == "" {
		return ""
	}
	name := values["os_name"]
	if name == "" {
		name = "DSM"
	}
	out := name + " " + product
	if build := values["buildnumber"]; build != "" {
		out += "-" + build
	}
	if fix := values["smallfixnumber"]; fix != "" && fix != "0" {
		out += " Update " + fix
	}
	return out
}

// Model renvoie le modèle du NAS (« DS218+ »), vide s'il est illisible.
func Model() string {
	if data, err := os.ReadFile(modelFile); err == nil {
		if m := strings.TrimSpace(string(data)); m != "" {
			return m
		}
	}
	if data, err := os.ReadFile(synoinfoFile); err == nil {
		return keyValues(string(data))["upnpmodelname"]
	}
	return ""
}

// keyValues lit les lignes « clé="valeur" » des fichiers de DSM.
func keyValues(text string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return values
}
