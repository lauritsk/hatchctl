package reconcile

import (
	"context"
	"fmt"
	"strings"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
)

const passwdFilePath = "/etc/passwd"

func (e *Executor) ExecRequest(ctx context.Context, observed ObservedState, stdin bool, tty bool, extraEnv map[string]string, command []string, streams backend.Streams) (backend.ExecRequest, error) {
	containerID := observed.Target.PrimaryContainer
	user, err := e.effectiveExecUser(ctx, observed)
	if err != nil {
		return backend.ExecRequest{}, err
	}
	command, err = e.execCommand(ctx, observed, user, command)
	if err != nil {
		return backend.ExecRequest{}, err
	}
	env, err := e.execRemoteEnv(ctx, observed, user, extraEnv)
	if err != nil {
		return backend.ExecRequest{}, err
	}
	return backend.ExecRequest{ContainerID: containerID, User: user, Workdir: observed.Resolved.RemoteWorkspace, Interactive: stdin, TTY: tty, Env: env, Command: command, Streams: streams}, nil
}

func (e *Executor) execCommand(ctx context.Context, observed ObservedState, user string, command []string) ([]string, error) {
	if len(command) != 0 {
		return command, nil
	}
	shell, err := e.resolveExecShell(ctx, observed, user)
	if err != nil {
		return nil, err
	}
	return []string{spec.FirstNonEmptyString(shell, "/bin/sh")}, nil
}

func (e *Executor) effectiveExecUser(ctx context.Context, observed ObservedState) (string, error) {
	if user := spec.FirstNonEmptyString(observed.Resolved.Merged.RemoteUser, observed.Resolved.Merged.ContainerUser); user != "" {
		return user, nil
	}
	if observed.Container != nil {
		return observed.Container.Config.User, nil
	}
	containerID := observed.Target.PrimaryContainer
	if containerID == "" {
		return "", nil
	}
	inspect, err := e.engine.InspectContainer(ctx, containerID)
	if err != nil {
		return "", err
	}
	return inspect.Config.User, nil
}

func (e *Executor) execRemoteEnv(ctx context.Context, observed ObservedState, user string, extraEnv map[string]string) (map[string]string, error) {
	env := make(map[string]string, len(observed.Resolved.Merged.RemoteEnv)+len(extraEnv)+1)
	for key, value := range observed.Resolved.Merged.RemoteEnv {
		env[key] = value
	}
	for key, value := range extraEnv {
		env[key] = value
	}
	if _, ok := env["HOME"]; ok {
		return env, nil
	}
	home, err := e.resolveExecHome(ctx, observed, user)
	if err != nil {
		return nil, err
	}
	if home != "" {
		env["HOME"] = home
	}
	return env, nil
}

func (e *Executor) resolveExecShell(ctx context.Context, observed ObservedState, user string) (string, error) {
	entry, found, err := e.lookupExecUserEntry(ctx, observed, user)
	if err != nil {
		return "", err
	}
	if !found || entry.Shell == "" {
		return "/bin/sh", nil
	}
	return entry.Shell, nil
}

func (e *Executor) resolveExecHome(ctx context.Context, observed ObservedState, user string) (string, error) {
	entry, found, err := e.lookupExecUserEntry(ctx, observed, user)
	if err != nil {
		return "", err
	}
	if !found || entry.Home == "" {
		return fallbackExecHome(observed, user), nil
	}
	return entry.Home, nil
}

func (e *Executor) lookupExecUserEntry(ctx context.Context, observed ObservedState, user string) (passwdEntry, bool, error) {
	containerID := observed.Target.PrimaryContainer
	passwd, err := e.engine.ExecOutput(ctx, backend.ExecRequest{ContainerID: containerID, User: user, Command: []string{"cat", passwdFilePath}})
	if err != nil {
		return passwdEntry{}, false, fmt.Errorf("resolve passwd entry for container user %q: %w", spec.FirstNonEmptyString(user, "default"), err)
	}
	entry, found := passwdEntryFromPasswd(passwd, user)
	return entry, found, nil
}

type passwdEntry struct {
	Home  string
	Shell string
}

func passwdEntryFromPasswd(passwd string, user string) (passwdEntry, bool) {
	lookupName, lookupUID := passwdLookup(user)
	for _, line := range strings.Split(passwd, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		if lookupUID != "" {
			if fields[2] == lookupUID {
				return passwdEntry{Home: fields[5], Shell: fields[6]}, true
			}
			continue
		}
		if fields[0] == lookupName {
			return passwdEntry{Home: fields[5], Shell: fields[6]}, true
		}
	}
	return passwdEntry{}, false
}

func passwdLookup(user string) (string, string) {
	name := user
	if before, _, ok := strings.Cut(user, ":"); ok {
		name = before
	}
	name = spec.FirstNonEmptyString(name, "root")
	if spec.IsNumericString(name) {
		return "", name
	}
	return name, ""
}

func fallbackExecHome(observed ObservedState, user string) string {
	if observed.Container != nil {
		for _, entry := range observed.Container.Config.Env {
			key, value, ok := strings.Cut(entry, "=")
			if ok && key == "HOME" && value != "" {
				return value
			}
		}
	}
	name, uid := passwdLookup(user)
	if uid == "0" || name == "root" {
		return "/root"
	}
	if name != "" && !spec.IsNumericString(name) {
		return "/home/" + name
	}
	return ""
}

func effectiveRemoteUserFromContainerInspect(inspect *backend.ContainerInspect, resolved devcontainer.ResolvedConfig) string {
	if user := spec.FirstNonEmptyString(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser); user != "" {
		return user
	}
	if inspect != nil {
		return inspect.Config.User
	}
	return ""
}
