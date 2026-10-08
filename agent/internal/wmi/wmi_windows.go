package wmi

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// MaxItems borne le nombre d'objets lus par requête.
const MaxItems = 32

// WithCOM exécute f sur un fil système fixe, initialisé pour COM.
func WithCOM(f func() error) error {
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

// Connect ouvre l'espace de noms WMI (root\cimv2…) ; à appeler dans WithCOM.
func Connect(namespace string) (*ole.IDispatch, func(), error) {
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

// Each exécute une requête WQL et appelle f pour chaque objet (MaxItems au plus).
func Each(service *ole.IDispatch, query string, f func(*ole.IDispatch) error) error {
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
	for i := 0; i < count && i < MaxItems; i++ {
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

// String lit une propriété texte ("" si elle est absente).
func String(item *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(item, name)
	if err != nil {
		return ""
	}
	defer v.Clear()
	s, _ := v.Value().(string)
	return s
}

// Int lit un nombre (les entiers 64 bits arrivent en texte par WMI) ; -1 s'il est absent.
func Int(item *ole.IDispatch, name string) int64 {
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
