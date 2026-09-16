package sub

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
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

// collectVPNConfigs builds one entry per (inbound, client) pair carrying a
// usable password. Server falls back through the inbound share-address chain.
func (s *SubService) collectVPNConfigs(inbounds []*model.Inbound, emails []string) []VPNClientConfig {
	out := []VPNClientConfig{}
	for _, ib := range inbounds {
		if ib == nil {
			continue
		}
		server := s.resolveInboundAddress(ib)
		if server == "" {
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
			entry := VPNClientConfig{
				Protocol: string(ib.Protocol),
				Remark:   ib.Remark,
				Server:   server,
				Port:     ib.Port,
				Email:    email,
				Username: email,
				Password: client.Password,
			}
			if ib.Protocol == model.OpenVPN {
				entry.Config = buildOpenVPNConfig(server, ib.Port, ib.Settings, ib.Remark)
			}
			out = append(out, entry)
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

// buildOpenVPNConfig renders a standard client .ovpn profile. Credentials
// stay out of the file on purpose: apps prompt for them via auth-user-pass.
func buildOpenVPNConfig(server string, port int, settings, remark string) string {
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
auth-user-pass
auth-nocache
cipher %s
auth %s
verb 3
`, proto, server, port, cipher, auth)
	return b.String()
}
