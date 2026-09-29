package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/diagnostic"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

const diagPassword = "mot de passe du rapport"

// useJournal remplace le journal par défaut par un journal temporaire, le temps du test.
func useJournal(t *testing.T) {
	t.Helper()
	j, err := diag.Open(filepath.Join(t.TempDir(), "diagnostics"))
	if err != nil {
		t.Fatal(err)
	}
	diag.SetDefault(j)
	t.Cleanup(func() { diag.SetDefault(nil) })
}

// exportReport exporte le rapport d'un service et le déchiffre.
func exportReport(t *testing.T, s *Service) string {
	t.Helper()
	platform := s.platform.(*fakePlatform)
	res, err := callJSON(t, s, "exportDiagnostic", `{"password":"`+diagPassword+`"}`)
	if err != nil || res["ok"] != true {
		t.Fatalf("export du rapport : %v %v", res, err)
	}
	if !strings.HasPrefix(platform.savedName, "patronus-diagnostic-") || !strings.HasSuffix(platform.savedName, ".diag") {
		t.Errorf("nom du fichier : %s", platform.savedName)
	}
	env, report, err := diagnostic.Open(platform.saved, diagPassword)
	if err != nil {
		t.Fatal(err)
	}
	if env.App != "Patronus Windows test" {
		t.Errorf("programme : %q", env.App)
	}
	if _, _, err := diagnostic.Open(platform.saved, "autre mot de passe"); err == nil {
		t.Error("rapport ouvert avec un autre mot de passe")
	}
	return report
}

