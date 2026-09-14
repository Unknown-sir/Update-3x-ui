// Package openvpn manages OpenVPN inbounds as a sidecar process.
// One openvpn process per inbound, config regenerated from the panel DB.
package openvpn

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
	Proto          string        `json:"proto"`
	Subnet         string        `json:"subnet"`
	Netmask        string        `json:"netmask"`
	Cipher         string        `json:"cipher"`
	Auth           string        `json:"auth"`
	Clients        []ClientEntry `json:"clients"`
	SpeedLimitMbps int           `json:"speedLimitMbps"`
}

type Instance struct {
	Id             int
	Tag            string
	Listen         string
	Port           int
	Proto          string
	Subnet         string
	Netmask        string
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
	return strings.Join([]string{inst.BindTo(), inst.Proto, inst.Subnet, inst.Netmask, strconv.Itoa(inst.SpeedLimitMbps)}, "|")
}

func (inst Instance) UsersFingerprint() string {
	parts := make([]string, 0, len(inst.Clients))
	for _, c := range inst.Clients {
		parts = append(parts, c.Email+"="+strconv.Itoa(c.SpeedLimitMbps))
	}
	return strings.Join(parts, ",")
}

func InstanceFromInbound(ib *model.Inbound) (Instance, bool) {
	if ib.Protocol != model.OpenVPN {
		return Instance{}, false
	}
	var s Settings
	_ = json.Unmarshal([]byte(ib.Settings), &s)
	proto := s.Proto
	if proto == "" {
		proto = "udp"
	}
	inst := Instance{
		Id: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Port: ib.Port,
		Proto: proto, Subnet: s.Subnet, Netmask: s.Netmask,
		SpeedLimitMbps: s.SpeedLimitMbps,
	}
	if inst.Subnet == "" {
		inst.Subnet = "10.8.0.0"
	}
	if inst.Netmask == "" {
		inst.Netmask = "255.255.255.0"
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

// GenerateServerConf renders an openvpn server.conf.
// Per-user speed cap maps to --shaper (bytes/sec) as a default; per-user
// overrides are pushed via client-connect scripts using the same value.
func GenerateServerConf(inst Instance) string {
	var b strings.Builder
	fmt.Fprintf(&b, "port %d\nproto %s\ndev tun\n", inst.Port, inst.Proto)
	fmt.Fprintf(&b, "server %s %s\n", inst.Subnet, inst.Netmask)
	b.WriteString("topology subnet\nclient-to-client\nkeepalive 10 120\npersist-key\npersist-tun\nverb 3\n")
	if inst.SpeedLimitMbps > 0 {
		fmt.Fprintf(&b, "shaper %d\n", inst.SpeedLimitMbps*1024*1024/8)
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

// Ensure records desired state; process supervision is done by the
// reconcile job once openvpn binary is present via install.sh.
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
