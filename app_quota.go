package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

func clampRefreshConcurrentLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > 8 {
		return 8
	}
	return limit
}

func refreshBatchPause(limit int) time.Duration {
	switch {
	case limit >= 6:
		return 120 * time.Millisecond
	case limit >= 3:
		return 180 * time.Millisecond
	default:
		return 260 * time.Millisecond
	}
}

type accountRefreshOutcome struct {
	label   string
	status  string
	account models.Account
	updated bool
}

func authTokenOrEmpty(auth *services.WindsurfAuthJSON) string {
	if auth == nil {
		return ""
	}
	return strings.TrimSpace(auth.Token)
}

func runAccountRefreshBatches(accounts []models.Account, concurrency int, pause time.Duration, worker func(models.Account) accountRefreshOutcome) []accountRefreshOutcome {
	if len(accounts) == 0 {
		return nil
	}
	limit := clampRefreshConcurrentLimit(concurrency)
	outcomes := make([]accountRefreshOutcome, 0, len(accounts))
	for start := 0; start < len(accounts); start += limit {
		end := start + limit
		if end > len(accounts) {
			end = len(accounts)
		}
		batch := accounts[start:end]
		results := make([]accountRefreshOutcome, len(batch))
		var wg sync.WaitGroup
		for i, acc := range batch {
			i := i
			acc := acc
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = worker(acc)
			}()
		}
		wg.Wait()
		outcomes = append(outcomes, results...)
		if end < len(accounts) && pause > 0 {
			time.Sleep(pause)
		}
	}
	return outcomes
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

	var auth *services.WindsurfAuthJSON
	if got, err := a.switchSvc.GetCurrentAuth(); err == nil {
		auth = got
	}
	authToken := authTokenOrEmpty(auth)
	curID := a.findCurrentMonitoredAccountID(auth, settings.MitmOnly)
	if curID == "" {
		return
	}
	cur, err := a.store.GetAccount(curID)
	if err != nil {
		return
	}
	if cur.WindsurfAPIKey == "" && strings.TrimSpace(cur.Token) == "" && authToken == "" &&
		cur.RefreshToken == "" && (cur.Email == "" || cur.Password == "") {
		return
	}

	copyAcc := cur
	if authToken != "" {
		copyAcc.Token = authToken
	} else {
		a.syncAccountCredentials(&copyAcc)
	}
	// 热轮询仅拉额度，避免 RegisterUser / GetAccountInfo 等拖慢后台与重复请求
	a.enrichAccountQuotaOnly(&copyAcc)
	copyAcc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
	if err := a.store.UpdateAccount(copyAcc); err != nil {
		return
	}
	a.syncMitmPoolKeys()
	if !utils.AccountQuotaExhausted(&copyAcc) {
		return
	}
	if settings.MitmOnly {
		if _, err := a.rotateMitmToNextAvailable(curID, settings.AutoSwitchPlanFilter); err == nil {
			a.lastQuotaHotSwitchMu.Lock()
			a.lastQuotaHotSwitch = time.Now()
			a.lastQuotaHotSwitchMu.Unlock()
		}
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
	a.quotaRefreshRunMu.Lock()
	defer a.quotaRefreshRunMu.Unlock()

	var switchAfterUnlock struct {
		currentID  string
		planFilter string
	}
	updatedPool := false

	settings := a.store.GetSettings()
	if !settings.AutoRefreshQuotas {
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
		if auth, err := a.switchSvc.GetCurrentAuth(); err == nil {
			skipAccountID = a.findCurrentMonitoredAccountID(auth, settings.MitmOnly)
		}
	}
	svc := a.windsurfSvc
	if svc == nil {
		return
	}
	dueAccounts := make([]models.Account, 0, len(accounts))
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
		dueAccounts = append(dueAccounts, acc)
	}
	pause := refreshBatchPause(settings.ConcurrentLimit)
	outcomes := runAccountRefreshBatches(dueAccounts, settings.ConcurrentLimit, pause, func(acc models.Account) accountRefreshOutcome {
		copyAcc := acc
		a.syncAccountCredentialsWithService(svc, &copyAcc)
		a.enrichAccountInfoWithService(svc, &copyAcc)
		copyAcc.LastQuotaUpdate = now.Format(time.RFC3339)
		return accountRefreshOutcome{
			label:   labelAccountResult(acc),
			account: copyAcc,
			updated: true,
		}
	})
	for _, outcome := range outcomes {
		if !outcome.updated {
			continue
		}
		if err := a.store.UpdateAccount(outcome.account); err == nil {
			updatedPool = true
		}
	}

	if settings.AutoSwitchOnQuotaExhausted {
		auth, _ := a.switchSvc.GetCurrentAuth()
		curID := a.findCurrentMonitoredAccountID(auth, settings.MitmOnly)
		if curID != "" {
			if cur, err := a.store.GetAccount(curID); err == nil && utils.AccountQuotaExhausted(&cur) {
				switchAfterUnlock.currentID = curID
				switchAfterUnlock.planFilter = settings.AutoSwitchPlanFilter
			}
		}
	}

	if updatedPool {
		a.syncMitmPoolKeys()
	}

	if switchAfterUnlock.currentID != "" {
		if settings := a.store.GetSettings(); settings.MitmOnly {
			_, _ = a.rotateMitmToNextAvailable(switchAfterUnlock.currentID, switchAfterUnlock.planFilter)
			return
		}
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

func findAccountIDForMITMAPIKey(accounts []models.Account, apiKey string) string {
	want := strings.TrimSpace(apiKey)
	if want == "" {
		return ""
	}
	for _, acc := range accounts {
		if strings.TrimSpace(acc.WindsurfAPIKey) == want {
			return acc.ID
		}
	}
	return ""
}

func resolveCurrentAccountID(accounts []models.Account, auth *services.WindsurfAuthJSON, activeMITMKey string, authResolver func(*services.WindsurfAuthJSON) string) string {
	if id := findAccountIDForMITMAPIKey(accounts, activeMITMKey); id != "" {
		return id
	}
	if authResolver != nil {
		return authResolver(auth)
	}
	return ""
}

func (a *App) findCurrentMonitoredAccountID(auth *services.WindsurfAuthJSON, preferMITMKey bool) string {
	accounts := a.store.GetAllAccounts()
	activeMITMKey := ""
	if preferMITMKey && a.mitmProxy != nil {
		activeMITMKey = a.mitmProxy.CurrentAPIKey()
	}
	return resolveCurrentAccountID(accounts, auth, activeMITMKey, func(auth *services.WindsurfAuthJSON) string {
		return a.findAccountIDForWindsurfAuth(auth)
	})
}

func (a *App) syncAccountCredentials(acc *models.Account) {
	a.syncAccountCredentialsWithService(a.windsurfSvc, acc)
}

func (a *App) syncAccountCredentialsWithService(svc *services.WindsurfService, acc *models.Account) {
	if svc == nil || acc == nil {
		return
	}
	if acc.WindsurfAPIKey != "" {
		if jwt, err := svc.GetJWTByAPIKey(acc.WindsurfAPIKey); err == nil {
			acc.Token = jwt
		}
		return
	}
	if acc.RefreshToken != "" {
		if resp, err := svc.RefreshToken(acc.RefreshToken); err == nil {
			acc.Token = resp.IDToken
			acc.RefreshToken = resp.RefreshToken
			acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
			return
		}
	}
	if acc.Email != "" && acc.Password != "" {
		if resp, err := svc.LoginWithEmail(acc.Email, acc.Password); err == nil {
			acc.Token = resp.IDToken
			acc.RefreshToken = resp.RefreshToken
			acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		}
	}
}

func (a *App) RefreshAllTokens() map[string]string { return a.refreshAllTokens() }

func (a *App) refreshAllTokens() map[string]string {
	a.tokenRefreshRunMu.Lock()
	defer a.tokenRefreshRunMu.Unlock()

	results := make(map[string]string)
	accounts := a.store.GetAllAccounts()
	settings := a.store.GetSettings()
	svc := a.windsurfSvc
	if svc == nil {
		for _, acc := range accounts {
			results[labelAccountResult(acc)] = "刷新服务未初始化"
		}
		return results
	}
	pause := refreshBatchPause(settings.ConcurrentLimit)
	updatedPool := false
	outcomes := runAccountRefreshBatches(accounts, settings.ConcurrentLimit, pause, func(acc models.Account) accountRefreshOutcome {
		label := labelAccountResult(acc)
		if acc.WindsurfAPIKey != "" {
			jwt, err := svc.GetJWTByAPIKey(acc.WindsurfAPIKey)
			if err != nil {
				return accountRefreshOutcome{label: label, status: "JWT刷新失败: " + err.Error()}
			}
			acc.Token = jwt
			a.enrichAccountInfoWithService(svc, &acc)
			acc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
			return accountRefreshOutcome{label: label, status: "JWT刷新成功", account: acc, updated: true}
		}
		if acc.RefreshToken != "" {
			resp, err := svc.RefreshToken(acc.RefreshToken)
			if err != nil {
				return accountRefreshOutcome{label: label, status: "Token刷新失败: " + err.Error()}
			}
			acc.Token = resp.IDToken
			acc.RefreshToken = resp.RefreshToken
			acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
			a.enrichAccountInfoWithService(svc, &acc)
			return accountRefreshOutcome{label: label, status: "Token刷新成功", account: acc, updated: true}
		}
		return accountRefreshOutcome{label: label, status: "无可用刷新凭证"}
	})
	for _, outcome := range outcomes {
		results[outcome.label] = outcome.status
		if !outcome.updated {
			continue
		}
		if err := a.store.UpdateAccount(outcome.account); err != nil {
			results[outcome.label] = "保存失败: " + err.Error()
			continue
		}
		updatedPool = true
	}
	if updatedPool {
		a.syncMitmPoolKeys()
	}
	return results
}

// RefreshAccountQuota 手动同步单账号额度（同步凭证 + 拉取 profile，不校验策略间隔）
func (a *App) RefreshAccountQuota(id string) error {
	a.quotaRefreshRunMu.Lock()
	defer a.quotaRefreshRunMu.Unlock()
	acc, err := a.store.GetAccount(id)
	if err != nil {
		return err
	}
	if acc.WindsurfAPIKey == "" && acc.Token == "" && acc.RefreshToken == "" && (acc.Email == "" || acc.Password == "") {
		return fmt.Errorf("该账号没有可用于拉取额度的凭证")
	}
	copyAcc := acc
	svc := a.windsurfSvc
	if svc == nil {
		return fmt.Errorf("刷新服务未初始化")
	}
	a.syncAccountCredentialsWithService(svc, &copyAcc)
	a.enrichAccountInfoWithService(svc, &copyAcc)
	copyAcc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
	if err := a.store.UpdateAccount(copyAcc); err != nil {
		return err
	}
	a.syncMitmPoolKeys()
	return nil
}

// RefreshAllQuotas 手动同步全部账号额度（忽略 auto_refresh_quotas 与策略）
func (a *App) RefreshAllQuotas() map[string]string {
	a.quotaRefreshRunMu.Lock()
	defer a.quotaRefreshRunMu.Unlock()

	results := make(map[string]string)
	now := time.Now().Format(time.RFC3339)
	settings := a.store.GetSettings()
	accounts := a.store.GetAllAccounts()
	svc := a.windsurfSvc
	if svc == nil {
		for _, acc := range accounts {
			results[labelAccountResult(acc)] = "刷新服务未初始化"
		}
		return results
	}
	pause := refreshBatchPause(settings.ConcurrentLimit)
	updatedPool := false
	outcomes := runAccountRefreshBatches(accounts, settings.ConcurrentLimit, pause, func(acc models.Account) accountRefreshOutcome {
		label := labelAccountResult(acc)
		if acc.WindsurfAPIKey == "" && acc.Token == "" && acc.RefreshToken == "" && (acc.Email == "" || acc.Password == "") {
			return accountRefreshOutcome{label: label, status: "跳过：无可用凭证"}
		}
		copyAcc := acc
		a.syncAccountCredentialsWithService(svc, &copyAcc)
		a.enrichAccountInfoWithService(svc, &copyAcc)
		copyAcc.LastQuotaUpdate = now
		return accountRefreshOutcome{label: label, status: "额度已同步", account: copyAcc, updated: true}
	})
	for _, outcome := range outcomes {
		results[outcome.label] = outcome.status
		if !outcome.updated {
			continue
		}
		if err := a.store.UpdateAccount(outcome.account); err != nil {
			results[outcome.label] = "失败: " + err.Error()
			continue
		}
		updatedPool = true
	}
	if updatedPool {
		a.syncMitmPoolKeys()
	}
	return results
}

func labelAccountResult(acc models.Account) string {
	if acc.Email != "" {
		return acc.Email
	}
	return acc.ID
}
