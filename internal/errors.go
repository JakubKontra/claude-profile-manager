package internal

import "errors"

// ErrSyncConflict is wrapped by cloud sync errors about files changed both
// locally and in the cloud.
var ErrSyncConflict = errors.New("sync conflict")

// ErrUsage matches errors caused by how a command was called: an unknown
// command or flag, or wrong arguments. See UsageError.
var ErrUsage = errors.New("usage error")

type usageError struct{ error }

func (e usageError) Is(target error) bool { return target == ErrUsage }
func (e usageError) Unwrap() error        { return e.error }

// UsageError marks err as a usage error without changing its message.
func UsageError(err error) error {
	if err == nil {
		return nil
	}
	return usageError{err}
}

// Exit codes; documented in docs/stability.md.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitCheckFailed = 3
	ExitLoginFailed = 4
)

// ExitCode maps an error returned by a command to the process exit code.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, ErrUsage):
		return ExitUsage
	case errors.Is(err, ErrDoctorFailed), errors.Is(err, ErrDiverged), errors.Is(err, ErrSyncConflict):
		return ExitCheckFailed
	case errors.Is(err, ErrLoginFailed):
		return ExitLoginFailed
	}
	return ExitError
}
