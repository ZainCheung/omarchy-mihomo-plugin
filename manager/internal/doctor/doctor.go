package doctor

import (
	"fmt"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/core"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/store"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/validator"
	"os"
	"os/exec"
	"strings"
)

type Check struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type Report struct {
	Checks []Check `json:"checks"`
}

func Run(s *store.Store) Report {
	r := Report{}
	add := func(id, status, msg string) { r.Checks = append(r.Checks, Check{id, status, msg}) }
	info, e := core.CoreInfo()
	if e != nil {
		add("controller", "error", e.Error())
		return r
	}
	add("controller", "ok", info.ControllerTarget)
	if info.Exe != "" {
		add("coreExecutable", "ok", info.Exe)
	} else {
		add("coreExecutable", "warning", "mihomo executable was not reported")
	}
	if info.ConfigPath != "" {
		add("currentConfig", "ok", info.ConfigPath)
	} else {
		add("currentConfig", "warning", "running config path unavailable")
	}
	if _, e = os.Stat(s.CurrentPath()); e == nil {
		add("runtime", "ok", s.CurrentPath())
		if e = validator.Validate(core.Binary(), s.CurrentPath()); e != nil {
			add("validation", "error", e.Error())
		} else {
			add("validation", "ok", "runtime config is valid")
		}
	} else {
		add("runtime", "warning", "managed runtime has not been created")
	}
	if b, e := exec.Command("getcap", info.Exe).CombinedOutput(); e == nil {
		if strings.Contains(string(b), "cap_net_admin") {
			add("tunCapability", "ok", strings.TrimSpace(string(b)))
		} else {
			add("tunCapability", "warning", "mihomo is missing cap_net_admin")
		}
	} else {
		add("tunCapability", "warning", fmt.Sprintf("cannot inspect capabilities: %v", e))
	}
	return r
}
