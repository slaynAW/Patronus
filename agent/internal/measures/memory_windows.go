package measures

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// memoryStatusEx : structure MEMORYSTATUSEX de Windows.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// memory lit la mémoire physique totale et disponible, comme le Gestionnaire des tâches.
func memory() *protocol.Measure {
	var s memoryStatusEx
	s.Length = uint32(unsafe.Sizeof(s))
	if ok, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&s))); ok == 0 {
		return nil
	}
	return memoryMeasure(s.TotalPhys, s.AvailPhys)
}
