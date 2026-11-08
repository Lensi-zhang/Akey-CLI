//go:build !windows

package config

import (
	"errors"
	"os"
)

func atomicReplace(source, destination string) error {
	return os.Rename(source, destination)
}

func isLockContention(err error) bool {
	return errors.Is(err, os.ErrExist)
}
