package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

const passwdFilePath = "/etc/passwd"

func (r *Runner) dockerExecArgs(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, stdin bool, tty bool, extraEnv map[string]string, command []string) ([]string, error) {
	user, err := r.effectiveExecUser(ctx, containerID, resolved)
	if err != nil {
		return nil, err
	}
	env, err := r.execRemoteEnv(ctx, containerID, resolved, user, extraEnv)
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
	for _, key := range devcontainer.SortedMapKeys(env) {
		args = append(args, "-e", key+"="+env[key])
	}
	args = append(args, containerID)
	args = append(args, command...)
	return args, nil
}

func (r *Runner) effectiveExecUser(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig) (string, error) {
	if user := firstNonEmpty(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser); user != "" {
		return user, nil
	}
	if containerID == "" {
		return "", nil
	}
	inspect, err := r.backend.InspectContainer(ctx, containerID)
	if err != nil {
		return "", err
	}
	return inspect.Config.User, nil
}

func (r *Runner) execRemoteEnv(ctx context.Context, containerID string, resolved devcontainer.ResolvedConfig, user string, extraEnv map[string]string) (map[string]string, error) {
	env := make(map[string]string, len(resolved.Merged.RemoteEnv)+len(extraEnv)+1)
	for key, value := range resolved.Merged.RemoteEnv {
		env[key] = value
	}
	for key, value := range extraEnv {
		env[key] = value
	}
	if _, ok := env["HOME"]; ok {
		return env, nil
	}
	home, err := r.resolveExecHome(ctx, containerID, user)
	if err != nil {
		return nil, err
	}
	if home != "" {
		env["HOME"] = home
	}
	return env, nil
}

func (r *Runner) resolveExecHome(ctx context.Context, containerID string, user string) (string, error) {
	args := []string{"exec"}
	if user != "" {
		args = append(args, "-u", user)
	}
	args = append(args, containerID, "cat", passwdFilePath)
	passwd, err := r.backend.Output(ctx, runtimeCommand{Kind: runtimeCommandDocker, Args: args})
	if err != nil {
		return "", fmt.Errorf("resolve home for container user %q: %w", firstNonEmpty(user, "default"), err)
	}
	home, _ := homeFromPasswd(passwd, user)
	return home, nil
}

func homeFromPasswd(passwd string, user string) (string, bool) {
	lookupName, lookupUID := passwdLookup(user)
	for _, line := range strings.Split(passwd, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		if lookupUID != "" {
			if fields[2] == lookupUID {
				return fields[5], true
			}
			continue
		}
		if fields[0] == lookupName {
			return fields[5], true
		}
	}
	return "", false
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

func effectiveRemoteUserFromContainerInspect(inspect *docker.ContainerInspect, resolved devcontainer.ResolvedConfig) string {
	if user := firstNonEmpty(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser); user != "" {
		return user
	}
	if inspect != nil {
		return inspect.Config.User
	}
	return ""
}
