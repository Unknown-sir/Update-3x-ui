package cisco

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/speedlimit"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnutil"
)

// InstanceDir is the on-disk home of one inbound's ocserv configs and logs.
// It lives next to the database (not under bin/) so panel updates, which
// replace bin/, never wipe passwords and certificates.
func InstanceDir(id int) string {
	return filepath.Join(config.GetDBFolderPath(), "cisco", fmt.Sprintf("cisco-%d", id))
}

func confPath(id int) string   { return filepath.Join(InstanceDir(id), "ocserv.conf") }
func passwdPath(id int) string { return filepath.Join(InstanceDir(id), "ocpasswd") }
func perUserDir(id int) string { return filepath.Join(InstanceDir(id), "per-user") }
func socketPath(id int) string { return filepath.Join(InstanceDir(id), "ocserv.sock") }
func pidPath(id int) string    { return filepath.Join(InstanceDir(id), "ocserv.pid") }
func certPath(id int) string   { return filepath.Join(InstanceDir(id), "server.crt") }
func keyPath(id int) string    { return filepath.Join(InstanceDir(id), "server.key") }
func runDir(id int) string     { return filepath.Join(InstanceDir(id), "run") }
func perUserFile(id int, e string) string {
	return filepath.Join(perUserDir(id), sanitizeUser(e))
}

// sanitizeUser maps an email to a safe per-user filename.
func sanitizeUser(email string) string {
	var b strings.Builder
	for _, r := range email {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}

// instanceDNS returns the pushed resolvers, defaulting to public DNS.
func instanceDNS(inst Instance) []string {
	if len(inst.DNS) > 0 {
		return inst.DNS
	}
	return []string{"8.8.8.8", "8.8.4.4"}
}

// EnsureCert mints a self-signed TLS certificate for the daemon once.
func EnsureCert(inst Instance, serverName string) error {
	dir := InstanceDir(inst.Id)
	if err := vpnutil.EnsureDir(dir, 0o755); err != nil {
		return err
	}
	if serverName == "" {
		serverName = "ocserv"
	}
	return vpnutil.EnsureSelfSigned(dir, serverName, "server.crt", "server.key")
}

// RenderConf builds the ocserv.conf text.
func RenderConf(inst Instance) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tcp-port = %d\n", inst.Port)
	fmt.Fprintf(&b, "udp-port = %d\n", inst.Port)
	fmt.Fprintf(&b, "auth = \"%s[passwd=%s]\"\n", inst.Auth, passwdPath(inst.Id))
	network, ones, ok := inst.NetworkCIDR()
	if !ok {
		network, ones = "10.9.0.0", 24
	}
	fmt.Fprintf(&b, "ipv4-network = %s/%d\n", network, ones)
	for _, dns := range instanceDNS(inst) {
		fmt.Fprintf(&b, "dns = %s\n", dns)
	}
	b.WriteString("route = default\n")
	b.WriteString("tunnel-all-dns = true\n")
	b.WriteString("try-mtu-discovery = true\n")
	b.WriteString("cisco-client-compat = true\n")
	b.WriteString("keepalive = 32400\n")
	b.WriteString("dpd = 90\n")
	b.WriteString("mobile-dpd = 1800\n")
	fmt.Fprintf(&b, "server-cert = %s\n", certPath(inst.Id))
	fmt.Fprintf(&b, "server-key = %s\n", keyPath(inst.Id))
	fmt.Fprintf(&b, "socket-dir = %s\n", runDir(inst.Id))
	fmt.Fprintf(&b, "pid-file = %s\n", pidPath(inst.Id))
	b.WriteString("run-as-user = root\nrun-as-group = root\n")
	fmt.Fprintf(&b, "log-level = 2\n")
	fmt.Fprintf(&b, "config-per-user = %s\n", perUserDir(inst.Id))
	if inst.SpeedLimitMbps > 0 {
		bps := speedlimit.BytesPerSec(inst.SpeedLimitMbps)
		fmt.Fprintf(&b, "rx-data-per-sec = %d\ntx-data-per-sec = %d\n", bps, bps)
	}
	return b.String()
}

// WriteConf persists ocserv.conf, reporting whether it changed.
func WriteConf(inst Instance) (bool, error) {
	for _, d := range []string{InstanceDir(inst.Id), runDir(inst.Id), perUserDir(inst.Id)} {
		if err := vpnutil.EnsureDir(d, 0o755); err != nil {
			return false, err
		}
	}
	return vpnutil.WriteFile(confPath(inst.Id), []byte(RenderConf(inst)), 0o644)
}

// WritePerUserFiles writes one bandwidth file per capped client and prunes
// stale ones, reporting whether anything changed.
func WritePerUserFiles(inst Instance) (bool, error) {
	changed := false
	want := map[string]bool{}
	for _, c := range inst.Clients {
		if c.SpeedLimitMbps <= 0 {
			continue
		}
		name := sanitizeUser(c.Email)
		want[name] = true
		bps := speedlimit.BytesPerSec(c.SpeedLimitMbps)
		body := fmt.Sprintf("rx-data-per-sec = %d\ntx-data-per-sec = %d\n", bps, bps)
		ok, err := vpnutil.WriteFile(perUserFile(inst.Id, c.Email), []byte(body), 0o644)
		if err != nil {
			return false, err
		}
		changed = changed || ok
	}
	stale, err := os.ReadDir(perUserDir(inst.Id))
	if err != nil {
		return changed, nil
	}
	for _, e := range stale {
		if e.IsDir() || want[e.Name()] {
			continue
		}
		if strings.HasSuffix(e.Name(), ".conf") || !strings.Contains(e.Name(), ".") {
			_ = os.Remove(filepath.Join(perUserDir(inst.Id), e.Name()))
			changed = true
		}
	}
	return changed, nil
}
