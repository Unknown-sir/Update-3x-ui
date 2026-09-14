package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// ListByReseller returns client records owned by one reseller.
func (s *ClientService) ListByReseller(resellerID int) ([]model.ClientRecord, error) {
	db := database.GetDB()
	var rows []model.ClientRecord
	if err := db.Where("reseller_id = ?", resellerID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ResellerSetClientEnabled lets a reseller stop/start its own client.
// Blocked while the reseller quota/expiry is exhausted; only the admin
// raising the cap re-enables the accounts via the limit job.
func (s *ClientService) ResellerSetClientEnabled(resellerID int, email string, enable bool) error {
	db := database.GetDB()
	row := &model.Reseller{}
	if err := db.First(row, resellerID).Error; err != nil {
		return err
	}
	rec := &model.ClientRecord{}
	if err := db.Where("email = ? AND reseller_id = ?", email, resellerID).First(rec).Error; err != nil {
		return common.NewError("client not found")
	}
	if enable {
		var emails []string
		if err := db.Model(&model.ClientRecord{}).Where("reseller_id = ?", resellerID).Pluck("email", &emails).Error; err != nil {
			return err
		}
		var used int64
		if len(emails) > 0 {
			var agg struct{ Up, Down int64 }
			if err := db.Model(&xray.ClientTraffic{}).Where("email IN ?", emails).Select("COALESCE(SUM(up),0) AS up, COALESCE(SUM(down),0) AS down").Scan(&agg).Error; err != nil {
				return err
			}
			used = agg.Up + agg.Down
		}
		if row.IsExhausted(used, time.Now().UnixMilli()) {
			return common.NewError("reseller quota or expiry exhausted, ask admin to raise the cap")
		}
	}
	if err := db.Model(&model.ClientRecord{}).Where("email = ?", email).Update("enable", enable).Error; err != nil {
		return err
	}
	_ = db.Model(&xray.ClientTraffic{}).Where("email = ?", email).Update("enable", enable).Error
	return nil
}
