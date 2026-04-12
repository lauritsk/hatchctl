package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type OutputRoots struct {
	StateRoot string
	CacheRoot string
}

func ResolveOutputRoots(stateRoot string, cacheRoot string) (OutputRoots, error) {
	roots, err := DefaultOutputRoots()
	if err != nil {
		return OutputRoots{}, err
	}
	if stateRoot != "" {
		roots.StateRoot = stateRoot
	}
	if cacheRoot != "" {
		roots.CacheRoot = cacheRoot
	}
	return roots, nil
}

func WorkspaceOutputDirs(stateRoot string, cacheRoot string, workspace string, configPath string) (OutputRoots, string, string, error) {
	roots, err := ResolveOutputRoots(stateRoot, cacheRoot)
	if err != nil {
		return OutputRoots{}, "", "", err
	}
	return roots, WorkspaceScopedDir(roots.StateRoot, workspace, configPath), WorkspaceScopedDir(roots.CacheRoot, workspace, configPath), nil
}

func DefaultOutputRoots() (OutputRoots, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return OutputRoots{}, err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return OutputRoots{}, err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return OutputRoots{}, err
	}
	return outputRoots(runtime.GOOS, homeDir, configDir, cacheDir, os.Getenv("XDG_STATE_HOME")), nil
}

func OutputRootsForPlatform(goos string, homeDir string, configDir string, cacheDir string, xdgStateHome string) OutputRoots {
	return outputRoots(goos, homeDir, configDir, cacheDir, xdgStateHome)
}

func WorkspaceStateDir(workspace string, configPath string) (string, error) {
	roots, err := DefaultOutputRoots()
	if err != nil {
		return "", err
	}
	return WorkspaceScopedDir(roots.StateRoot, workspace, configPath), nil
}

func WorkspaceCacheDir(workspace string, configPath string) (string, error) {
	roots, err := DefaultOutputRoots()
	if err != nil {
		return "", err
	}
	return WorkspaceScopedDir(roots.CacheRoot, workspace, configPath), nil
}

func WorkspaceScopedDir(root string, workspace string, configPath string) string {
	key := hashKey(workspace + "\n" + configPath)
	return filepath.Join(root, "workspaces", key)
}

func ContainerName(workspace string, configPath string) string {
	return fmt.Sprintf("hatchctl-%s", hashKey(workspace+"\n"+configPath))
}

func ImageName(workspace string, configPath string) string {
	return fmt.Sprintf("hatchctl-%s", hashKey(workspace+"\n"+configPath))
}

func outputRoots(goos string, homeDir string, configDir string, cacheDir string, xdgStateHome string) OutputRoots {
	configRoot := filepath.Join(configDir, "hatchctl")
	cacheRoot := filepath.Join(cacheDir, "hatchctl")
	stateRoot := configRoot
	if goos != "darwin" {
		if xdgStateHome != "" {
			stateRoot = filepath.Join(xdgStateHome, "hatchctl")
		} else if homeDir != "" {
			stateRoot = filepath.Join(homeDir, ".local", "state", "hatchctl")
		}
	}
	return OutputRoots{StateRoot: stateRoot, CacheRoot: cacheRoot}
}

func hashKey(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:8])
}
