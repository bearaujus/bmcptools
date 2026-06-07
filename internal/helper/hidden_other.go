//go:build !windows

package helper

func pathHasHiddenAttribute(string) bool {
	return false
}
