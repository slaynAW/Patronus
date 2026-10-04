package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type vector struct {
	Name         string `json:"name"`
	Key          string `json:"key"`
	Nonce        string `json:"nonce"`
	CNonce       string `json:"cnonce"`
	RequestBody  string `json:"requestBody"`
	RequestMac   string `json:"requestMac"`
	ResponseBody string `json:"responseBody"`
	ResponseMac  string `json:"responseMac"`
}

// Les mêmes vecteurs sont vérifiés par l'application Android (module core) : compatibilité garantie.
func TestSharedVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "protocol", "test-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Protocol string   `json:"protocol"`
		Vectors  []vector `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Protocol != Proto || len(doc.Vectors) == 0 {
		t.Fatalf("vecteurs inattendus : %q (%d)", doc.Protocol, len(doc.Vectors))
	}
	for _, v := range doc.Vectors {
		key, err := DecodeKey(v.Key)
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		if got := RequestMAC(key, v.Nonce, v.CNonce, v.RequestBody); got != v.RequestMac {
			t.Errorf("%s: requête %s, attendu %s", v.Name, got, v.RequestMac)
		}
		if got := ResponseMAC(key, v.Nonce, v.CNonce, v.ResponseBody); got != v.ResponseMac {
			t.Errorf("%s: réponse %s, attendu %s", v.Name, got, v.ResponseMac)
		}
		if !MACEqual(v.RequestMac, v.RequestMac) || MACEqual(v.RequestMac, v.ResponseMac) {
			t.Errorf("%s: comparaison incorrecte", v.Name)
		}
	}
}

func TestKeysAndNonces(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 43 {
		t.Fatalf("longueur de clé %d", len(key))
	}
	if _, err := DecodeKey(key); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeKey("trop-court"); err == nil {
		t.Fatal("clé courte acceptée")
	}
	n, _ := NewNonce(ServerNonceBytes)
	if DecodedLen(n) != ServerNonceBytes || DecodedLen("!!") != -1 {
		t.Fatal("DecodedLen incorrect")
	}
	if MACEqual("pas du base64 !", "pas du base64 !") {
		t.Fatal("MAC invalide acceptée")
	}
}

func TestMergeEvent(t *testing.T) {
	lost := HistoryEvent{T: 10, K: HistoryLost}
	events, changed := MergeEvent(nil, lost)
	if !changed || len(events) != 1 {
		t.Fatal("ajout")
	}
	precise := HistoryEvent{T: 10, K: HistoryLost, R: LostBSOD, D: "0x7E"}
	if events, changed = MergeEvent(events, precise); !changed || len(events) != 1 || events[0] != precise {
		t.Fatalf("précision : %+v", events)
	}
	if events, changed = MergeEvent(events, lost); changed || events[0] != precise {
		t.Fatalf("recul : %+v", events)
	}
	if events, changed = MergeEvent(events, HistoryEvent{T: 11, K: HistoryBoot}); !changed || len(events) != 2 {
		t.Fatalf("autre évènement : %+v", events)
	}
}
