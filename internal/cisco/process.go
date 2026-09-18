package cisco

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// FindBinary locates the ocserv daemon binary.
func FindBinary() string {
	for _, p := range []string{"/usr/sbin/ocserv", "/usr/bin/ocserv", "/usr/local/sbin/ocserv"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("ocserv"); err == nil {
		return path
	}
	return ""
}

// FindOcctl locates the occtl control client.
func FindOcctl() string {
	for _, p := range []string{"/usr/bin/occtl", "/usr/local/bin/occtl"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("occtl"); err == nil {
		return path
	}
	return ""
}

// Process wraps one supervised ocserv child process, kept in the foreground.
type Process struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	exited  chan struct{}
}

// Start launches ocserv in the foreground on the given config.
func (p *Process) Start(bin, conf, pidFile string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return nil
	}
	if bin == "" {
		return fmt.Errorf("ocserv binary not found")
	}
	_ = os.Remove(pidFile)
	cmd := exec.Command(bin, "-f", "-c", conf, "--pid-file", pidFile)
	cmd.Stdout = &procLogWriter{label: conf}
	cmd.Stderr = &procLogWriter{label: conf}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	p.running = true
	p.exited = make(chan struct{})
	go func() {
		err := cmd.Wait()
		if err != nil {
			logger.Warningf("ocserv %s exited: %v", conf, err)
		}
		p.mu.Lock()
		p.running = false
		close(p.exited)
		p.mu.Unlock()
	}()
	// A config error kills the daemon within the first second; surface it
	// instead of reporting a phantom running process.
	select {
	case <-p.exited:
		return fmt.Errorf("ocserv exited immediately, see the inbound log under bin/cisco/")
	case <-time.After(1500 * time.Millisecond):
		return nil
	}
}

// Stop terminates the child process.
func (p *Process) Stop() {
	p.mu.Lock()
	cmd := p.cmd
	exited := p.exited
	p.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
	}
}

// IsRunning reports whether the child is alive.
func (p *Process) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return false
	}
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

type procLogWriter struct {
	mu    sync.Mutex
	label string
	buf   string
}

func (w *procLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf += string(p)
	for {
		i := -1
		for j := 0; j < len(w.buf); j++ {
			if w.buf[j] == '\n' {
				i = j
				break
			}
		}
		if i < 0 {
			break
		}
		line := stringsTrim(w.buf[:i])
		w.buf = w.buf[i+1:]
		if line != "" {
			logger.Infof("ocserv %s | %s", shortLabel(w.label), line)
		}
	}
	return len(p), nil
}

func stringsTrim(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func shortLabel(label string) string {
	return filepath.Base(label)
}
