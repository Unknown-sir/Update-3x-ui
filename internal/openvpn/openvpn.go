// Package openvpn manages OpenVPN inbounds as supervised sidecar processes.
// One openvpn process per inbound: the panel owns a private PKI per inbound,
// issues a client certificate per user (no passwords involved), writes
// server.conf plus per-client CCD entries, and restarts the daemon when the
// desired state changes. Traffic comes from the daemon status file.
package openvpn

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
	Proto          string        `json:"proto"`
	Subnet         string        `json:"subnet"`
	Netmask        string        `json:"netmask"`
	Cipher         string        `json:"cipher"`
	Auth           string        `json:"auth"`
	DNS            []string      `json:"dns"`
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
	Cipher         string
	Auth           string
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
		inst.BindTo(), inst.Proto, inst.Subnet, inst.Netmask,
		inst.Cipher, inst.Auth, strings.Join(inst.DNS, ","),
		strconv.Itoa(inst.SpeedLimitMbps),
	}, "|")
}

func (inst Instance) UsersFingerprint() string {
	emails := make([]string, 0, len(inst.Clients))
	for _, c := range inst.Clients {
		emails = append(emails, c.Email)
	}
	sort.Strings(emails)
	return strings.Join(emails, ",")
}

// SortedEmails returns client emails in stable CCD allocation order.
func (inst Instance) SortedEmails() []string {
	emails := make([]string, 0, len(inst.Clients))
	for _, c := range inst.Clients {
		if c.Email != "" {
			emails = append(emails, c.Email)
		}
	}
	sort.Strings(emails)
	return emails
}

func InstanceFromInbound(ib *model.Inbound) (Instance, bool) {
	if ib.Protocol != model.OpenVPN {
		return Instance{}, false
	}
	var s Settings
	_ = json.Unmarshal([]byte(ib.Settings), &s)
	proto := strings.ToLower(strings.TrimSpace(s.Proto))
	if proto != "tcp" {
		proto = "udp"
	}
	inst := Instance{
		Id: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Port: ib.Port,
		Proto: proto, Subnet: s.Subnet, Netmask: s.Netmask,
		Cipher: s.Cipher, Auth: s.Auth, DNS: s.DNS,
		SpeedLimitMbps: s.SpeedLimitMbps,
	}
	if inst.Subnet == "" {
		inst.Subnet = "10.8.0.0"
	}
	if inst.Netmask == "" {
		inst.Netmask = "255.255.255.0"
	}
	if inst.Cipher == "" {
		inst.Cipher = "AES-256-GCM"
	}
	if inst.Auth == "" {
		inst.Auth = "SHA256"
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

// ClientIP allocates a stable tunnel address per client: server takes .1,
// clients take .2 and up in sorted-email order.
func (inst Instance) ClientIP(email string) string {
	base := net.ParseIP(inst.Subnet).To4()
	if base == nil {
		return ""
	}
	idx := -1
	for i, e := range inst.SortedEmails() {
		if e == email {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}
	ip := make(net.IP, 4)
	copy(ip, base)
	v := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	v += uint32(idx + 2)
	ip[0], ip[1], ip[2], ip[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
	return ip.String()
}

// SubnetCIDR renders the tunnel subnet in CIDR notation for NAT rules.
func (inst Instance) SubnetCIDR() string {
	ip := net.ParseIP(inst.Subnet).To4()
	mask := net.IPMask(net.ParseIP(inst.Netmask).To4())
	if ip == nil || mask == nil {
		return ""
	}
	ones, _ := mask.Size()
	return fmt.Sprintf("%s/%d", ip.Mask(mask).String(), ones)
}
