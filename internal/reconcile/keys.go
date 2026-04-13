package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
)

func ManagedImageKey(resolved devcontainer.ResolvedConfig, targetImage string) (string, error) {
	h := sha256.New()
	writeKeyValue(h, "target-image", targetImage)
	writeKeyValue(h, "source-kind", resolved.SourceKind)
	writeKeyValue(h, "config-path", resolved.ConfigPath)
	writeKeyValue(h, "workspace-folder", resolved.WorkspaceFolder)
	writeKeyValue(h, "image-name", resolved.ImageName)
	writeKeyValue(h, "compose-project", resolved.ComposeProject)
	writeKeyValue(h, "compose-service", resolved.ComposeService)
	writeKeyValue(h, "base-image", resolved.Config.Image)
	for _, path := range resolved.ComposeFiles {
		writeKeyValue(h, "compose-file", filepath.Clean(path))
		if err := hashFile(h, path); err != nil {
			return "", err
		}
	}
	if resolved.SourceKind != "compose" {
		dockerfile := filepath.Join(resolved.ConfigDir, spec.ResolvedDockerfile(resolved.ConfigDir, resolved.Config))
		writeKeyValue(h, "dockerfile", filepath.Clean(dockerfile))
		if err := hashFile(h, dockerfile); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		contextDir := filepath.Join(resolved.ConfigDir, spec.EffectiveContext(resolved.Config))
		writeKeyValue(h, "context-dir", filepath.Clean(contextDir))
		if err := hashDir(h, contextDir); err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	for _, feature := range resolved.Features {
		writeKeyValue(h, "feature-source", feature.Source)
		writeKeyValue(h, "feature-resolved", feature.Resolved)
		writeKeyValue(h, "feature-integrity", feature.Integrity)
		writeKeyValue(h, "feature-path", feature.Path)
		if err := hashJSON(h, feature.Options); err != nil {
			return "", err
		}
		if feature.Path != "" {
			if err := hashDir(h, feature.Path); err != nil && !os.IsNotExist(err) {
				return "", err
			}
		}
	}
	return digest(h), nil
}

func ContainerKey(resolved devcontainer.ResolvedConfig, imageIdentity string, bridgeEnabled bool, sshAgent bool) (string, error) {
	h := sha256.New()
	writeKeyValue(h, "image", imageIdentity)
	writeKeyValue(h, "source-kind", resolved.SourceKind)
	writeKeyValue(h, "container-name", resolved.ContainerName)
	writeKeyValue(h, "compose-project", resolved.ComposeProject)
	writeKeyValue(h, "compose-service", resolved.ComposeService)
	writeKeyValue(h, "workspace-mount", resolved.WorkspaceMount)
	writeKeyValue(h, "remote-workspace", resolved.RemoteWorkspace)
	writeKeyValue(h, "bridge-enabled", fmt.Sprintf("%t", bridgeEnabled))
	writeKeyValue(h, "ssh-agent", fmt.Sprintf("%t", sshAgent))
	writeKeyValue(h, "container-user", resolved.Merged.ContainerUser)
	for _, mount := range resolved.Merged.Mounts {
		writeKeyValue(h, "mount", mount)
	}
	for _, cap := range resolved.Merged.CapAdd {
		writeKeyValue(h, "cap-add", cap)
	}
	for _, sec := range resolved.Merged.SecurityOpt {
		writeKeyValue(h, "security-opt", sec)
	}
	for _, arg := range resolved.Config.RunArgs {
		writeKeyValue(h, "run-arg", arg)
	}
	for _, part := range spec.ContainerCommand(resolved.Config) {
		writeKeyValue(h, "command", part)
	}
	writeKeyValue(h, "init", fmt.Sprintf("%t", resolved.Merged.Init))
	writeKeyValue(h, "privileged", fmt.Sprintf("%t", resolved.Merged.Privileged))
	if err := hashJSON(h, resolved.Merged.ContainerEnv); err != nil {
		return "", err
	}
	if err := hashJSON(h, resolved.Labels); err != nil {
		return "", err
	}
	return digest(h), nil
}

func LifecycleKey(resolved devcontainer.ResolvedConfig, containerKey string, dotfiles DotfilesConfig) (string, error) {
	h := sha256.New()
	writeKeyValue(h, "container-key", containerKey)
	for _, command := range resolved.Merged.OnCreateCommands {
		if err := hashJSON(h, command); err != nil {
			return "", err
		}
	}
	for _, command := range resolved.Merged.UpdateContentCommands {
		if err := hashJSON(h, command); err != nil {
			return "", err
		}
	}
	for _, command := range resolved.Merged.PostCreateCommands {
		if err := hashJSON(h, command); err != nil {
			return "", err
		}
	}
	for _, command := range resolved.Merged.PostStartCommands {
		if err := hashJSON(h, command); err != nil {
			return "", err
		}
	}
	for _, command := range resolved.Merged.PostAttachCommands {
		if err := hashJSON(h, command); err != nil {
			return "", err
		}
	}
	if err := hashJSON(h, resolved.Config.InitializeCommand); err != nil {
		return "", err
	}
	if err := hashJSON(h, dotfiles); err != nil {
		return "", err
	}
	return digest(h), nil
}

func digest(h hash.Hash) string {
	return hex.EncodeToString(h.Sum(nil))
}

func writeKeyValue(h hash.Hash, key string, value string) {
	_, _ = io.WriteString(h, key)
	_, _ = io.WriteString(h, "=")
	_, _ = io.WriteString(h, value)
	_, _ = io.WriteString(h, "\n")
}

func hashJSON(h hash.Hash, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = h.Write(data)
	return err
}

func hashFile(h hash.Hash, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(h, file)
	return err
}

func hashDir(h hash.Hash, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		writeKeyValue(h, "path", rel)
		if d.IsDir() {
			return nil
		}
		if err := hashFile(h, path); err != nil {
			return err
		}
		return nil
	})
}
