package disks

import (
	"errors"
	"strings"
	"time"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/agent/internal/eventlog"
	"github.com/slaynaw/wakeonlan/agent/internal/wmi"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

var errUnsupported = errors.New("non pris en charge")

// volumes lit les lecteurs fixes (C:, D:…) et leur espace.
func volumes() ([]protocol.Volume, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, err
	}
	var out []protocol.Volume
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		path, _ := windows.UTF16PtrFromString(root)
		if windows.GetDriveType(path) != windows.DRIVE_FIXED {
			continue
		}
		var free, total, totalFree uint64
		if err := windows.GetDiskFreeSpaceEx(path, &free, &total, &totalFree); err != nil || total == 0 {
			continue
		}
		v := protocol.Volume{Mount: root[:2], Total: total, Free: free}
		label := make([]uint16, windows.MAX_PATH+1)
		fs := make([]uint16, windows.MAX_PATH+1)
		if windows.GetVolumeInformation(path, &label[0], uint32(len(label)), nil, nil, nil, &fs[0], uint32(len(fs))) == nil {
			v.Label, v.FS = windows.UTF16ToString(label), windows.UTF16ToString(fs)
		}
		out = append(out, v)
	}
	return out, nil
}

// drives lit la santé des disques physiques (WMI, espace de noms root\Microsoft\Windows\Storage).
func drives() (out []protocol.Drive, err error) {
	err = wmi.WithCOM(func() error {
		service, release, err := wmi.Connect(`root\Microsoft\Windows\Storage`)
		if err != nil {
			return err
		}
		defer release()
		var disks []physicalDisk
		err = wmi.Each(service, "SELECT DeviceId, FriendlyName, MediaType, BusType, HealthStatus, Size FROM MSFT_PhysicalDisk", func(item *ole.IDispatch) error {
			p := physicalDisk{
				DeviceID: wmi.String(item, "DeviceId"), Name: strings.TrimSpace(wmi.String(item, "FriendlyName")),
				MediaType: int(wmi.Int(item, "MediaType")), BusType: int(wmi.Int(item, "BusType")),
				HealthStatus: int(wmi.Int(item, "HealthStatus")),
			}
			if size := wmi.Int(item, "Size"); size > 0 {
				p.Size = uint64(size)
			}
			disks = append(disks, p)
			return nil
		})
		if err != nil {
			return err
		}
		counters := map[string]*reliability{}
		// Compteurs de fiabilité (droits d'administrateur : l'agent est un service système).
		_ = wmi.Each(service, "SELECT DeviceId, Temperature, TemperatureMax, Wear, PowerOnHours, ReadErrorsUncorrected, WriteErrorsUncorrected FROM MSFT_StorageReliabilityCounter", func(item *ole.IDispatch) error {
			counters[wmi.String(item, "DeviceId")] = &reliability{
				Temperature: int(wmi.Int(item, "Temperature")), TemperatureMax: int(wmi.Int(item, "TemperatureMax")),
				Wear: int(wmi.Int(item, "Wear")), PowerOnHours: wmi.Int(item, "PowerOnHours"),
				ReadErrors: wmi.Int(item, "ReadErrorsUncorrected"), WriteErrors: wmi.Int(item, "WriteErrorsUncorrected"),
			}
			return nil
		})
		for _, p := range disks {
			if virtualBus(p.BusType) {
				continue
			}
			out = append(out, toDrive(p, counters[p.DeviceID]))
		}
		return nil
	})
	return out, err
}

// diskErrors compte les erreurs d'accès aux disques du journal d'événements depuis since.
func diskErrors(since time.Time) (int, time.Time, error) {
	var selectors []eventlog.Selector
	for _, p := range diskErrorProviders {
		selectors = append(selectors, eventlog.Selector{Provider: p.Provider, IDs: p.IDs})
	}
	events, err := eventlog.Query("System", eventlog.XPath(selectors, since), 1000)
	if err != nil {
		return 0, time.Time{}, err
	}
	var last time.Time
	for _, e := range events {
		if e.Time.After(last) {
			last = e.Time
		}
	}
	return len(events), last, nil
}
