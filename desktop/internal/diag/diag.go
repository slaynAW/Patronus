// Package diag tient le journal de diagnostic de l'application Windows : évènements, avertissements et
// erreurs (avec leur contexte), gardés sur le PC et joints au rapport que l'utilisateur exporte
// lui-même (Réglages → Diagnostic). Rien n'est envoyé automatiquement.
//
// Chaque évènement est une ligne chiffrée en AES-256-GCM avec une clé de données aléatoire, elle-même
// protégée par DPAPI (lisible seulement par ce compte Windows sur ce PC). Deux fichiers de 1 Mio au
// plus : le plus ancien est remplacé. N'y écrire aucun secret (clés d'agent, jetons, clés privées,
// mots de passe).
package diag

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/slaynaw/wakeonlan/desktop/internal/dpapi"
)

// Level est la gravité d'un évènement (lettre écrite dans le journal).
type Level byte

const (
	LevelDebug Level = 'D'
	LevelInfo  Level = 'I'
	LevelWarn  Level = 'W'
	LevelError Level = 'E'
)

const (
	fileCurrent  = "journal-0.log"
	filePrevious = "journal-1.log"
	keyFile      = "journal.key"
	// crashFile reçoit le rapport du moteur Go si l'application s'arrête brutalement (erreur fatale,
	// panique non rattrapée) ; il est versé dans le journal chiffré au lancement suivant, puis vidé.
	crashFile = "plantage.txt"

	maxFileBytes   = 1 << 20
	maxRecordBytes = 16_000
	maxCrashBytes  = 64_000
)

var (
	entropy = []byte("patronus-desktop-diagnostics/1")
	aad     = []byte("patronus/diagnostics")
)

// Journal est le journal chiffré d'un dossier.
type Journal struct {
	dir  string
	aead cipher.AEAD
	// Mirror reçoit aussi chaque évènement en clair (mode développement : sortie d'erreur).
	Mirror io.Writer
	// Now est l'horloge (remplaçable pour les tests).
	Now func() time.Time

	mu sync.Mutex
}

// Open ouvre (ou crée) le journal du dossier dir. Si la clé de données est perdue (dossier copié
// depuis un autre compte, fichier abîmé), les anciens évènements sont illisibles : ils sont effacés.
func Open(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := loadKey(dir)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Journal{dir: dir, aead: aead, Now: time.Now}, nil
}

func loadKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, keyFile)
	if sealed, err := os.ReadFile(path); err == nil {
		if key, err := dpapi.Unprotect(sealed, entropy); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	_ = os.Remove(filepath.Join(dir, fileCurrent))
	_ = os.Remove(filepath.Join(dir, filePrevious))
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	sealed, err := dpapi.Protect(key, entropy)
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return key, nil
}

// Dir renvoie le dossier du journal.
func (j *Journal) Dir() string { return j.dir }

// Write ajoute un évènement.
func (j *Journal) Write(level Level, area, message string) {
	j.write(format(j.Now(), level, area, message, maxRecordBytes))
}

func format(now time.Time, level Level, area, message string, limit int) string {
	text := now.UTC().Format("2006-01-02T15:04:05.000Z") + " " + string(level) + " " + area + " : " + message
	return truncate(strings.TrimRight(text, "\n"), limit)
}

// truncate coupe text à limit octets au plus, sans couper un caractère.
func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func (j *Journal) write(text string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.Mirror != nil {
		_, _ = io.WriteString(j.Mirror, text+"\n")
	}
	nonce := make([]byte, j.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return
	}
	sealed := j.aead.Seal(nonce, nonce, []byte(text), aad)
	line := base64.StdEncoding.EncodeToString(sealed) + "\n"
	current := filepath.Join(j.dir, fileCurrent)
	f, err := os.OpenFile(current, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, err = f.WriteString(line)
	info, statErr := f.Stat()
	_ = f.Close()
	if err == nil && statErr == nil && info.Size() > maxFileBytes {
		previous := filepath.Join(j.dir, filePrevious)
		_ = os.Remove(previous)
		_ = os.Rename(current, previous)
	}
}

// Records renvoie tous les évènements, du plus ancien au plus récent.
func (j *Journal) Records() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []string
	for _, name := range []string{filePrevious, fileCurrent} {
		f, err := os.Open(filepath.Join(j.dir, name))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			out = append(out, j.open(line))
		}
		_ = f.Close()
	}
	return out
}

func (j *Journal) open(line string) string {
	raw, err := base64.StdEncoding.DecodeString(line)
	size := j.aead.NonceSize()
	if err != nil || len(raw) < size {
		return "[ligne illisible]"
	}
	plain, err := j.aead.Open(nil, raw[:size], raw[size:], aad)
	if err != nil {
		return "[ligne illisible : déchiffrement impossible]"
	}
	return string(plain)
}

// CaptureCrashes verse dans le journal le rapport d'un arrêt brutal précédent (écrit par le moteur
// Go), puis y dirige le prochain. Ce fichier est en clair le temps d'un lancement, dans le dossier de
// l'application (réservé à l'utilisateur) ; il ne contient que des traces d'exécution.
func (j *Journal) CaptureCrashes() error {
	path := filepath.Join(j.dir, crashFile)
	if data, err := os.ReadFile(path); err == nil {
		if data = bytes.TrimSpace(data); len(data) > 0 {
			when := "date inconnue"
			if info, err := os.Stat(path); err == nil {
				when = info.ModTime().UTC().Format(time.RFC3339)
			}
			j.write(format(j.Now(), LevelError, "plantage",
				"arrêt brutal lors du lancement précédent ("+when+") :\n"+string(data), maxCrashBytes))
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close() // le moteur Go garde sa propre copie du descripteur
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}

// --- Journal par défaut de l'application ---

var std atomic.Pointer[Journal]

// SetDefault choisit le journal utilisé par Log, Info, Warn et Error (nil : aucun, évènements ignorés).
func SetDefault(j *Journal) { std.Store(j) }

// Default renvoie le journal par défaut (nil s'il n'y en a pas).
func Default() *Journal { return std.Load() }

// Log écrit un évènement dans le journal par défaut.
func Log(level Level, area, format string, args ...any) {
	j := std.Load()
	if j == nil {
		return
	}
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	j.Write(level, area, message)
}

// Info note un évènement normal (action de l'utilisateur, étape importante).
func Info(area, format string, args ...any) { Log(LevelInfo, area, format, args...) }

// Warn note une anomalie gérée (erreur réseau, refus, donnée ignorée).
func Warn(area, format string, args ...any) { Log(LevelWarn, area, format, args...) }

// Error note une erreur (donnée perdue, plantage évité, état incohérent).
func Error(area, format string, args ...any) { Log(LevelError, area, format, args...) }

// Records renvoie les évènements du journal par défaut (vide s'il n'y en a pas).
func Records() []string {
	if j := std.Load(); j != nil {
		return j.Records()
	}
	return nil
}

// Writer renvoie un io.Writer pour le paquet log de Go : chaque message devient un évènement du
// niveau et du domaine donnés (log.SetOutput, avec log.SetFlags(0)).
func Writer(level Level, area string) io.Writer { return logWriter{level: level, area: area} }

type logWriter struct {
	level Level
	area  string
}

func (w logWriter) Write(p []byte) (int, error) {
	Log(w.level, w.area, "%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
