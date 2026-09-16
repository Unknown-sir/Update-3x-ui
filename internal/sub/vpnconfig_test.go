package sub

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedVPNInbound(t *testing.T, protocol model.Protocol, settings string) *model.Inbound {
	t.Helper()
	db := database.GetDB()
	inbound := &model.Inbound{
		Listen:            "0.0.0.0",
		Port:              1194,
		Protocol:          protocol,
		Remark:            string(protocol) + "-test",
		Enable:            true,
		ShareAddrStrategy: "custom",
		ShareAddr:         "vpn.example.com",
		Settings:          settings,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client := &model.ClientRecord{Email: "u@vpn", SubID: "sub-vpn", Enable: true, Password: "s3cr3t"}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatalf("attach client: %v", err)
	}
	return inbound
}

func TestVPNConfigsForSubID(t *testing.T) {
	initSubDB(t)
	seedVPNInbound(t, model.OpenVPN, `{"proto":"udp","clients":[{"email":"u@vpn","password":"s3cr3t","enable":true}]}`)

	got := NewSubService("").VPNConfigsForSubID("sub-vpn")
	if len(got) != 1 {
		t.Fatalf("configs = %d, want 1: %+v", len(got), got)
	}
	c := got[0]
	if c.Protocol != "openvpn" || c.Server != "vpn.example.com" || c.Port != 1194 {
		t.Fatalf("unexpected endpoint: %+v", c)
	}
	if c.Username != "u@vpn" || c.Password != "s3cr3t" {
		t.Fatalf("unexpected credentials: %+v", c)
	}
	for _, want := range []string{"remote vpn.example.com 1194", "proto udp", "auth-user-pass", "cipher AES-256-GCM"} {
		if !strings.Contains(c.Config, want) {
			t.Fatalf("ovpn config missing %q:\n%s", want, c.Config)
		}
	}
}

func TestVPNConfigsForEmailCisco(t *testing.T) {
	initSubDB(t)
	seedVPNInbound(t, model.Cisco, `{"auth":"plain","clients":[{"email":"u@vpn","password":"s3cr3t","enable":true}]}`)

	got := NewSubService("").VPNConfigsForEmail("u@vpn")
	if len(got) != 1 {
		t.Fatalf("configs = %d, want 1: %+v", len(got), got)
	}
	if got[0].Protocol != "cisco" || got[0].Config != "" {
		t.Fatalf("cisco entry must carry credentials without a file: %+v", got[0])
	}
	if got[0].Username != "u@vpn" || got[0].Password != "s3cr3t" {
		t.Fatalf("unexpected credentials: %+v", got[0])
	}
}

func TestBuildOpenVPNConfigTCP(t *testing.T) {
	got := buildOpenVPNConfig("vpn.example.com", 443, `{"proto":"tcp","cipher":"AES-128-GCM","auth":"SHA512"}`, "r")
	for _, want := range []string{"proto tcp", "remote vpn.example.com 443", "cipher AES-128-GCM", "auth SHA512"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ovpn config missing %q:\n%s", want, got)
		}
	}
}
