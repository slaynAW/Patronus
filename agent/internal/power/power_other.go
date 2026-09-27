//go:build !windows && !linux && !darwin

package power

type systemController struct{}

func (systemController) Do(action Action, _ bool) error { return unsupported(action) }
