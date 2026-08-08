//go:build !windows

package pathsafety

import "os"

func hasLinkOrReparse(info os.FileInfo) bool {
	return info != nil && info.Mode()&os.ModeSymlink != 0
}
