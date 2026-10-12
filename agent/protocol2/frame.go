package protocol2

import (
	"encoding/binary"
	"errors"
	"io"
)

// Erreurs de trame : dans les trois cas, la connexion est fermée sans réponse (§4.1).
var (
	ErrEmptyFrame     = errors.New("wolagent/2 : trame vide")
	ErrFrameTooLarge  = errors.New("wolagent/2 : trame trop longue")
	ErrTruncatedFrame = errors.New("wolagent/2 : trame tronquée")
)

// ReadFrame lit une trame (§4.1) d'au plus limit octets : la longueur sur 4 octets (gros-boutiste), puis
// le message. Elle renvoie io.EOF si le flux se termine proprement avant la trame. La longueur est
// vérifiée avant toute allocation.
func ReadFrame(r io.Reader, limit int) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if err == io.ErrUnexpectedEOF {
			return nil, ErrTruncatedFrame
		}
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	switch {
	case n == 0:
		return nil, ErrEmptyFrame
	case uint64(n) > uint64(limit):
		return nil, ErrFrameTooLarge
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, ErrTruncatedFrame
		}
		return nil, err
	}
	return payload, nil
}

// WriteFrame écrit une trame d'au plus limit octets, en un seul appel à Write.
func WriteFrame(w io.Writer, payload []byte, limit int) error {
	switch {
	case len(payload) == 0:
		return ErrEmptyFrame
	case len(payload) > limit:
		return ErrFrameTooLarge
	}
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame, uint32(len(payload)))
	copy(frame[4:], payload)
	_, err := w.Write(frame)
	return err
}
