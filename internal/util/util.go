package util

import (
	"fmt"
	"os"
	"path/filepath"
)

func IsDevice(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}

	switch {
	case fi.Mode()&os.ModeSymlink == os.ModeDevice:
		return true
	case fi.Mode()&os.ModeSymlink == os.ModeDevice:
		return true
	default:
		return false
	}
}

func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}

	if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		return true
	}

	return false
}

func FollowSymlink(path string) (string, error) {
	if !IsSymlink(path) {
		return "", fmt.Errorf("not a symlink")
	}

	symPath, err := os.Readlink(path)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(symPath) {
		symPath = filepath.Join(filepath.Dir(path), symPath)
	}

	_, err = os.Stat(symPath)
	if err != nil {
		return "", err
	}

	return symPath, nil
}
