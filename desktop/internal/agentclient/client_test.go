package agentclient

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient/agenttest"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

var (
	keyText, _ = protocol.NewKey()
	keyBytes   = model.DecodeAgentKey(keyText)
)

func start(t *testing.T, b agenttest.Behavior) (*agenttest.Server, model.AgentSettings) {
	t.Helper()
	s, err := agenttest.Start(keyBytes, b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, model.AgentSettings{Port: s.Port(), Key: keyText}
}

func TestStatusAndPower(t *testing.T) {
	s, settings := start(t, agenttest.Normal)
	c := New()
	st, err := c.Status(context.Background(), "127.0.0.1", settings)
	if err != nil || st.Hostname != "PC-TEST" || st.Uptime != 3600 || st.OS != "windows" {
		t.Fatalf("statut : %+v %v", st, err)
	}
	msg, err := c.Power(context.Background(), "127.0.0.1", settings, Shutdown, 0, true)
	if err != nil || msg != "OK" {
		t.Fatalf("extinction : %q %v", msg, err)
	}
	if !reflect.DeepEqual(s.Commands(), []string{"status", "shutdown"}) {
		t.Errorf("commandes : %v", s.Commands())
	}
}

func TestStatusTemperatures(t *testing.T) {
	s, settings := start(t, agenttest.Normal)
	st, err := New().Status(context.Background(), "127.0.0.1", settings)
	if err != nil || st.Temperatures != nil {
		t.Fatalf("agent sans températures : %+v %v", st.Temperatures, err)
	}
	cpu, gpu := 54.5, 61.0
	want := &protocol.Temperatures{CPU: &cpu, GPU: &gpu, GPUName: "NVIDIA GeForce RTX 4070"}
	s.SetTemperatures(want)
	st, err = New().Status(context.Background(), "127.0.0.1", settings)
	if err != nil || !reflect.DeepEqual(st.Temperatures, want) {
		t.Fatalf("températures : %+v %v", st.Temperatures, err)
	}
	// Processeur illisible : l'indication accompagne la seule carte graphique.
	s.SetTemperatures(&protocol.Temperatures{GPU: &gpu, CPUHint: protocol.CPUHintLHM})
	st, err = New().Status(context.Background(), "127.0.0.1", settings)
	if err != nil || st.Temperatures.CPU != nil || st.Temperatures.CPUHint != protocol.CPUHintLHM {
		t.Fatalf("indication : %+v %v", st.Temperatures, err)
	}
}

func TestStatusDisks(t *testing.T) {
	s, settings := start(t, agenttest.Normal)
	if st, err := New().Status(context.Background(), "127.0.0.1", settings); err != nil || st.Disks != nil {
		t.Fatalf("agent sans disques : %+v %v", st.Disks, err)
	}
	wear := 3
	want := &protocol.Disks{Volumes: []protocol.Volume{{Mount: "C:", Label: "Windows", FS: "NTFS", Total: 1 << 40, Free: 1 << 38}},
		Drives: []protocol.Drive{{Name: "Samsung SSD 980", Media: protocol.DriveSSD, Bus: "NVMe", Health: protocol.DriveHealthy, Wear: &wear}}, Errors: 2, LastError: 1791000000}
	s.SetDisks(want)
	st, err := New().Status(context.Background(), "127.0.0.1", settings)
	if err != nil || !reflect.DeepEqual(st.Disks, want) {
		t.Fatalf("disques : %+v %v", st.Disks, err)
	}
}

func TestSpecs(t *testing.T) {
	s, settings := start(t, agenttest.Normal)
	// Agent antérieur à 1.10.0 : commande refusée.
	if _, err := New().Specs(context.Background(), "127.0.0.1", settings); CodeOf(err) != Rejected {
		t.Fatalf("agent ancien : %v", err)
	}
	want := &protocol.Specs{
		Model:  "Dell XPS 15 9520",
		CPU:    &protocol.SpecsCPU{Name: "Intel Core i7-12700H", Cores: 14, Threads: 20, MHz: 2300},
		Memory: &protocol.SpecsMemory{Total: 32 << 30, Slots: 2, Modules: []protocol.SpecsModule{{Slot: "DIMM A", Size: 16 << 30, Type: "DDR5", MTs: 4800, Maker: "SK hynix"}}},
		GPUs:   []protocol.SpecsGPU{{Name: "NVIDIA GeForce RTX 3050 Ti Laptop GPU", VRAM: 4 << 30, Driver: "32.0.15.6094"}, {Name: "Intel Iris Xe Graphics", Integrated: true}},
		Board:  &protocol.SpecsBoard{Maker: "Dell", Model: "0RH1JY", BIOS: "1.22.0", BIOSDate: "2024-05-14"},
		OS:     &protocol.SpecsOS{Name: "Windows 11 Pro", Version: "24H2, build 26100.2314"},
	}
	s.SetSpecs(want)
	got, err := New().Specs(context.Background(), "127.0.0.1", settings)
	if err != nil || !reflect.DeepEqual(&got, want) {
		t.Fatalf("fiche : %+v %v", got, err)
	}
}

func TestHistory(t *testing.T) {
	s, settings := start(t, agenttest.Normal)
	// Agent trop ancien : la commande est refusée.
	if _, err := New().History(context.Background(), "127.0.0.1", settings); CodeOf(err) != Rejected {
		t.Fatalf("agent ancien : %v", err)
	}
	// Journal volumineux (au-delà de la taille d'un message ordinaire).
	var events []protocol.HistoryEvent
	for i := range 1500 {
		events = append(events, protocol.HistoryEvent{T: int64(1_790_000_000 + i*60), K: protocol.HistoryCommand, A: "sleep", C: "192.168.100.100"})
	}
	s.SetHistory(&protocol.History{From: 1_789_000_000, Events: events})
	h, err := New().History(context.Background(), "127.0.0.1", settings)
	if err != nil || h.From != 1_789_000_000 || len(h.Events) != 1500 || h.Events[1499].C != "192.168.100.100" {
		t.Fatalf("journal : %d évènements, %v", len(h.Events), err)
	}
}

func TestWrongKey(t *testing.T) {
	s, settings := start(t, agenttest.Normal)
	other, _ := protocol.NewKey()
	settings.Key = other
	_, err := New().Status(context.Background(), "127.0.0.1", settings)
	if CodeOf(err) != Unauthorized || !HostAnswered(err) || len(s.Commands()) != 0 {
		t.Errorf("mauvaise clé : %v", err)
	}
}

func TestFailures(t *testing.T) {
	cases := map[agenttest.Behavior]Code{
		agenttest.BadResponseMAC: Protocol,
		agenttest.Reject:         Rejected,
		agenttest.RateLimited:    RateLimited,
		agenttest.WrongProto:     Protocol,
	}
	for behavior, want := range cases {
		_, settings := start(t, behavior)
		_, err := New().Power(context.Background(), "127.0.0.1", settings, Reboot, 0, false)
		if CodeOf(err) != want || !HostAnswered(err) {
			t.Errorf("comportement %d : %v (attendu %s)", behavior, err, want)
		}
	}
}

func TestClosedPortSilentAgentAndNoKey(t *testing.T) {
	l, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	_, err := New().Status(context.Background(), "127.0.0.1", model.AgentSettings{Port: port, Key: keyText})
	if CodeOf(err) != Refused || !HostAnswered(err) {
		t.Errorf("port fermé : %v", err)
	}

	_, settings := start(t, agenttest.Hang)
	c := &Client{ConnectTimeout: time.Second, ReadTimeout: 300 * time.Millisecond}
	_, err = c.Status(context.Background(), "127.0.0.1", settings)
	if CodeOf(err) != Protocol || !HostAnswered(err) {
		t.Errorf("agent muet : %v", err)
	}

	_, err = New().Status(context.Background(), "127.0.0.1", model.AgentSettings{Port: 9770, Key: ""})
	if CodeOf(err) != NoKey || HostAnswered(err) {
		t.Errorf("sans clé : %v", err)
	}
	_, err = New().Status(context.Background(), "hote-inexistant.invalid", model.AgentSettings{Port: 9770, Key: keyText})
	if c := CodeOf(err); c != UnknownHost && c != Unreachable {
		t.Errorf("hôte inconnu : %v", err)
	}
}

func TestCancellationIsImmediate(t *testing.T) {
	_, settings := start(t, agenttest.Hang)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	begin := time.Now()
	_, err := New().Status(ctx, "127.0.0.1", settings)
	if err == nil || time.Since(begin) > time.Second {
		t.Errorf("annulation lente : %v en %v", err, time.Since(begin))
	}
}

// Test de bout en bout contre le VRAI agent Go, lancé en mode simulation par la CI.
func TestEndToEndWithRealAgent(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("WOL_E2E_PORT"))
	if err != nil {
		t.Skip("WOL_E2E_PORT non défini (test lancé par la CI)")
	}
	settings := model.AgentSettings{Port: port, Key: os.Getenv("WOL_E2E_KEY")}
	st, err := New().Status(context.Background(), "127.0.0.1", settings)
	if err != nil || st.Hostname == "" {
		t.Fatalf("statut : %+v %v", st, err)
	}
	if _, err := New().Power(context.Background(), "127.0.0.1", settings, Shutdown, 0, false); err != nil {
		t.Fatalf("extinction simulée : %v", err)
	}
	h, err := New().History(context.Background(), "127.0.0.1", settings)
	if err != nil || len(h.Events) == 0 {
		t.Fatalf("journal de l'agent réel : %+v %v", h, err)
	}
	var sawBoot, sawCmd bool
	for _, e := range h.Events {
		sawBoot = sawBoot || e.K == protocol.HistoryBoot
		sawCmd = sawCmd || (e.K == protocol.HistoryCommand && e.A == "shutdown")
	}
	if !sawBoot || !sawCmd {
		t.Errorf("démarrage ou commande absents du journal : %+v", h.Events)
	}
	// Fiche du PC lue par le vrai agent (« busy » tant que sa première lecture n'est pas finie).
	var specs protocol.Specs
	for try := 0; ; try++ {
		specs, err = New().Specs(context.Background(), "127.0.0.1", settings)
		if CodeOf(err) != Rejected || try == 10 {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil || specs.CPU == nil || specs.Memory == nil || specs.Memory.Total == 0 || specs.OS == nil {
		t.Fatalf("fiche de l'agent réel : %+v %v", specs, err)
	}
	raw, _ := json.Marshal(specs)
	t.Logf("fiche de l'agent réel : %s", raw)
	other, _ := protocol.NewKey()
	if _, err := New().Status(context.Background(), "127.0.0.1", model.AgentSettings{Port: port, Key: other}); CodeOf(err) != Unauthorized {
		t.Errorf("mauvaise clé : %v", err)
	}
}
