package openvpn

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/vpnutil"
)

// opensslCNF is the minimal CA database config for signing client/server
// certificates and regenerating the CRL.
const opensslCNF = `[ ca ]
default_ca = CA_default
[ CA_default ]
dir = %s
database = $dir/index.txt
new_certs_dir = $dir/newcerts
certificate = $dir/ca.crt
private_key = $dir/ca.key
serial = $dir/serial
crlnumber = $dir/crlnumber
default_md = sha256
default_days = 825
policy = policy_any
[ policy_any ]
commonName = supplied
[ req ]
distinguished_name = req_dn
[ req_dn ]
`

func caDir(id int) string {
	return filepath.Join(InstanceDir(id), "pki")
}

// sanitizeCN maps an email to a filesystem-safe certificate basename.
func sanitizeCN(email string) string {
	var b strings.Builder
	for _, r := range email {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "client"
	}
	return b.String()
}

func runOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}

// EnsurePKI creates the CA, server certificate and tls-crypt key once, then
// issues client certificates for every enabled client and rebuilds the CRL
// from the on-disk client set minus the enabled set.
func EnsurePKI(openvpnBin string, inst Instance) error {
	dir := caDir(inst.Id)
	for _, d := range []string{dir, filepath.Join(dir, "newcerts"), filepath.Join(dir, "clients")} {
		if err := vpnutil.EnsureDir(d, 0o755); err != nil {
			return err
		}
	}
	cnfPath := filepath.Join(dir, "openssl.cnf")
	if _, err := vpnutil.WriteFile(cnfPath, []byte(fmt.Sprintf(opensslCNF, dir)), 0o644); err != nil {
		return err
	}
	touch := func(name, content string) error {
		p := filepath.Join(dir, name)
		if vpnutil.FileExists(p) {
			return nil
		}
		return os.WriteFile(p, []byte(content), 0o644)
	}
	for _, tc := range [][2]string{{"index.txt", ""}, {"serial", "1000\n"}, {"crlnumber", "1000\n"}} {
		if err := touch(tc[0], tc[1]); err != nil {
			return err
		}
	}

	caCrt := filepath.Join(dir, "ca.crt")
	caKey := filepath.Join(dir, "ca.key")
	if !vpnutil.FileExists(caCrt) || !vpnutil.FileExists(caKey) {
		if err := vpnutil.Run("openssl", "req", "-x509", "-newkey", "rsa:2048",
			"-nodes", "-keyout", caKey, "-out", caCrt,
			"-days", "3650", "-subj", "/CN=x-ui-openvpn-ca"); err != nil {
			return fmt.Errorf("openvpn %d: mint CA: %w", inst.Id, err)
		}
		_ = os.Chmod(caKey, 0o600)
	}

	serverCrt := filepath.Join(dir, "server.crt")
	serverKey := filepath.Join(dir, "server.key")
	if !vpnutil.FileExists(serverCrt) || !vpnutil.FileExists(serverKey) {
		csr := filepath.Join(dir, "server.csr")
		if err := vpnutil.Run("openssl", "req", "-newkey", "rsa:2048",
			"-nodes", "-keyout", serverKey, "-out", csr,
			"-subj", fmt.Sprintf("/CN=x-ui-openvpn-%d", inst.Id)); err != nil {
			return fmt.Errorf("openvpn %d: server key: %w", inst.Id, err)
		}
		_ = os.Chmod(serverKey, 0o600)
		if err := vpnutil.Run("openssl", "ca", "-batch", "-config", cnfPath,
			"-in", csr, "-out", serverCrt, "-days", "825"); err != nil {
			return fmt.Errorf("openvpn %d: sign server cert: %w", inst.Id, err)
		}
		_ = os.Remove(csr)
	}

	if openvpnBin != "" {
		taKey := filepath.Join(dir, "ta.key")
		if !vpnutil.FileExists(taKey) {
			if err := vpnutil.Run(openvpnBin, "--genkey", "secret", taKey); err != nil {
				return fmt.Errorf("openvpn %d: tls-crypt key: %w", inst.Id, err)
			}
		}
	}

	enabled := map[string]bool{}
	for _, email := range inst.SortedEmails() {
		enabled[email] = true
		if err := issueClientCert(dir, cnfPath, inst.Id, email); err != nil {
			return err
		}
	}
	return rebuildCRL(dir, cnfPath, inst.Id, enabled)
}

