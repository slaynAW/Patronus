package history

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/eventlog"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Cause explique un arrêt non enregistré : Reason (protocol.LostBSOD, LostButton, LostPower,
// LostHardware ; vide si inconnue) et Detail (code de l'écran bleu…).
type Cause struct {
	Reason string
	Detail string
}

// rank classe les causes de la plus vague à la plus précise.
func (c Cause) rank() int {
	switch c.Reason {
	case protocol.LostHardware:
		return 4
	case protocol.LostBSOD:
		if c.Detail != "" {
			return 3
		}
		return 2
	case protocol.LostButton, protocol.LostPower:
		return 1
	}
	return 0
}

// CrashSelectors : événements du journal « System » qui expliquent un arrêt anormal.
var CrashSelectors = []eventlog.Selector{
	// Redémarrage sans arrêt propre (code d'écran bleu, bouton d'alimentation maintenu…).
	{Provider: "Microsoft-Windows-Kernel-Power", IDs: []int{41}},
	// « L'ordinateur a redémarré après une vérification d'erreur » : code de l'écran bleu.
	{Provider: "Microsoft-Windows-WER-SystemErrorReporting", IDs: []int{1001}},
	// Erreur matérielle fatale.
	{Provider: "Microsoft-Windows-WHEA-Logger", IDs: []int{18}},
}

// ExplainEvents déduit la cause d'un arrêt anormal des événements écrits par Windows après lui.
func ExplainEvents(events []eventlog.Event) Cause {
	var best Cause
	for _, e := range events {
		var c Cause
		switch {
		case e.Provider == "Microsoft-Windows-WHEA-Logger" && e.ID == 18:
			c = Cause{Reason: protocol.LostHardware}
		case e.Provider == "Microsoft-Windows-WER-SystemErrorReporting" && e.ID == 1001:
			c = Cause{Reason: protocol.LostBSOD, Detail: bugcheckFromText(e.Data["param1"])}
		case e.Provider == "Microsoft-Windows-Kernel-Power" && e.ID == 41:
			code, _ := strconv.ParseUint(strings.TrimSpace(e.Data["BugcheckCode"]), 10, 32)
			ts := strings.TrimSpace(e.Data["PowerButtonTimestamp"])
			switch {
			case code != 0:
				c = Cause{Reason: protocol.LostBSOD, Detail: bugcheckName(uint32(code))}
			case ts != "" && ts != "0", strings.EqualFold(e.Data["LongPowerButtonPressDetected"], "true"):
				c = Cause{Reason: protocol.LostButton}
			default:
				c = Cause{Reason: protocol.LostPower}
			}
		default:
			continue
		}
		if c.rank() > best.rank() {
			best = c
		}
	}
	return best
}

// bugcheckFromText lit le code d'un écran bleu (« 0x0000007e (0xffffffffc0000005, …) »).
func bugcheckFromText(text string) string {
	first := strings.Fields(text)
	if len(first) == 0 {
		return ""
	}
	code, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(first[0]), "0x"), 16, 32)
	if err != nil || code == 0 {
		return ""
	}
	return bugcheckName(uint32(code))
}

// bugcheckName renvoie le code d'arrêt et son nom quand il est connu (« 0x7E SYSTEM_THREAD_EXCEPTION_NOT_HANDLED »).
func bugcheckName(code uint32) string {
	text := fmt.Sprintf("0x%X", code)
	if name, ok := bugchecks[code]; ok {
		text += " " + name
	}
	return text
}

// bugchecks : codes d'arrêt les plus fréquents (documentation Microsoft « Bug check code reference »).
var bugchecks = map[uint32]string{
	0x0A: "IRQL_NOT_LESS_OR_EQUAL", 0x19: "BAD_POOL_HEADER", 0x1A: "MEMORY_MANAGEMENT",
	0x1E: "KMODE_EXCEPTION_NOT_HANDLED", 0x24: "NTFS_FILE_SYSTEM", 0x3B: "SYSTEM_SERVICE_EXCEPTION",
	0x50: "PAGE_FAULT_IN_NONPAGED_AREA", 0x7A: "KERNEL_DATA_INPAGE_ERROR", 0x7B: "INACCESSIBLE_BOOT_DEVICE",
	0x7E: "SYSTEM_THREAD_EXCEPTION_NOT_HANDLED", 0x7F: "UNEXPECTED_KERNEL_MODE_TRAP", 0x9F: "DRIVER_POWER_STATE_FAILURE",
	0xA0: "INTERNAL_POWER_ERROR", 0xC2: "BAD_POOL_CALLER", 0xC4: "DRIVER_VERIFIER_DETECTED_VIOLATION",
	0xD1: "DRIVER_IRQL_NOT_LESS_OR_EQUAL", 0xEF: "CRITICAL_PROCESS_DIED", 0xF4: "CRITICAL_OBJECT_TERMINATION",
	0x101: "CLOCK_WATCHDOG_TIMEOUT", 0x109: "CRITICAL_STRUCTURE_CORRUPTION", 0x10D: "WDF_VIOLATION",
	0x113: "VIDEO_DXGKRNL_FATAL_ERROR", 0x116: "VIDEO_TDR_FAILURE", 0x117: "VIDEO_TDR_TIMEOUT_DETECTED",
	0x119: "VIDEO_SCHEDULER_INTERNAL_ERROR", 0x124: "WHEA_UNCORRECTABLE_ERROR", 0x133: "DPC_WATCHDOG_VIOLATION",
	0x139: "KERNEL_SECURITY_CHECK_FAILURE", 0x13A: "KERNEL_MODE_HEAP_CORRUPTION", 0x154: "UNEXPECTED_STORE_EXCEPTION",
	0x15F: "CONNECTED_STANDBY_WATCHDOG_TIMEOUT_LIVEDUMP", 0x1000007E: "SYSTEM_THREAD_EXCEPTION_NOT_HANDLED_M",
	0x1000008E: "KERNEL_MODE_EXCEPTION_NOT_HANDLED_M", 0xC000021A: "STATUS_SYSTEM_PROCESS_TERMINATED",
}

// explainWindow : période où chercher la cause d'un arrêt survenu après lastSeen (Windows l'écrit au
// démarrage suivant, parfois quelques minutes après).
func explainWindow(lastSeen time.Time) time.Time { return lastSeen.Add(-time.Minute) }
