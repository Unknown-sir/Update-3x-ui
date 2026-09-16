package service

import (
	"errors"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type ResellerService struct{}

func (s *ResellerService) List() ([]model.Reseller, error) {
	db := database.GetDB()
	var rows []model.Reseller
	if err := db.Model(&model.Reseller{}).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Password = ""
	}
	return rows, nil
}

func (s *ResellerService) Create(r *model.Reseller) (*model.Reseller, error) {
	if r.Username == "" {
		return nil, errors.New("username is required")
	}
	hash, err := crypto.HashPasswordAsBcrypt(r.Password)
	if err != nil {
		return nil, err
	}
	r.Password = hash
	db := database.GetDB()
	if err := db.Create(r).Error; err != nil {
		return nil, err
	}
	out := *r
	out.Password = ""
	return &out, nil
}

func (s *ResellerService) Update(id int, patch *model.Reseller, changePassword bool) (*model.Reseller, error) {
	db := database.GetDB()
	row := &model.Reseller{}
	if err := db.First(row, id).Error; err != nil {
		return nil, err
	}
	row.TotalGB = patch.TotalGB
	row.ExpiryTime = patch.ExpiryTime
	row.SpeedLimitMbps = patch.SpeedLimitMbps
	row.Enable = patch.Enable
	if changePassword && patch.Password != "" {
		hash, err := crypto.HashPasswordAsBcrypt(patch.Password)
		if err != nil {
			return nil, err
		}
		row.Password = hash
		row.LoginEpoch = time.Now().UnixMilli()
	}
	if err := db.Save(row).Error; err != nil {
		return nil, err
	}
	row.Password = ""
	return row, nil
}

func (s *ResellerService) Delete(id int) error {
	db := database.GetDB()
	return db.Delete(&model.Reseller{}, id).Error
}

// UsageBytes sums up+down across all clients owned by the reseller.
func (s *ResellerService) UsageBytes(resellerID int) (int64, error) {
	db := database.GetDB()
	var emails []string
	if err := db.Model(&model.ClientRecord{}).Where("reseller_id = ?", resellerID).Pluck("email", &emails).Error; err != nil {
		return 0, err
	}
	if len(emails) == 0 {
		return 0, nil
	}
	var agg struct{ Up, Down int64 }
	if err := db.Model(&xray.ClientTraffic{}).Where("email IN ?", emails).Select("COALESCE(SUM(up),0) AS up, COALESCE(SUM(down),0) AS down").Scan(&agg).Error; err != nil {
		return 0, err
	}
	return agg.Up + agg.Down, nil
}

// Status loads the reseller row with its current usage and exhaustion flag.
func (s *ResellerService) Status(resellerID int) (*model.Reseller, int64, bool, error) {
	db := database.GetDB()
	row := &model.Reseller{}
	if err := db.First(row, resellerID).Error; err != nil {
		return nil, 0, true, err
	}
	used, err := s.UsageBytes(resellerID)
	if err != nil {
		return nil, 0, true, err
	}
	return row, used, row.IsExhausted(used, time.Now().UnixMilli()), nil
}

// ClampSpeed caps a requested per-user limit to the reseller cap.
// A reseller cap of 0 means unlimited and leaves the request untouched.
func (s *ResellerService) ClampSpeed(resellerID int, want int) (int, error) {
	db := database.GetDB()
	row := &model.Reseller{}
	if err := db.Select("speed_limit_mbps").First(row, resellerID).Error; err != nil {
		return 0, err
	}
	if row.SpeedLimitMbps > 0 && (want <= 0 || want > row.SpeedLimitMbps) {
		return row.SpeedLimitMbps, nil
	}
	return want, nil
}

// SetClientsEnabled flips enable on every owned client record + traffic row.
func (s *ResellerService) SetClientsEnabled(resellerID int, enable bool) error {
	db := database.GetDB()
	var emails []string
	if err := db.Model(&model.ClientRecord{}).Where("reseller_id = ?", resellerID).Pluck("email", &emails).Error; err != nil {
		return err
	}
	if err := db.Model(&model.ClientRecord{}).Where("reseller_id = ?", resellerID).Update("enable", enable).Error; err != nil {
		return err
	}
	if len(emails) > 0 {
		_ = db.Model(&xray.ClientTraffic{}).Where("email IN ?", emails).Update("enable", enable).Error
	}
	return nil
}

func (s *ResellerService) CheckLogin(username, password string) (*model.Reseller, error) {
	db := database.GetDB()
	row := &model.Reseller{}
	if err := db.Where("username = ?", username).First(row).Error; err != nil {
		return nil, errors.New("invalid credentials")
	}
	if !row.Enable {
		return nil, errors.New("reseller disabled")
	}
	if !crypto.CheckPasswordHash(row.Password, password) {
		return nil, errors.New("invalid credentials")
	}
	return row, nil
}
