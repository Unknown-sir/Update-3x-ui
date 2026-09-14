package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// ResellerLimitJob stops every owned account once the reseller hits its
// volume cap or expiry. Raising the cap does not auto-start accounts;
// the admin restarts them explicitly.
type ResellerLimitJob struct{}

func NewResellerLimitJob() *ResellerLimitJob { return &ResellerLimitJob{} }

func (j *ResellerLimitJob) Run() {
	db := database.GetDB()
	if db == nil {
		return
	}
	var sellers []model.Reseller
	if err := db.Find(&sellers).Error; err != nil {
		return
	}
	now := time.Now().UnixMilli()
	for _, r := range sellers {
		var emails []string
		if err := db.Model(&model.ClientRecord{}).Where("reseller_id = ?", r.Id).Pluck("email", &emails).Error; err != nil || len(emails) == 0 {
			continue
		}
		var agg struct{ Up, Down int64 }
		if err := db.Model(&xray.ClientTraffic{}).Where("email IN ?", emails).Select("COALESCE(SUM(up),0) AS up, COALESCE(SUM(down),0) AS down").Scan(&agg).Error; err != nil {
			continue
		}
		if !r.IsExhausted(agg.Up+agg.Down, now) {
			continue
		}
		if err := db.Model(&model.ClientRecord{}).Where("reseller_id = ?", r.Id).Update("enable", false).Error; err != nil {
			continue
		}
		_ = db.Model(&xray.ClientTraffic{}).Where("email IN ?", emails).Update("enable", false).Error
		logger.Infof("Reseller %s exhausted, stopped %d accounts", r.Username, len(emails))
	}
}
