package internal

import (
	"errors"
	"fmt"
)

// Step describes one change of a multi-step command such as rename or
// adopt. Kind names what is touched (keychain, move, config, …).
type Step struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

func (s Step) String() string { return fmt.Sprintf("%-9s %s", s.Kind, s.Detail) }

// undoStep is a Step with the action that performs it and the one that
// reverts it.
type undoStep struct {
	Step
	do   func() error
	undo func() error
}

// runSteps prints and runs steps in order; with dryRun it only prints them.
// When a step fails, the ones already done are reverted in reverse order.
// finish, if set, runs last and is never reverted: its failure is only a
// warning (for example deleting a source that was copied).
func (a *App) runSteps(steps []undoStep, finish *undoStep, dryRun bool) ([]Step, error) {
	done := []Step{}
	for i, step := range steps {
		a.progressf("  %s\n", step.Step)
		done = append(done, step.Step)
		if dryRun {
			continue
		}
		if err := step.do(); err != nil {
			return done, a.rollback(steps[:i], fmt.Errorf("%s: %w", step.Step, err))
		}
	}
	if finish != nil {
		a.progressf("  %s\n", finish.Step)
		done = append(done, finish.Step)
		if !dryRun {
			if err := finish.do(); err != nil {
				fmt.Fprintf(a.Err, "Warning: %s: %v\n", finish.Step, err)
			}
		}
	}
	return done, nil
}

// rollback reverts done in reverse order and returns cause, joined with any
// errors from reverting.
func (a *App) rollback(done []undoStep, cause error) error {
	errs := []error{cause}
	for i := len(done) - 1; i >= 0; i-- {
		if err := done[i].undo(); err != nil {
			errs = append(errs, fmt.Errorf("cannot revert %q: %w", done[i].Step, err))
		} else {
			a.progressf("  reverted  %s\n", done[i].Step)
		}
	}
	return errors.Join(errs...)
}
