package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func rootFSSetup(newRoot string) error {
	putOld := "/old_root"
	putOldAbsPath := filepath.Join(newRoot, putOld)

	// Mount root file system as a mountpoint, then it can be used to pivot_root
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to mount new root filesystem %s: %w", newRoot, err)
	}

	// Create putOld directory if it doesn't exist already
	if err := os.MkdirAll(putOldAbsPath, 0700); err != nil {
		return fmt.Errorf("failed to mkdir %s: %w", putOldAbsPath, err)
	}

	// pivot_root to the newRoot
	if err := syscall.PivotRoot(newRoot, putOldAbsPath); err != nil {
		return fmt.Errorf("failed to pivot_root(%s, %s): %w", newRoot, putOldAbsPath, err)
	}

	// change root directory
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir to /: %w", err)
	}

	// mount /proc
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /proc: %w", err)
	}

	// unmount the old root filesystem
	if err := syscall.Unmount(putOld, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount the old root filesystem %s: %w", putOld, err)
	}

	// remove old root filesystem
	if err := os.RemoveAll(putOld); err != nil {
		return fmt.Errorf("failed to remove old root filesystem %s: %w", putOld, err)
	}

	return nil
}
