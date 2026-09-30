package netalert

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/netprofile"
	"github.com/slaynaw/wakeonlan/agent/internal/session"
)

type fake struct {
	profiles []netprofile.Profile
	fw       netprofile.Firewall
	answer   session.Answer
	setErr   error
	asked    []string
	set      []int
	notes    []string
	now      time.Time
}

func (f *fake) checker(t *testing.T, dir string) *Checker {
	return &Checker{
		Interface:  func() string { return "Ethernet" },
		Profiles:   func() ([]netprofile.Profile, error) { return f.profiles, nil },
		Firewall:   func() (netprofile.Firewall, error) { return f.fw, nil },
		Ask:        func(m string) (session.Answer, error) { f.asked = append(f.asked, m); return f.answer, nil },
		Notify:     func(m string, _ bool) { f.notes = append(f.notes, m) },
		SetPrivate: func(i int) error { f.set = append(f.set, i); return f.setErr },
		StatePath:  filepath.Join(dir, "network-alert.json"),
		Logf:       t.Logf,
		Now:        func() time.Time { return f.now },
	}
}

var blocked = netprofile.Firewall{PublicEnabled: true, RuleFound: true}

func TestPublicNetworkAcceptedIsMadePrivate(t *testing.T) {
	f := &fake{
		profiles: []netprofile.Profile{
			{Name: "VPN", Interface: "Tailscale", Index: 20, Category: netprofile.Public},
			{Name: "Livebox-1A2B", Interface: "Ethernet", Index: 7, Category: netprofile.Public},
		},
		fw: blocked, answer: session.Yes, now: time.Unix(1_800_000_000, 0),
	}
	f.checker(t, t.TempDir()).Once()
	if len(f.asked) != 1 || !strings.Contains(f.asked[0], "« Livebox-1A2B »") {
		t.Fatalf("question : %q", f.asked)
	}
	if len(f.set) != 1 || f.set[0] != 7 {
		t.Errorf("réseau classé en Privé : %v (carte 7 attendue, pas le VPN)", f.set)
	}
	if len(f.notes) != 1 || !strings.Contains(f.notes[0], "désormais Privé") {
		t.Errorf("confirmation : %q", f.notes)
	}
}

func TestNoQuestionWhenNotBlocked(t *testing.T) {
	cases := map[string]*fake{
		"réseau Privé": {profiles: []netprofile.Profile{{Name: "Maison", Interface: "Ethernet", Category: netprofile.Private}}, fw: blocked},
		"pare-feu inactif": {profiles: []netprofile.Profile{{Name: "Maison", Interface: "Ethernet", Category: netprofile.Public}},
			fw: netprofile.Firewall{PublicEnabled: false}},
		"règle ouverte aux réseaux Publics": {profiles: []netprofile.Profile{{Name: "Maison", Interface: "Ethernet", Category: netprofile.Public}},
			fw: netprofile.Firewall{PublicEnabled: true, RuleFound: true, RuleAllowsPublic: true}},
		"seul le VPN est Public": {profiles: []netprofile.Profile{
			{Name: "Maison", Interface: "Ethernet", Category: netprofile.Private},
			{Name: "VPN", Interface: "Tailscale", Category: netprofile.Public},
		}, fw: blocked},
	}
	for name, f := range cases {
		f.checker(t, t.TempDir()).Once()
		if len(f.asked) != 0 {
			t.Errorf("%s : question posée", name)
		}
	}
}

func TestRuleMissingCountsAsBlocked(t *testing.T) {
	if !(netprofile.Firewall{PublicEnabled: true}).BlocksPublic() {
		t.Error("règle absente : blocage non détecté")
	}
}

func TestDeclinedIsNotAskedAgainBeforeReminder(t *testing.T) {
	dir := t.TempDir()
	f := &fake{profiles: []netprofile.Profile{{Name: "Maison", Interface: "Ethernet", Index: 3, Category: netprofile.Public}},
		fw: blocked, answer: session.No, now: time.Unix(1_800_000_000, 0)}
	f.checker(t, dir).Once()
	f.checker(t, dir).Once()
	if len(f.asked) != 1 || len(f.set) != 0 {
		t.Fatalf("après un refus : %d question(s), %v", len(f.asked), f.set)
	}
	f.now = f.now.Add(RemindAfter + time.Minute)
	f.answer = session.Yes
	f.checker(t, dir).Once()
	if len(f.asked) != 2 || len(f.set) != 1 {
		t.Errorf("rappel après %s : %d question(s), %v", RemindAfter, len(f.asked), f.set)
	}
}

func TestNobodyLoggedInAsksLater(t *testing.T) {
	dir := t.TempDir()
	f := &fake{profiles: []netprofile.Profile{{Name: "Maison", Interface: "Ethernet", Index: 3, Category: netprofile.Public}},
		fw: blocked, answer: session.NoUser, now: time.Unix(1_800_000_000, 0)}
	f.checker(t, dir).Once()
	f.answer = session.Yes
	f.checker(t, dir).Once()
	if len(f.asked) != 2 || len(f.set) != 1 {
		t.Errorf("question non reposée après l'ouverture de session : %d, %v", len(f.asked), f.set)
	}
}

func TestSetPrivateFailureExplainsManualFix(t *testing.T) {
	f := &fake{profiles: []netprofile.Profile{{Name: "Maison", Interface: "Ethernet", Index: 3, Category: netprofile.Public}},
		fw: blocked, answer: session.Yes, setErr: errors.New("accès refusé"), now: time.Unix(1_800_000_000, 0)}
	f.checker(t, t.TempDir()).Once()
	if len(f.notes) != 1 || !strings.Contains(f.notes[0], "Type de profil réseau : Privé") {
		t.Errorf("explication : %q", f.notes)
	}
}
