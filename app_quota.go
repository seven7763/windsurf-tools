package main

import (
	"context"
	"fmt"
	"log"
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
	log.Printf("[额度同步] 定时同步已启动 (间隔=5min)")
	utils.DLog("[额度同步] 定时同步已启动 (间隔=5min)")
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		a.refreshDueQuotas()
		for {
			select {
			case <-ctx.Done():
				utils.DLog("[额度同步] 定时同步已停止")
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
		delay := a.nextQuotaHotPollDelay()
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

func (a *App) nextQuotaHotPollDelay() time.Duration {
	settings := a.store.GetSettings()
	base := time.Duration(clampQuotaHotPollSeconds(settings.QuotaHotPollSeconds)) * time.Second
	curID := a.findCurrentMonitoredAccountID()
	if curID == "" {
		return base
	}
	cur, err := a.store.GetAccount(curID)
	if err != nil {
		return base
	}
	delay := utils.NextQuotaResetWakeDelayForExhausted(cur, time.Now(), base)
	if delay < base {
		utils.DLog("[热轮询/reset] wake-schedule account=%s base=%s next=%s reason=reset-window daily={%s} weekly={%s}",
			labelAccountResult(cur), base, delay, describeQuotaResetField(cur.DailyRemaining, cur.DailyResetAt, cur.LastQuotaUpdate, time.Now()), describeQuotaResetField(cur.WeeklyRemaining, cur.WeeklyResetAt, cur.LastQuotaUpdate, time.Now()))
	}
	return delay
}

func (a *App) pollCurrentSessionQuotaAndMaybeSwitch() {
	settings := a.store.GetSettings()
	if !settings.AutoRefreshQuotas || !settings.AutoSwitchOnQuotaExhausted {
		utils.DLog("[热轮询] 跳过: AutoRefreshQuotas=%v AutoSwitch=%v", settings.AutoRefreshQuotas, settings.AutoSwitchOnQuotaExhausted)
		return
	}

	curID := a.findCurrentMonitoredAccountID()
	if curID == "" {
		utils.DLog("[热轮询] 跳过: 无法匹配当前账号 (号池=%d)", a.store.AccountCount())
		return
	}
	cur, err := a.store.GetAccount(curID)
	if err != nil {
		utils.DLog("[热轮询] 跳过: GetAccount(%s) 失败: %v", curID, err)
		return
	}
	now := time.Now()
	forceRefreshAfterReset := utils.QuotaRefreshDueAfterOfficialReset(cur, now)
	a.logHotPollResetSnapshot("precheck", cur, forceRefreshAfterReset, now)
	a.lastQuotaHotSwitchMu.Lock()
	if t := a.lastQuotaHotSwitch; !t.IsZero() && time.Since(t) < 12*time.Second && !forceRefreshAfterReset {
		a.lastQuotaHotSwitchMu.Unlock()
		utils.DLog("[热轮询] 跳过: 距上次切号仅 %.1fs (<12s 冷却)", time.Since(t).Seconds())
		return
	}
	a.lastQuotaHotSwitchMu.Unlock()
	if forceRefreshAfterReset {
		utils.DLog("[热轮询/reset] due-now account=%s action=force-refresh reason=official-reset-reached daily={%s} weekly={%s}",
			labelAccountResult(cur), describeQuotaResetField(cur.DailyRemaining, cur.DailyResetAt, cur.LastQuotaUpdate, now), describeQuotaResetField(cur.WeeklyRemaining, cur.WeeklyResetAt, cur.LastQuotaUpdate, now))
	}
	if cur.WindsurfAPIKey == "" && strings.TrimSpace(cur.Token) == "" &&
		cur.RefreshToken == "" && (cur.Email == "" || cur.Password == "") {
		utils.DLog("[热轮询] 跳过: %s 无任何可用凭证", cur.Email)
		return
	}

	utils.DLog("[热轮询] 开始查额度: %s (id=%s plan=%s)", cur.Email, curID[:min(8, len(curID))], cur.PlanName)
	copyAcc := cur
	a.syncAccountCredentials(&copyAcc)
	// 热轮询仅拉额度，避免 RegisterUser / GetAccountInfo 等拖慢后台与重复请求
	quotaOK := a.enrichAccountQuotaOnly(&copyAcc)
	utils.DLog("[热轮询] enrichQuota 结果: ok=%v daily=%s weekly=%s total=%d used=%d", quotaOK, copyAcc.DailyRemaining, copyAcc.WeeklyRemaining, copyAcc.TotalQuota, copyAcc.UsedQuota)
	if quotaOK {
		copyAcc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
	}
	a.logHotPollResetSnapshot("post-refresh", copyAcc, false, time.Now())
	if err := a.store.UpdateAccount(copyAcc); err != nil {
		utils.DLog("[热轮询] UpdateAccount 失败: %v", err)
		return
	}
	a.syncMitmPoolKeys()
	if !utils.AccountQuotaExhausted(&copyAcc) {
		utils.DLog("[热轮询] %s 额度正常 (daily=%s weekly=%s)", copyAcc.Email, copyAcc.DailyRemaining, copyAcc.WeeklyRemaining)
		utils.DLog("[热轮询/reset] decision account=%s result=keep-current daily={%s} weekly={%s}",
			labelAccountResult(copyAcc), describeQuotaResetField(copyAcc.DailyRemaining, copyAcc.DailyResetAt, copyAcc.LastQuotaUpdate, time.Now()), describeQuotaResetField(copyAcc.WeeklyRemaining, copyAcc.WeeklyResetAt, copyAcc.LastQuotaUpdate, time.Now()))
		return
	}
	utils.DLog("[热轮询] ★ %s 额度用尽! (daily=%s weekly=%s plan=%s) → 触发切号", copyAcc.Email, copyAcc.DailyRemaining, copyAcc.WeeklyRemaining, copyAcc.PlanName)
	utils.DLog("[热轮询/reset] decision account=%s result=exhausted-switch daily={%s} weekly={%s}",
		labelAccountResult(copyAcc), describeQuotaResetField(copyAcc.DailyRemaining, copyAcc.DailyResetAt, copyAcc.LastQuotaUpdate, time.Now()), describeQuotaResetField(copyAcc.WeeklyRemaining, copyAcc.WeeklyResetAt, copyAcc.LastQuotaUpdate, time.Now()))
	if next, err := a.rotateMitmToNextAvailable(curID, settings.AutoSwitchPlanFilter); err == nil {
		utils.DLog("[热轮询] MITM轮换成功 → %s", next.Email)
		a.lastQuotaHotSwitchMu.Lock()
		a.lastQuotaHotSwitch = time.Now()
		a.lastQuotaHotSwitchMu.Unlock()
	} else {
		utils.DLog("[热轮询] MITM轮换失败: %v", err)
	}
}

func describeQuotaResetField(remaining, resetAt, lastQuotaUpdate string, now time.Time) string {
	parts := make([]string, 0, 4)
	if strings.TrimSpace(remaining) == "" {
		parts = append(parts, "remaining=<empty>")
	} else {
		parts = append(parts, "remaining="+strings.TrimSpace(remaining))
	}
	resetAt = strings.TrimSpace(resetAt)
	if resetAt == "" {
		parts = append(parts, "reset=<none>")
	} else if resetTime, err := time.Parse(time.RFC3339, resetAt); err == nil {
		parts = append(parts, "reset="+resetAt)
		delta := resetTime.Sub(now)
		switch {
		case delta > 0:
			parts = append(parts, fmt.Sprintf("reset_in=%s", delta.Round(time.Second)))
		case delta < 0:
			parts = append(parts, fmt.Sprintf("reset_ago=%s", (-delta).Round(time.Second)))
		default:
			parts = append(parts, "reset_now=true")
		}
	} else {
		parts = append(parts, "reset="+resetAt)
		parts = append(parts, "reset_parse=invalid")
	}
	if strings.TrimSpace(lastQuotaUpdate) == "" {
		parts = append(parts, "last=<none>")
	} else {
		parts = append(parts, "last="+strings.TrimSpace(lastQuotaUpdate))
	}
	return strings.Join(parts, " ")
}

func (a *App) logHotPollResetSnapshot(stage string, acc models.Account, force bool, now time.Time) {
	utils.DLog("[热轮询/reset] %s account=%s plan=%s force=%v daily={%s} weekly={%s}",
		stage, labelAccountResult(acc), acc.PlanName, force, describeQuotaResetField(acc.DailyRemaining, acc.DailyResetAt, acc.LastQuotaUpdate, now), describeQuotaResetField(acc.WeeklyRemaining, acc.WeeklyResetAt, acc.LastQuotaUpdate, now))
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
		utils.DLog("[额度同步] AutoRefreshQuotas=false，跳过")
		return
	}
	policy := strings.TrimSpace(settings.QuotaRefreshPolicy)
	if policy == "" {
		policy = utils.QuotaPolicyHybrid
	}
	now := time.Now()
	customMins := settings.QuotaCustomIntervalMinutes
	accounts := a.store.GetAllAccounts()
	svc := a.windsurfSvc
	if svc == nil {
		utils.DLog("[额度同步] windsurfSvc=nil，跳过")
		return
	}
	dueAccounts := make([]models.Account, 0, len(accounts))
	for _, acc := range accounts {
		if !utils.QuotaRefreshDue(acc.LastQuotaUpdate, policy, customMins, now) &&
			!utils.QuotaRefreshDueAfterOfficialReset(acc, now) {
			continue
		}
		if acc.WindsurfAPIKey == "" && acc.Token == "" && acc.RefreshToken == "" && (acc.Email == "" || acc.Password == "") {
			continue
		}
		dueAccounts = append(dueAccounts, acc)
	}
	utils.DLog("[额度同步] policy=%s 总账号=%d due=%d", policy, len(accounts), len(dueAccounts))
	pause := refreshBatchPause(settings.ConcurrentLimit)
	outcomes := runAccountRefreshBatches(dueAccounts, settings.ConcurrentLimit, pause, func(acc models.Account) accountRefreshOutcome {
		copyAcc := acc
		a.syncAccountCredentialsWithService(svc, &copyAcc)
		gotData := a.enrichAccountInfoWithService(svc, &copyAcc)
		if gotData {
			copyAcc.LastQuotaUpdate = now.Format(time.RFC3339)
		}
		return accountRefreshOutcome{
			label:   labelAccountResult(acc),
			account: copyAcc,
			updated: true,
		}
	})
	var updatedCount int
	for _, outcome := range outcomes {
		if !outcome.updated {
			continue
		}
		if err := a.store.UpdateAccount(outcome.account); err == nil {
			updatedPool = true
			updatedCount++
		}
	}
	if len(dueAccounts) > 0 {
		utils.DLog("[额度同步] 完成: 更新=%d/%d updatedPool=%v", updatedCount, len(dueAccounts), updatedPool)
	}

	if settings.AutoSwitchOnQuotaExhausted {
		curID := a.findCurrentMonitoredAccountID()
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
		_, _ = a.rotateMitmToNextAvailable(switchAfterUnlock.currentID, switchAfterUnlock.planFilter)
	}
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

func (a *App) findCurrentMonitoredAccountID() string {
	accounts := a.store.GetAllAccounts()
	activeMITMKey := ""
	if a.mitmProxy != nil {
		activeMITMKey = a.mitmProxy.CurrentAPIKey()
	}
	id := findAccountIDForMITMAPIKey(accounts, activeMITMKey)
	if id != "" {
		utils.DLog("[匹配] findCurrentMonitored → id=%s (mitmKey=%v)", id[:min(8, len(id))], activeMITMKey != "")
	} else {
		utils.DLog("[匹配] findCurrentMonitored → 未匹配 (mitmKey=%v accounts=%d)", activeMITMKey != "", len(accounts))
	}
	return id
}

func (a *App) syncAccountCredentials(acc *models.Account) {
	a.syncAccountCredentialsWithService(a.windsurfSvc, acc)
}

func (a *App) syncAccountCredentialsWithService(svc *services.WindsurfService, acc *models.Account) {
	if svc == nil || acc == nil {
		return
	}
	label := acc.Email
	if label == "" {
		label = acc.ID
	}
	utils.DLog("[凭证] %s 开始同步 (hasKey=%v hasRefresh=%v hasPass=%v)", label, acc.WindsurfAPIKey != "", acc.RefreshToken != "", acc.Password != "")
	if acc.WindsurfAPIKey != "" {
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			jwt, err := svc.GetJWTByAPIKey(acc.WindsurfAPIKey)
			if err == nil && jwt != "" {
				acc.Token = jwt
				utils.DLog("[凭证] %s JWT获取成功(APIKey) tokenLen=%d", label, len(jwt))
				return
			}
			lastErr = err
			if attempt == 0 {
				time.Sleep(500 * time.Millisecond)
			}
		}
		utils.DLog("[凭证] %s JWT获取失败(APIKey): %v", label, lastErr)
		log.Printf("[切号] %s JWT获取失败(APIKey): %v", label, lastErr)
		acc.Token = ""
		acc.TokenExpiresAt = ""
		applyAccessErrorStatus(acc, lastErr)
		return
	}
	if acc.RefreshToken != "" {
		resp, err := svc.RefreshToken(acc.RefreshToken)
		if err == nil {
			acc.Token = resp.IDToken
			acc.RefreshToken = resp.RefreshToken
			acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
			utils.DLog("[凭证] %s RefreshToken成功 tokenLen=%d", label, len(resp.IDToken))
			return
		}
		utils.DLog("[凭证] %s RefreshToken刷新失败: %v", label, err)
		log.Printf("[切号] %s RefreshToken刷新失败: %v", label, err)
	}
	if acc.Email != "" && acc.Password != "" {
		resp, err := svc.LoginWithEmail(acc.Email, acc.Password)
		if err == nil {
			acc.Token = resp.IDToken
			acc.RefreshToken = resp.RefreshToken
			acc.TokenExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
			utils.DLog("[凭证] %s 邮箱登录成功 tokenLen=%d", label, len(resp.IDToken))
			return
		}
		utils.DLog("[凭证] %s 邮箱密码登录失败: %v", label, err)
		log.Printf("[切号] %s 邮箱密码登录失败: %v", label, err)
	}
	utils.DLog("[凭证] %s 所有凭证同步路径均失败", label)
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
				before := acc
				applyAccessErrorStatus(&acc, err)
				return accountRefreshOutcome{
					label:   label,
					status:  "JWT刷新失败: " + err.Error(),
					account: acc,
					updated: acc != before,
				}
			}
			acc.Token = jwt
			if a.enrichAccountInfoWithService(svc, &acc) {
				acc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
			}
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
	if a.enrichAccountInfoWithService(svc, &copyAcc) {
		copyAcc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
	}
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
		gotData := a.enrichAccountInfoWithService(svc, &copyAcc)
		status := "额度已同步"
		if gotData {
			copyAcc.LastQuotaUpdate = now
		} else {
			status = "额度同步失败（API返回为空）"
		}
		return accountRefreshOutcome{label: label, status: status, account: copyAcc, updated: true}
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
