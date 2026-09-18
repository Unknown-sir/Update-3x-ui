package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/cisco"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// disabledClientEmails mirrors the tuic reconcile filter: traffic rows
// flipped off by quota/expiry pruning keep the user out of the daemon.
func disabledClientEmails(ids []int) map[string]struct{} {
	off := map[string]struct{}{}
	if len(ids) == 0 {
		return off
	}
	var rows []xray.ClientTraffic
	if err := database.GetDB().Model(xray.ClientTraffic{}).
		Where("inbound_id IN ? AND enable = ?", ids, false).
		Select("email").Find(&rows).Error; err != nil {
		return off
	}
	for _, row := range rows {
		off[row.Email] = struct{}{}
	}
	return off
}

// resellerByEmail maps client emails to their owning reseller for speed clamp.
func resellerByEmail(emails []string) map[string]int {
	out := map[string]int{}
	if len(emails) == 0 {
		return out
	}
	var rows []model.ClientRecord
	if err := database.GetDB().Model(model.ClientRecord{}).
		Where("email IN ?", emails).Select("email", "reseller_id").Find(&rows).Error; err != nil {
		return out
	}
	for _, row := range rows {
		out[row.Email] = row.ResellerId
	}
	return out
}

// DesiredOpenvpnInstances lists enabled local OpenVPN inbounds with
// disabled clients filtered and per-user speed clamped to the reseller cap.
func (s *InboundService) DesiredOpenvpnInstances() ([]openvpn.Instance, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ? AND enable = ? AND node_id IS NULL", model.OpenVPN, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(inbounds))
	for _, ib := range inbounds {
		ids = append(ids, ib.Id)
	}
	off := disabledClientEmails(ids)
	out := make([]openvpn.Instance, 0, len(inbounds))
	for _, ib := range inbounds {
		inst, ok := openvpn.InstanceFromInbound(ib)
		if !ok {
			continue
		}
		emails := make([]string, 0, len(inst.Clients))
		for _, c := range inst.Clients {
			emails = append(emails, c.Email)
		}
		owners := resellerByEmail(emails)
		rs := ResellerService{}
		kept := inst.Clients[:0]
		for _, c := range inst.Clients {
			if _, skip := off[c.Email]; skip {
				continue
			}
			if rid := owners[c.Email]; rid > 0 {
				if clamped, err := rs.ClampSpeed(rid, c.SpeedLimitMbps); err == nil {
					c.SpeedLimitMbps = clamped
				}
			}
			kept = append(kept, c)
		}
		inst.Clients = kept
		if len(inst.Clients) == 0 {
			continue
		}
		out = append(out, inst)
	}
	return out, nil
}

// DesiredCiscoInstances lists enabled local ocserv inbounds, same filtering.
func (s *InboundService) DesiredCiscoInstances() ([]cisco.Instance, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ? AND enable = ? AND node_id IS NULL", model.Cisco, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(inbounds))
	for _, ib := range inbounds {
		ids = append(ids, ib.Id)
	}
	off := disabledClientEmails(ids)
	out := make([]cisco.Instance, 0, len(inbounds))
	for _, ib := range inbounds {
		inst, ok := cisco.InstanceFromInbound(ib)
		if !ok {
			continue
		}
		emails := make([]string, 0, len(inst.Clients))
		for _, c := range inst.Clients {
			emails = append(emails, c.Email)
		}
		owners := resellerByEmail(emails)
		rs := ResellerService{}
		kept := inst.Clients[:0]
		for _, c := range inst.Clients {
			if _, skip := off[c.Email]; skip {
				continue
			}
			if rid := owners[c.Email]; rid > 0 {
				if clamped, err := rs.ClampSpeed(rid, c.SpeedLimitMbps); err == nil {
					c.SpeedLimitMbps = clamped
				}
			}
			kept = append(kept, c)
		}
		inst.Clients = kept
		if len(inst.Clients) == 0 {
			continue
		}
		out = append(out, inst)
	}
	return out, nil
}

func (s *InboundService) applyLocalOpenvpn(inboundId int) {
	desired, err := s.DesiredOpenvpnInstances()
	if err != nil {
		return
	}
	for _, inst := range desired {
		if inst.Id == inboundId {
			_ = openvpn.GetManager().Ensure(inst)
			return
		}
	}
	openvpn.GetManager().Remove(inboundId)
}

func (s *InboundService) applyLocalCisco(inboundId int) {
	desired, err := s.DesiredCiscoInstances()
	if err != nil {
		return
	}
	for _, inst := range desired {
		if inst.Id == inboundId {
			_ = cisco.GetManager().Ensure(inst)
			return
		}
	}
	cisco.GetManager().Remove(inboundId)
}
