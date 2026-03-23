package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
)

// ═══════════════════════════════════════
// 批量导入 + 单个添加
// ═══════════════════════════════════════

type ImportResult struct {
	Email   string `json:"email"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type EmailPasswordItem struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	AltPassword string `json:"alt_password,omitempty"`
	Remark      string `json:"remark"`
}
type TokenItem struct {
	Token  string `json:"token"`
	Remark string `json:"remark"`
}
type APIKeyItem struct {
	APIKey string `json:"api_key"`
	Remark string `json:"remark"`
}
type JWTItem struct {
	JWT    string `json:"jwt"`
	Remark string `json:"remark"`
}

func (a *App) ImportByEmailPassword(items []EmailPasswordItem) []ImportResult {
	defer a.syncMitmPoolKeys() // 导入完成后同步号池
	var results []ImportResult
	for _, item := range items {
		passwords := []string{item.Password}
		if item.AltPassword != "" && item.AltPassword != item.Password {
			passwords = append(passwords, item.AltPassword)
		}
		var resp *services.FirebaseSignInResp
		var err error
		var usedPassword string
		for _, pw := range passwords {
			if pw == "" {
				continue
			}
			resp, err = a.windsurfSvc.LoginWithEmail(item.Email, pw)
			if err == nil {
				usedPassword = pw
				break
			}
		}
		if err != nil {
			results = append(results, ImportResult{Email: item.Email, Success: false, Error: err.Error()})
			continue
		}
		nickname := item.Remark
		if nickname == "" {
			nickname = strings.Split(item.Email, "@")[0]
		}
		acc := models.NewAccount(item.Email, usedPassword, nickname)
		acc.Token = resp.IDToken
		acc.RefreshToken = resp.RefreshToken
		acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		acc.Remark = item.Remark
		a.enrichAccountInfo(acc) // 完整版：包含 RegisterUser 获取 API Key（MITM 号池需要）
		if err := a.store.AddAccount(*acc); err != nil {
			results = append(results, ImportResult{Email: item.Email, Success: false, Error: err.Error()})
			continue
		}
		results = append(results, ImportResult{Email: item.Email, Success: true})
	}
	return results
}

func (a *App) ImportByRefreshToken(items []TokenItem) []ImportResult {
	defer a.syncMitmPoolKeys()
	var results []ImportResult
	for i, item := range items {
		resp, err := a.windsurfSvc.RefreshToken(item.Token)
		if err != nil {
			results = append(results, ImportResult{
				Email: fmt.Sprintf("Token #%d", i+1), Success: false, Error: err.Error(),
			})
			continue
		}
		email, _ := a.windsurfSvc.GetAccountInfo(resp.IDToken)
		if email == "" {
			email = fmt.Sprintf("user_%s", resp.UserID[:minInt(8, len(resp.UserID))])
		}
		nickname := item.Remark
		if nickname == "" {
			nickname = strings.Split(email, "@")[0]
		}
		acc := models.NewAccount(email, "", nickname)
		acc.Token = resp.IDToken
		acc.RefreshToken = resp.RefreshToken
		acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		acc.Remark = item.Remark
		a.enrichAccountInfo(acc) // 完整版：获取 API Key
		if err := a.store.AddAccount(*acc); err != nil {
			results = append(results, ImportResult{Email: email, Success: false, Error: err.Error()})
			continue
		}
		results = append(results, ImportResult{Email: email, Success: true})
	}
	return results
}

func (a *App) ImportByAPIKey(items []APIKeyItem) []ImportResult {
	defer a.syncMitmPoolKeys()
	var results []ImportResult
	for i, item := range items {
		jwt, err := a.windsurfSvc.GetJWTByAPIKey(item.APIKey)
		if err != nil {
			results = append(results, ImportResult{
				Email: fmt.Sprintf("Key #%d", i+1), Success: false, Error: err.Error(),
			})
			continue
		}

		email := fmt.Sprintf("%s...%s", item.APIKey[:minInt(12, len(item.APIKey))],
			item.APIKey[maxInt(0, len(item.APIKey)-6):])

		acc := models.NewAccount(email, "", item.Remark)
		acc.Token = jwt
		acc.WindsurfAPIKey = item.APIKey
		acc.Remark = item.Remark
		a.enrichAccountInfoLite(acc)
		if item.Remark == "" {
			acc.Nickname = strings.Split(acc.Email, "@")[0]
		}

		if err := a.store.AddAccount(*acc); err != nil {
			results = append(results, ImportResult{Email: acc.Email, Success: false, Error: err.Error()})
			continue
		}
		results = append(results, ImportResult{Email: acc.Email, Success: true})
	}
	return results
}

func (a *App) ImportByJWT(items []JWTItem) []ImportResult {
	defer a.syncMitmPoolKeys()
	var results []ImportResult
	for i, item := range items {
		email := fmt.Sprintf("JWT #%d", i+1)
		acc := models.NewAccount(email, "", item.Remark)
		acc.Token = item.JWT
		acc.Remark = item.Remark
		a.enrichAccountInfoLite(acc)
		// 尝试通过 RegisterUser 获取 API Key，使账号后续可通过 GetJWTByAPIKey 持续刷新凭证
		if acc.WindsurfAPIKey == "" && acc.Token != "" {
			if reg, err := a.windsurfSvc.RegisterUser(acc.Token); err == nil && reg != nil && reg.APIKey != "" {
				acc.WindsurfAPIKey = reg.APIKey
			}
		}
		if item.Remark == "" {
			acc.Nickname = strings.Split(acc.Email, "@")[0]
		}

		if err := a.store.AddAccount(*acc); err != nil {
			results = append(results, ImportResult{Email: acc.Email, Success: false, Error: err.Error()})
			continue
		}
		results = append(results, ImportResult{Email: acc.Email, Success: true})
	}
	return results
}

// 单个添加
func (a *App) AddSingleAccount(mode string, value string, remark string) ImportResult {
	switch mode {
	case "api_key":
		items := []APIKeyItem{{APIKey: value, Remark: remark}}
		r := a.ImportByAPIKey(items)
		if len(r) > 0 {
			return r[0]
		}
	case "jwt":
		items := []JWTItem{{JWT: value, Remark: remark}}
		r := a.ImportByJWT(items)
		if len(r) > 0 {
			return r[0]
		}
	case "refresh_token":
		items := []TokenItem{{Token: value, Remark: remark}}
		r := a.ImportByRefreshToken(items)
		if len(r) > 0 {
			return r[0]
		}
	case "password":
		var cred struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			AltPassword string `json:"alt_password"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &cred); err != nil {
			return ImportResult{Email: "?", Success: false, Error: "邮箱密码格式错误"}
		}
		if cred.Email == "" || cred.Password == "" {
			return ImportResult{Email: "?", Success: false, Error: "请填写邮箱与密码"}
		}
		r := a.ImportByEmailPassword([]EmailPasswordItem{{
			Email: cred.Email, Password: cred.Password, AltPassword: cred.AltPassword, Remark: remark,
		}})
		if len(r) > 0 {
			return r[0]
		}
	}
	return ImportResult{Email: "?", Success: false, Error: "无效的导入类型"}
}
