package openvpn

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// FindBinary locates the openvpn daemon binary.
func FindBinary() string {
	for _, p := range []string{"/usr/sbin/openvpn", "/usr/bin/openvpn", "/usr/local/sbin/openvpn"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("openvpn"); err == nil {
		return path
	}
	return ""
}

// Process wraps one supervised openvpn child process.
type Process struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	exited  chan struct{}
}

// Start launches openvpn in the foreground on the given config.
func (p *Process) Start(bin, conf string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return nil
	}
	if bin == "" {
		return fmt.Errorf("openvpn binary not found")
	}
	cmd := exec.CommandContext(context.Background(), bin, "--config", conf)
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
			logger.Warningf("openvpn %s exited: %v", conf, err)
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
		return fmt.Errorf("openvpn exited immediately, see the inbound log under bin/openvpn/")
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
		i := indexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := trimSpace(w.buf[:i])
		w.buf = w.buf[i+1:]
		if line != "" {
			logger.Infof("openvpn %s | %s", filepath.Base(w.label), line)
		}
	}
	return len(p), nil
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
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
