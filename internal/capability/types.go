package capability

type Set struct {
	SSHAgent SSHAgent
	UIDRemap UIDRemap
	Dotfiles Dotfiles
	Bridge   Bridge
}

type SSHAgent struct {
	Enabled bool
}

type UIDRemap struct {
	Enabled bool
}

type Dotfiles struct {
	Repository     string
	InstallCommand string
	TargetPath     string
}

func (d Dotfiles) Enabled() bool {
	return d.Repository != ""
}

type Bridge struct {
	Enabled bool
}
