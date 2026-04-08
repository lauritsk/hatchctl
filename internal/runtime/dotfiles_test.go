package runtime

import "testing"

func TestNormalizeDotfilesRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "username shorthand", input: "lauritsk", want: "https://github.com/lauritsk/dotfiles.git"},
		{name: "owner repo shorthand", input: "lauritsk/dotfiles", want: "https://github.com/lauritsk/dotfiles.git"},
		{name: "host username shorthand", input: "gitlab.com/lauritsk", want: "https://gitlab.com/lauritsk/dotfiles.git"},
		{name: "host repo shorthand", input: "gitlab.com/lauritsk/dotfiles", want: "https://gitlab.com/lauritsk/dotfiles.git"},
		{name: "github host username shorthand", input: "github.com/lauritsk", want: "https://github.com/lauritsk/dotfiles.git"},
		{name: "github host shorthand", input: "github.com/lauritsk/dotfiles", want: "https://github.com/lauritsk/dotfiles.git"},
		{name: "sourcehut username shorthand", input: "sr.ht/~lauritsk", want: "https://git.sr.ht/~lauritsk/dotfiles.git"},
		{name: "sourcehut repo shorthand", input: "sr.ht/~lauritsk/dotfiles", want: "https://git.sr.ht/~lauritsk/dotfiles.git"},
		{name: "full url", input: "https://github.com/lauritsk/dotfiles.git", want: "https://github.com/lauritsk/dotfiles.git"},
		{name: "full url without git suffix", input: "https://gitlab.com/lauritsk/dotfiles", want: "https://gitlab.com/lauritsk/dotfiles.git"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeDotfilesRepository(tc.input)
			if err != nil {
				t.Fatalf("normalize repo: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected repository %q want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeDotfilesOptions(t *testing.T) {
	t.Parallel()

	got, err := (DotfilesOptions{Repository: "github.com/lauritsk/dotfiles", TargetPath: "~/dotfiles"}).Normalized()
	if err != nil {
		t.Fatalf("normalize options: %v", err)
	}
	if got.Repository != "https://github.com/lauritsk/dotfiles.git" {
		t.Fatalf("unexpected repository %q", got.Repository)
	}
	if got.TargetPath != "$HOME/dotfiles" {
		t.Fatalf("unexpected target path %q", got.TargetPath)
	}
}

func TestNormalizeDotfilesRejectsLocalPaths(t *testing.T) {
	t.Parallel()

	if _, err := normalizeDotfilesRepository("./dotfiles"); err == nil {
		t.Fatal("expected local repository path to be rejected")
	}
}

func TestNormalizeDotfilesRejectsInvalidRepository(t *testing.T) {
	t.Parallel()

	if _, err := normalizeDotfilesRepository("owner/repo/extra"); err == nil {
		t.Fatal("expected invalid repository to be rejected")
	}
}
