package config

import (
	"fmt"
	"os"

	"github.com/ZainCheung/omarchy-mihomo-plugin/manager/internal/profile"
)

type Compiler struct {
	Settings  profile.Settings
	Protected map[string]any
}

func (c Compiler) Compile(source, global, override []byte) ([]byte, error) {
	src, e := Parse(source)
	if e != nil {
		return nil, fmt.Errorf("parse source: %w", e)
	}
	g, e := ParseOptional(global)
	if e != nil {
		return nil, fmt.Errorf("parse global override: %w", e)
	}
	p, e := ParseOptional(override)
	if e != nil {
		return nil, fmt.Errorf("parse profile override: %w", e)
	}
	out := DeepMerge(src, g)
	out = DeepMerge(out, p)
	out = c.managed(out)
	out = c.protected(out)
	return Marshal(out)
}
func (c Compiler) managed(m map[string]any) map[string]any {
	out := clone(m).(map[string]any)
	if c.Settings.DNSManagement == "managed" {
		d := map[string]any{"enable": c.Settings.DNS.Enable, "ipv6": c.Settings.DNS.IPv6, "enhanced-mode": c.Settings.DNS.EnhancedMode, "fake-ip-range": c.Settings.DNS.FakeIPRange, "default-nameserver": toAny(c.Settings.DNS.DefaultNameserver), "nameserver": toAny(c.Settings.DNS.Nameserver), "proxy-server-nameserver": toAny(c.Settings.DNS.ProxyServerNameserver), "fake-ip-filter": toAny(c.Settings.DNS.FakeIPFilter)}
		out["dns"] = d
	}
	if c.Settings.TUNManagement == "managed" {
		t := map[string]any{
			"enable":                c.Settings.TUN.Enable,
			"stack":                 profile.NormalizeTUNStack(c.Settings.TUN.Stack),
			"auto-route":            c.Settings.TUN.AutoRoute,
			"auto-detect-interface": c.Settings.TUN.AutoDetectInterface,
			"strict-route":          c.Settings.TUN.StrictRoute,
			"dns-hijack":            toAny(c.Settings.TUN.DNSHijack),
		}
		out["tun"] = t
	}
	return out
}
func (c Compiler) protected(m map[string]any) map[string]any {
	out := clone(m).(map[string]any)
	// A nil map means that no running core was available (for example while
	// adding a profile before mihomo starts). Preserve source fields until the
	// manager has a real controller snapshot to enforce.
	if c.Protected == nil {
		return out
	}
	for _, k := range []string{"external-controller", "external-controller-unix", "secret", "external-ui"} {
		delete(out, k)
		if v, ok := c.Protected[k]; ok {
			out[k] = clone(v)
		}
	}
	return out
}
func toAny(s []string) []any {
	a := make([]any, len(s))
	for i, v := range s {
		a[i] = v
	}
	return a
}
func ReadProtected(path string) (map[string]any, error) {
	if path == "" {
		return map[string]any{}, nil
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return map[string]any{}, e
	}
	m, e := Parse(b)
	if e != nil {
		return nil, e
	}
	out := map[string]any{}
	for _, k := range []string{"external-controller", "external-controller-unix", "secret", "external-ui"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}
