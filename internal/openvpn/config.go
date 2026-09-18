package openvpn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnutil"
)

// InstanceDir is the on-disk home of one inbound's PKI, configs and logs.
// It lives next to the database (not under bin/) so panel updates, which
// replace bin/, never wipe the CA and client certificates.
func InstanceDir(id int) string {
	return filepath.Join(config.GetDBFolderPath(), "openvpn", fmt.Sprintf("openvpn-%d", id))
}

func serverConfPath(id int) string { return filepath.Join(InstanceDir(id), "server.conf") }
func ccdDir(id int) string         { return filepath.Join(InstanceDir(id), "ccd") }
func statusPath(id int) string     { return filepath.Join(InstanceDir(id), "status.log") }
func logPath(id int) string        { return filepath.Join(InstanceDir(id), "openvpn.log") }

// instanceDNS returns the pushed resolvers, defaulting to public DNS.
func instanceDNS(inst Instance) []string {
	if len(inst.DNS) > 0 {
		return inst.DNS
	}
	return []string{"8.8.8.8", "8.8.4.4"}
}

// RenderServerConf builds the openvpn server.conf text. Authentication is
// certificate-only: clients present a panel-issued certificate, no passwords.
func RenderServerConf(inst Instance) string {
	pki := caDir(inst.Id)
	listen := inst.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "port %d\n", inst.Port)
	fmt.Fprintf(&b, "proto %s\n", inst.Proto)
	fmt.Fprintf(&b, "local %s\n", listen)
	b.WriteString("dev tun\n")
	fmt.Fprintf(&b, "ca %s\n", filepath.Join(pki, "ca.crt"))
	fmt.Fprintf(&b, "cert %s\n", filepath.Join(pki, "server.crt"))
	fmt.Fprintf(&b, "key %s\n", filepath.Join(pki, "server.key"))
	b.WriteString("dh none\n")
	if vpnutil.FileExists(filepath.Join(pki, "ta.key")) {
		fmt.Fprintf(&b, "tls-crypt %s\n", filepath.Join(pki, "ta.key"))
	}
	b.WriteString("topology subnet\n")
	fmt.Fprintf(&b, "server %s %s\n", inst.Subnet, inst.Netmask)
	b.WriteString("ifconfig-pool-persist ipp.txt\n")
	b.WriteString("push \"redirect-gateway def1 bypass-dhcp\"\n")
	for _, dns := range instanceDNS(inst) {
		fmt.Fprintf(&b, "push \"dhcp-option DNS %s\"\n", dns)
	}
	fmt.Fprintf(&b, "client-config-dir %s\n", ccdDir(inst.Id))
	fmt.Fprintf(&b, "crl-verify %s\n", filepath.Join(pki, "crl.pem"))
	fmt.Fprintf(&b, "cipher %s\n", inst.Cipher)
	fmt.Fprintf(&b, "auth %s\n", inst.Auth)
	b.WriteString("keepalive 10 120\npersist-key\npersist-tun\n")
	fmt.Fprintf(&b, "status %s 5\n", statusPath(inst.Id))
	fmt.Fprintf(&b, "log-append %s\n", logPath(inst.Id))
	b.WriteString("verb 3\nexplicit-exit-notify 1\n")
	return b.String()
}

// WriteConfigs persists server.conf plus one CCD file per client with a
// stable tunnel address, reporting whether anything changed.
func WriteConfigs(inst Instance) (bool, error) {
	changed := false
	ok, err := vpnutil.WriteFile(serverConfPath(inst.Id), []byte(RenderServerConf(inst)), 0o644)
	if err != nil {
		return false, err
	}
	changed = changed || ok
	ccd := ccdDir(inst.Id)
	if err := vpnutil.EnsureDir(ccd, 0o755); err != nil {
		return false, err
	}
	want := map[string]bool{}
	for _, email := range inst.SortedEmails() {
		ip := inst.ClientIP(email)
		if ip == "" {
			continue
		}
		name := sanitizeCN(email)
		want[name] = true
		body := fmt.Sprintf("ifconfig-push %s %s\n", ip, inst.Netmask)
		ok, err := vpnutil.WriteFile(filepath.Join(ccd, name), []byte(body), 0o644)
		if err != nil {
			return false, err
		}
		changed = changed || ok
	}
	stale, err := os.ReadDir(ccd)
	if err != nil {
		return changed, nil
	}
	for _, e := range stale {
		if !e.IsDir() && !want[e.Name()] {
			_ = os.Remove(filepath.Join(ccd, e.Name()))
			changed = true
		}
	}
	return changed, nil
}