func issueClientCert(dir, cnfPath string, id int, email string) error {
	name := sanitizeCN(email)
	crt := filepath.Join(dir, "clients", name+".crt")
	key := filepath.Join(dir, "clients", name+".key")
	if vpnutil.FileExists(crt) && vpnutil.FileExists(key) {
		return nil
	}
	csr := filepath.Join(dir, "clients", name+".csr")
	if err := vpnutil.Run("openssl", "req", "-newkey", "rsa:2048",
		"-nodes", "-keyout", key, "-out", csr,
		"-subj", "/CN="+email); err != nil {
		return fmt.Errorf("openvpn %d: client key %s: %w", id, email, err)
	}
	_ = os.Chmod(key, 0o600)
	if err := vpnutil.Run("openssl", "ca", "-batch", "-config", cnfPath,
		"-in", csr, "-out", crt, "-days", "825"); err != nil {
		return fmt.Errorf("openvpn %d: sign client cert %s: %w", id, email, err)
	}
	_ = os.Remove(csr)
	return nil
}

// certSerialEnddate reads a certificate's hex serial and index-format expiry.
func certSerialEnddate(crt string) (serial, enddate string, err error) {
	sOut, err := runOutput("openssl", "x509", "-in", crt, "-noout", "-serial")
	if err != nil {
		return "", "", err
	}
	serial = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sOut), "serial="))
	dOut, err := runOutput("openssl", "x509", "-in", crt, "-noout", "-enddate")
	if err != nil {
		return "", "", err
	}
	t, terr := time.Parse("Jan _2 15:04:05 2006 MST", strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(dOut), "notAfter=")))
	if terr != nil {
		return "", "", fmt.Errorf("unparseable enddate %q", strings.TrimSpace(dOut))
	}
	return serial, t.UTC().Format("060102150405Z"), nil
}

// rebuildCRL rewrites index.txt (V for enabled, R for on-disk-but-disabled
// client certs) and regenerates crl.pem.
func rebuildCRL(dir, cnfPath string, id int, enabled map[string]bool) error {
	entries, err := os.ReadDir(filepath.Join(dir, "clients"))
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format("060102150405Z")
	var index strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".crt") {
			continue
		}
		serial, enddate, err := certSerialEnddate(filepath.Join(dir, "clients", e.Name()))
		if err != nil || serial == "" {
			continue
		}
		email := clientEmailForCert(e.Name(), enabled)
		if enabled[email] {
			fmt.Fprintf(&index, "V\t%s\t%s\tunknown\t/CN=%s\n", enddate, serial, email)
		} else {
			fmt.Fprintf(&index, "R\t%s\t%s\t%s\tunknown\t/CN=%s\n", enddate, now, serial, email)
		}
	}
	if _, err := vpnutil.WriteFile(filepath.Join(dir, "index.txt"), []byte(index.String()), 0o644); err != nil {
		return err
	}
	if err := vpnutil.Run("openssl", "ca", "-gencrl", "-config", cnfPath, "-out", filepath.Join(dir, "crl.pem")); err != nil {
		return fmt.Errorf("openvpn %d: gencrl: %w", id, err)
	}
	return nil
}

// clientEmailForCert maps a client cert filename back to its email using the
// enabled set, falling back to the basename for revoked unknowns.
func clientEmailForCert(filename string, enabled map[string]bool) string {
	base := strings.TrimSuffix(filename, ".crt")
	for email := range enabled {
		if sanitizeCN(email) == base {
			return email
		}
	}
	return base
}

// ClientCertPaths returns the on-disk certificate/key for embedding into the
// downloadable .ovpn profile.
func ClientCertPaths(id int, email string) (crt, key string) {
	name := sanitizeCN(email)
	return filepath.Join(caDir(id), "clients", name+".crt"),
		filepath.Join(caDir(id), "clients", name+".key")
}

// CABundle returns CA + tls-crypt key paths for profile embedding.
func CABundle(id int) (ca, ta string) {
	return filepath.Join(caDir(id), "ca.crt"), filepath.Join(caDir(id), "ta.key")
}
