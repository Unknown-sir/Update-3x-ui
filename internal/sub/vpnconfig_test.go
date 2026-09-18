package sub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
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

// seedClientPKI writes stub key material so profile embedding has files to read.
func seedClientPKI(t *testing.T, id int, email string) {
	t.Helper()
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	caCrt, _ := openvpn.CABundle(id)
	crt, key := openvpn.ClientCertPaths(id, email)
	for _, p := range []string{caCrt, crt, key} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("stub-"+filepath.Base(p)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVPNConfigsForSubID(t *testing.T) {
	initSubDB(t)
	ib := seedVPNInbound(t, model.OpenVPN, `{"proto":"udp","clients":[{"email":"u@vpn","password":"s3cr3t","enable":true}]}`)
	seedClientPKI(t, ib.Id, "u@vpn")

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
	for _, want := range []string{"remote vpn.example.com 1194", "proto udp", "cipher AES-256-GCM", "<ca>", "<cert>", "<key>"} {
		if !strings.Contains(c.Config, want) {
			t.Fatalf("ovpn config missing %q:\n%s", want, c.Config)
		}
	}
	if strings.Contains(c.Config, "auth-user-pass") {
		t.Fatalf("ovpn profile must be passwordless:\n%s", c.Config)
	}
}

func TestVPNConfigsUseAssignedHosts(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	ib := seedVPNInbound(t, model.OpenVPN, `{"proto":"udp","clients":[{"email":"u@vpn","password":"s3cr3t","enable":true}]}`)
	seedClientPKI(t, ib.Id, "u@vpn")
	if err := db.Create(&model.Host{
		InboundId: ib.Id,
		Remark:    "edge",
		Address:   "edge.example.com",
		Port:      443,
	}).Error; err != nil {
		t.Fatalf("create host: %v", err)
	}

	got := NewSubService("").VPNConfigsForSubID("sub-vpn")
	if len(got) != 1 {
		t.Fatalf("configs = %d, want 1 host entry: %+v", len(got), got)
	}
	if got[0].Server != "edge.example.com" || got[0].Port != 443 {
		t.Fatalf("host endpoint not honored: %+v", got[0])
	}
	if !strings.Contains(got[0].Config, "remote edge.example.com 443") {
		t.Fatalf("ovpn config missing host remote:\n%s", got[0].Config)
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

func TestBuildOpenVPNProfileTCP(t *testing.T) {
	initSubDB(t)
	seedClientPKI(t, 999, "u@vpn")
	// Point the builder at the temp PKI by running under that bin folder id.
	got := buildOpenVPNProfile(999, "u@vpn", "vpn.example.com", 443, `{"proto":"tcp","cipher":"AES-128-GCM","auth":"SHA512"}`, "r")
	for _, want := range []string{"proto tcp", "remote vpn.example.com 443", "cipher AES-128-GCM", "auth SHA512"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ovpn config missing %q:\n%s", want, got)
		}
	}
}
