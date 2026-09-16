package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

type ResellerController struct {
	resellerService service.ResellerService
	clientService   service.ClientService
	inboundService  service.InboundService
	xrayService     service.XrayService
}

// requireAdmin rejects reseller-only sessions: the management endpoints
// below stay admin-only even though they share the /resellers/ prefix.
func (a *ResellerController) requireAdmin(c *gin.Context) bool {
	if session.IsLogin(c) {
		return true
	}
	jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("admin login required"))
	return false
}

func NewResellerController(g *gin.RouterGroup) *ResellerController {
	a := &ResellerController{}
	a.initRouter(g)
	return a
}

func (a *ResellerController) initRouter(g *gin.RouterGroup) {
	r := g.Group("/resellers")
	r.GET("/list", a.list)
	r.POST("/add", a.add)
	r.POST("/update/:id", a.update)
	r.POST("/del/:id", a.del)
	r.GET("/usage/:id", a.usage)
	r.POST("/login", a.login)
	r.GET("/myInbounds", a.myInbounds)
	r.GET("/myClients", a.myClients)
	r.POST("/myClients/add", a.myAddClient)
	r.POST("/myClients/update/:email", a.myUpdateClient)
	r.POST("/myClients/del/:email", a.myDelClient)
	r.POST("/myClients/setEnable", a.mySetEnable)
}

func (a *ResellerController) list(c *gin.Context) {
	if !a.requireAdmin(c) {
		return
	}
	rows, err := a.resellerService.List()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

type resellerBody struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	TotalGB        int64  `json:"totalGB"`
	ExpiryTime     int64  `json:"expiryTime"`
	SpeedLimitMbps int    `json:"speedLimitMbps"`
	Enable         *bool  `json:"enable"`
}

func (a *ResellerController) add(c *gin.Context) {
	if !a.requireAdmin(c) {
		return
	}
	var body resellerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	enable := true
	if body.Enable != nil {
		enable = *body.Enable
	}
	row, err := a.resellerService.Create(&model.Reseller{
		Username: body.Username, Password: body.Password,
		TotalGB: body.TotalGB, ExpiryTime: body.ExpiryTime,
		SpeedLimitMbps: body.SpeedLimitMbps, Enable: enable,
	})
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, row, nil)
}

func (a *ResellerController) update(c *gin.Context) {
	if !a.requireAdmin(c) {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	var body resellerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	enable := true
	if body.Enable != nil {
		enable = *body.Enable
	}
	row, err := a.resellerService.Update(id, &model.Reseller{
		Password: body.Password, TotalGB: body.TotalGB,
		ExpiryTime: body.ExpiryTime, SpeedLimitMbps: body.SpeedLimitMbps,
		Enable: enable,
	}, body.Password != "")
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, row, nil)
}

func (a *ResellerController) del(c *gin.Context) {
	if !a.requireAdmin(c) {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	if err := a.resellerService.Delete(id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"id": id}, nil)
}

func (a *ResellerController) usage(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	// A reseller may only inspect its own usage.
	if !session.IsLogin(c) {
		self, ok := a.resellerID(c)
		if !ok || self != id {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("admin login required"))
			return
		}
	}
	used, err := a.resellerService.UsageBytes(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"resellerId": id, "usedBytes": used}, nil)
}

type resellerLoginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *ResellerController) login(c *gin.Context) {
	var body resellerLoginBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	row, err := a.resellerService.CheckLogin(body.Username, body.Password)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := session.SetResellerID(c, row.Id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"id": row.Id, "username": row.Username}, nil)
}

func (a *ResellerController) resellerID(c *gin.Context) (int, bool) {
	if id := session.GetResellerID(c); id > 0 {
		return id, true
	}
	return 0, false
}

func (a *ResellerController) myClients(c *gin.Context) {
	id, ok := a.resellerID(c)
	if !ok {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("reseller login required"))
		return
	}
	rows, err := a.clientService.ListByReseller(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, rows, nil)
}

type resellerSetEnableBody struct {
	Email  string `json:"email"`
	Enable bool   `json:"enable"`
}

