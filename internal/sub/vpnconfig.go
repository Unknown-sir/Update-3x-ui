package sub

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
)

// VPNClientConfig is a downloadable/credential view of one OpenVPN or Cisco
// AnyConnect client. Neither protocol has a share-link URI form, so the panel
// and the subscription page render these instead of a link line.
type VPNClientConfig struct {
	Protocol string `json:"protocol" example:"openvpn"`
	Remark   string `json:"remark" example:"OpenVPN-1194"`
	Server   string `json:"server" example:"vpn.example.com"`
	Port     int    `json:"port" example:"1194"`
	Email    string `json:"email" example:"user1"`
	Username string `json:"username" example:"user1"`
	Password string `json:"password" example:"s3cr3t"`
	Config   string `json:"config,omitempty"`
}

func vpnInboundsWhere(extra string, arg any) ([]*model.Inbound, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := db.Model(model.Inbound{}).Where(`id in (
		SELECT DISTINCT inbounds.id
		FROM inbounds
		JOIN client_inbounds ON client_inbounds.inbound_id = inbounds.id
		JOIN clients ON clients.id = client_inbounds.client_id
		WHERE inbounds.protocol in ('openvpn','cisco')
		AND inbounds.enable = ? AND `+extra+`)`, true, arg).
		Order("id ASC").Find(&inbounds).Error
	return inbounds, err
}

func vpnClientEmailsForSubID(subId string) []string {
	db := database.GetDB()
	var emails []string
	_ = db.Model(&model.ClientRecord{}).Where("sub_id = ?", subId).Pluck("email", &emails).Error
	return emails
}

// vpnEndpoint is one reachable address of an inbound: either the inbound
// itself or one of its assigned hosts.
type vpnEndpoint struct {
	server string
	port   int
	remark string
}

// vpnEndpoints fans out like the share-link path: one entry per assigned
// host, falling back to the inbound's own address when it has no hosts.
func (s *SubService) vpnEndpoints(ib *model.Inbound, client model.Client) []vpnEndpoint {
	eps := s.hostEndpoints(ib, "raw")
	if len(eps) == 0 {
		server := s.resolveInboundAddress(ib)
		if server == "" {
			return nil
		}
		return []vpnEndpoint{{server: server, port: ib.Port, remark: s.genRemark(ib, client.Email, "", "")}}
	}
	out := make([]vpnEndpoint, 0, len(eps))
	for _, ep := range eps {
		cp := maps.Clone(ep)
		s.renderHostRemark(ib, client, cp, "")
		dest, _ := cp["dest"].(string)
		if dest == "" {
			continue
		}
		port := ib.Port
		switch p := cp["port"].(type) {
		case float64:
			if int(p) > 0 {
				port = int(p)
			}
		case int:
			if p > 0 {
				port = p
			}
		}
		out = append(out, vpnEndpoint{
			server: dest,
			port:   port,
			remark: s.endpointRemark(ib, client.Email, cp, ""),
		})
	}
	return out
}

// collectVPNConfigs builds one entry per (endpoint, client) pair carrying a
// usable password.
func (s *SubService) collectVPNConfigs(inbounds []*model.Inbound, emails []string) []VPNClientConfig {
	out := []VPNClientConfig{}
	for _, ib := range inbounds {
		if ib == nil {
			continue
		}
		for _, email := range emails {
			if email == "" {
				continue
			}
			client, ok := s.clientForLink(ib, email)
			if !ok || client.Password == "" {
				continue
			}
			for _, ep := range s.vpnEndpoints(ib, client) {
				entry := VPNClientConfig{
					Protocol: string(ib.Protocol),
					Remark:   ep.remark,
					Server:   ep.server,
					Port:     ep.port,
					Email:    email,
					Username: email,
					Password: client.Password,
				}
				if ib.Protocol == model.OpenVPN {
					entry.Config = buildOpenVPNProfile(ib.Id, email, ep.server, ep.port, ib.Settings, ep.remark)
				}
				out = append(out, entry)
			}
		}
	}
	return out
}

// VPNConfigsForSubID lists OpenVPN/Cisco configs for every client of a
// subscription, backing the subpage extra section.
func (s *SubService) VPNConfigsForSubID(subId string) []VPNClientConfig {
	if database.GetDB() == nil {
		return nil
	}
	inbounds, err := vpnInboundsWhere("clients.sub_id = ?", subId)
	if err != nil || len(inbounds) == 0 {
		return nil
	}
	return s.collectVPNConfigs(inbounds, vpnClientEmailsForSubID(subId))
}

// VPNConfigsForEmail lists OpenVPN/Cisco configs of a single client,
// backing the panel client-info dialog.
func (s *SubService) VPNConfigsForEmail(email string) []VPNClientConfig {
	if strings.TrimSpace(email) == "" || database.GetDB() == nil {
		return nil
	}
	inbounds, err := vpnInboundsWhere("clients.email = ?", email)
	if err != nil || len(inbounds) == 0 {
		return nil
	}
	return s.collectVPNConfigs(inbounds, []string{email})
}

// buildOpenVPNProfile renders a passwordless client .ovpn profile: the
// panel-issued client certificate authenticates, so importing the file is
// enough to connect. Empty when any key material is missing.
func buildOpenVPNProfile(id int, email, server string, port int, settings, remark string) string {
	caCrt, taKey := openvpn.CABundle(id)
	crt, key := openvpn.ClientCertPaths(id, email)
	ca, err := os.ReadFile(caCrt)
	if err != nil {
		return ""
	}
	cert, err := os.ReadFile(crt)
	if err != nil {
		return ""
	}
	pkey, err := os.ReadFile(key)
	if err != nil {
		return ""
	}
	proto, cipher, auth := "udp", "AES-256-GCM", "SHA256"
	if settings != "" {
		var m map[string]any
		if json.Unmarshal([]byte(settings), &m) == nil {
			if p, _ := m["proto"].(string); strings.EqualFold(strings.TrimSpace(p), "tcp") {
				proto = "tcp"
			}
			if c, _ := m["cipher"].(string); strings.TrimSpace(c) != "" {
				cipher = strings.TrimSpace(c)
			}
			if a, _ := m["auth"].(string); strings.TrimSpace(a) != "" {
				auth = strings.TrimSpace(a)
			}
		}
	}
	var b strings.Builder
	if strings.TrimSpace(remark) != "" {
		fmt.Fprintf(&b, "# %s\n", strings.TrimSpace(remark))
	}
	fmt.Fprintf(&b, `client
dev tun
proto %s
remote %s %d
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
cipher %s
auth %s
verb 3
<ca>
%s
</ca>
<cert>
%s
</cert>
<key>
%s
</key>
`, proto, server, port, cipher, auth,
		strings.TrimSpace(string(ca)), strings.TrimSpace(string(cert)), strings.TrimSpace(string(pkey)))
	if ta, err := os.ReadFile(taKey); err == nil && len(strings.TrimSpace(string(ta))) > 0 {
		fmt.Fprintf(&b, "<tls-crypt>\n%s\n</tls-crypt>\n", strings.TrimSpace(string(ta)))
	}
	return b.String()
}
