//go:build !windows && !linux

package disks

import (
	"errors"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

var errUnsupported = errors.New("non pris en charge")

func volumes() ([]protocol.Volume, error) { return nil, errUnsupported }

func drives() ([]protocol.Drive, error) { return nil, errUnsupported }

func diskErrors(time.Time) (int, time.Time, error) { return 0, time.Time{}, errUnsupported }
