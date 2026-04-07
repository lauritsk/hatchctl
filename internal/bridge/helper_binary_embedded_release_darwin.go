//go:build darwin && release

package bridge

import _ "embed"

var (
	//go:embed embedded/hatchctl-linux-amd64
	embeddedHelperBinaryLinuxAMD64 []byte
	//go:embed embedded/hatchctl-linux-arm64
	embeddedHelperBinaryLinuxARM64 []byte
)

func embeddedHelperBinary(arch string) ([]byte, bool) {
	switch arch {
	case "amd64":
		return embeddedHelperBinaryLinuxAMD64, len(embeddedHelperBinaryLinuxAMD64) > 0
	case "arm64":
		return embeddedHelperBinaryLinuxARM64, len(embeddedHelperBinaryLinuxARM64) > 0
	default:
		return nil, false
	}
}
