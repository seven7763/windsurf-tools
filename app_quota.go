package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// ═══════════════════════════════════════
// 自动刷新 Token / JWT + 额度监控
// ═══════════════════════════════════════

func (a *App) startAutoRefresh() {
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelAutoRefresh = cancel
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.refreshAllTokens()
			}
		}
	}()
}

func (a *App) startAutoQuotaRefresh() {
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelAutoQuotaRefresh = cancel
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		a.refreshDueQuotas()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.refreshDueQuotas()
			}
		}
	}()
}

func clampQuotaHotPollSeconds(sec int) int {
	if sec < 5 {
		return 5
	}
	if sec > 60 {
		return 60
	}
	return sec
}

func (a *App) stopQuotaHotPoll() {
	if a.cancelQuotaHotPoll != nil {
		a.cancelQuotaHotPoll()
		a.cancelQuotaHotPoll = nil
	}
}

// restartQuotaHotPollIfNeeded 在「定期同步额度 + 用尽自动切号」同时开启时，对当前 windsurf 会话高频拉额度以便尽快切号。
func (a *App) restartQuotaHotPollIfNeeded() {
	a.stopQuotaHotPoll()
	settings := a.store.GetSettings()
	if !settings.AutoRefreshQuotas || !settings.AutoSwitchOnQuotaExhausted {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelQuotaHotPoll = cancel
	go a.quotaHotPollLoop(ctx)
}

func (a *App) quotaHotPollLoop(ctx context.Context) {
	for {
		a.pollCurrentSessionQuotaAndMaybeSwitch()
		sec := clampQuotaHotPollSeconds(a.store.GetSettings().QuotaHotPollSeconds)
		t := time.NewTimer(time.Duration(sec) * time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

func (a *App) pollCurrentSessionQuotaAndMaybeSwitch() {
	settings := a.store.GetSettings()
	if !settings.AutoRefreshQuotas || !settings.AutoSwitchOnQuotaExhausted {
		return
	}

	a.lastQuotaHotSwitchMu.Lock()
	if t := a.lastQuotaHotSwitch; !t.IsZero() && time.Since(t) < 12*time.Second {
		a.lastQuotaHotSwitchMu.Unlock()
		return
	}
	a.lastQuotaHotSwitchMu.Unlock()

	auth, err := a.switchSvc.GetCurrentAuth()
	if err != nil || auth == nil {
		return
	}
	curID := a.findAccountIDForWindsurfAuth(auth)
	if curID == "" {
		return
	}
	cur, err := a.store.GetAccount(curID)
	if err != nil {
		return
	}
	if cur.WindsurfAPIKey == "" && strings.TrimSpace(cur.Token) == "" && strings.TrimSpace(auth.Token) == "" &&
		cur.RefreshToken == "" && (cur.Email == "" || cur.Password == "") {
		return
	}

	copyAcc := cur
	if t := strings.TrimSpace(auth.Token); t != "" {
		copyAcc.Token = t
	} else {
		a.syncAccountCredentials(&copyAcc)
	}
	// 热轮询仅拉额度，避免 RegisterUser / GetAccountInfo 等拖慢后台与重复请求
	a.enrichAccountQuotaOnly(&copyAcc)
	copyAcc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
	if err := a.store.UpdateAccount(copyAcc); err != nil {
		return
	}
	if !utils.AccountQuotaExhausted(&copyAcc) {
		return
	}
	if settings.MitmOnly {
		return
	}
	if _, err := a.AutoSwitchToNext(curID, settings.AutoSwitchPlanFilter); err != nil {
		return
	}
	a.lastQuotaHotSwitchMu.Lock()
	a.lastQuotaHotSwitch = time.Now()
	a.lastQuotaHotSwitchMu.Unlock()
}

func (a *App) refreshDueQuotas() {
	var switchAfterUnlock struct {
		currentID  string
		planFilter string
	}

	a.mu.Lock()
	settings := a.store.GetSettings()
	if !settings.AutoRefreshQuotas {
		a.mu.Unlock()
		return
	}
	policy := strings.TrimSpace(settings.QuotaRefreshPolicy)
	if policy == "" {
		policy = utils.QuotaPolicyHybrid
	}
	now := time.Now()
	customMins := settings.QuotaCustomIntervalMinutes
	accounts := a.store.GetAllAccounts()
	// 当前 Windsurf 会话对应账号由热轮询高频盯额度 + 用尽切号；此处只按策略刷新「非当前」号，避免重复打接口。
	var skipAccountID string
	if settings.AutoSwitchOnQuotaExhausted {
		if auth, err := a.switchSvc.GetCurrentAuth(); err == nil && auth != nil {
			skipAccountID = a.findAccountIDForWindsurfAuth(auth)
		}
	}
	for _, acc := range accounts {
		if skipAccountID != "" && acc.ID == skipAccountID {
			continue
		}
		if !utils.QuotaRefreshDue(acc.LastQuotaUpdate, policy, customMins, now) {
			continue
		}
		if acc.WindsurfAPIKey == "" && acc.Token == "" && acc.RefreshToken == "" && (acc.Email == "" || acc.Password == "") {
			continue
		}
		copyAcc := acc
		a.syncAccountCredentials(&copyAcc)
		a.enrichAccountInfo(&copyAcc)
		copyAcc.LastQuotaUpdate = now.Format(time.RFC3339)
		_ = a.store.UpdateAccount(copyAcc)
	}

	if settings.AutoSwitchOnQuotaExhausted && !settings.MitmOnly {
		auth, err := a.switchSvc.GetCurrentAuth()
		if err == nil && auth != nil {
			curID := a.findAccountIDForWindsurfAuth(auth)
			if curID != "" {
				if cur, err := a.store.GetAccount(curID); err == nil && utils.AccountQuotaExhausted(&cur) {
					switchAfterUnlock.currentID = curID
					switchAfterUnlock.planFilter = settings.AutoSwitchPlanFilter
				}
			}
		}
	}
	a.mu.Unlock()

	if switchAfterUnlock.currentID != "" {
		_, _ = a.AutoSwitchToNext(switchAfterUnlock.currentID, switchAfterUnlock.planFilter)
	}
}

func (a *App) findAccountIDForWindsurfAuth(auth *services.WindsurfAuthJSON) string {
	if auth == nil {
		return ""
	}
	accounts := a.store.GetAllAccounts()
	emailWant := strings.TrimSpace(strings.ToLower(auth.Email))
	tokenWant := strings.TrimSpace(auth.Token)
	for _, acc := range accounts {
		if emailWant != "" && strings.TrimSpace(strings.ToLower(acc.Email)) == emailWant {
			return acc.ID
		}
	}
	if tokenWant != "" {
		if claims, err := a.windsurfSvc.DecodeJWTClaims(tokenWant); err == nil && claims != nil && claims.Email != "" {
			je := strings.TrimSpace(strings.ToLower(claims.Email))
			for _, acc := range accounts {
				if strings.TrimSpace(strings.ToLower(acc.Email)) == je {
					return acc.ID
				}
			}
		}
		for _, acc := range accounts {
			if acc.Token != "" && acc.Token == tokenWant {
				return acc.ID
			}
		}
	}
	return ""
}

func (a *App) syncAccountCredentials(acc *models.Account) {
	if acc.WindsurfAPIKey != "" {
		if jwt, err := a.windsurfSvc.GetJWTByAPIKey(acc.WindsurfAPIKey); err == nil {
			acc.Token = jwt
		}
		return
	}
	if acc.RefreshToken != "" {
		if resp, err := a.windsurfSvc.RefreshToken(acc.RefreshToken); err == nil {
			acc.Token = resp.IDToken
			acc.RefreshToken = resp.RefreshToken
			acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
			return
		}
	}
	if acc.Email != "" && acc.Password != "" {
		if resp, err := a.windsurfSvc.LoginWithEmail(acc.Email, acc.Password); err == nil {
			acc.Token = resp.IDToken
			acc.RefreshToken = resp.RefreshToken
			acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		}
	}
}

func (a *App) RefreshAllTokens() map[string]string { return a.refreshAllTokens() }

func (a *App) refreshAllTokens() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	results := make(map[string]string)
	accounts := a.store.GetAllAccounts()
	for _, acc := range accounts {
		if acc.WindsurfAPIKey != "" {
			jwt, err := a.windsurfSvc.GetJWTByAPIKey(acc.WindsurfAPIKey)
			if err != nil {
				results[acc.Email] = "JWT刷新失败: " + err.Error()
				continue
			}
			acc.Token = jwt
			a.enrichAccountInfo(&acc)
			acc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
			_ = a.store.UpdateAccount(acc)
			results[acc.Email] = "JWT刷新成功"
			continue
		}
		if acc.RefreshToken != "" {
			resp, err := a.windsurfSvc.RefreshToken(acc.RefreshToken)
			if err != nil {
				results[acc.Email] = "Token刷新失败: " + err.Error()
				continue
			}
			acc.Token = resp.IDToken
			acc.RefreshToken = resp.RefreshToken
			acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
			a.enrichAccountInfo(&acc)
			_ = a.store.UpdateAccount(acc)
			results[acc.Email] = "Token刷新成功"
			continue
		}
		results[acc.Email] = "无可用刷新凭证"
	}
	return results
}

// RefreshAccountQuota 手动同步单账号额度（同步凭证 + 拉取 profile，不校验策略间隔）
func (a *App) RefreshAccountQuota(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	acc, err := a.store.GetAccount(id)
	if err != nil {
		return err
	}
	if acc.WindsurfAPIKey == "" && acc.Token == "" && acc.RefreshToken == "" && (acc.Email == "" || acc.Password == "") {
		return fmt.Errorf("该账号没有可用于拉取额度的凭证")
	}
	copyAcc := acc
	a.syncAccountCredentials(&copyAcc)
	a.enrichAccountInfo(&copyAcc)
	copyAcc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
	return a.store.UpdateAccount(copyAcc)
}

// RefreshAllQuotas 手动同步全部账号额度（忽略 auto_refresh_quotas 与策略）
func (a *App) RefreshAllQuotas() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	results := make(map[string]string)
	now := time.Now().Format(time.RFC3339)
	for _, acc := range a.store.GetAllAccounts() {
		if acc.WindsurfAPIKey == "" && acc.Token == "" && acc.RefreshToken == "" && (acc.Email == "" || acc.Password == "") {
			results[labelAccountResult(acc)] = "跳过：无可用凭证"
			continue
		}
		copyAcc := acc
		a.syncAccountCredentials(&copyAcc)
		a.enrichAccountInfo(&copyAcc)
		copyAcc.LastQuotaUpdate = now
		if err := a.store.UpdateAccount(copyAcc); err != nil {
			results[labelAccountResult(acc)] = "失败: " + err.Error()
			continue
		}
		results[labelAccountResult(acc)] = "额度已同步"
	}
	return results
}

func labelAccountResult(acc models.Account) string {
	if acc.Email != "" {
		return acc.Email
	}
	return acc.ID
}
