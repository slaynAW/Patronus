package disks

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/agent/internal/eventlog"
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
	err = withCOM(func() error {
		service, release, err := connect(`root\Microsoft\Windows\Storage`)
		if err != nil {
			return err
		}
		defer release()
		var disks []physicalDisk
		err = each(service, "SELECT DeviceId, FriendlyName, MediaType, BusType, HealthStatus, Size FROM MSFT_PhysicalDisk", func(item *ole.IDispatch) error {
			p := physicalDisk{
				DeviceID: stringProp(item, "DeviceId"), Name: strings.TrimSpace(stringProp(item, "FriendlyName")),
				MediaType: int(intProp(item, "MediaType")), BusType: int(intProp(item, "BusType")),
				HealthStatus: int(intProp(item, "HealthStatus")),
			}
			if size := intProp(item, "Size"); size > 0 {
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
		_ = each(service, "SELECT DeviceId, Temperature, TemperatureMax, Wear, PowerOnHours, ReadErrorsUncorrected, WriteErrorsUncorrected FROM MSFT_StorageReliabilityCounter", func(item *ole.IDispatch) error {
			counters[stringProp(item, "DeviceId")] = &reliability{
				Temperature: int(intProp(item, "Temperature")), TemperatureMax: int(intProp(item, "TemperatureMax")),
				Wear: int(intProp(item, "Wear")), PowerOnHours: intProp(item, "PowerOnHours"),
				ReadErrors: intProp(item, "ReadErrorsUncorrected"), WriteErrors: intProp(item, "WriteErrorsUncorrected"),
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

// --- WMI ---

// withCOM exécute f sur un fil système fixe, initialisé pour COM.
func withCOM(f func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		var oleErr *ole.OleError
		if !errors.As(err, &oleErr) || oleErr.Code() != 1 { // S_FALSE : déjà initialisé
			return err
		}
	}
	defer ole.CoUninitialize()
	return f()
}

func connect(namespace string) (*ole.IDispatch, func(), error) {
	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, nil, err
	}
	locator, err := unknown.QueryInterface(ole.IID_IDispatch)
	unknown.Release()
	if err != nil {
		return nil, nil, err
	}
	serviceRaw, err := oleutil.CallMethod(locator, "ConnectServer", nil, namespace)
	if err != nil {
		locator.Release()
		return nil, nil, fmt.Errorf("WMI %s : %w", namespace, err)
	}
	return serviceRaw.ToIDispatch(), func() { _ = serviceRaw.Clear(); locator.Release() }, nil
}

func each(service *ole.IDispatch, query string, f func(*ole.IDispatch) error) error {
	resultRaw, err := oleutil.CallMethod(service, "ExecQuery", query)
	if err != nil {
		return err
	}
	defer resultRaw.Clear()
	result := resultRaw.ToIDispatch()
	countRaw, err := oleutil.GetProperty(result, "Count")
	if err != nil {
		return err
	}
	count := int(countRaw.Val)
	_ = countRaw.Clear()
	for i := 0; i < count && i < 32; i++ {
		itemRaw, err := oleutil.CallMethod(result, "ItemIndex", i)
		if err != nil {
			continue
		}
		err = f(itemRaw.ToIDispatch())
		_ = itemRaw.Clear()
		if err != nil {
			return err
		}
	}
	return nil
}

func stringProp(item *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(item, name)
	if err != nil {
		return ""
	}
	defer v.Clear()
	s, _ := v.Value().(string)
	return s
}

// intProp lit un nombre (les entiers 64 bits arrivent en texte par WMI) ; -1 s'il est absent.
func intProp(item *ole.IDispatch, name string) int64 {
	v, err := oleutil.GetProperty(item, name)
	if err != nil {
		return -1
	}
	defer v.Clear()
	switch x := v.Value().(type) {
	case uint8:
		return int64(x)
	case int8:
		return int64(x)
	case uint16:
		return int64(x)
	case int16:
		return int64(x)
	case uint32:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case uint64:
		return int64(x)
	case int:
		return int64(x)
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
			return n
		}
	}
	return -1
}
