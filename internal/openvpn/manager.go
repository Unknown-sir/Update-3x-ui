package openvpn

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/vpnutil"
)

// TrafficDelta is one client's byte counters since the last collection.
type TrafficDelta struct {
	Tag   string
	Email string
	Up    int64
	Down  int64
}

type managed struct {
	proc         *Process
	tag          string
	dir          string
	structuralFP string
	usersFP      string
	last         map[string]ClientCounters
}

type Manager struct {
	mu        sync.Mutex
	procs     map[int]*managed
	lastStart map[int]string
}

var (
	managerInstance *Manager
	managerOnce     sync.Once
)

// GetManager returns the process manager singleton.
func GetManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{procs: make(map[int]*managed), lastStart: make(map[int]string)}
	})
	return managerInstance
}

// Ensure converges one inbound's daemon to the desired instance.
func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureLocked(inst)
}

func (m *Manager) ensureLocked(inst Instance) error {
	if len(inst.Clients) == 0 {
		m.removeLocked(inst.Id)
		return nil
	}
	structuralFP := inst.StructuralFingerprint()
	usersFP := inst.UsersFingerprint()
	if existing, ok := m.procs[inst.Id]; ok && existing != nil {
		if existing.proc != nil && existing.proc.IsRunning() &&
			existing.structuralFP == structuralFP && existing.usersFP == usersFP {
			existing.tag = inst.Tag
			return nil
		}
		m.stopLocked(inst.Id)
	}
	if err := m.startLocked(inst, structuralFP, usersFP); err != nil {
		if m.lastStart[inst.Id] != err.Error() {
			m.lastStart[inst.Id] = err.Error()
			logger.Warningf("openvpn: failed to start daemon for inbound %d (%s): %v", inst.Id, inst.Tag, err)
		}
		return err
	}
	delete(m.lastStart, inst.Id)
	return nil
}

func (m *Manager) startLocked(inst Instance, structuralFP, usersFP string) error {
	bin := FindBinary()
	if bin == "" {
		return fmt.Errorf("openvpn binary not found; install the openvpn package")
	}
	dir := InstanceDir(inst.Id)
	if err := vpnutil.EnsureDir(dir, 0o755); err != nil {
		return err
	}
	if err := EnsurePKI(bin, inst); err != nil {
		return err
	}
	if _, err := WriteConfigs(inst); err != nil {
		return err
	}
	if cidr := inst.SubnetCIDR(); cidr != "" {
		vpnutil.EnsureForwardingAndNAT(cidr, "openvpn")
	}
	proc := &Process{}
	if err := proc.Start(bin, serverConfPath(inst.Id)); err != nil {
		return err
	}
	if inst.Proto == "tcp" {
		if err := vpnutil.WaitTCPListening("openvpn", inst.Listen, inst.Port, 8*time.Second); err != nil {
			proc.Stop()
			if tail := proc.LastLog(); tail != "" {
				return fmt.Errorf("%w; daemon log:\n%s", err, tail)
			}
			return err
		}
	} else {
		deadline := time.Now().Add(4 * time.Second)
		for proc.IsRunning() && time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
		}
		if !proc.IsRunning() {
			if tail := proc.LastLog(); tail != "" {
				return fmt.Errorf("openvpn exited during startup; daemon log:\n%s", tail)
			}
			return fmt.Errorf("openvpn exited during startup")
		}
	}
	m.procs[inst.Id] = &managed{
		proc: proc, tag: inst.Tag, dir: dir,
		structuralFP: structuralFP, usersFP: usersFP,
		last: make(map[string]ClientCounters),
	}
	logger.Infof("openvpn: daemon started for inbound %d (%s) on %s", inst.Id, inst.Tag, inst.BindTo())
	return nil
}

// SidecarStatus describes one supervised daemon for the status API.
type SidecarStatus struct {
	Id        int
	Protocol  string
	Tag       string
	Running   bool
	LastError string
}

// Status snapshots every tracked daemon plus recorded start failures.
func (m *Manager) Status() []SidecarStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SidecarStatus, 0, len(m.procs)+len(m.lastStart))
	for id, mg := range m.procs {
		running := mg.proc != nil && mg.proc.IsRunning()
		errMsg := m.lastStart[id]
		if running {
			errMsg = ""
		}
		out = append(out, SidecarStatus{Id: id, Protocol: "openvpn", Tag: mg.tag, Running: running, LastError: errMsg})
	}
	return out
}

// CollectTraffic scrapes per-client deltas from each daemon status file and
// reports the connected emails for online tracking.
func (m *Manager) CollectTraffic() ([]TrafficDelta, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var deltas []TrafficDelta
	var online []string
	for id, mg := range m.procs {
		if mg.proc == nil || !mg.proc.IsRunning() {
			continue
		}
		counters, connected := ParseStatusFile(statusPath(id))
		online = append(online, connected...)
		for cn, cur := range counters {
			prev := mg.last[cn]
			mg.last[cn] = cur
			up := cur.Received - prev.Received
			down := cur.Sent - prev.Sent
			if up < 0 {
				up = 0
			}
			if down < 0 {
				down = 0
			}
			if up > 0 || down > 0 {
				deltas = append(deltas, TrafficDelta{Tag: mg.tag, Email: cn, Up: up, Down: down})
			}
		}
		for cn := range mg.last {
			if _, ok := counters[cn]; !ok {
				delete(mg.last, cn)
			}
		}
	}
	sort.Strings(online)
	return deltas, online
}

// Remove stops and forgets one inbound's daemon.
func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(id)
}

func (m *Manager) removeLocked(id int) {
	m.stopLocked(id)
	delete(m.lastStart, id)
}

func (m *Manager) stopLocked(id int) {
	if existing, ok := m.procs[id]; ok && existing != nil {
		if existing.proc != nil {
			existing.proc.Stop()
		}
		delete(m.procs, id)
	}
}

// Reconcile converges running daemons to the desired set, restarting crashed
// ones and dropping removed inbounds.
func (m *Manager) Reconcile(desired []Instance) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := make(map[int]Instance, len(desired))
	for _, inst := range desired {
		want[inst.Id] = inst
	}
	for id := range m.procs {
		if _, ok := want[id]; !ok {
			m.removeLocked(id)
		}
	}
	for _, inst := range desired {
		_ = m.ensureLocked(inst)
	}
}

// StopAll terminates every managed daemon.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.procs {
		m.stopLocked(id)
	}
}
