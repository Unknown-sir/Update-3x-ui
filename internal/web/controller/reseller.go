package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

type ResellerController struct {
	resellerService service.ResellerService
	clientService   service.ClientService
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
	r.GET("/myClients", a.myClients)
	r.POST("/myClients/setEnable", a.mySetEnable)
}

func (a *ResellerController) list(c *gin.Context) {
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
	id, _ := strconv.Atoi(c.Param("id"))
	if err := a.resellerService.Delete(id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"id": id}, nil)
}

func (a *ResellerController) usage(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
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
	c.Set("reseller_id", row.Id)
	c.Set("reseller_username", row.Username)
	jsonObj(c, gin.H{"id": row.Id, "username": row.Username}, nil)
}

func (a *ResellerController) resellerID(c *gin.Context) (int, bool) {
	if v, ok := c.Get("reseller_id"); ok {
		if id, ok2 := v.(int); ok2 && id > 0 {
			return id, true
		}
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
	// Block edits while the reseller quota/expiry is exhausted.
	rows, err := a.clientService.ListByReseller(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	_ = rows
	if err := a.clientService.ResellerSetClientEnabled(id, body.Email, body.Enable); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"email": body.Email, "enable": body.Enable}, nil)
}
