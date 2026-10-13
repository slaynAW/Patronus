package share

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

func TestAllowedRight(t *testing.T) {
	for _, c := range []struct {
		right                    Right
		identity, pinned, hasKey bool
		want                     Right
	}{
		{RightWake, true, true, true, RightWake},
		{RightView, true, true, false, RightView},
		{RightView, false, true, true, RightWake},
		{RightView, true, false, true, RightWake},
		{RightFull, true, true, false, RightFull},
		{RightFull, false, true, true, RightFull},
		{RightFull, true, false, true, RightFull},
		{RightFull, false, true, false, RightWake},
		{RightFull, true, false, false, RightWake},
	} {
		if got := AllowedRight(c.right, c.identity, c.pinned, c.hasKey); got != c.want {
			t.Errorf("%+v : %s", c, got)
		}
	}
}

func TestNeedsKey(t *testing.T) {
	o := &Owner{People: []Person{
		{Device: "a", Identity: testIdentity("a"), Rights: map[string]Right{"c": RightFull, "d": RightView}},
		{Device: "b", Rights: map[string]Right{"d": RightFull, "e": RightWake}},
	}}
	for _, c := range []struct {
		device string
		pinned bool
		want   bool
	}{
		{"c", true, false}, {"c", false, true}, {"d", true, true}, {"e", true, false}, {"z", true, false},
	} {
		if got := o.NeedsKey(c.device, c.pinned); got != c.want {
			t.Errorf("%+v : %t", c, got)
		}
	}
}

// v2Plan est un scénario de PlanAgent puis ApplyAgent sur un PC, commun avec Kotlin.
type v2Plan struct {
	Name        string       `json:"name"`
	Device      string       `json:"device"`
	Now         int64        `json:"now"`
	People      []Person     `json:"people"`
	Pairings    []Pairing    `json:"pairings"`
	Revocations []Revocation `json:"revocations"`
	View        struct {
		Devices []protocol2.DeviceInfo `json:"devices"`
		Invites []protocol2.InviteInfo `json:"invites"`
	} `json:"view"`
	// Work et Poll : AgentWork avant le plan.
	Work bool       `json:"work"`
	Poll bool       `json:"poll"`
	Plan AgentPlan  `json:"plan"`
	Res  PlanResult `json:"result"`
	// AfterPairings, AfterRevocations : état après ApplyAgent ; puis AgentWork de nouveau.
	AfterPairings    []Pairing    `json:"afterPairings"`
	AfterRevocations []Revocation `json:"afterRevocations"`
	AfterWork        bool         `json:"afterWork"`
	AfterPoll        bool         `json:"afterPoll"`
}

func (p v2Plan) owner() *Owner {
	return &Owner{Name: "Hugo", People: slices.Clone(p.People), Pairings: slices.Clone(p.Pairings), Revocations: slices.Clone(p.Revocations)}
}

func (p v2Plan) view() AgentView { return AgentView{Devices: p.View.Devices, Invites: p.View.Invites} }

