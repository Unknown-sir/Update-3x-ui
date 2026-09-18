package cisco

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/vpnutil"
)

// FindOcpasswd locates the ocpasswd helper binary.
func FindOcpasswd() string {
	for _, p := range []string{"/usr/bin/ocpasswd", "/usr/local/bin/ocpasswd"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("ocpasswd"); err == nil {
		return path
	}
	return ""
}

// SyncPasswords reconciles the ocserv password file with the desired client
// set: ocpasswd upserts entries, deletions drop the user's line directly.
func SyncPasswords(id int, clients []ClientEntry) error {
	helper := FindOcpasswd()
	if helper == "" {
		return fmt.Errorf("ocpasswd binary not found; install the ocserv package")
	}
	path := passwdPath(id)
	if err := vpnutil.EnsureDir(InstanceDir(id), 0o755); err != nil {
		return err
	}
	if !vpnutil.FileExists(path) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_ = f.Close()
	}
	want := map[string]string{}
	for _, c := range clients {
		if c.Email == "" || c.Password == "" {
			continue
		}
		want[c.Email] = c.Password
	}
	for email, password := range want {
		cmd := exec.Command(helper, "-c", path, email)
		cmd.Stdin = strings.NewReader(password + "\n" + password + "\n")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ocpasswd %s: %w: %s", email, err, strings.TrimSpace(string(out)))
		}
	}
	return dropStalePasswdUsers(path, want)
}

// dropStalePasswdUsers removes file lines for users no longer desired.
// The ocpasswd line format is user:group:hash — parsed textually so no
// password hashing knowledge is required.
func dropStalePasswdUsers(path string, want map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var kept []string
	changed := false
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := sc.Text()
		user := line
		if i := strings.IndexByte(line, ':'); i >= 0 {
			user = line[:i]
		}
		if _, ok := want[user]; ok {
			kept = append(kept, line)
		} else {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	out := strings.Join(kept, "\n")
	if out != "" {
		out += "\n"
	}
	return os.WriteFile(path, []byte(out), 0o600)
}
