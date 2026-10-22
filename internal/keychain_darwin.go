package internal

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// platformKeychain returns the macOS login Keychain, accessed through security(1).
func platformKeychain() KeychainStore { return securityKeychain{} }

type securityKeychain struct{}

// Exists deliberately omits -w, so the token is never read.
func (securityKeychain) Exists(service, account string) bool {
	return exec.Command("security", "find-generic-password", "-a", account, "-s", service).Run() == nil
}

// Read returns the password field (-w) of the entry.
func (securityKeychain) Read(service, account string) ([]byte, error) {
	out, err := exec.Command("security", "find-generic-password", "-a", account, "-s", service, "-w").Output()
	if err != nil {
		return nil, err
	}
	return []byte(strings.TrimSpace(string(out))), nil
}

// Write sends the command to `security -i` on stdin with the secret
// hex-encoded (-X), so the secret never appears on a command line where ps
// would show it.
func (securityKeychain) Write(service, account string, secret []byte) error {
	for _, s := range []string{service, account} {
		if strings.ContainsAny(s, "\"\\\n") {
			return fmt.Errorf("unsupported keychain name %q", s)
		}
	}
	cmd := exec.Command("security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a \"%s\" -s \"%s\" -X %s\n", account, service, hex.EncodeToString(secret)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security add-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (securityKeychain) Delete(service, account string) error {
	out, err := exec.Command("security", "delete-generic-password", "-a", account, "-s", service).CombinedOutput()
	if err != nil {
		// security exits 44 (errSecItemNotFound) when there is nothing to delete.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 44 {
			return nil
		}
		return fmt.Errorf("security delete-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
