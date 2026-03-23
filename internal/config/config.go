package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all configuration for LOOTup
type Config struct {
	// Transfer source
	Source string

	// Remote host
	Host    string
	User    string
	KeyPath string

	// Remote destination
	DestPath string

	// Template
	Template string

	// Performance
	Concurrency int

	// Post-transfer
	RemoteCmd string

	// Options
	DryRun  bool
	Verbose bool

	// Archive mode
	ArchiveMode   bool
	ArchiveSource string
	ArchiveDest   string

	// Version info
	Version string
}

// DefaultConfig returns config with sensible defaults
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		KeyPath:     filepath.Join(home, ".ssh", "id_ed25519"),
		Concurrency: 4,
		RemoteCmd:   "nextcloud-scan.sh",
		DryRun:      false,
		Verbose:     false,
	}
}

// ParseFlags parses command line arguments and returns Config
func ParseFlags(version string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.Version = version

	// Check for archive subcommand
	if len(os.Args) > 1 && os.Args[1] == "archive" {
		cfg.ArchiveMode = true
		archiveFS := flag.NewFlagSet("archive", flag.ExitOnError)
		archiveFS.StringVar(&cfg.ArchiveSource, "source", "", "Source path (hot storage)")
		archiveFS.StringVar(&cfg.ArchiveSource, "s", "", "Source path (shorthand)")
		archiveFS.StringVar(&cfg.ArchiveDest, "dest", "", "Destination path (cold storage)")
		archiveFS.StringVar(&cfg.ArchiveDest, "d", "", "Destination path (shorthand)")
		archiveFS.BoolVar(&cfg.DryRun, "dry-run", false, "Simulate without syncing")
		archiveFS.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose logging")

		archiveFS.Usage = func() {
			fmt.Fprintf(os.Stderr, "LOOTup archive — rclone sync wrapper\n\n")
			fmt.Fprintf(os.Stderr, "Usage:\n")
			fmt.Fprintf(os.Stderr, "  lootup archive --source <hot> --dest <cold>\n\n")
			fmt.Fprintf(os.Stderr, "Flags:\n")
			archiveFS.PrintDefaults()
		}

		if err := archiveFS.Parse(os.Args[2:]); err != nil {
			return nil, err
		}

		if cfg.ArchiveSource == "" || cfg.ArchiveDest == "" {
			return nil, fmt.Errorf("both --source and --dest are required for archive mode")
		}
		return cfg, nil
	}

	// Main command flags
	flag.StringVar(&cfg.Source, "source", "", "Source directory to transfer")
	flag.StringVar(&cfg.Source, "s", "", "Source directory (shorthand)")
	flag.StringVar(&cfg.Host, "host", "", "Remote host address")
	flag.StringVar(&cfg.User, "user", "", "SSH username")
	flag.StringVar(&cfg.User, "u", "", "SSH username (shorthand)")
	flag.StringVar(&cfg.KeyPath, "key", cfg.KeyPath, "Path to Ed25519 private key")
	flag.StringVar(&cfg.KeyPath, "k", cfg.KeyPath, "Path to Ed25519 private key (shorthand)")
	flag.StringVar(&cfg.DestPath, "dest-path", "", "Destination path on remote host")
	flag.StringVar(&cfg.DestPath, "d", "", "Destination path (shorthand)")
	flag.StringVar(&cfg.Template, "template", "", "Project template: film, photo, generic")
	flag.StringVar(&cfg.Template, "t", "", "Project template (shorthand)")
	flag.StringVar(&cfg.RemoteCmd, "remote-cmd", cfg.RemoteCmd, "Command to run on host after transfer")
	flag.IntVar(&cfg.Concurrency, "concurrency", cfg.Concurrency, "Number of parallel workers")
	flag.IntVar(&cfg.Concurrency, "c", cfg.Concurrency, "Number of parallel workers (shorthand)")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Simulate without transferring")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose logging")

	versionFlag := flag.Bool("version", false, "Print version")
	v := flag.Bool("v", false, "Print version (shorthand)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "LOOTup — Network Transfer Companion for LOOT\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  lootup [flags]                    Interactive mode (TUI)\n")
		fmt.Fprintf(os.Stderr, "  lootup [flags] --source <src>     CLI mode\n")
		fmt.Fprintf(os.Stderr, "  lootup archive --source <hot> --dest <cold>\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Handle version
	if *versionFlag || *v {
		fmt.Printf("lootup version %s\n", version)
		os.Exit(0)
	}

	return cfg, nil
}

// IsInteractive returns true when no source was provided (TUI mode)
func (c *Config) IsInteractive() bool {
	return c.Source == "" && !c.ArchiveMode
}

// Validate checks that required fields are set for CLI mode
func (c *Config) Validate() error {
	if c.ArchiveMode {
		if c.ArchiveSource == "" {
			return fmt.Errorf("archive source is required")
		}
		if c.ArchiveDest == "" {
			return fmt.Errorf("archive destination is required")
		}
		return nil
	}

	if c.Source == "" {
		return fmt.Errorf("source is required")
	}
	if _, err := os.Stat(c.Source); os.IsNotExist(err) {
		return fmt.Errorf("source '%s' does not exist", c.Source)
	}
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.User == "" {
		return fmt.Errorf("user is required")
	}
	if c.DestPath == "" {
		return fmt.Errorf("dest-path is required")
	}
	return nil
}
