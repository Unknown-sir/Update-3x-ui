package cisco

import (
	"fmt"
	"sort"
	"sync"

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
	structuralFP string
	usersFP      string
	last         map[string][2]int64
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
			logger.Warningf("cisco: failed to start ocserv for inbound %d (%s): %v", inst.Id, inst.Tag, err)
		}
		return err
	}
	delete(m.lastStart, inst.Id)
	return nil
}

func (m *Manager) startLocked(inst Instance, structuralFP, usersFP string) error {
	bin := FindBinary()
	if bin == "" {
		return fmt.Errorf("ocserv binary not found; install the ocserv package")
	}
	serverName := inst.Listen
	if serverName == "" || serverName == "0.0.0.0" {
		serverName = "ocserv"
	}
	if err := EnsureCert(inst, serverName); err != nil {
		return err
	}
	if _, err := WriteConf(inst); err != nil {
		return err
	}
	if _, err := WritePerUserFiles(inst); err != nil {
		return err
	}
	if err := SyncPasswords(inst.Id, inst.Clients); err != nil {
		return err
	}
	if cidr := inst.SubnetCIDR(); cidr != "" {
		vpnutil.EnsureForwardingAndNAT(cidr, "cisco")
	}
	proc := &Process{}
	if err := proc.Start(bin, confPath(inst.Id)); err != nil {
		return err
	}
	m.procs[inst.Id] = &managed{
		proc: proc, tag: inst.Tag,
		structuralFP: structuralFP, usersFP: usersFP,
		last: make(map[string][2]int64),
	}
	logger.Infof("cisco: ocserv started for inbound %d (%s) on %s", inst.Id, inst.Tag, inst.BindTo())
	return nil
}

// CollectTraffic scrapes per-user deltas via occtl and reports online users.
func (m *Manager) CollectTraffic() ([]TrafficDelta, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var deltas []TrafficDelta
	var online []string
	for id, mg := range m.procs {
		if mg.proc == nil || !mg.proc.IsRunning() {
			continue
		}
		cur, connected := OcctlUsers(id)
		online = append(online, connected...)
		for user, c := range cur {
			prev := mg.last[user]
			mg.last[user] = c
			up := c[0] - prev[0]
			down := c[1] - prev[1]
			if up < 0 {
				up = 0
			}
			if down < 0 {
				down = 0
			}
			if up > 0 || down > 0 {
				deltas = append(deltas, TrafficDelta{Tag: mg.tag, Email: user, Up: up, Down: down})
			}
		}
		for user := range mg.last {
			if _, ok := cur[user]; !ok {
				delete(mg.last, user)
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
