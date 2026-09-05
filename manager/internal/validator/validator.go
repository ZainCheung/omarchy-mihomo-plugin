package validator

import (
	"fmt"
	"os/exec"
	"strings"
)

func Validate(binary, path string) error {
	if binary == "" {
		return fmt.Errorf("mihomo binary not found (set MIHOMO_BIN)")
	}
	cmd := exec.Command(binary, "-t", "-f", path)
	out, e := cmd.CombinedOutput()
	if e != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = e.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
