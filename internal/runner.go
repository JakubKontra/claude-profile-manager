package internal

import "os/exec"

// Runner runs external programs such as git. Tests substitute a fake to
// exercise failure paths without a real binary.
type Runner interface {
	// Run executes name with args in dir ("" means the current directory)
	// and returns its combined stdout and stderr.
	Run(dir, name string, args ...string) ([]byte, error)
}

// ExecRunner runs programs with os/exec.
type ExecRunner struct{}

func (ExecRunner) Run(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
