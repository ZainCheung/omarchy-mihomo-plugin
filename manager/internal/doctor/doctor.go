package doctor

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/core"
	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/profile"
	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/store"
	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/validator"
)

type Check struct {
	ID          string   `json:"id"`
	Status      string   `json:"status"`
	Message     string   `json:"message"`
	MessageKey  string   `json:"messageKey,omitempty"`
	MessageArgs []string `json:"messageArgs,omitempty"`
}

type Report struct {
	Checks []Check `json:"checks"`
}

// appendCheck keeps the human-readable CLI message while also carrying a
// stable translation key for the QML diagnostics page. Dynamic values remain
// in MessageArgs so the UI can translate the surrounding sentence.
func appendCheck(r *Report, id, status, message string, localization ...string) {
	check := Check{ID: id, Status: status, Message: message}
	if len(localization) > 0 {
		check.MessageKey = localization[0]
		if len(localization) > 1 {
			check.MessageArgs = append([]string(nil), localization[1:]...)
		}
	}
	r.Checks = append(r.Checks, check)
}

func Run(s *store.Store) Report {
	r := Report{}
	add := func(id, status, message string, localization ...string) {
		appendCheck(&r, id, status, message, localization...)
	}

	idx, indexErr := profile.LoadIndex(s)
	if indexErr != nil {
		add("activeProfile", "error", "profile index could not be read", "diagnosticProfileIndexUnreadable")
	} else if idx.ActiveProfile == "" {
		add("activeProfile", "warning", "no profile is active", "diagnosticNoProfile")
	} else if meta, err := profile.LoadMeta(s, idx.ActiveProfile); err != nil {
		add("activeProfile", "error", "active profile metadata could not be read", "diagnosticProfileMetadataUnreadable")
	} else {
		add("activeProfile", "ok", meta.Name)
	}

	info, err := core.CoreInfo()
	if err != nil {
		add("process", "warning", "mihomo process information is unavailable", "diagnosticProcessUnavailable")
		add("version", "warning", "mihomo version could not be queried", "diagnosticVersionUnavailable")
		add("coreExecutable", "warning", "mihomo executable was not reported", "diagnosticExecutableUnreported")
		add("controller", "error", "mihomo controller is unavailable", "diagnosticControllerUnavailable")
		add("currentConfig", "warning", "running config path is unavailable", "diagnosticConfigPathUnavailable")
		add("configApi", "error", "running config could not be queried", "diagnosticConfigUnavailable")
		addNetworkChecks(&r, map[string]any{})
		addRuntimeChecks(&r, s, false)
		addCapabilityCheck(&r, "")
		addResolverChecks(&r)
		addDNSQueryCheck(&r)
		addProxyConnectivityCheck(&r, map[string]any{})
		return r
	}

	if info.PID > 0 {
		if _, statErr := os.Stat(fmt.Sprintf("/proc/%d", info.PID)); statErr == nil {
			add("process", "ok", fmt.Sprintf("PID %d", info.PID), "diagnosticProcessPID", strconv.Itoa(info.PID))
		} else {
			add("process", "warning", fmt.Sprintf("reported PID %d is not present", info.PID), "diagnosticProcessMissing", strconv.Itoa(info.PID))
		}
	} else {
		add("process", "warning", "mihomo process ID was not reported", "diagnosticProcessIDUnreported")
	}

	if info.Exe != "" {
		if stat, statErr := os.Stat(info.Exe); statErr == nil && stat.Mode().IsRegular() {
			add("coreExecutable", "ok", info.Exe)
		} else {
			add("coreExecutable", "error", "mihomo executable is not available", "diagnosticExecutableUnavailable")
		}
	} else {
		add("coreExecutable", "warning", "mihomo executable was not reported", "diagnosticExecutableUnreported")
	}

	if info.ControllerTarget != "" {
		add("controller", "ok", info.ControllerTarget)
	} else {
		add("controller", "warning", "controller target was not reported", "diagnosticControllerTargetUnreported")
	}

	if info.ConfigPath != "" {
		if stat, statErr := os.Stat(info.ConfigPath); statErr == nil && stat.Mode().IsRegular() {
			add("currentConfig", "ok", info.ConfigPath)
		} else {
			add("currentConfig", "error", "running config file is not available", "diagnosticConfigFileUnavailable")
		}
	} else {
		add("currentConfig", "warning", "running config path unavailable", "diagnosticConfigPathUnavailable")
	}

	if version, versionErr := coreObject("/version"); versionErr != nil {
		add("version", "warning", "mihomo version could not be queried", "diagnosticVersionUnavailable")
	} else if value := firstString(version, "version", "meta.version"); value != "" {
		add("version", "ok", value)
	} else {
		add("version", "warning", "mihomo returned no version", "diagnosticVersionEmpty")
	}

	configs, configErr := coreObject("/configs")
	if configErr != nil {
		add("configApi", "error", "running config could not be queried", "diagnosticConfigUnavailable")
		addNetworkChecks(&r, map[string]any{})
	} else {
		add("configApi", "ok", "running config is readable", "diagnosticConfigReadable")
		addNetworkChecks(&r, configs)
	}

	addRuntimeChecks(&r, s, configErr == nil)
	addCapabilityCheck(&r, info.Exe)
	addResolverChecks(&r)
	addDNSQueryCheck(&r)
	if configErr == nil {
		addProxyConnectivityCheck(&r, configs)
	} else {
		addProxyConnectivityCheck(&r, map[string]any{})
	}
	return r
}