func (a *ResellerController) mySetEnable(c *gin.Context) {
	id, ok := a.resellerID(c)
	if !ok {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("reseller login required"))
		return
	}
	var body resellerSetEnableBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	// Blocked while the reseller quota/expiry is exhausted.
	if err := a.clientService.ResellerSetClientEnabled(id, body.Email, body.Enable); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"email": body.Email, "enable": body.Enable}, nil)
}

type resellerInboundBrief struct {
	Id       int    `json:"id"`
	Remark   string `json:"remark"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Enable   bool   `json:"enable"`
}

// myInbounds lists enabled inbounds (without settings) so the reseller
// portal can offer an inbound picker when creating a client.
func (a *ResellerController) myInbounds(c *gin.Context) {
	if _, ok := a.resellerID(c); !ok {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("reseller login required"))
		return
	}
	inbounds, err := a.inboundService.GetAllInbounds()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	briefs := make([]resellerInboundBrief, 0, len(inbounds))
	for _, ib := range inbounds {
		if ib == nil || !ib.Enable {
			continue
		}
		briefs = append(briefs, resellerInboundBrief{
			Id: ib.Id, Remark: ib.Remark,
			Protocol: string(ib.Protocol), Port: ib.Port, Enable: ib.Enable,
		})
	}
	jsonObj(c, briefs, nil)
}

// ownedRecord loads a client record and verifies reseller ownership.
func (a *ResellerController) ownedRecord(resellerID int, email string) error {
	rec, err := a.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return err
	}
	if rec.ResellerId != resellerID {
		return common.NewError("client not found")
	}
	return nil
}

func (a *ResellerController) myAddClient(c *gin.Context) {
	id, ok := a.resellerID(c)
	if !ok {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("reseller login required"))
		return
	}
	var payload service.ClientCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if _, _, exhausted, err := a.resellerService.Status(id); err != nil || exhausted {
		if err == nil {
			err = common.NewError("reseller quota or expiry exhausted, ask admin to raise the cap")
		}
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	payload.Client.ResellerId = id
	clamped, err := a.resellerService.ClampSpeed(id, payload.Client.SpeedLimitMbps)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	payload.Client.SpeedLimitMbps = clamped
	needRestart, err := a.clientService.Create(&a.inboundService, &payload)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if needRestart || err == nil {
		notifyClientsChanged()
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientAddSuccess"), pendingNodeObj(a.inboundService.AnyNodePending(payload.InboundIds)), nil)
}

func (a *ResellerController) myUpdateClient(c *gin.Context) {
	id, ok := a.resellerID(c)
	if !ok {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("reseller login required"))
		return
	}
	email := c.Param("email")
	var req struct {
		model.Client
		LimitHwid int `json:"limitHwid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.ownedRecord(id, email); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	req.Client.ResellerId = id
	clamped, err := a.resellerService.ClampSpeed(id, req.Client.SpeedLimitMbps)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	req.Client.SpeedLimitMbps = clamped
	if req.Client.Enable {
		if _, _, exhausted, err := a.resellerService.Status(id); err != nil || exhausted {
			if err == nil {
				err = common.NewError("reseller quota or expiry exhausted, ask admin to raise the cap")
			}
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
	}
	needRestart, err := a.clientService.UpdateByEmail(&a.inboundService, email, req.Client, req.LimitHwid)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if needRestart || err == nil {
		notifyClientsChanged()
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientUpdateSuccess"), pendingNodeObj(a.clientService.HasPendingNode(&a.inboundService, email)), nil)
}

func (a *ResellerController) myDelClient(c *gin.Context) {
	id, ok := a.resellerID(c)
	if !ok {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("reseller login required"))
		return
	}
	email := c.Param("email")
	if err := a.ownedRecord(id, email); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	keepTraffic := c.Query("keepTraffic") == "1"
	needRestart, err := a.clientService.DeleteByEmail(&a.inboundService, email, keepTraffic)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	if needRestart || err == nil {
		notifyClientsChanged()
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientDeleteSuccess"), nil)
}
