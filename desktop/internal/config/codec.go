// Package config lit, valide, stocke, exporte et importe la configuration, dans le même format
// que l'application Android (core/config) : les sauvegardes sont interchangeables.
package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// MaxDevices borne le nombre de terminaux (protection contre un fichier malveillant).
const MaxDevices = 200

// Reason classe les erreurs de lecture d'une configuration.
type Reason string

const (
	NotABackup       Reason = "NOT_A_BACKUP"
	NewerVersion     Reason = "NEWER_VERSION"
	InvalidData      Reason = "INVALID_DATA"
	PasswordRequired Reason = "PASSWORD_REQUIRED"
	WrongPassword    Reason = "WRONG_PASSWORD"
)

// Error est une erreur de configuration, avec un message destiné à l'utilisateur.
type Error struct {
	Reason  Reason
	Message string
}

func (e *Error) Error() string { return e.Message }

func fail(reason Reason, format string, args ...any) *Error {
	return &Error{Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// Encode sérialise la configuration en JSON compact.
func Encode(c model.AppConfig) ([]byte, error) { return json.Marshal(c) }

// Decode lit une configuration JSON, applique les migrations de schéma et la valide entièrement.
func Decode(data []byte) (model.AppConfig, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return model.AppConfig{}, fail(InvalidData, "Fichier JSON illisible")
	}
	return fromJSON(root)
}

func fromJSON(root map[string]json.RawMessage) (model.AppConfig, error) {
	version := 1
	if raw, ok := root["schemaVersion"]; ok {
		var v int
		if json.Unmarshal(raw, &v) == nil {
			version = v
		}
	}
	if version > model.CurrentSchemaVersion {
		return model.AppConfig{}, fail(NewerVersion,
			"Cette configuration vient d'une version plus récente de l'application : mettez-la à jour.")
	}
	// Migrations n → n+1 : aucune tant qu'il n'existe qu'un schéma (voir docs/ARCHITECTURE.md).
	data, _ := json.Marshal(root)
	var c model.AppConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return model.AppConfig{}, fail(InvalidData, "Configuration invalide : %s", cleanError(err))
	}
	return Sanitize(c)
}

// Sanitize valide chaque terminal et remet les réglages dans leurs bornes.
func Sanitize(c model.AppConfig) (model.AppConfig, error) {
	if len(c.Devices) > MaxDevices {
		return model.AppConfig{}, fail(InvalidData, "Trop de terminaux (max %d)", MaxDevices)
	}
	ids := map[string]bool{}
	for _, d := range c.Devices {
		if strings.TrimSpace(d.ID) == "" || model.UTF16Len(d.ID) > 64 {
			return model.AppConfig{}, fail(InvalidData, "Identifiant de terminal invalide")
		}
		if ids[d.ID] {
			return model.AppConfig{}, fail(InvalidData, "Identifiant de terminal en double : %s", d.ID)
		}
		ids[d.ID] = true
		if errs := model.Validate(d); len(errs) > 0 {
			return model.AppConfig{}, fail(InvalidData, "« %s » : %s", d.Name, errs[0].Message)
		}
	}
	c = c.Clone()
	c.SchemaVersion = model.CurrentSchemaVersion
	c.Settings = c.Settings.Clamped()
	return c, nil
}

// cleanError retire le jargon de encoding/json (« json: cannot unmarshal… ») autant que possible.
func cleanError(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "json: cannot unmarshal") {
		return "type de valeur inattendu (" + strings.TrimPrefix(msg, "json: ") + ")"
	}
	return strings.TrimPrefix(msg, "json: ")
}
