package main

import (
	"fmt"
	"os"

	"github.com/Mald0r0r000/LOOTup/internal/archive"
	"github.com/Mald0r0r000/LOOTup/internal/config"
	"github.com/Mald0r0r000/LOOTup/internal/transfer"
	"github.com/Mald0r0r000/LOOTup/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// version will be set via ldflags during build
// Example: go build -ldflags "-X main.version=1.0.0"
var version = "dev"

func main() {
	cfg, err := config.ParseFlags(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Archive subcommand
	if cfg.ArchiveMode {
		if err := runArchive(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Dry run (CLI mode)
	if cfg.DryRun && !cfg.IsInteractive() {
		if err := runDryRun(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// TUI mode (interactive or CLI with progress)
	p := tea.NewProgram(ui.NewModel(cfg))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running lootup: %v\n", err)
		os.Exit(1)
	}
}

// runArchive handles the `lootup archive` subcommand
func runArchive(cfg *config.Config) error {
	fmt.Printf("=== LOOTup Archive ===\n")
	fmt.Printf("Source: %s\n", cfg.ArchiveSource)
	fmt.Printf("Dest:   %s\n", cfg.ArchiveDest)
	if cfg.DryRun {
		fmt.Println("Mode:   DRY RUN")
	}
	fmt.Println()

	return archive.Run(cfg.ArchiveSource, cfg.ArchiveDest, cfg.DryRun)
}

// runDryRun simulates a transfer without connecting
func runDryRun(cfg *config.Config) error {
	// For dry run we just scan the source directory
	fmt.Println("=== DRY RUN ===")
	fmt.Printf("Source:    %s\n", cfg.Source)
	fmt.Printf("Host:      %s\n", cfg.Host)
	fmt.Printf("Dest Path: %s\n", cfg.DestPath)
	fmt.Printf("Template:  %s\n", cfg.Template)
	fmt.Println()

	// Walk source to count files
	t := &dryRunWalker{source: cfg.Source}
	if err := t.walk(); err != nil {
		return err
	}

	fmt.Printf("Files found: %d\n", t.count)
	fmt.Printf("Total Size:  %s\n", transfer.FormatBytes(t.totalSize))
	return nil
}

type dryRunWalker struct {
	source    string
	count     int
	totalSize int64
}

func (d *dryRunWalker) walk() error {
	return walkSource(d.source, func(path string, size int64) {
		d.count++
		d.totalSize += size
	})
}

func walkSource(source string, fn func(path string, size int64)) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := source + "/" + e.Name()
		if e.IsDir() {
			if err := walkSource(path, fn); err != nil {
				return err
			}
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fn(path, info.Size())
	}
	return nil
}
