package share

import (
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

func testDevices(t *testing.T) []model.Device {
	agent := &model.AgentSettings{Port: 9770, Key: "kY5jjMI6cQpU1gqHSiok1wAPlr6OUVgey66jYkJw5DI"}
	return []model.Device{
		{ID: "a", Name: "PC streaming", MAC: mustMAC(t, "AA:BB:CC:DD:EE:01"), Host: "192.168.1.20", WolPort: 9, ProbePorts: []int{3389}, Agent: agent},
		{ID: "b", Name: "Bureau", MAC: mustMAC(t, "AA:BB:CC:DD:EE:02"), Host: "192.168.1.21", WolPort: 9, ProbePorts: []int{3389}, Agent: agent},
		{ID: "c", Name: "NAS", MAC: mustMAC(t, "AA:BB:CC:DD:EE:03"), WolPort: 9, ProbePorts: []int{}},
	}
}

func TestOwnerPublishAndRecipientSync(t *testing.T) {
	ownerKey, _ := NewOwnerKey()
	deviceKey, _ := NewDeviceKey()
	devicePub := DevicePublic(deviceKey)
	owner := &Owner{Key: EncodePrivate(OwnerPrivateBytes(ownerKey)), Name: "Hugo", People: []Person{
		{Name: "Léa", Device: devicePub, Rights: map[string]Right{"a": RightWake, "b": RightFull}},
	}}
	devices := testDevices(t)
	now := time.UnixMilli(1_790_600_000_000)

	pub, err := owner.Prepare(devices, now, false)
	if err != nil || len(pub.Files) != 1 || pub.Files[FileName(devicePub)] == nil {
		t.Fatalf("publication : %+v %v", pub, err)
	}
	owner.Commit(pub)
	// Rien de changé : rien à publier.
	if again, _ := owner.Prepare(devices, now.Add(time.Minute), false); !again.Empty() {
		t.Errorf("republication inutile : %v", again.Files)
	}

	// Réception : accès accordé, PC dans l'ordre, sans clé d'agent pour « démarrer ».
	access := Access{Owner: OwnerPublic(ownerKey), User: "slaynAW", Gist: "0123456789abcdef0123456789abcdef"}
	key := func() (string, error) { return EncodePrivate(deviceKey.Bytes()), nil }
	files := map[string]string{FileName(devicePub): *pub.Files[FileName(devicePub)]}
	res, err := access.Apply(files, devicePub, key, now)
	if err != nil || res != Granted || !access.Active || len(access.Devices) != 2 {
		t.Fatalf("réception : %v %v %+v", res, err, access)
	}
	if access.Devices[0].Name != "PC streaming" || access.Devices[0].Agent != nil || access.Devices[1].Agent == nil {
		t.Errorf("droits : %+v", access.Devices)
	}
	if access.OwnerName != "Hugo" {
		t.Errorf("nom : %q", access.OwnerName)
	}
	// Même fichier relu : rien de nouveau.
	if res, err := access.Apply(files, devicePub, key, now); err != nil || res != Unchanged {
		t.Errorf("relecture : %v %v", res, err)
	}

	// Changement d'adresse d'un PC partagé : republication, mise à jour reçue.
	devices[0].Host = "192.168.1.30"
	pub2, _ := owner.Prepare(devices, now.Add(time.Minute), false)
	if len(pub2.Files) != 1 {
		t.Fatalf("changement non republié : %v", pub2.Files)
	}
	owner.Commit(pub2)
	files[FileName(devicePub)] = *pub2.Files[FileName(devicePub)]
	if res, err := access.Apply(files, devicePub, key, now); err != nil || res != Updated || access.Devices[0].Host != "192.168.1.30" {
		t.Errorf("mise à jour : %v %v %+v", res, err, access.Devices)
	}
	// L'ancienne version est refusée (retour en arrière).
	old := map[string]string{FileName(devicePub): *pub.Files[FileName(devicePub)]}
	if _, err := access.Apply(old, devicePub, key, now); err == nil {
		t.Error("ancienne version acceptée")
	}
	// Changement d'un PC non partagé : rien à publier.
	devices[2].Name = "NAS salon"
	if p, _ := owner.Prepare(devices, now.Add(2*time.Minute), false); !p.Empty() {
		t.Error("PC non partagé republié")
	}

	// Retrait : fichier supprimé, accès retiré côté réception.
	if !owner.Withdraw(devicePub) || len(owner.People) != 0 {
		t.Fatal("retrait")
	}
	pub3, _ := owner.Prepare(devices, now.Add(3*time.Minute), false)
	if f, ok := pub3.Files[FileName(devicePub)]; !ok || f != nil {
		t.Fatalf("suppression non publiée : %v", pub3.Files)
	}
	owner.Commit(pub3)
	if len(owner.Withdrawn) != 0 {
		t.Error("retrait non soldé")
	}
	if res, err := access.Apply(map[string]string{}, devicePub, key, now); err != nil || res != Withdrawn || access.Active || !access.Removed || len(access.Devices) != 0 {
		t.Errorf("retrait reçu : %v %v %+v", res, err, access)
	}
}

func TestPendingAccessStaysPending(t *testing.T) {
	a := Access{Owner: "x"}
	res, err := a.Apply(nil, "device", func() (string, error) { return "", nil }, time.Now())
	if err != nil || res != Unchanged || a.Active || a.Removed {
		t.Errorf("demande en attente : %v %v %+v", res, err, a)
	}
}

func TestSharedIDStable(t *testing.T) {
	id := SharedID("owner", "a")
	if id != SharedID("owner", "a") || id == SharedID("owner2", "a") || len(id) != 34 {
		t.Errorf("identifiant : %s", id)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Load()
	if err != nil || st.Owner != nil || len(st.Received) != 0 {
		t.Fatalf("état initial : %+v %v", st, err)
	}
	st.Owner = &Owner{Name: "Hugo", People: []Person{{Name: "Léa", Rights: map[string]Right{"a": RightWake}}}}
	st.Received = append(st.Received, Access{Owner: "k", OwnerName: "Paul"})
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	back, err := store.Load()
	if err != nil || back.Owner.Name != "Hugo" || back.Owner.People[0].Rights["a"] != RightWake || back.Received[0].OwnerName != "Paul" {
		t.Errorf("relu : %+v %v", back, err)
	}
}
