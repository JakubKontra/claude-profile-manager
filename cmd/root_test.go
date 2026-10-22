package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/jakubkontra/cpm/internal"
)

func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{"bogus"},
		{"lst"},
		{"rename", "only-one"},
		{"list", "--nope"},
		{"remove"},
		{"login"},
	}
	for _, args := range cases {
		root := NewRoot()
		root.SetArgs(append(args, "--config", t.TempDir()+"/config.toml"))
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		err := run(root)
		if got := internal.ExitCode(err); got != internal.ExitUsage {
			t.Errorf("cpm %v: exit %d (%v), want %d", args, got, err, internal.ExitUsage)
		}
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{errors.New("boom"), 1},
		{internal.UsageError(errors.New("bad flag")), 2},
		{internal.ErrDoctorFailed, 3},
		{internal.ErrDiverged, 3},
		{fmt.Errorf("pull: %w", errors.Join(fmt.Errorf("%w: a", internal.ErrSyncConflict))), 3},
		{internal.ErrLoginFailed, 4},
	}
	for _, c := range cases {
		if got := internal.ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
	if err := internal.UsageError(errors.New("same text")); err.Error() != "same text" {
		t.Errorf("UsageError changed the message: %q", err)
	}
}