func coreObject(path string) (map[string]any, error) {
	b, err := core.Run("get", path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		value := anyAt(data, key)
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func anyAt(data map[string]any, dotted string) any {
	var current any = data
	for _, part := range strings.Split(dotted, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = object[part]
		if !ok {
			return nil
		}
	}
	return current
}

func boolAt(data map[string]any, key string) (bool, bool) {
	value := anyAt(data, key)
	result, ok := value.(bool)
	return result, ok
}

func intAt(data map[string]any, key string) int {
	value := anyAt(data, key)
	switch number := value.(type) {
	case float64:
		return int(number)
	case json.Number:
		parsed, _ := number.Int64()
		return int(parsed)
	case int:
		return number
	case string:
		parsed, _ := strconv.Atoi(number)
		return parsed
	default:
		return 0
	}
}

func objectAt(data map[string]any, key string) map[string]any {
	value, _ := anyAt(data, key).(map[string]any)
	return value
}

func addNetworkChecks(r *Report, configs map[string]any) {
	tun := objectAt(configs, "tun")
	if enabled, ok := boolAt(tun, "enable"); ok {
		if enabled {
			appendCheck(r, "tunEnabled", "ok", "enabled", "enabled")
		} else {
			appendCheck(r, "tunEnabled", "warning", "disabled", "disabled")
		}
	} else {
		appendCheck(r, "tunEnabled", "warning", "TUN setting was not reported", "diagnosticSettingUnreported", "TUN")
	}
	if stack := firstString(tun, "stack"); stack != "" {
		appendCheck(r, "tunStack", "ok", stack)
	} else {
		appendCheck(r, "tunStack", "warning", "TUN stack was not reported", "diagnosticFieldUnreported", "TUN stack")
	}
	if device := firstString(tun, "device"); device != "" {
		if _, err := exec.LookPath("ip"); err != nil {
			appendCheck(r, "tunInterface", "warning", "ip command is unavailable", "diagnosticIPUnavailable")
		} else if err := exec.Command("ip", "link", "show", device).Run(); err != nil {
			appendCheck(r, "tunInterface", "warning", device+" is not present", "diagnosticInterfaceMissing", device)
		} else {
			appendCheck(r, "tunInterface", "ok", device)
		}
	} else {
		appendCheck(r, "tunInterface", "warning", "TUN interface was not reported", "diagnosticFieldUnreported", "TUN interface")
	}

	dns := objectAt(configs, "dns")
	if enabled, ok := boolAt(dns, "enable"); ok {
		status := "warning"
		message := "disabled"
		if enabled {
			status, message = "ok", "enabled"
		}
		if enabled {
			appendCheck(r, "dnsEnabled", status, message, "enabled")
		} else {
			appendCheck(r, "dnsEnabled", status, message, "disabled")
		}
	} else {
		appendCheck(r, "dnsEnabled", "warning", "DNS setting was not reported", "diagnosticSettingUnreported", "DNS")
	}
	if mode := firstString(dns, "enhanced-mode"); mode != "" {
		appendCheck(r, "dnsMode", "ok", mode)
	} else {
		appendCheck(r, "dnsMode", "warning", "DNS mode was not reported", "diagnosticFieldUnreported", "DNS mode")
	}

	port := core.HTTPProxyPort(configs)
	if port > 0 {
		appendCheck(r, "httpProxyPort", "ok", strconv.Itoa(port))
	} else {
		appendCheck(r, "httpProxyPort", "warning", "no proxy port is enabled", "diagnosticNoProxyPort")
	}
}

func addRuntimeChecks(r *Report, s *store.Store, controllerReadable bool) {
	if stat, err := os.Stat(s.CurrentPath()); err == nil && stat.Mode().IsRegular() {
		appendCheck(r, "runtime", "ok", s.CurrentPath())
		if controllerReadable {
			if err := validator.Validate(core.Binary(), s.CurrentPath()); err != nil {
				appendCheck(r, "validation", "error", err.Error())
			} else {
				appendCheck(r, "validation", "ok", "runtime config is valid", "diagnosticRuntimeValid")
			}
		} else {
			appendCheck(r, "validation", "warning", "controller is unavailable; validation was skipped", "diagnosticValidationSkipped")
		}
	} else {
		appendCheck(r, "runtime", "warning", "managed runtime has not been created", "diagnosticRuntimeMissing")
	}
}

func addCapabilityCheck(r *Report, executable string) {
	if executable == "" {
		appendCheck(r, "tunCapability", "warning", "mihomo executable is unavailable", "diagnosticExecutableUnavailable")
		return
	}
	if _, err := exec.LookPath("getcap"); err != nil {
		appendCheck(r, "tunCapability", "warning", "getcap is unavailable", "diagnosticGetcapUnavailable")
		return
	}
	out, err := exec.Command("getcap", executable).CombinedOutput()
	if err == nil && strings.Contains(string(out), "cap_net_admin") {
		appendCheck(r, "tunCapability", "ok", strings.TrimSpace(string(out)))
		return
	}
	appendCheck(r, "tunCapability", "warning",
		fmt.Sprintf("mihomo is missing cap_net_admin; run: sudo setcap cap_net_admin,cap_net_raw=+ep %s", executable),
		"diagnosticCapabilityMissing", "sudo setcap cap_net_admin,cap_net_raw=+ep "+executable)
}

func addResolverChecks(r *Report) {
	if err := exec.Command("systemctl", "is-active", "--quiet", "systemd-resolved").Run(); err == nil {
		appendCheck(r, "systemdResolved", "ok", "systemd-resolved is active", "diagnosticResolvedActive")
	} else {
		appendCheck(r, "systemdResolved", "warning", "systemd-resolved is not active", "diagnosticResolvedInactive")
	}
	if target, err := os.Readlink("/etc/resolv.conf"); err == nil {
		appendCheck(r, "resolvConf", "ok", target)
	} else {
		appendCheck(r, "resolvConf", "warning", "/etc/resolv.conf could not be inspected", "diagnosticResolvConfUnavailable")
	}
	if _, err := exec.LookPath("resolvectl"); err == nil {
		appendCheck(r, "resolvectl", "ok", "resolvectl is available", "diagnosticResolvectlAvailable")
	} else {
		appendCheck(r, "resolvectl", "warning", "resolvectl is unavailable", "diagnosticResolvectlUnavailable")
	}
}

func addDNSQueryCheck(r *Report) {
	path := "/dns/query?name=" + url.QueryEscape("example.com") + "&type=A"
	if _, err := core.Run("get", path); err == nil {
		appendCheck(r, "dnsQuery", "ok", "example.com resolved through the core", "diagnosticDNSQuerySucceeded")
	} else {
		appendCheck(r, "dnsQuery", "warning", "DNS query through the core failed", "diagnosticDNSQueryFailed")
	}
}

func addProxyConnectivityCheck(r *Report, configs map[string]any) {
	port := core.HTTPProxyPort(configs)
	if port <= 0 {
		appendCheck(r, "proxyConnectivity", "warning", "no HTTP-capable proxy port is enabled", "diagnosticNoHTTPProxyPort")
		return
	}
	curl, err := exec.LookPath("curl")
	if err != nil {
		appendCheck(r, "proxyConnectivity", "warning", "curl is unavailable", "diagnosticCurlUnavailable")
		return
	}
	proxy := "http://127.0.0.1:" + strconv.Itoa(port)
	cmd := exec.Command(curl, "--fail", "--silent", "--show-error", "--max-time", "8", "--connect-timeout", "5", "--proxy", proxy, "--head", "https://www.gstatic.com/generate_204")
	if err := cmd.Run(); err != nil {
		appendCheck(r, "proxyConnectivity", "warning", "proxy connectivity check failed", "diagnosticProxyCheckFailed")
		addFirewallTUNCompatibilityCheck(r, configs)
		return
	}
	appendCheck(r, "proxyConnectivity", "ok", "proxy request succeeded", "diagnosticProxyCheckSucceeded")
}

func addFirewallTUNCompatibilityCheck(r *Report, configs map[string]any) {
	stack, ok := firewallSensitiveTUNStack(configs)
	if !ok {
		return
	}
	firewall, active := activeFirewall()
	if !active {
		return
	}
	appendCheck(r, "firewallTunCompatibility", "warning",
		fmt.Sprintf("%s TUN stack may conflict with %s; try gVisor or adjust firewall rules", stack, firewall),
		"diagnosticFirewallTunCompatibility", stack, firewall)
}

func firewallSensitiveTUNStack(configs map[string]any) (string, bool) {
	tun := objectAt(configs, "tun")
	enabled, ok := boolAt(tun, "enable")
	if !ok || !enabled {
		return "", false
	}
	stack := strings.ToLower(strings.TrimSpace(firstString(tun, "stack")))
	if stack != "system" && stack != "mixed" {
		return "", false
	}
	return stack, true
}

func activeFirewall() (string, bool) {
	if ufw, err := exec.LookPath("ufw"); err == nil {
		if output, runErr := exec.Command(ufw, "status").CombinedOutput(); runErr == nil && strings.Contains(strings.ToLower(string(output)), "status: active") {
			return "UFW", true
		}
	}
	if firewallCmd, err := exec.LookPath("firewall-cmd"); err == nil {
		if output, runErr := exec.Command(firewallCmd, "--state").CombinedOutput(); runErr == nil && strings.EqualFold(strings.TrimSpace(string(output)), "running") {
			return "firewalld", true
		}
	}
	return "", false
}
