// internal/welcome/helper_steps.go

package welcome

import (
	"fmt"

	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/installer"
)

// buildHelperSteps adapts one streaming helper operation to the
// step list InstallProgressScreen renders. The operation runs
// entirely on the root side as ONE serialized request; each
// local step's Fn simply blocks until the root side reports
// that step complete, so the widget shows real progress without
// this process performing any of the work.
//
// The names must mirror the helper's own step list for the verb
// (the shared lists in internal/helper/wire.go; a helper-side
// unit test keeps them aligned). The final step also consumes
// the terminator and decodes the operation's result into
// result (which may be nil).
func buildHelperSteps(
	verb string, params any, names []string, result any,
) []installer.InstallStep {
	var session *helper.Session
	steps := make([]installer.InstallStep, len(names))
	for i := range names {
		i := i
		steps[i] = installer.InstallStep{
			Name: names[i],
			Fn: func() error {
				if i == 0 {
					s, err := helper.Start(verb, params)
					if err != nil {
						return err
					}
					session = s
				}
				if session == nil {
					return fmt.Errorf(
						"%s: no helper session (defect)", verb)
				}
				if err := session.WaitStep(i); err != nil {
					session.Close()
					return err
				}
				if i == len(names)-1 {
					return session.Wait(result)
				}
				return nil
			},
		}
	}
	return steps
}
