//go:build !windows

package update

import "os"

func replaceFile(source, target string) error {
	return os.Rename(source, target)
}