// v2Plans construit les scénarios : un PC avec des invitations à garder, à renouveler et à créer,
// des rôles à aligner, des retraits, un administrateur et cet appareil intouchables ; puis un PC
// où tout est fait (rien à faire, invitations à surveiller) ; puis un retrait d'un administrateur.
func v2Plans(t *testing.T) []v2Plan {
	now := v2Now.Unix()
	key := func(name string) string { return testShareKey(t, "plan "+name) }
	id := testIdentity
	person := func(name string, identity string, right Right) Person {
		return Person{Name: name, Device: key(name), Identity: identity, Added: 1_790_000_000_000, Rights: map[string]Right{"c": right}}
	}
	lea, paul, zoe := person("Léa", id("lea"), RightFull), person("Paul", id("paul"), RightView), person("Zoé", id("zoe"), RightFull)
	max, kim, tom := person("Max", id("max"), RightView), person("Kim", id("kim"), RightFull), person("Tom", "", RightFull)
	invited := func(p Person, invite, code, role string, expires int64) Pairing {
		return Pairing{Person: p.Device, Device: "c", Identity: p.Identity, Role: role, Invite: invite, Code: code, Expires: expires}
	}
	dev := func(identity, name, role string, self bool) protocol2.DeviceInfo {
		return protocol2.DeviceInfo{Device: identity, Name: name, Role: role, Added: 1_790_000_000, Via: "pair", Self: self}
	}
	inv := func(invite, identity, role string, expires int64) protocol2.InviteInfo {
		return protocol2.InviteInfo{ID: invite, Role: role, Name: "x", Expires: expires, Device: identity}
	}

	first := v2Plan{
		Name: "invitations gardée, renouvelée et créée ; rôle aligné ; retraits ; administrateurs intouchables", Device: "c", Now: now,
		People: []Person{lea, paul, zoe, max, kim, tom},
		Pairings: []Pairing{
			invited(lea, "AAAAAAAAAAA", "7KQ29MXP", "control", now+3*86400),
			invited(paul, "BBBBBBBBBBB", "9MXP7KQ2", "control", now+3*86400),
		},
		Revocations: []Revocation{{Device: "c", Identity: id("old")}, {Device: "c", Identity: id("gone")}, {Device: "c", Identity: id("self")},
			{Device: "d", Identity: id("other")}},
	}
	first.View.Devices = []protocol2.DeviceInfo{
		dev(id("self"), "PC de Hugo", "admin", true), dev(id("max"), "Max", "admin", false),
		dev(id("old"), "Ancien", "control", false), dev(id("kim"), "Kim", "wake", false),
	}
	first.View.Invites = []protocol2.InviteInfo{
		inv("AAAAAAAAAAA", id("lea"), "control", now+3*86400), inv("BBBBBBBBBBB", id("paul"), "control", now+3*86400),
		inv("CCCCCCCCCCC", id("old"), "control", now+600), inv("DDDDDDDDDDD", id("zoe"), "control", now+600),
		inv("EEEEEEEEEEE", "", "view", now+600),
	}
	first.Res = PlanResult{Changed: true, Created: map[string]protocol2.InviteInfo{
		id("paul"): {ID: "FFFFFFFFFFF", Code: "ab12-cd34", Role: "wake", Expires: now + 604800},
		id("zoe"):  {ID: "GGGGGGGGGGG", Code: "EF56GH78", Role: "control", Expires: now + 604800},
	}}

	// Tout est fait : l'invitation de Léa attend d'être utilisée, Paul est enregistré.
	done := v2Plan{Name: "rien à faire : invitation en attente, appareil enregistré", Device: "c", Now: now,
		People: []Person{lea, paul},
		Pairings: []Pairing{
			invited(lea, "AAAAAAAAAAA", "7KQ29MXP", "control", now+3*86400),
			{Person: paul.Device, Device: "c", Identity: paul.Identity, Role: "wake", Registered: true},
		},
		Revocations: []Revocation{},
	}
	done.View.Devices = []protocol2.DeviceInfo{dev(id("self"), "PC de Hugo", "admin", true), dev(id("paul"), "Paul", "wake", false)}
	done.View.Invites = []protocol2.InviteInfo{inv("AAAAAAAAAAA", id("lea"), "control", now+3*86400)}
	done.Res = PlanResult{Changed: true, Created: map[string]protocol2.InviteInfo{}}

	// L'invitation de Léa a servi ; celle de Paul expire bientôt ; une invitation créée pour une
	// personne retirée entre-temps part dans la file des retraits.
	used := v2Plan{Name: "invitation utilisée, invitation à renouveler, personne retirée pendant l'échange", Device: "c", Now: now,
		People: []Person{lea, paul},
		Pairings: []Pairing{
			invited(lea, "AAAAAAAAAAA", "7KQ29MXP", "control", now+3*86400),
			invited(paul, "BBBBBBBBBBB", "9MXP7KQ2", "wake", now+3600),
		},
		Revocations: []Revocation{},
	}
	used.View.Devices = []protocol2.DeviceInfo{dev(id("self"), "PC de Hugo", "admin", true), dev(id("lea"), "Léa", "control", false)}
	used.View.Invites = []protocol2.InviteInfo{inv("BBBBBBBBBBB", id("paul"), "wake", now+3600)}
	used.Res = PlanResult{Changed: true, Created: map[string]protocol2.InviteInfo{
		id("paul"): {ID: "HHHHHHHHHHH", Code: "JK90MN12", Role: "wake", Expires: now + 604800},
	}}

	plans := []v2Plan{first, done, used}
	for i := range plans {
		p := &plans[i]
		o := p.owner()
		p.Work, p.Poll = o.AgentWork(p.Device, time.Unix(p.Now, 0))
		p.Plan = o.PlanAgent(p.Device, p.view(), time.Unix(p.Now, 0))
		if i == 2 {
			// Paul est retiré pendant l'échange : son invitation, créée, doit être annulée.
			o.Withdraw(paul.Device)
		}
		o.ApplyAgent(p.Device, p.Plan, p.Res)
		p.AfterPairings, p.AfterRevocations = nonNil(o.Pairings), nonNil(o.Revocations)
		p.AfterWork, p.AfterPoll = o.AgentWork(p.Device, time.Unix(p.Now, 0))
	}
	return plans
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func TestPlanAgent(t *testing.T) {
	plans := v2Plans(t)
	id := testIdentity
	now := v2Now.Unix()

	first := plans[0]
	if !first.Work {
		t.Error("premier PC : travail attendu")
	}
	want := AgentPlan{
		Cancel:     []string{"CCCCCCCCCCC", "BBBBBBBBBBB", "DDDDDDDDDDD"},
		Revoke:     []string{id("old")},
		Roles:      []RoleChange{{Identity: id("kim"), Role: "control"}},
		Invite:     []InviteRequest{{Identity: id("paul"), Role: "wake", Name: "Paul"}, {Identity: id("zoe"), Role: "control", Name: "Zoé"}},
		Registered: []RoleChange{{Identity: id("max"), Role: "admin"}, {Identity: id("kim"), Role: "control"}},
		Done:       []Revocation{{Device: "c", Identity: id("old")}, {Device: "c", Identity: id("gone")}, {Device: "c", Identity: id("self")}},
		Skipped:    []string{id("self"), id("max")},
	}
	if !reflect.DeepEqual(first.Plan, want) {
		t.Errorf("plan :\n%+v\nattendu\n%+v", first.Plan, want)
	}
	if !reflect.DeepEqual(first.AfterRevocations, []Revocation{{Device: "d", Identity: id("other")}}) {
		t.Errorf("retraits restants : %v", first.AfterRevocations)
	}
	byPerson := map[string]Pairing{}
	for _, x := range first.AfterPairings {
		byPerson[x.Person] = x
	}
	people := map[string]Person{}
	for _, p := range first.People {
		people[p.Name] = p
	}
	check := func(name string, want Pairing) {
		t.Helper()
		p := people[name]
		want.Person, want.Device, want.Identity = p.Device, "c", p.Identity
		if got := byPerson[p.Device]; got != want {
			t.Errorf("%s : %v, attendu %v", name, got, want)
		}
	}
	check("Léa", Pairing{Role: "control", Invite: "AAAAAAAAAAA", Code: "7KQ29MXP", Expires: now + 3*86400})
	check("Paul", Pairing{Role: "wake", Invite: "FFFFFFFFFFF", Code: "AB12CD34", Expires: now + 604800})
	check("Zoé", Pairing{Role: "control", Invite: "GGGGGGGGGGG", Code: "EF56GH78", Expires: now + 604800})
	check("Max", Pairing{Role: "admin", Registered: true})
	check("Kim", Pairing{Role: "control", Registered: true})
	if _, ok := byPerson[people["Tom"].Device]; ok || len(first.AfterPairings) != 5 {
		t.Errorf("appairages : %v", first.AfterPairings)
	}
	if first.AfterWork || !first.AfterPoll {
		t.Errorf("après le plan : travail %t, surveillance %t", first.AfterWork, first.AfterPoll)
	}

	done := plans[1]
	if done.Work || !done.Poll || !done.Plan.Empty() || len(done.Plan.Registered) != 1 {
		t.Errorf("rien à faire : %+v %t %t", done.Plan, done.Work, done.Poll)
	}

	used := plans[2]
	if !used.Work || !reflect.DeepEqual(used.Plan.Invite, []InviteRequest{{Identity: id("paul"), Role: "wake", Name: "Paul"}}) ||
		!reflect.DeepEqual(used.Plan.Cancel, []string{"BBBBBBBBBBB"}) {
		t.Errorf("invitation utilisée : %+v", used.Plan)
	}
	lea := used.People[0]
	if !reflect.DeepEqual(used.AfterPairings, []Pairing{{Person: lea.Device, Device: "c", Identity: lea.Identity, Role: "control", Registered: true}}) {
		t.Errorf("après : %v", used.AfterPairings)
	}
	if !reflect.DeepEqual(used.AfterRevocations, []Revocation{{Device: "c", Identity: id("paul")}}) || !used.AfterWork {
		t.Errorf("invitation orpheline : %v", used.AfterRevocations)
	}
}

// Une invitation au code illisible n'est pas enregistrée ; un échec avant les invitations laisse
// la file des retraits intacte.
func TestApplyAgentFailures(t *testing.T) {
	p := v2Plans(t)[0]
	o := p.owner()
	plan := o.PlanAgent(p.Device, p.view(), time.Unix(p.Now, 0))
	if o.ApplyAgent(p.Device, plan, PlanResult{}) {
		t.Error("échec : état modifié")
	}
	if !reflect.DeepEqual(o.Revocations, p.Revocations) {
		t.Errorf("retraits : %v", o.Revocations)
	}
	bad := PlanResult{Changed: true, Created: map[string]protocol2.InviteInfo{testIdentity("zoe"): {ID: "X", Code: "???", Role: "control"}}}
	o.ApplyAgent(p.Device, plan, bad)
	for _, x := range o.Pairings {
		if x.Identity == testIdentity("zoe") {
			t.Errorf("invitation illisible enregistrée : %v", x)
		}
	}
}