func TestDiagnosticReportOfSharing(t *testing.T) {
	useJournal(t)
	_, srv := newGistServer(t)
	ctx := context.Background()
	agentKey := "kY5jjMI6cQpU1gqHSiok1wAPlr6OUVgey66jYkJw5DI"
	secureOn := "01:23:45:67:89:AB"
	streaming := testDevice(t, "dev-1", "PC streaming", "192.168.1.20", agentKey)
	streaming.SecureOnPassword = &secureOn
	owner := newShareService(t, srv, streaming, testDevice(t, "dev-2", "Bureau", "192.168.1.21", agentKey))
	guest := newShareService(t, srv)

	// Partage complet : Hugo invite Léa, qui reçoit le PC streaming.
	call(t, owner, "shareLogin", map[string]any{"name": "Hugo"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (owner.State().Share.Owner == nil || !owner.State().Share.Owner.Connected) {
		time.Sleep(10 * time.Millisecond)
	}
	link := call(t, owner, "shareInvite", nil)["link"].(string)
	reqLink := call(t, guest, "shareRequest", map[string]any{"text": link, "name": "Léa"})["link"].(string)
	read := call(t, owner, "shareReadRequest", map[string]any{"text": reqLink})
	call(t, owner, "shareGrant", map[string]any{"device": read["device"], "name": "Léa", "rights": map[string]string{"dev-1": "wake"}})
	if err := owner.publishShares(ctx); err != nil {
		t.Fatal(err)
	}
	guest.syncAccess(ctx, guest.State().Share.Received[0].Owner)

	ownerReport := exportReport(t, owner)
	guestReport := exportReport(t, guest)

	// Jamais de secret : clé d'agent, mot de passe SecureOn, clés privées, jeton, mot de passe du rapport.
	owner.sharing.mu.Lock()
	ownerPriv := owner.sharing.state.Owner.Key
	owner.sharing.mu.Unlock()
	guest.sharing.mu.Lock()
	devicePriv := guest.sharing.state.DeviceKey
	guest.sharing.mu.Unlock()
	for _, secret := range []string{agentKey, secureOn, ownerPriv, devicePriv, `"tok"`, "Bearer", diagPassword} {
		if strings.Contains(ownerReport, secret) || strings.Contains(guestReport, secret) {
			t.Errorf("secret dans le rapport : %q", secret)
		}
	}

	for _, want := range []string{
		"Application : Patronus Windows test",
		"« PC streaming » [dev-1]",
		"SecureOn oui",
		"agent port 9770, clé présente",
		"Je partage mes PC : oui, nom « Hugo », clé k:",
		"compte @hugo",
		"jeton présent",
		"1 personne(s) autorisée(s) :",
		"publiée oui : PC streaming = wake",
		"I partage : connexion GitHub réussie : compte @hugo",
		"I partage : autorisation de « Léa »",
		"I partage : publication réussie",
	} {
		if !strings.Contains(ownerReport, want) {
			t.Errorf("rapport de Hugo : %q manquant", want)
		}
	}
	for _, want := range []string{
		"PC partagés avec moi : 1",
		"- de « Hugo » k:",
		"actif true",
		"1 PC reçus d'autres personnes :",
		"partagé par « Hugo »",
	} {
		if !strings.Contains(guestReport, want) {
			t.Errorf("rapport de Léa : %q manquant", want)
		}
	}

	// La même empreinte identifie l'appareil de Léa dans les deux rapports.
	guestKey := regexp.MustCompile(`Clé de réception de cet appareil : (k:[0-9a-f]{8})`).FindStringSubmatch(guestReport)
	if guestKey == nil || !strings.Contains(ownerReport, "« Léa » "+guestKey[1]) {
		t.Errorf("empreinte de l'appareil de Léa introuvable chez Hugo : %v", guestKey)
	}
}

// panicPlatform plante à l'enregistrement d'un fichier (erreur imprévue dans une action).
type panicPlatform struct{ *fakePlatform }

func (panicPlatform) SaveFile(string, []byte) (string, error) { panic("boum") }

func TestCallRecoversFromPanic(t *testing.T) {
	useJournal(t)
	store, _ := config.NewStore(t.TempDir())
	s := New(Options{
		Version: "test", Store: store, Platform: panicPlatform{&fakePlatform{}},
		Prober:   status.ProberFunc(func(context.Context, model.Device) status.ProbeResult { return status.ProbeResult{} }),
		NetState: func() (netstate.State, error) { return netstate.State{}, nil },
	})
	_, err := s.Call("exportConfig", json.RawMessage(`{"withSecrets":false}`))
	if err == nil || !strings.Contains(err.Error(), "Erreur imprévue (boum)") {
		t.Fatalf("panique non rattrapée : %v", err)
	}
	// L'application continue de répondre.
	if _, err := s.Call("getState", nil); err != nil {
		t.Fatal(err)
	}
	records := strings.Join(diag.Records(), "\n")
	if !strings.Contains(records, "E interface : « exportConfig » (sans secret) : erreur imprévue : boum") ||
		!strings.Contains(records, "diagnostic_test.go") {
		t.Errorf("panique absente du journal (avec sa trace) :\n%s", records)
	}
}

func TestDiagnosticReportUnreadableShare(t *testing.T) {
	useJournal(t)
	_, srv := newGistServer(t)
	dir := t.TempDir()
	name := "share.json" // en clair hors Windows (développement et tests)
	if runtime.GOOS == "windows" {
		name = "share.dat"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("abîmé"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, _ := config.NewStore(dir)
	shareStore, _ := share.NewStore(dir)
	s := New(Options{
		Version: "test", Store: store, Platform: &fakePlatform{},
		Prober:   status.ProberFunc(func(context.Context, model.Device) status.ProbeResult { return status.ProbeResult{} }),
		NetState: func() (netstate.State, error) { return netstate.State{}, nil },
		Share:    &ShareOptions{Store: shareStore, GitHub: &share.GitHub{API: srv.URL, Web: srv.URL, ClientID: "client", Client: srv.Client()}},
	})
	if e := s.State().Share.Error; !strings.Contains(e, "partage illisible, mis de côté") {
		t.Fatalf("erreur de chargement non signalée à l'interface : %q", e)
	}
	report := exportReport(t, s)
	for _, want := range []string{
		"état du partage lu au démarrage : NON, partage illisible, mis de côté",
		name + ".illisible-",
		"← fichier illisible mis de côté",
		"E données : partage : partage illisible, mis de côté",
		"ÉTAT ILLISIBLE",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("rapport : %q manquant\n%s", want, report)
		}
	}
}

func TestExportDiagnosticRejectsShortPassword(t *testing.T) {
	s, platform := newService(t)
	if _, err := callJSON(t, s, "exportDiagnostic", `{"password":"court"}`); err == nil || platform.saved != nil {
		t.Errorf("mot de passe trop court accepté : %v", err)
	}
	platform.cancel = true
	if res, err := callJSON(t, s, "exportDiagnostic", `{"password":"`+diagPassword+`"}`); err != nil || res["cancelled"] != true {
		t.Errorf("annulation : %v %v", res, err)
	}
}

func TestLogClientIsLimited(t *testing.T) {
	useJournal(t)
	s, _ := newService(t)
	for i := 0; i < clientLogMax+10; i++ {
		if _, err := s.Call("logClient", json.RawMessage(`{"level":"error","text":"TypeError: x is undefined"}`)); err != nil {
			t.Fatal(err)
		}
	}
	records := diag.Records()
	count := 0
	for _, r := range records {
		if strings.Contains(r, "E interface : page : TypeError") {
			count++
		}
	}
	if count != clientLogMax || !strings.Contains(strings.Join(records, "\n"), "les suivants sont ignorés") {
		t.Errorf("%d erreurs de la page notées (%d attendues)", count, clientLogMax)
	}
}
