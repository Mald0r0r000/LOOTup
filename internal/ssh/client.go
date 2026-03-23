package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Client wraps an SSH connection to a remote host
type Client struct {
	conn *ssh.Client
	host string
	user string
}

// Connect establishes an SSH connection using an Ed25519 private key
func Connect(host, user, keyPath string) (*Client, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", keyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse key %s: %w", keyPath, err)
	}

	// Load known_hosts for host key verification
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("known_hosts not found — connect manually first: ssh %s@%s", user, host)
	}
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
	}

	addr := host
	if !strings.Contains(addr, ":") {
		addr = addr + ":22"
	}

	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s@%s: %w", user, addr, err)
	}

	return &Client{
		conn: conn,
		host: host,
		user: user,
	}, nil
}

// Run executes a command on the remote host and returns combined output
func (c *Client) Run(cmd string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(output), fmt.Errorf("run '%s': %w (output: %s)", cmd, err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// Conn returns the underlying ssh.Client for use by SFTP
func (c *Client) Conn() *ssh.Client {
	return c.conn
}

// Close terminates the SSH connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// String returns a display-friendly representation
func (c *Client) String() string {
	return fmt.Sprintf("%s@%s", c.user, c.host)
}
