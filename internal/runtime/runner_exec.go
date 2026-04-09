package runtime

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/reconcile"
)

const passwdFilePath = "/etc/passwd"

func (r *Runner) dockerExecArgs(ctx context.Context, observed reconcile.ObservedState, stdin bool, tty bool, extraEnv map[string]string, command []string) ([]string, error) {
	containerID := observed.Target.PrimaryContainer
	user, err := r.effectiveExecUser(ctx, observed)
	if err != nil {
		return nil, err
	}
	command, err = r.execCommand(ctx, observed, user, command)
	if err != nil {
		return nil, err
	}
	env, err := r.execRemoteEnv(ctx, observed, user, extraEnv)
	if err != nil {
		return nil, err
	}
	args := []string{"exec"}
	if stdin {
		args = append(args, "-i")
	}
	if tty {
		args = append(args, "-t")
	}
	if user != "" {
		args = append(args, "-u", user)
	}
	if observed.Resolved.RemoteWorkspace != "" {
		args = append(args, "--workdir", observed.Resolved.RemoteWorkspace)
	}
	for _, key := range sortedExecEnvKeys(env) {
		args = append(args, "-e", key+"="+env[key])
	}
	args = append(args, containerID)
	args = append(args, command...)
	return args, nil
}

func (r *Runner) execCommand(ctx context.Context, observed reconcile.ObservedState, user string, command []string) ([]string, error) {
	if len(command) != 0 {
		return command, nil
	}
	shell, err := r.resolveExecShell(ctx, observed, user)
	if err != nil {
		return nil, err
	}
	return []string{firstNonEmpty(shell, "/bin/sh")}, nil
}

func (r *Runner) effectiveExecUser(ctx context.Context, observed reconcile.ObservedState) (string, error) {
	if user := firstNonEmpty(observed.Resolved.Merged.RemoteUser, observed.Resolved.Merged.ContainerUser); user != "" {
		return user, nil
	}
	if observed.Container != nil {
		return observed.Container.Config.User, nil
	}
	containerID := observed.Target.PrimaryContainer
	if containerID == "" {
		return "", nil
	}
	inspect, err := r.backend.InspectContainer(ctx, containerID)
	if err != nil {
		return "", err
	}
	return inspect.Config.User, nil
}

func (r *Runner) execRemoteEnv(ctx context.Context, observed reconcile.ObservedState, user string, extraEnv map[string]string) (map[string]string, error) {
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
	home, err := r.resolveExecHome(ctx, observed, user)
	if err != nil {
		return nil, err
	}
	if home != "" {
		env["HOME"] = home
	}
	return env, nil
}

func (r *Runner) resolveExecShell(ctx context.Context, observed reconcile.ObservedState, user string) (string, error) {
	entry, err := r.lookupExecUserEntry(ctx, observed, user)
	if err != nil {
		return "", err
	}
	return entry.Shell, nil
}

func (r *Runner) resolveExecHome(ctx context.Context, observed reconcile.ObservedState, user string) (string, error) {
	entry, err := r.lookupExecUserEntry(ctx, observed, user)
	if err != nil {
		return "", err
	}
	return entry.Home, nil
}

func (r *Runner) lookupExecUserEntry(ctx context.Context, observed reconcile.ObservedState, user string) (passwdEntry, error) {
	containerID := observed.Target.PrimaryContainer
	args := []string{"exec"}
	if user != "" {
		args = append(args, "-u", user)
	}
	args = append(args, containerID, "cat", passwdFilePath)
	passwd, err := r.backend.Output(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args})
	if err != nil {
		return passwdEntry{}, fmt.Errorf("resolve passwd entry for container user %q: %w", firstNonEmpty(user, "default"), err)
	}
	entry, _ := passwdEntryFromPasswd(passwd, user)
	return entry, nil
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
	name = firstNonEmpty(name, "root")
	if isNumericUser(name) {
		return "", name
	}
	return name, ""
}

func sortedExecEnvKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	// Match existing stable output ordering.
	slices.Sort(keys)
	return keys
}

func effectiveRemoteUserFromContainerInspect(inspect *docker.ContainerInspect, resolved devcontainer.ResolvedConfig) string {
	if user := firstNonEmpty(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser); user != "" {
		return user
	}
	if inspect != nil {
		return inspect.Config.User
	}
	return ""
}
