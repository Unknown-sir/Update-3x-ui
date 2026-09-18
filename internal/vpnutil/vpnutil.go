// Package vpnutil holds small shared helpers for the OpenVPN and ocserv
// sidecar supervisors: shelling out to openssl, minting self-signed TLS
// certificates, and ensuring kernel forwarding plus NAT for VPN subnets.
package vpnutil

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// Run executes a helper binary, returning combined output on failure.
func Run(name string, args ...string) error {
	cmd := exec.CommandContext(context.Background(), name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(buf.String()))
	}
	return nil
}

// EnsureDir creates dir (and parents) when missing.
func EnsureDir(dir string, perm os.FileMode) error {
	return os.MkdirAll(dir, perm)
}

// WriteFile writes data with the given mode only when the content changed,
// keeping mtimes stable so supervisors don't restart on no-op reconciles.
func WriteFile(path string, data []byte, perm os.FileMode) (bool, error) {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

// FileExists reports whether path exists and is non-empty.
func FileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

// EnsureSelfSigned mints a self-signed RSA certificate/key pair once,
// reusing it while both files exist.
func EnsureSelfSigned(dir, cn, certFile, keyFile string) error {
	certPath := filepath.Join(dir, certFile)
	keyPath := filepath.Join(dir, keyFile)
	if FileExists(certPath) && FileExists(keyPath) {
		return nil
	}
	if err := EnsureDir(dir, 0o755); err != nil {
		return err
	}
	if err := Run("openssl", "req", "-x509", "-newkey", "rsa:2048",
		"-nodes", "-keyout", keyPath, "-out", certPath,
		"-days", "825", "-subj", "/CN="+cn); err != nil {
		return err
	}
	_ = os.Chmod(keyPath, 0o600)
	return nil
}

// DialHost resolves the local address to probe for a daemon bound to listen.
func DialHost(listen string) string {
	if listen == "" || listen == "0.0.0.0" || listen == "::" {
		return "127.0.0.1"
	}
	return listen
}

// WaitTCPListening polls a TCP accept on host:port until timeout, proving
// the supervised daemon actually bound its port (a live process alone does
// not prove that — a bad config can leave it running yet deaf).
func WaitTCPListening(label, listen string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort(DialHost(listen), fmt.Sprintf("%d", port))
	dialer := &net.Dialer{Timeout: time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		cancel()
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%s: nothing listening on %s: %w", label, addr, lastErr)
}

// EnsureForwardingAndNAT enables IPv4 forwarding (runtime + persisted) and
// a MASQUERADE rule for subnet, best-effort: failures only log, since the
// panel may run in environments without iptables (docker without privileges).
func EnsureForwardingAndNAT(subnet, label string) {
	if data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); err == nil {
		if strings.TrimSpace(string(data)) != "1" {
			if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0o644); err != nil {
				logger.Warningf("%s: enable ip_forward failed: %v", label, err)
			}
		}
	}
	const persist = "/etc/sysctl.d/99-x-ui-vpn.conf"
	if cur, err := os.ReadFile(persist); err != nil || !strings.Contains(string(cur), "net.ipv4.ip_forward=1") {
		_ = os.WriteFile(persist, []byte("# managed by 3x-ui VPN sidecars\nnet.ipv4.ip_forward=1\n"), 0o644)
	}
	if _, err := exec.LookPath("iptables"); err != nil {
		return
	}
	check := exec.CommandContext(context.Background(), "iptables", "-t", "nat", "-C", "POSTROUTING", "-s", subnet, "-j", "MASQUERADE")
	if check.Run() == nil {
		return
	}
	add := exec.CommandContext(context.Background(), "iptables", "-t", "nat", "-A", "POSTROUTING", "-s", subnet, "-j", "MASQUERADE")
	if out, err := add.CombinedOutput(); err != nil {
		logger.Warningf("%s: iptables MASQUERADE for %s failed: %v: %s", label, subnet, err, strings.TrimSpace(string(out)))
	}
}
