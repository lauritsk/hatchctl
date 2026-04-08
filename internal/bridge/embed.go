package bridge

import _ "embed"

var embeddedHelpers = map[string][]byte{
	"amd64": embeddedLinuxAMD64,
	"arm64": embeddedLinuxARM64,
}

//go:embed assets/linux_amd64/hatchctl
var embeddedLinuxAMD64 []byte

//go:embed assets/linux_arm64/hatchctl
var embeddedLinuxARM64 []byte
