# LOOTup

**Network transfer companion for [LOOT](https://github.com/Mald0r0r000/LOOT).**

LOOTup extends the LOOT workflow with SFTP transfers to a home server, SSH automation, project folder templating, and cold storage archiving.

## Features

- **SFTP Transfer** — Parallel workers with on-the-fly xxHash64 verification via `io.TeeReader`
- **SSH Automation** — Run remote commands after transfer (e.g. Nextcloud rescan)
- **Project Templates** — Create standardized folder structures on remote host (FILM/Rushes, EDIT/Output, DOCS/, etc.)
- **Archive** — `rclone sync` wrapper for hot → cold storage migration
- **TUI** — Bubble Tea interface, same visual language as LOOT

## Install

### Homebrew

```bash
brew tap Mald0r0r000/loot
brew install lootup
```

### From source

```bash
git clone https://github.com/Mald0r0r000/LOOTup.git
cd LOOTup
make build
```

## Usage

### Interactive mode (TUI)

```bash
lootup
```

### CLI mode

```bash
lootup --source /Volumes/CARD --host 192.168.1.100 --user admin \
       --key ~/.ssh/id_ed25519 --dest-path /data/projects/MyFilm \
       --template film
```

### Archive subcommand

```bash
lootup archive --source /mnt/exos/projects --dest /mnt/dock/archive
```

### Dry run

```bash
lootup --source /Volumes/CARD --host 192.168.1.100 --user admin \
       --key ~/.ssh/id_ed25519 --dest-path /data/projects/MyFilm --dry-run
```

## Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--source` | `-s` | Source directory | — |
| `--host` | | Remote host address | — |
| `--user` | `-u` | SSH username | — |
| `--key` | `-k` | Path to Ed25519 private key | `~/.ssh/id_ed25519` |
| `--dest-path` | `-d` | Destination path on remote host | — |
| `--template` | `-t` | Project template (film, photo, generic) | — |
| `--remote-cmd` | | Command to run on host after transfer | `nextcloud-scan.sh` |
| `--concurrency` | `-c` | Number of parallel workers | `4` |
| `--dry-run` | | Simulate without transferring | `false` |
| `--version` | `-v` | Print version | — |

## Project Structure

```
LOOTup/
├── cmd/lootup/main.go       # Entry point
├── internal/
│   ├── config/               # CLI flag parsing
│   ├── transfer/             # SFTP engine + xxHash64
│   ├── ssh/                  # SSH client
│   ├── template/             # Folder structure templates
│   ├── archive/              # rclone sync wrapper
│   └── ui/                   # Bubble Tea TUI
├── .github/workflows/
│   └── release.yml
├── go.mod
├── Makefile
└── README.md
```

## License

MIT
