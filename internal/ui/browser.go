package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// detectVolumes returns mounted volumes/media directories
func detectVolumes() []string {
	var volumes []string

	switch runtime.GOOS {
	case "darwin":
		volumes = listDirs("/Volumes")
	case "linux":
		volumes = append(listDirs("/media"), listDirs("/mnt")...)
	default:
		volumes = listDirs("/mnt")
	}

	sort.Strings(volumes)
	return volumes
}

// listDirs returns the full path of subdirectories in a directory
func listDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	return dirs
}

// dirEntry represents a directory in the browser
type dirEntry struct {
	Name string
	Path string
	IsUp bool // ".." entry
}

// listDirEntries returns subdirectories for the browser
func listDirEntries(dir string) []dirEntry {
	var entries []dirEntry

	// Add parent directory entry
	parent := filepath.Dir(dir)
	if parent != dir {
		entries = append(entries, dirEntry{
			Name: "..",
			Path: parent,
			IsUp: true,
		})
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		return entries
	}

	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}
		entries = append(entries, dirEntry{
			Name: item.Name(),
			Path: filepath.Join(dir, item.Name()),
		})
	}

	return entries
}
