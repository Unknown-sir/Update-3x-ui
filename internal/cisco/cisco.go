// Package cisco manages Cisco AnyConnect (ocserv) inbounds as supervised
// sidecar processes. One ocserv daemon per inbound: the panel writes
// ocserv.conf, maintains the plain password file, optionally writes per-user
// bandwidth caps, and restarts the daemon when the desired state changes.
// Traffic comes from occtl over the daemon control socket.
package cisco

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

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
	Netmask        string
	DNS            []string
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
	return strings.Join([]string{
		inst.BindTo(), inst.Auth, inst.Subnet, inst.Netmask,
		strings.Join(inst.DNS, ","), strconv.Itoa(inst.SpeedLimitMbps),
	}, "|")
}

func (inst Instance) UsersFingerprint() string {
	parts := make([]string, 0, len(inst.Clients))
	for _, c := range inst.Clients {
		parts = append(parts, c.Email+"="+c.Password+"="+strconv.Itoa(c.SpeedLimitMbps))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func InstanceFromInbound(ib *model.Inbound) (Instance, bool) {
	if ib.Protocol != model.Cisco {
		return Instance{}, false
	}
	var s Settings
	_ = json.Unmarshal([]byte(ib.Settings), &s)
	auth := strings.TrimSpace(s.Auth)
	if auth == "" {
		auth = "plain"
	}
	inst := Instance{
		Id: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Port: ib.Port,
		Auth: auth, Subnet: s.Subnet, Netmask: s.Netmask, DNS: s.DNS,
		SpeedLimitMbps: s.SpeedLimitMbps,
	}
	if inst.Subnet == "" {
		inst.Subnet = "10.9.0.0"
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

// NetworkCIDR parses Subnet+Netmask into network address and mask size.
func (inst Instance) NetworkCIDR() (network string, ones int, ok bool) {
	ip := net.ParseIP(inst.Subnet).To4()
	mask := net.IPMask(net.ParseIP(inst.Netmask).To4())
	if ip == nil || mask == nil {
		return "", 0, false
	}
	ones, bits := mask.Size()
	if bits != 32 {
		return "", 0, false
	}
	return ip.Mask(mask).String(), ones, true
}

// NetworkMask returns the tunnel network address and dotted netmask.
func (inst Instance) NetworkMask() (string, string) {
	ip := net.ParseIP(inst.Subnet).To4()
	mask := net.IPMask(net.ParseIP(inst.Netmask).To4())
	if ip == nil || mask == nil {
		return "", ""
	}
	return ip.Mask(mask).String(), net.IP(mask).String()
}

// SubnetCIDR renders the tunnel subnet in CIDR notation for NAT rules.
func (inst Instance) SubnetCIDR() string {
	network, ones, ok := inst.NetworkCIDR()
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s/%d", network, ones)
}
