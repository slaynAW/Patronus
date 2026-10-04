package eventlog

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wevtapi        = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtQuery   = wevtapi.NewProc("EvtQuery")
	procEvtNext    = wevtapi.NewProc("EvtNext")
	procEvtRender  = wevtapi.NewProc("EvtRender")
	procEvtClose   = wevtapi.NewProc("EvtClose")
	errNoMoreItems = syscall.Errno(259)  // ERROR_NO_MORE_ITEMS
	errTooSmall    = syscall.Errno(122)  // ERROR_INSUFFICIENT_BUFFER
	errTimeout     = syscall.Errno(1460) // ERROR_TIMEOUT
)

const (
	evtQueryChannelPath      = 0x1
	evtQueryReverseDirection = 0x200
	evtRenderEventXML        = 1
)

// Query renvoie au plus max événements du journal channel (« System ») répondant à la requête XPath,
// du plus récent au plus ancien.
func Query(channel, query string, max int) ([]Event, error) {
	if err := procEvtQuery.Find(); err != nil {
		return nil, ErrUnsupported
	}
	ch, err := windows.UTF16PtrFromString(channel)
	if err != nil {
		return nil, err
	}
	q, err := windows.UTF16PtrFromString(query)
	if err != nil {
		return nil, err
	}
	results, _, callErr := procEvtQuery.Call(0, uintptr(unsafe.Pointer(ch)), uintptr(unsafe.Pointer(q)), evtQueryChannelPath|evtQueryReverseDirection)
	if results == 0 {
		return nil, callErr
	}
	defer procEvtClose.Call(results)
	var out []Event
	handles := make([]uintptr, 32)
	for len(out) < max {
		var returned uint32
		ok, _, callErr := procEvtNext.Call(results, uintptr(len(handles)), uintptr(unsafe.Pointer(&handles[0])), 2000, 0, uintptr(unsafe.Pointer(&returned)))
		if ok == 0 {
			if errors.Is(callErr, errNoMoreItems) || errors.Is(callErr, errTimeout) {
				break
			}
			return out, callErr
		}
		for _, h := range handles[:returned] {
			if text, err := render(h); err == nil && len(out) < max {
				if e, err := Parse(text); err == nil {
					out = append(out, e)
				}
			}
			procEvtClose.Call(h)
		}
	}
	return out, nil
}

// render rend un événement en XML.
func render(h uintptr) (string, error) {
	var used, props uint32
	ok, _, err := procEvtRender.Call(0, h, evtRenderEventXML, 0, 0, uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props)))
	if ok == 0 && !errors.Is(err, errTooSmall) {
		return "", err
	}
	buf := make([]uint16, used/2+1)
	ok, _, err = procEvtRender.Call(0, h, evtRenderEventXML, uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props)))
	if ok == 0 {
		return "", err
	}
	return windows.UTF16ToString(buf), nil
}
