package dotfiles

import (
	"testing"

	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func TestNormalizeExpandsShortRepositoryAndTarget(t *testing.T) {
	t.Parallel()

	cfg, err := Normalize(Config{Repository: "lauritsk", TargetPath: "dotfiles"})
	if err != nil {
		t.Fatalf("normalize dotfiles config: %v", err)
	}
	if cfg.Repository != "https://github.com/lauritsk/dotfiles.git" {
		t.Fatalf("unexpected repository %q", cfg.Repository)
	}
	if cfg.TargetPath != "$HOME/dotfiles" {
		t.Fatalf("unexpected target path %q", cfg.TargetPath)
	}
}

func TestNormalizeRejectsLocalRepositories(t *testing.T) {
	t.Parallel()

	if _, err := NormalizeRepository("./dotfiles"); err == nil {
		t.Fatal("expected local repository path to be rejected")
	}
}

func TestStatusForTracksWhetherInstallIsNeeded(t *testing.T) {
	t.Parallel()

	cfg := Config{Repository: "https://github.com/lauritsk/dotfiles.git", InstallCommand: "install.sh", TargetPath: "$HOME/.dotfiles"}
	state := storefs.WorkspaceState{DotfilesReady: true, DotfilesRepo: cfg.Repository, DotfilesInstall: cfg.InstallCommand, DotfilesTarget: cfg.TargetPath}
	status := StatusFor(state, cfg)
	if status == nil || status.NeedsInstall || !status.Applied {
		t.Fatalf("expected matching dotfiles state, got %#v", status)
	}
	state.DotfilesInstall = "other.sh"
	status = StatusFor(state, cfg)
	if status == nil || !status.NeedsInstall {
		t.Fatalf("expected mismatched dotfiles state to need install, got %#v", status)
	}
}
