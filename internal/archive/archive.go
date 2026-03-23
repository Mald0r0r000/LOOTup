package archive

import (
	"fmt"
	"os/exec"
)

// Run executes rclone sync from source to dest
func Run(source, dest string, dryRun bool) error {
	// Verify rclone is available
	if _, err := exec.LookPath("rclone"); err != nil {
		return fmt.Errorf("rclone not found in PATH: %w — install it: https://rclone.org/install/", err)
	}

	args := []string{"sync", source, dest, "--progress", "--stats-one-line"}
	if dryRun {
		args = append(args, "--dry-run")
	}

	cmd := exec.Command("rclone", args...)
	cmd.Stdout = nil // TODO: capture for TUI progress
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rclone sync failed: %w", err)
	}

	return nil
}
