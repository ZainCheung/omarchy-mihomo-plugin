package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/config"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/core"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/doctor"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/fetcher"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/profile"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/store"
	"github.com/lijiawei0305-pixel/omarchy-mihomo-plugin/manager/internal/validator"
)

var st = store.New()

type response struct {
	OK      bool   `json:"ok"`
	Stage   string `json:"stage,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func emit(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
func ok(v any)   { emit(response{OK: true, Data: v}) }
func fail(stage string, err error) {
	message := err.Error()
	// Keep stdout machine-readable for QML, while retaining a useful
	// diagnostic for interactive callers. Never print raw command arguments
	// here: fetcher/core errors are expected to redact subscription URLs.
	fmt.Fprintf(os.Stderr, "omarchy-mihomo-manager: %s: %s\n", stage, message)
	emit(response{OK: false, Stage: stage, Error: message})
	os.Exit(1)
}
func runLocked(fn func() error) error {
	release, err := st.Lock()
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

func main() {
	if err := st.Ensure(); err != nil {
		fail("store", err)
	}
	args := os.Args[1:]
	if len(args) == 0 {
		fail("args", fmt.Errorf("command required"))
	}
	switch args[0] {
	case "status":
		status()
	case "profile":
		profileCommand(args[1:])
	case "config":
		configCommand(args[1:])
	case "reconcile":
		reconcile()
	case "settings":
		settingsCommand(args[1:])
	case "override":
		overrideCommand(args[1:])
	case "doctor":
		emit(doctor.Run(st))
	case "redact":
		if len(args) != 2 {
			fail("args", fmt.Errorf("URL required"))
		}
		fmt.Println(redact(args[1]))
	default:
		fail("args", fmt.Errorf("unknown command: %s", args[0]))
	}
}

func publicMeta(m profile.Meta) profile.Meta {
	if m.URL != "" {
		m.URL = redact(m.URL)
	}
	return m
}
func publicMetas(in []profile.Meta) []profile.Meta {
	out := make([]profile.Meta, len(in))
	for i, m := range in {
		out[i] = publicMeta(m)
	}
	return out
}
func status() {
	idx, err := profile.LoadIndex(st)
	if err != nil {
		fail("store", err)
	}
	list, _ := profile.List(st, idx)
	info, _ := core.CoreInfo()
	ok(map[string]any{"activeProfile": idx.ActiveProfile, "profiles": publicMetas(list), "core": info})
}

func profileCommand(args []string) {
	if len(args) == 0 {
		fail("args", fmt.Errorf("profile command required"))
	}
	switch args[0] {
	case "list":
		idx, err := profile.LoadIndex(st)
		if err != nil {
			fail("store", err)
		}
		list, _ := profile.List(st, idx)
		ok(publicMetas(list))
	case "get":
		requireID(args)
		m, err := profile.LoadMeta(st, args[1])
		if err != nil {
			fail("profile", err)
		}
		ok(publicMeta(m))
	case "source":
		requireID(args)
		b, err := profile.ReadSource(st, args[1])
		if err != nil {
			fail("store", err)
		}
		emit(map[string]any{"ok": true, "id": args[1], "source": string(b)})
	case "url":
		requireID(args)
		m, err := profile.LoadMeta(st, args[1])
		if err != nil {
			fail("profile", err)
		}
		if m.Type != "remote" || m.URL == "" {
			fail("profile", fmt.Errorf("local profile has no subscription URL"))
		}
		// This command is used only by the explicit URL editor flow. All
		// listing/status APIs use publicMeta and remain redacted.
		emit(map[string]any{"ok": true, "id": args[1], "url": m.URL})
	case "override":
		requireID(args)
		b, err := profile.ReadOverride(st, args[1])
		if err != nil {
			fail("store", err)
		}
		emit(map[string]any{"ok": true, "id": args[1], "override": string(b)})
	case "runtime":
		requireID(args)
		if args[1] != currentActive() {
			fail("runtime", fmt.Errorf("profile is not active"))
		}
		b, err := os.ReadFile(st.CurrentPath())
		if err != nil {
			fail("store", err)
		}
		info, _ := os.Stat(st.CurrentPath())
		data := map[string]any{"ok": true, "id": args[1], "runtime": string(b), "path": st.CurrentPath()}
		if info != nil {
			data["size"] = info.Size()
			data["mtime"] = info.ModTime().Unix()
		}
		emit(data)
	case "add":
		addProfile(args[1:])
	case "import-current":
		importCurrent(args[1:])
	case "rename":
		requireID(args)
		if len(args) < 3 {
			fail("args", fmt.Errorf("id and name required"))
		}
		name := strings.TrimSpace(strings.Join(args[2:], " "))
		if name == "" {
			fail("args", fmt.Errorf("name must not be empty"))
		}
		if err := runLocked(func() error { return profile.Rename(st, args[1], name) }); err != nil {
			fail("store", err)
		}
		ok(map[string]string{"id": args[1], "name": name})
	case "set-url":
		requireID(args)
		if len(args) < 3 {
			fail("args", fmt.Errorf("id and URL required"))
		}
		rawURL := strings.TrimSpace(args[2])
		parsed, parseErr := url.Parse(rawURL)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			fail("args", fmt.Errorf("profile URL must be HTTP or HTTPS"))
		}
		if err := runLocked(func() error {
			meta, err := profile.LoadMeta(st, args[1])
			if err != nil {
				return err
			}
			if meta.Type != "remote" {
				return fmt.Errorf("local profile has no subscription URL")
			}
			meta.URL = rawURL
			meta.ETag = ""
			meta.LastModified = ""
			meta.LastError = ""
			return profile.SaveMeta(st, meta)
		}); err != nil {
			fail("store", err)
		}
		ok(map[string]string{"id": args[1], "url": redact(rawURL)})
	case "delete":
		requireID(args)
		if err := deleteProfile(args[1]); err != nil {
			fail("store", err)
		}
	case "select":
		requireID(args)
		selectProfile(args[1])
	case "update":
		requireID(args)
		result, err := updateProfile(args[1], len(args) > 2 && args[2] == "--via-proxy")
		if err != nil {
			fail(operationStage(err), operationError(err))
		}
		ok(result)
	case "update-due":
		updateDue()
	default:
		fail("args", fmt.Errorf("unknown profile command: %s", args[0]))
	}
}
func requireID(args []string) {
	if len(args) < 2 || !store.ValidID(args[1]) {
		fail("args", fmt.Errorf("valid profile id required"))
	}
}
func deleteProfile(id string) error {
	err := runLocked(func() error {
		idx, err := profile.LoadIndex(st)
		if err != nil {
			return err
		}
		found := false
		for _, item := range idx.Profiles {
			if item == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("profile not found")
		}
		// Deleting the active profile would leave the controller running a
		// configuration whose source no longer exists. Require an explicit
		// switch first so index, runtime, and controller stay coherent.
		if idx.ActiveProfile == id {
			return fmt.Errorf("cannot delete the active profile; select another profile first")
		}
		dir := st.ProfileDir(id)
		tombstone := st.ProfileDir(".delete-" + id)
		_ = os.RemoveAll(tombstone)
		if err := os.Rename(dir, tombstone); err != nil {
			return err
		}
		committed := false
		tombstonePreserved := false
		defer func() {
			if committed {
				_ = os.RemoveAll(tombstone)
			} else if !tombstonePreserved {
				_ = os.RemoveAll(tombstone)
			}
		}()
		next := make([]string, 0, len(idx.Profiles))
		for _, item := range idx.Profiles {
			if item != id {
				next = append(next, item)
			}
		}
		idx.Profiles = next
		if err := profile.SaveIndex(st, idx); err != nil {
			if restoreErr := os.Rename(tombstone, dir); restoreErr != nil {
				tombstonePreserved = true
				return rollbackErrors(err, restoreErr)
			}
			return err
		}
		committed = true
		return nil
	})
	if err != nil {
		return err
	}
	ok(map[string]bool{"deleted": true})
	return nil
}

func addProfile(args []string) {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	rawURL := fs.String("url", "", "subscription URL")
	name := fs.String("name", "", "profile name")
	interval := fs.Int("update-interval", 21600, "update interval in seconds")
	if err := fs.Parse(args); err != nil {
		fail("args", err)
	}
	if *rawURL == "" || strings.TrimSpace(*name) == "" {
		fail("args", fmt.Errorf("--url and --name are required"))
	}
	if *interval < 0 {
		fail("args", fmt.Errorf("update interval must not be negative"))
	}
	result, err := fetcher.Fetch(*rawURL, "", "", "", false, 0)
	if err != nil {
		fail("fetch", err)
	}
	if _, err = config.Parse(result.Body); err != nil {
		fail("parse", err)
	}
	if err = validateCompiled(result.Body, []byte("{}\n")); err != nil {
		fail("validate", err)
	}
	id, err := store.RandomID()
	if err != nil {
		fail("store", err)
	}
	meta := profile.TouchSuccess(profile.Meta{ID: id, Name: strings.TrimSpace(*name), Type: "remote", URL: *rawURL, UpdateIntervalSec: *interval}, result.ETag, result.LastModified)
	err = runLocked(func() error {
		if err := profile.Add(st, meta, result.Body, []byte("{}\n")); err != nil {
			return err
		}
		idx, err := profile.LoadIndex(st)
		if err != nil {
			_ = profile.Delete(st, id)
			return err
		}
		idx.Profiles = append(idx.Profiles, id)
		if err = profile.SaveIndex(st, idx); err != nil {
			_ = profile.Delete(st, id)
			return err
		}
		return nil
	})
	if err != nil {
		fail("store", err)
	}
	ok(publicMeta(meta))
}

func importCurrent(args []string) {
	fs := flag.NewFlagSet("import-current", flag.ContinueOnError)
	name := fs.String("name", "Local Config", "profile name")
	if err := fs.Parse(args); err != nil {
		fail("args", err)
	}
	info, err := core.CoreInfo()
	if err != nil {
		fail("coreinfo", err)
	}
	if info.ConfigPath == "" {
		fail("coreinfo", fmt.Errorf("running config path unavailable"))
	}
	source, err := os.ReadFile(info.ConfigPath)
	if err != nil {
		fail("read", err)
	}
	if _, err = config.Parse(source); err != nil {
		fail("parse", err)
	}
	if err = validateCompiled(source, []byte("{}\n")); err != nil {
		fail("validate", err)
	}
	id, err := store.RandomID()
	if err != nil {
		fail("store", err)
	}
	meta := profile.Meta{ID: id, Name: strings.TrimSpace(*name), Type: "local"}
	err = runLocked(func() error {
		if err := profile.Add(st, meta, source, []byte("{}\n")); err != nil {
			return err
		}
		idx, err := profile.LoadIndex(st)
		if err != nil {
			_ = profile.Delete(st, id)
			return err
		}
		idx.Profiles = append(idx.Profiles, id)
		if err = profile.SaveIndex(st, idx); err != nil {
			_ = profile.Delete(st, id)
			return err
		}
		return nil
	})
	if err != nil {
		fail("store", err)
	}
	ok(publicMeta(meta))
}

func selectProfile(id string) {
	err := runLocked(func() error {
		idx, err := profile.LoadIndex(st)
		if err != nil {
			return err
		}
		found := false
		for _, candidate := range idx.Profiles {
			if candidate == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("profile not found")
		}
		meta, err := profile.LoadMeta(st, id)
		if err != nil {
			return err
		}
		oldIndex := idx
		oldState, hadState, err := snapshot(st.StatePath())
		if err != nil {
			return err
		}
		oldRuntime, err := snapshotRuntime()
		if err != nil {
			return err
		}
		if err := applyLocked(id, meta, nil); err != nil {
			return err
		}
		idx.ActiveProfile = id
		if err = profile.SaveIndex(st, idx); err != nil {
			runtimeErr := restoreRuntimeWithFallback(oldRuntime, restoreMode{applyPreviousIfCurrentMissing: true})
			indexErr := profile.SaveIndex(st, oldIndex)
			stateErr := restoreSnapshot(st.StatePath(), oldState, hadState)
			if runtimeErr != nil || indexErr != nil || stateErr != nil {
				return rollbackErrors(err, runtimeErr, indexErr, stateErr)
			}
			return err
		}
		return nil
	})
	if err != nil {
		fail("apply", err)
	}
	ok(map[string]string{"activeProfile": id})
}

type operationFailure struct {
	stage string
	err   error
}

func (e operationFailure) Error() string { return e.err.Error() }
func operationStage(err error) string {
	if e, ok := err.(operationFailure); ok {
		return e.stage
	}
	return "profile"
}
func operationError(err error) error {
	if e, ok := err.(operationFailure); ok {
		return e.err
	}
	return err
}

func rollbackErrors(primary error, rollbackErrs ...error) error {
	parts := []string{primary.Error()}
	for _, err := range rollbackErrs {
		if err != nil {
			parts = append(parts, "rollback: "+err.Error())
		}
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}
func snapshot(path string) ([]byte, bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return b, err == nil, err
}
func restoreSnapshot(path string, data []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return st.WriteAtomic(path, data)
}

func updateDue() {
	idx, err := profile.LoadIndex(st)
	if err != nil {
		fail("store", err)
	}
	results := []any{}
	failures := []string{}
	now := time.Now()
	for _, id := range idx.Profiles {
		meta, loadErr := profile.LoadMeta(st, id)
		if loadErr != nil || !profile.Due(meta, now) {
			continue
		}
		result, updateErr := updateProfile(id, meta.UpdateViaProxy)
		if updateErr != nil {
			failures = append(failures, id+": "+updateErr.Error())
			continue
		}
		results = append(results, result)
	}
	if len(failures) > 0 {
		fail("update", fmt.Errorf("%s", strings.Join(failures, "; ")))
	}
	ok(map[string]any{"updated": results})
}

func updateProfile(id string, viaProxy bool) (map[string]any, error) {
	meta, err := profile.LoadMeta(st, id)
	if err != nil {
		return nil, operationFailure{"profile", err}
	}
	if meta.Type != "remote" {
		return map[string]any{"id": id, "updated": false, "reason": "local profile"}, nil
	}
	port := 0
	if viaProxy {
		port, err = core.MixedPort()
		if err != nil {
			return nil, operationFailure{"fetch", err}
		}
	}
	result, err := fetcher.Fetch(meta.URL, meta.FetchUserAgent, meta.ETag, meta.LastModified, viaProxy, port)
	if err != nil {
		_ = runLocked(func() error {
			current, loadErr := profile.LoadMeta(st, id)
			if loadErr != nil || current.URL != meta.URL {
				return nil
			}
			return profile.SaveMeta(st, profile.SetError(current, err))
		})
		return nil, operationFailure{"fetch", err}
	}
	if result.NotModified {
		err = runLocked(func() error {
			current, loadErr := profile.LoadMeta(st, id)
			if loadErr != nil {
				return loadErr
			}
			if current.URL != meta.URL {
				return fmt.Errorf("profile changed during update; retry")
			}
			etag, lastModified := result.ETag, result.LastModified
			if etag == "" {
				etag = current.ETag
			}
			if lastModified == "" {
				lastModified = current.LastModified
			}
			return profile.SaveMeta(st, profile.TouchSuccess(current, etag, lastModified))
		})
		if err != nil {
			return nil, operationFailure{"store", err}
		}
		return map[string]any{"id": id, "notModified": true}, nil
	}
	if _, err = config.Parse(result.Body); err != nil {
		_ = runLocked(func() error {
			current, loadErr := profile.LoadMeta(st, id)
			if loadErr != nil || current.URL != meta.URL {
				return nil
			}
			return profile.SaveMeta(st, profile.SetError(current, err))
		})
		return nil, operationFailure{"parse", err}
	}
	newMeta := profile.TouchSuccess(meta, result.ETag, result.LastModified)
	active := false
	err = runLocked(func() error {
		// Re-read the index while holding the lock. A profile can be selected
		// while the subscription request is in flight; deciding active/inactive
		// from the earlier snapshot could otherwise skip the required apply.
		idx, indexErr := profile.LoadIndex(st)
		if indexErr != nil {
			return operationFailure{"store", indexErr}
		}
		currentMeta, metaErr := profile.LoadMeta(st, id)
		if metaErr != nil {
			return operationFailure{"store", metaErr}
		}
		if currentMeta.URL != meta.URL {
			return operationFailure{"store", fmt.Errorf("profile changed during update; retry")}
		}
		newMeta = profile.TouchSuccess(currentMeta, result.ETag, result.LastModified)
		active = idx.ActiveProfile == id
		oldSource, readErr := profile.ReadSource(st, id)
		if readErr != nil {
			return operationFailure{"store", readErr}
		}
		if active {
			oldState, hadState, stateErr := snapshot(st.StatePath())
			if stateErr != nil {
				return operationFailure{"store", stateErr}
			}
			oldRuntime, runtimeSnapshotErr := snapshotRuntime()
			if runtimeSnapshotErr != nil {
				return operationFailure{"store", runtimeSnapshotErr}
			}
			if applyErr := applyLocked(id, meta, result.Body); applyErr != nil {
				return operationFailure{"apply", applyErr}
			}
			if writeErr := st.WriteAtomic(st.ProfilePath(id, "source.yaml"), result.Body); writeErr != nil {
				if rollbackErr := restoreAppliedRuntime(oldSource, id, oldState, hadState, oldRuntime); rollbackErr != nil {
					return operationFailure{"rollback", fmt.Errorf("%w; rollback: %v", writeErr, rollbackErr)}
				}
				return operationFailure{"store", writeErr}
			}
			if saveErr := profile.SaveMeta(st, newMeta); saveErr != nil {
				sourceErr := st.WriteAtomic(st.ProfilePath(id, "source.yaml"), oldSource)
				rollbackErr := restoreAppliedRuntime(oldSource, id, oldState, hadState, oldRuntime)
				if sourceErr != nil || rollbackErr != nil {
					return operationFailure{"rollback", rollbackErrors(saveErr, rollbackErr, sourceErr)}
				}
				return operationFailure{"store", saveErr}
			}
			return nil
		}
		if writeErr := st.WriteAtomic(st.ProfilePath(id, "source.yaml"), result.Body); writeErr != nil {
			return operationFailure{"store", writeErr}
		}
		if saveErr := profile.SaveMeta(st, newMeta); saveErr != nil {
			if restoreErr := st.WriteAtomic(st.ProfilePath(id, "source.yaml"), oldSource); restoreErr != nil {
				return operationFailure{"rollback", fmt.Errorf("%w; source rollback: %v", saveErr, restoreErr)}
			}
			return operationFailure{"store", saveErr}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "updated": true, "active": active}, nil
}

func snapshotRuntime() (runtimeSnapshot, error) {
	var out runtimeSnapshot
	var err error
	if out.current.data, out.current.existed, err = snapshot(st.CurrentPath()); err != nil {
		return out, err
	}
	if out.previous.data, out.previous.existed, err = snapshot(st.PreviousPath()); err != nil {
		return out, err
	}
	if out.candidate.data, out.candidate.existed, err = snapshot(st.CandidatePath()); err != nil {
		return out, err
	}
	return out, nil
}

type runtimeFileSnapshot struct {
	data    []byte
	existed bool
}

type runtimeSnapshot struct {
	current, previous, candidate runtimeFileSnapshot
}

type restoreMode struct {
	applyPreviousIfCurrentMissing bool
}

func restoreRuntime(old runtimeSnapshot) error {
	return restoreRuntimeWithFallback(old, restoreMode{})
}

func restoreRuntimeFiles(old runtimeSnapshot) error {
	var errs []error
	if err := restoreSnapshot(st.PreviousPath(), old.previous.data, old.previous.existed); err != nil {
		errs = append(errs, err)
	}
	if err := restoreSnapshot(st.CandidatePath(), old.candidate.data, old.candidate.existed); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return rollbackErrors(fmt.Errorf("runtime file restoration failed"), errs...)
	}
	return nil
}

func restoreRuntimeWithFallback(old runtimeSnapshot, mode restoreMode) error {
	var errs []error
	if old.current.existed {
		if err := st.WriteAtomic(st.CurrentPath(), old.current.data); err != nil {
			errs = append(errs, err)
		} else if err := core.Apply(st.CurrentPath()); err != nil {
			errs = append(errs, err)
		}
	} else {
		if mode.applyPreviousIfCurrentMissing {
			// current.yaml did not exist before this transaction. backupRuntime
			// copied the live core config to previous.yaml.
			if err := applyPrevious(); err != nil {
				errs = append(errs, err)
			}
		}
		if err := os.Remove(st.CurrentPath()); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	if err := restoreRuntimeFiles(old); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return rollbackErrors(fmt.Errorf("runtime restoration failed"), errs...)
	}
	return nil
}

func restoreAppliedRuntime(oldSource []byte, id string, oldState []byte, hadState bool, oldRuntime runtimeSnapshot) error {
	var errs []error
	if id != "" && oldSource != nil {
		if err := st.WriteAtomic(st.ProfilePath(id, "source.yaml"), oldSource); err != nil {
			errs = append(errs, err)
		}
	}
	if err := restoreRuntimeWithFallback(oldRuntime, restoreMode{applyPreviousIfCurrentMissing: true}); err != nil {
		errs = append(errs, err)
	}
	if err := restoreSnapshot(st.StatePath(), oldState, hadState); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return rollbackErrors(fmt.Errorf("runtime restoration failed"), errs...)
	}
	return nil
}

func configCommand(args []string) {
	if len(args) == 0 {
		fail("args", fmt.Errorf("config command required"))
	}
	switch args[0] {
	case "compile":
		requireID(args)
		err := runLocked(func() error {
			b, err := compile(args[1], nil)
			if err != nil {
				return err
			}
			return st.WriteAtomic(st.CandidatePath(), b)
		})
		if err != nil {
			fail("compile", err)
		}
		emit(map[string]any{"ok": true, "stage": "compile", "path": st.CandidatePath()})
	case "apply":
		requireID(args)
		selectProfile(args[1])
	case "rollback":
		rollback()
	default:
		fail("args", fmt.Errorf("unknown config command: %s", args[0]))
	}
}

func saveUpdatedSource(s *store.Store, id string, body []byte) error {
	if _, err := config.Parse(body); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	return s.WriteAtomic(s.ProfilePath(id, "source.yaml"), body)
}

func compile(id string, source []byte) ([]byte, error) {
	if source == nil {
		var err error
		source, err = profile.ReadSource(st, id)
		if err != nil {
			return nil, err
		}
	}
	override, err := profile.ReadOverride(st, id)
	if err != nil {
		return nil, err
	}
	global, err := os.ReadFile(st.GlobalOverridePath())
	if os.IsNotExist(err) {
		global = nil
	} else if err != nil {
		return nil, err
	}
	return compileBytes(source, global, override)
}
func compileBytes(source, global, override []byte) ([]byte, error) {
	settings, err := profile.LoadSettings(st)
	if err != nil {
		return nil, err
	}
	protected := map[string]any{}
	info, infoErr := core.CoreInfo()
	if infoErr == nil && info.ConfigPath != "" {
		protected, err = config.ReadProtected(info.ConfigPath)
		if err != nil {
			return nil, err
		}
	}
	// Adding a profile validates the source before a running core is required;
	// an existing core is still mandatory for select/apply/reconcile.
	return (config.Compiler{Settings: settings, Protected: protected}).Compile(source, global, override)
}
func validateCompiled(source, override []byte) error {
	b, err := compileBytes(source, nil, override)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(st.RuntimeDir(), ".validate-*.yaml")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		binary := core.Binary()
		// A profile may be added while the core is stopped. Parse and compile
		// still protect the store in that case; apply/reconcile perform the
		// mandatory Mihomo validation once a core/binary is available.
		if binary != "" {
			err = validator.Validate(binary, path)
		}
	}
	return err
}

func applyLocked(id string, meta profile.Meta, source []byte) error {
	oldState, hadState, err := snapshot(st.StatePath())
	if err != nil {
		return err
	}
	oldRuntime, err := snapshotRuntime()
	if err != nil {
		return err
	}
	b, err := compile(id, source)
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}
	if err = st.WriteAtomic(st.CandidatePath(), b); err != nil {
		return err
	}
	if err = validator.Validate(core.Binary(), st.CandidatePath()); err != nil {
		fileErr := restoreRuntimeFiles(oldRuntime)
		if fileErr != nil {
			return rollbackErrors(fmt.Errorf("validate: %w", err), fileErr)
		}
		return fmt.Errorf("validate: %w", err)
	}
	if err = backupRuntime(); err != nil {
		primary := fmt.Errorf("backup: %w", err)
		if fileErr := restoreRuntimeFiles(oldRuntime); fileErr != nil {
			return rollbackErrors(primary, fileErr)
		}
		return primary
	}

	restore := func(primary error) error {
		runtimeErr := restoreRuntimeWithFallback(oldRuntime, restoreMode{applyPreviousIfCurrentMissing: true})
		stateErr := restoreSnapshot(st.StatePath(), oldState, hadState)
		if runtimeErr != nil || stateErr != nil {
			return rollbackErrors(primary, runtimeErr, stateErr)
		}
		return primary
	}
	if err = core.Apply(st.CandidatePath()); err != nil {
		return restore(fmt.Errorf("apply: %w", err))
	}
	if err = st.WriteAtomic(st.CurrentPath(), b); err != nil {
		return restore(fmt.Errorf("promote runtime: %w", err))
	}
	if err = st.SaveRuntimeState(store.RuntimeState{ActiveProfile: id, LastAppliedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		return restore(fmt.Errorf("save runtime state: %w", err))
	}
	return nil
}
func restorePreviousRuntime() error {
	// Read before applying: if the backup disappeared, do not leave the core
	// on the previous config while current.yaml still points at the candidate.
	previous, err := os.ReadFile(st.PreviousPath())
	if err != nil {
		return err
	}
	if err = applyPrevious(); err != nil {
		return err
	}
	if err = st.WriteAtomic(st.CurrentPath(), previous); err != nil {
		return err
	}
	return nil
}
func backupRuntime() error {
	if b, err := os.ReadFile(st.CurrentPath()); err == nil {
		return st.WriteAtomic(st.PreviousPath(), b)
	} else if !os.IsNotExist(err) {
		return err
	}
	info, err := core.CoreInfo()
	if err != nil || info.ConfigPath == "" {
		if err != nil {
			return err
		}
		return fmt.Errorf("no current runtime to back up")
	}
	b, err := os.ReadFile(info.ConfigPath)
	if err != nil {
		return err
	}
	return st.WriteAtomic(st.PreviousPath(), b)
}
func applyPrevious() error { return core.Apply(st.PreviousPath()) }
func rollback() {
	err := runLocked(func() error {
		if err := validator.Validate(core.Binary(), st.PreviousPath()); err != nil {
			return fmt.Errorf("validate: %w", err)
		}
		if err := applyPrevious(); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
		b, err := os.ReadFile(st.PreviousPath())
		if err != nil {
			return err
		}
		return st.WriteAtomic(st.CurrentPath(), b)
	})
	if err != nil {
		fail("rollback", err)
	}
	ok(map[string]bool{"rolledBack": true})
}
func reconcile() {
	err := runLocked(func() error {
		idx, err := profile.LoadIndex(st)
		if err != nil {
			return err
		}
		if idx.ActiveProfile == "" {
			return nil
		}
		meta, err := profile.LoadMeta(st, idx.ActiveProfile)
		if err != nil {
			return err
		}
		return applyLocked(idx.ActiveProfile, meta, nil)
	})
	if err != nil {
		fail("reconcile", err)
	}
	idx, err := profile.LoadIndex(st)
	if err != nil {
		fail("store", err)
	}
	ok(map[string]bool{"reconciled": idx.ActiveProfile != ""})
}

func overrideCommand(args []string) {
	if len(args) == 0 {
		fail("args", fmt.Errorf("override command required"))
	}
	switch args[0] {
	case "global":
		b, err := os.ReadFile(st.GlobalOverridePath())
		if os.IsNotExist(err) {
			b = []byte("{}\n")
		} else if err != nil {
			fail("store", err)
		}
		emit(map[string]any{"ok": true, "scope": "global", "override": string(b)})
	case "profile":
		if len(args) < 2 || !store.ValidID(args[1]) {
			fail("args", fmt.Errorf("valid profile id required"))
		}
		b, err := profile.ReadOverride(st, args[1])
		if err != nil {
			fail("store", err)
		}
		emit(map[string]any{"ok": true, "scope": "profile", "id": args[1], "override": string(b)})
	case "open-global":
		openOverride(st.GlobalOverridePath())
	case "open-profile":
		if len(args) < 2 || !store.ValidID(args[1]) {
			fail("args", fmt.Errorf("valid profile id required"))
		}
		openOverride(st.ProfilePath(args[1], "override.yaml"))
	default:
		fail("args", fmt.Errorf("unknown override command: %s", args[0]))
	}
}
func openOverride(path string) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if err = st.WriteAtomic(path, []byte("{}\n")); err != nil {
				fail("store", err)
			}
		} else {
			fail("store", err)
		}
	}
	launcher := ""
	for _, candidate := range []string{"omarchy-launch-config-editor", "omarchy-launch-editor", "xdg-open"} {
		if _, err := exec.LookPath(candidate); err == nil {
			launcher = candidate
			break
		}
	}
	if launcher == "" {
		fail("editor", fmt.Errorf("no editor launcher found"))
	}
	if err := exec.Command("setsid", launcher, path).Start(); err != nil {
		fail("editor", err)
	}
	ok(map[string]string{"path": path})
}
func settingsCommand(args []string) {
	if len(args) == 0 || args[0] == "get" {
		settings, err := profile.LoadSettings(st)
		if err != nil {
			fail("settings", err)
		}
		ok(settings)
		return
	}
	if args[0] != "set" || len(args) < 3 {
		fail("args", fmt.Errorf("settings set key value required"))
	}
	old, err := profile.LoadSettings(st)
	if err != nil {
		fail("settings", err)
	}
	next := old
	switch args[1] {
	case "dns-management":
		next.DNSManagement = args[2]
	case "tun-management":
		next.TUNManagement = args[2]
	default:
		fail("args", fmt.Errorf("unsupported setting: %s", args[1]))
	}
	if next.DNSManagement != "managed" && next.DNSManagement != "inherit" {
		fail("args", fmt.Errorf("dns-management must be managed or inherit"))
	}
	if next.TUNManagement != "managed" && next.TUNManagement != "inherit" {
		fail("args", fmt.Errorf("tun-management must be managed or inherit"))
	}
	idx, err := profile.LoadIndex(st)
	if err != nil {
		fail("store", err)
	}
	err = runLocked(func() error {
		if err := profile.SaveSettings(st, next); err != nil {
			return err
		}
		if idx.ActiveProfile == "" {
			return nil
		}
		meta, err := profile.LoadMeta(st, idx.ActiveProfile)
		if err != nil {
			_ = profile.SaveSettings(st, old)
			return err
		}
		if err = applyLocked(idx.ActiveProfile, meta, nil); err != nil {
			if restoreErr := profile.SaveSettings(st, old); restoreErr != nil {
				return fmt.Errorf("%w; settings rollback: %v", err, restoreErr)
			}
			return err
		}
		return nil
	})
	if err != nil {
		fail("apply", err)
	}
	ok(next)
}

func currentActive() string { idx, _ := profile.LoadIndex(st); return idx.ActiveProfile }
func redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		if i := strings.IndexByte(raw, '?'); i >= 0 {
			return raw[:i] + "?••••••••"
		}
		return raw
	}
	if u.RawQuery != "" {
		u.RawQuery = "••••••••"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "••••••••")
	}
	return u.String()
}
