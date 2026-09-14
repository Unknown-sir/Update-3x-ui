// Package cisco manages Cisco AnyConnect (ocserv) inbounds as sidecars.
package cisco

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type ClientEntry struct {
	Email          string `json:"email"`
	Password       string `json:"password,omitempty"`
	SpeedLimitMbps int    `json:"speedLimitMbps,omitempty"`
}

type Settings struct {
	Auth           string        `json:"auth"`
	Subnet         string        `json:"subnet"`
	Netmask        string        `json:"netmask"`
	DNS            []string      `json:"dns"`
	Clients        []ClientEntry `json:"clients"`
	SpeedLimitMbps int           `json:"speedLimitMbps"`
}

type Instance struct {
	Id             int
	Tag            string
	Listen         string
	Port           int
	Auth           string
	Subnet         string
	Clients        []ClientEntry
	SpeedLimitMbps int
}

func (inst Instance) BindTo() string {
	listen := inst.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}
	return net.JoinHostPort(listen, strconv.Itoa(inst.Port))
}

func (inst Instance) StructuralFingerprint() string {
	return strings.Join([]string{inst.BindTo(), inst.Auth, inst.Subnet, strconv.Itoa(inst.SpeedLimitMbps)}, "|")
}

func (inst Instance) UsersFingerprint() string {
	parts := make([]string, 0, len(inst.Clients))
	for _, c := range inst.Clients {
		parts = append(parts, c.Email+"="+strconv.Itoa(c.SpeedLimitMbps))
	}
	return strings.Join(parts, ",")
}

func InstanceFromInbound(ib *model.Inbound) (Instance, bool) {
	if ib.Protocol != model.Cisco {
		return Instance{}, false
	}
	var s Settings
	_ = json.Unmarshal([]byte(ib.Settings), &s)
	auth := s.Auth
	if auth == "" {
		auth = "plain"
	}
	inst := Instance{
		Id: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Port: ib.Port,
		Auth: auth, Subnet: s.Subnet, SpeedLimitMbps: s.SpeedLimitMbps,
	}
	if inst.Subnet == "" {
		inst.Subnet = "10.9.0.0/24"
	}
	for _, c := range s.Clients {
		limit := c.SpeedLimitMbps
		if limit == 0 {
			limit = inst.SpeedLimitMbps
		}
		inst.Clients = append(inst.Clients, ClientEntry{Email: c.Email, Password: c.Password, SpeedLimitMbps: limit})
	}
	return inst, true
}

// GenerateOcservConf renders a minimal ocserv.conf.
// Speed cap maps to rx/tx-data-per-sec (bytes/sec) defaults.
func GenerateOcservConf(inst Instance) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tcp-port = %d\nudp-port = %d\n", inst.Port, inst.Port)
	fmt.Fprintf(&b, "auth = \"%s\"\n", inst.Auth)
	fmt.Fprintf(&b, "ipv4-network = %s\n", inst.Subnet)
	b.WriteString("tunnel-all-dns = true\ntry-mtu-discovery = true\n")
	if inst.SpeedLimitMbps > 0 {
		bps := inst.SpeedLimitMbps * 1024 * 1024 / 8
		fmt.Fprintf(&b, "rx-data-per-sec = %d\ntx-data-per-sec = %d\n", bps, bps)
	}
	return b.String()
}

type Manager struct {
	mu    sync.Mutex
	confs map[int]Instance
}

var (
	mgrOnce sync.Once
	mgr     *Manager
)

func GetManager() *Manager {
	mgrOnce.Do(func() { mgr = &Manager{confs: make(map[int]Instance)} })
	return mgr
}

func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.confs[inst.Id] = inst
	return nil
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.confs, id)
}

func (m *Manager) Reconcile() {
	m.mu.Lock()
	defer m.mu.Unlock()
}
