package eventlog

import (
	"strings"
	"testing"
	"time"
)

// Événement réel (Kernel-Power 41 après un écran bleu), tel que Windows le rend en XML.
const kernelPower41 = `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-Kernel-Power' Guid='{331c3b3a-2005-44c2-ac5e-77220c37d6b4}'/><EventID>41</EventID><Version>9</Version><Level>1</Level><Task>63</Task><Opcode>0</Opcode><Keywords>0x8000400000000002</Keywords><TimeCreated SystemTime='2026-10-03T07:12:34.5678901Z'/><EventRecordID>12345</EventRecordID><Correlation/><Execution ProcessID='4' ThreadID='8'/><Channel>System</Channel><Computer>BUREAU</Computer><Security UserID='S-1-5-18'/></System><EventData><Data Name='BugcheckCode'>126</Data><Data Name='BugcheckParameter1'>0xffffffffc0000005</Data><Data Name='SleepInProgress'>0</Data><Data Name='PowerButtonTimestamp'>0</Data><Data Name='LongPowerButtonPressDetected'>false</Data></EventData></Event>`

func TestParse(t *testing.T) {
	e, err := Parse(kernelPower41)
	if err != nil {
		t.Fatal(err)
	}
	if e.Provider != "Microsoft-Windows-Kernel-Power" || e.ID != 41 || e.Data["BugcheckCode"] != "126" || e.Data["LongPowerButtonPressDetected"] != "false" {
		t.Errorf("événement : %+v", e)
	}
	if want := time.Date(2026, 10, 3, 7, 12, 34, 567890100, time.UTC); !e.Time.Equal(want) {
		t.Errorf("heure : %v", e.Time)
	}
	// Données sans nom (param1, param2…) : BugCheck 1001.
	e, err = Parse(`<Event><System><Provider Name='Microsoft-Windows-WER-SystemErrorReporting'/><EventID Qualifiers='16384'>1001</EventID><TimeCreated SystemTime='2026-10-03T07:13:00Z'/></System><EventData><Data>0x0000007e (0xffffffffc0000005, 0, 0, 0)</Data><Data>C:\WINDOWS\MEMORY.DMP</Data></EventData></Event>`)
	if err != nil || e.ID != 1001 || !strings.HasPrefix(e.Data["param1"], "0x0000007e") {
		t.Errorf("BugCheck : %+v %v", e, err)
	}
	if _, err := Parse("<Event><System><EventID>x</EventID></System></Event>"); err == nil {
		t.Error("événement invalide accepté")
	}
}

func TestXPath(t *testing.T) {
	q := XPath([]Selector{{Provider: "disk", IDs: []int{7, 11}}, {Provider: "Microsoft-Windows-Ntfs", IDs: []int{55}}}, time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC))
	want := "*[System[((Provider[@Name='disk'] and (EventID=7 or EventID=11)) or (Provider[@Name='Microsoft-Windows-Ntfs'] and (EventID=55))) and TimeCreated[@SystemTime>='2026-09-03T10:00:00.000Z']]]"
	if q != want {
		t.Errorf("requête :\n%s\n%s", q, want)
	}
}
