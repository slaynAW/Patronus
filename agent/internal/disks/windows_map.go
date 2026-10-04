package disks

import "github.com/slaynaw/wakeonlan/agent/protocol"

// Valeurs des classes WMI MSFT_PhysicalDisk et MSFT_StorageReliabilityCounter (espace de noms
// root\Microsoft\Windows\Storage), converties pour le protocole (fonctions pures, testées partout).

// physicalDisk : propriétés lues de MSFT_PhysicalDisk.
type physicalDisk struct {
	DeviceID     string
	Name         string
	MediaType    int
	BusType      int
	HealthStatus int
	Size         uint64
}

// reliability : propriétés lues de MSFT_StorageReliabilityCounter (valeurs absentes : -1).
type reliability struct {
	Temperature, TemperatureMax, Wear int
	PowerOnHours                      int64
	ReadErrors, WriteErrors           int64
}

var busNames = map[int]string{
	1: "SCSI", 3: "ATA", 6: "Fibre Channel", 7: "USB", 8: "RAID", 9: "iSCSI", 10: "SAS", 11: "SATA",
	12: "SD", 13: "MMC", 17: "NVMe", 18: "SCM", 19: "UFS",
}

// virtualBus : disques virtuels (VHD, espaces de stockage) : pas de santé matérielle à suivre.
func virtualBus(bus int) bool { return bus == 14 || bus == 15 || bus == 16 }

// toDrive convertit un disque Windows et ses compteurs (rel nil : illisibles).
func toDrive(p physicalDisk, rel *reliability) protocol.Drive {
	d := protocol.Drive{Name: p.Name, Bus: busNames[p.BusType], Size: p.Size}
	switch p.MediaType {
	case 3:
		d.Media = protocol.DriveHDD
	case 4, 5:
		d.Media = protocol.DriveSSD
	}
	if d.Media == "" && p.BusType == 17 {
		d.Media = protocol.DriveSSD
	}
	switch p.HealthStatus {
	case 0:
		d.Health = protocol.DriveHealthy
	case 1:
		d.Health = protocol.DriveWarning
	case 2:
		d.Health = protocol.DriveUnhealthy
	}
	if rel == nil {
		return d
	}
	// 0 °C : sonde non lue par le pilote.
	if rel.Temperature > 0 && rel.Temperature < 150 {
		t := float64(rel.Temperature)
		d.Temp = &t
	}
	if rel.TemperatureMax > 0 && rel.TemperatureMax < 150 {
		t := float64(rel.TemperatureMax)
		d.TempMax = &t
	}
	// Usure : propre aux SSD (un disque dur répond 0).
	if rel.Wear >= 0 && rel.Wear <= 100 && d.Media != protocol.DriveHDD {
		w := rel.Wear
		d.Wear = &w
	}
	if rel.PowerOnHours > 0 {
		h := rel.PowerOnHours
		d.Hours = &h
	}
	if rel.ReadErrors >= 0 {
		n := rel.ReadErrors
		d.ReadErrors = &n
	}
	if rel.WriteErrors >= 0 {
		n := rel.WriteErrors
		d.WriteErrors = &n
	}
	return d
}

// DiskErrorSelectors : erreurs d'accès aux disques du journal « System » (secteur illisible,
// erreur du contrôleur, erreur de pagination, accès réessayé, corruption du système de fichiers).
var diskErrorProviders = []struct {
	Provider string
	IDs      []int
}{
	{"disk", []int{7, 11, 51, 153}},
	{"Ntfs", []int{55}},
	{"Microsoft-Windows-Ntfs", []int{55}},
}
