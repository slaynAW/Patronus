package netprofile

import (
	"errors"
	"fmt"
	"runtime"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Profiles lit les réseaux connectés (classe WMI MSFT_NetConnectionProfile).
func Profiles() (profiles []Profile, err error) {
	err = withCOM(func() error {
		service, release, err := wmi(`root\StandardCimv2`)
		if err != nil {
			return err
		}
		defer release()
		return each(service, "SELECT Name, InterfaceAlias, InterfaceIndex, NetworkCategory FROM MSFT_NetConnectionProfile",
			func(item *ole.IDispatch) error {
				profiles = append(profiles, Profile{
					Name:      stringProperty(item, "Name"),
					Interface: stringProperty(item, "InterfaceAlias"),
					Index:     intProperty(item, "InterfaceIndex"),
					Category:  Category(intProperty(item, "NetworkCategory")),
				})
				return nil
			})
	})
	return profiles, err
}

// SetPrivate classe en Privé le réseau de la carte index (droits administrateur nécessaires).
func SetPrivate(index int) error {
	return withCOM(func() error {
		service, release, err := wmi(`root\StandardCimv2`)
		if err != nil {
			return err
		}
		defer release()
		found := false
		err = each(service, fmt.Sprintf("SELECT * FROM MSFT_NetConnectionProfile WHERE InterfaceIndex = %d", index),
			func(item *ole.IDispatch) error {
				found = true
				if _, err := oleutil.PutProperty(item, "NetworkCategory", int32(Private)); err != nil {
					return err
				}
				result, err := oleutil.CallMethod(item, "Put_")
				if err != nil {
					return err
				}
				return result.Clear()
			})
		if err == nil && !found {
			err = errors.New("réseau introuvable")
		}
		return err
	})
}

// Profils du pare-feu (NET_FW_PROFILE2_*).
const fwProfilePublic = 4

// ReadFirewall lit l'état du pare-feu de Windows pour les réseaux Publics et la règle rule de l'agent.
func ReadFirewall(rule string) (fw Firewall, err error) {
	err = withCOM(func() error {
		unknown, err := oleutil.CreateObject("HNetCfg.FwPolicy2")
		if err != nil {
			return err
		}
		defer unknown.Release()
		policy, err := unknown.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			return err
		}
		defer policy.Release()
		enabled, err := oleutil.GetProperty(policy, "FirewallEnabled", fwProfilePublic)
		if err != nil {
			return err
		}
		fw.PublicEnabled, _ = enabled.Value().(bool)
		_ = enabled.Clear()

		rulesRaw, err := oleutil.GetProperty(policy, "Rules")
		if err != nil {
			return err
		}
		defer rulesRaw.Clear()
		ruleRaw, err := oleutil.CallMethod(rulesRaw.ToIDispatch(), "Item", rule)
		if err != nil {
			return nil // règle absente (installation avec --no-firewall, ou supprimée)
		}
		defer ruleRaw.Clear()
		item := ruleRaw.ToIDispatch()
		ruleEnabled, err := oleutil.GetProperty(item, "Enabled")
		if err == nil {
			fw.RuleFound, _ = ruleEnabled.Value().(bool)
			_ = ruleEnabled.Clear()
		}
		fw.RuleAllowsPublic = intProperty(item, "Profiles")&fwProfilePublic != 0
		return nil
	})
	return fw, err
}

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

// wmi se connecte à un espace de noms WMI local.
func wmi(namespace string) (*ole.IDispatch, func(), error) {
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
		return nil, nil, err
	}
	return serviceRaw.ToIDispatch(), func() { _ = serviceRaw.Clear(); locator.Release() }, nil
}

// each exécute query et appelle f pour chaque objet renvoyé.
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
	for i := 0; i < count && i < 64; i++ {
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

func stringProperty(item *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(item, name)
	if err != nil {
		return ""
	}
	defer v.Clear()
	s, _ := v.Value().(string)
	return s
}

func intProperty(item *ole.IDispatch, name string) int {
	v, err := oleutil.GetProperty(item, name)
	if err != nil {
		return -1
	}
	defer v.Clear()
	switch x := v.Value().(type) {
	case int32:
		return int(x)
	case uint32:
		return int(x)
	case int64:
		return int(x)
	case uint8:
		return int(x)
	case int16:
		return int(x)
	case uint16:
		return int(x)
	case int:
		return x
	}
	return -1
}
