package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
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

// importConcurrency 返回导入并发数（钳位 1～20）
func (a *App) importConcurrency() int {
	c := a.store.GetSettings().ImportConcurrency
	if c < 1 {
		c = 3
	}
	if c > 20 {
		c = 20
	}
	return c
}

// importResult 内部导入结果（携带准备好的 Account）
type importSlot struct {
	index  int
	result ImportResult
	acc    *models.Account // nil 表示失败
}

// runConcurrentImport 通用并发导入框架：对 items 并行执行 processFn，然后批量写入 store。
func (a *App) runConcurrentImport(n int, processFn func(idx int) importSlot) []ImportResult {
	defer a.syncMitmPoolKeys()

	concurrency := a.importConcurrency()
	utils.DLog("[导入] 开始导入 %d 条，并发=%d", n, concurrency)

	slots := make([]importSlot, n)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			slots[idx] = processFn(idx)
		}(i)
	}
	wg.Wait()

	// 收集成功的账号，批量写入 store（单次持久化）
	var accs []models.Account
	accIdxMap := make([]int, 0, n) // 记录 accs 对应的 slots 下标
	for i, s := range slots {
		if s.acc != nil {
			accs = append(accs, *s.acc)
			accIdxMap = append(accIdxMap, i)
		}
	}
	if len(accs) > 0 {
		errs := a.store.AddAccountsBatch(accs)
		for j, err := range errs {
			si := accIdxMap[j]
			if err != nil {
				slots[si].result.Success = false
				slots[si].result.Error = err.Error()
			}
		}
	}

	results := make([]ImportResult, n)
	ok, fail := 0, 0
	for i, s := range slots {
		results[i] = s.result
		if s.result.Success {
			ok++
		} else {
			fail++
		}
	}
	utils.DLog("[导入] 完成: 成功=%d 失败=%d", ok, fail)
	return results
}

func (a *App) ImportByEmailPassword(items []EmailPasswordItem) []ImportResult {
	return a.runConcurrentImport(len(items), func(idx int) importSlot {
		item := items[idx]
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
			return importSlot{index: idx, result: ImportResult{Email: item.Email, Success: false, Error: err.Error()}}
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
		a.enrichAccountInfo(acc)
		return importSlot{index: idx, result: ImportResult{Email: item.Email, Success: true}, acc: acc}
	})
}

func (a *App) ImportByRefreshToken(items []TokenItem) []ImportResult {
	return a.runConcurrentImport(len(items), func(idx int) importSlot {
		item := items[idx]
		resp, err := a.windsurfSvc.RefreshToken(item.Token)
		if err != nil {
			return importSlot{index: idx, result: ImportResult{
				Email: fmt.Sprintf("Token #%d", idx+1), Success: false, Error: err.Error(),
			}}
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
		a.enrichAccountInfo(acc)
		return importSlot{index: idx, result: ImportResult{Email: email, Success: true}, acc: acc}
	})
}

func (a *App) ImportByAPIKey(items []APIKeyItem) []ImportResult {
	return a.runConcurrentImport(len(items), func(idx int) importSlot {
		item := items[idx]
		jwt, err := a.windsurfSvc.GetJWTByAPIKey(item.APIKey)
		if err != nil {
			return importSlot{index: idx, result: ImportResult{
				Email: fmt.Sprintf("Key #%d", idx+1), Success: false, Error: err.Error(),
			}}
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
		return importSlot{index: idx, result: ImportResult{Email: acc.Email, Success: true}, acc: acc}
	})
}

func (a *App) ImportByJWT(items []JWTItem) []ImportResult {
	return a.runConcurrentImport(len(items), func(idx int) importSlot {
		item := items[idx]
		email := fmt.Sprintf("JWT #%d", idx+1)
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
		return importSlot{index: idx, result: ImportResult{Email: acc.Email, Success: true}, acc: acc}
	})
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
