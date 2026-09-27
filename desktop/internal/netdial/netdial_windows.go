package netdial

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Paramètres de SIO_TCP_INITIAL_RTO (mstcpip.h).
const (
	sioTCPInitialRTO             = 0x98000011 // _WSAIOW(IOC_VENDOR, 17)
	tcpInitialRTOUnspecifiedRTT  = 0xFFFF
	tcpInitialRTONoSynRetransmit = 0xFE
)

type tcpInitialRTOParameters struct {
	Rtt                   uint16
	MaxSynRetransmissions uint8
}

// control désactive la retransmission du SYN. Sans cela, Windows réessaie la connexion après un
// refus (RST) et ne signale « connexion refusée » qu'au bout de ~2 s, au-delà du délai des sondes :
// un PC allumé dont les ports sont fermés passerait pour éteint. Les sondes ont de toute façon un délai
// (1,5 s) inférieur au premier délai de retransmission de Windows (3 s).
func control(_, _ string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		params := tcpInitialRTOParameters{Rtt: tcpInitialRTOUnspecifiedRTT, MaxSynRetransmissions: tcpInitialRTONoSynRetransmit}
		var returned uint32
		// Optimisation seulement : en cas d'échec (Windows ancien), le comportement par défaut s'applique.
		_ = windows.WSAIoctl(windows.Handle(fd), sioTCPInitialRTO, (*byte)(unsafe.Pointer(&params)),
			uint32(unsafe.Sizeof(params)), nil, 0, &returned, nil, 0)
	})
}

func isRefusedErrno(errno syscall.Errno) bool {
	return errno == windows.WSAECONNREFUSED || errno == syscall.ECONNREFUSED
}
