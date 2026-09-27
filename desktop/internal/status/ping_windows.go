package status

import (
	"context"
	"encoding/binary"
	"net/netip"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
)

// Ping envoie un « ping » ICMP (IPv4) via l'API Windows, utilisable sans droits administrateur.
func Ping(ctx context.Context, addr netip.Addr, timeout time.Duration) bool {
	addr = addr.Unmap()
	if !addr.Is4() || ctx.Err() != nil {
		return false
	}
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	if timeout <= 0 {
		return false
	}
	handle, _, _ := procIcmpCreateFile.Call()
	if handle == 0 || windows.Handle(handle) == windows.InvalidHandle {
		return false
	}
	defer procIcmpCloseHandle.Call(handle)

	ip := addr.As4()
	payload := []byte("wakeonlan")
	reply := make([]byte, 256) // ICMP_ECHO_REPLY + données + marge
	n, _, _ := procIcmpSendEcho.Call(
		handle,
		uintptr(binary.LittleEndian.Uint32(ip[:])), // IPAddr : octets dans l'ordre réseau
		uintptr(unsafe.Pointer(&payload[0])),
		uintptr(len(payload)),
		0,
		uintptr(unsafe.Pointer(&reply[0])),
		uintptr(len(reply)),
		uintptr(timeout.Milliseconds()),
	)
	// ICMP_ECHO_REPLY : Address (4 octets) puis Status (4 octets) ; IP_SUCCESS = 0.
	return n > 0 && binary.LittleEndian.Uint32(reply[4:8]) == 0
}
