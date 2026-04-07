//go:build !darwin || !release

package bridge

func embeddedHelperBinary(string) ([]byte, bool) {
	return nil, false
}
