package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
	"windsurf-tools-wails/backend/utils"
)

// ── 动态 DNS 解析（兼容 VPN / IP 漂移） ──

var (
	resolvedIP string
	resolvedAt time.Time
	resolveMu  sync.RWMutex
	resolveTTL = 5 * time.Minute
)

// ResolveUpstreamIP 动态解析上游 IP，带缓存（TTL 5 分钟），失败时回退硬编码。
func ResolveUpstreamIP() string {
	resolveMu.RLock()
	if resolvedIP != "" && time.Since(resolvedAt) < resolveTTL {
		ip := resolvedIP
		resolveMu.RUnlock()
		return ip
	}
	resolveMu.RUnlock()

	resolveMu.Lock()
	defer resolveMu.Unlock()
	// double-check after acquiring write lock
	if resolvedIP != "" && time.Since(resolvedAt) < resolveTTL {
		return resolvedIP
	}

	ips, err := net.LookupHost(UpstreamHost)
	if err == nil {
		for _, ip := range ips {
			if !strings.HasPrefix(ip, "127.") && !strings.Contains(ip, ":") {
				resolvedIP = ip
				resolvedAt = time.Now()
				log.Printf("[DNS] %s → %s", UpstreamHost, ip)
				return ip
			}
		}
	}
	// DNS 失败或返回 127.x（已被 hosts 劫持），回退硬编码
	if resolvedIP != "" {
		return resolvedIP // 用上次缓存
	}
	log.Printf("[DNS] 解析 %s 失败(%v)，回退 %s", UpstreamHost, err, UpstreamIP)
	return UpstreamIP
}

const (
	TargetDomain = "server.self-serve.windsurf.com"
	UpstreamIP   = "34.49.14.144"
	UpstreamHost = "server.self-serve.windsurf.com"

	defaultProxyPort  = 443
	jwtRefreshMinutes = 4
	maxConsecErrors   = 1
	keyCooldownSec    = 600
	recentEventLimit  = 12
	streamQuotaWindow = 4096
)

// PoolKeyState tracks the runtime state of each pool key.
type PoolKeyState struct {
	APIKey           string
	JWT              []byte
	Healthy          bool
	RuntimeExhausted bool
	CooldownUntil    time.Time
	ConsecutiveErrs  int
	RequestCount     int
	SuccessCount     int
	TotalExhausted   int
}

func newPoolKeyState(apiKey string) *PoolKeyState {
	return &PoolKeyState{
		APIKey:  apiKey,
		Healthy: true,
	}
}

func (s *PoolKeyState) markExhausted() {
	s.Healthy = false
	s.RuntimeExhausted = true
	s.CooldownUntil = time.Now().Add(keyCooldownSec * time.Second)
	s.ConsecutiveErrs = 0
	s.TotalExhausted++
}

func (s *PoolKeyState) isAvailable() bool {
	if s.Healthy {
		return true
	}
	// RuntimeExhausted 的 key 不靠冷却自动恢复；只有 recordSuccess / ClearKeyExhausted 能解除。
	// 这确保额度真正耗尽的 key 不会 10 分钟后被回收重试。
	if s.RuntimeExhausted {
		return false
	}
	// 非额度耗尽的瞬态错误冷却：到期后恢复
	if time.Now().After(s.CooldownUntil) {
		s.Healthy = true
		s.ConsecutiveErrs = 0
		return true
	}
	return false
}

func (s *PoolKeyState) recordSuccess() {
	s.RequestCount++
	s.SuccessCount++
	s.RuntimeExhausted = false
	s.ConsecutiveErrs = 0
}

// RecordKeySuccess 外部（如 Relay）通知号池某个 key 请求成功
func (p *MitmProxy) RecordKeySuccess(apiKey string) {
	p.mu.Lock()
	if state := p.keyStates[apiKey]; state != nil {
		state.recordSuccess()
	}
	p.mu.Unlock()
}

func (s *PoolKeyState) recordError() bool {
	s.RequestCount++
	s.ConsecutiveErrs++
	return s.ConsecutiveErrs >= maxConsecErrors
}

// MitmProxy is the core MITM reverse proxy that handles identity replacement.
type MitmProxy struct {
	mu       sync.RWMutex
	listener net.Listener
	running  bool
	port     int
	proxyURL string // 出站代理 (如 http://127.0.0.1:7890)

	poolKeys   []string // ordered list of api keys
	keyStates  map[string]*PoolKeyState
	currentIdx int
	jwtLock    sync.RWMutex

	windsurfSvc    *WindsurfService    // for JWT refresh
	logFn          func(string)        // log callback for UI
	onKeyExhausted func(apiKey string) // 额度耗尽回调（App 层刷新额度+同步号池）
	eventsMu       sync.RWMutex
	recentEvents   []MitmProxyEvent

	jwtReady chan struct{} // closed when at least one JWT is available
	jwtOnce  sync.Once
	stopCh   chan struct{}

	lastErrorKind    string
	lastErrorSummary string
	lastErrorAt      string
	lastErrorKey     string

	debugDump bool // 开启后 dump GetChatMessage 请求/响应的 protobuf 字段树
}

var injectCodeiumConfigFn = InjectCodeiumConfig
var getJWTByAPIKeyFn = func(s *WindsurfService, apiKey string) (string, error) {
	return s.GetJWTByAPIKey(apiKey)
}

// MitmProxyStatus is exposed to the frontend.
type MitmProxyStatus struct {
	Running          bool             `json:"running"`
	Port             int              `json:"port"`
	HostsMapped      bool             `json:"hosts_mapped"`
	CAInstalled      bool             `json:"ca_installed"`
	CurrentKey       string           `json:"current_key"`
	PoolStatus       []PoolKeyInfo    `json:"pool_status"`
	TotalReqs        int              `json:"total_requests"`
	LastErrorKind    string           `json:"last_error_kind"`
	LastErrorSummary string           `json:"last_error_summary"`
	LastErrorAt      string           `json:"last_error_at"`
	LastErrorKey     string           `json:"last_error_key"`
	RecentEvents     []MitmProxyEvent `json:"recent_events"`
}

type PoolKeyInfo struct {
	KeyShort         string `json:"key_short"`
	Healthy          bool   `json:"healthy"`
	RuntimeExhausted bool   `json:"runtime_exhausted"`
	CooldownUntil    string `json:"cooldown_until"`
	HasJWT           bool   `json:"has_jwt"`
	RequestCount     int    `json:"request_count"`
	SuccessCount     int    `json:"success_count"`
	TotalExhausted   int    `json:"total_exhausted"`
	IsCurrent        bool   `json:"is_current"`
}

type MitmProxyEvent struct {
	At      string `json:"at"`
	Message string `json:"message"`
	Tone    string `json:"tone"`
}

type quotaStreamWatchBody struct {
	inner      io.ReadCloser
	onQuota    func(detail string)
	onSuccess  func()
	recentText string
	sawQuota   bool
	finalized  bool
}

// NewMitmProxy creates a new proxy instance.
func NewMitmProxy(windsurfSvc *WindsurfService, logFn func(string), proxyURL string) *MitmProxy {
	return &MitmProxy{
		port:        defaultProxyPort,
		keyStates:   make(map[string]*PoolKeyState),
		windsurfSvc: windsurfSvc,
		logFn:       logFn,
		proxyURL:    proxyURL,
		jwtReady:    make(chan struct{}),
		stopCh:      make(chan struct{}),
	}
}

func (p *MitmProxy) syncCurrentAPIKeyToClient(apiKey string) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return
	}
	if err := injectCodeiumConfigFn(apiKey); err != nil {
		p.log("同步本地 API Key 失败: %s... (%v)", apiKey[:minStr(12, len(apiKey))], err)
		return
	}
	p.log("同步本地 API Key: %s...", apiKey[:minStr(12, len(apiKey))])
}

// SetOnKeyExhausted 设置额度耗尽回调（App 层用来触发额度刷新 + 同步号池）
func (p *MitmProxy) SetOnKeyExhausted(fn func(apiKey string)) {
	p.mu.Lock()
	p.onKeyExhausted = fn
	p.mu.Unlock()
}

func (p *MitmProxy) markRuntimeExhaustedAndRotate(usedKey, detail string) string {
	p.log("★ markRuntimeExhaustedAndRotate: key=%s... detail=%s", usedKey[:minStr(12, len(usedKey))], detail)
	rotatedKey := ""
	p.mu.Lock()
	if state := p.keyStates[usedKey]; state != nil {
		state.markExhausted()
	}
	rotatedKey = p.rotateKey()
	cb := p.onKeyExhausted
	poolSize := len(p.poolKeys)
	p.mu.Unlock()
	p.recordUpstreamFailure(upstreamFailureQuota, detail, usedKey)
	if rotatedKey != "" {
		p.log("★ 额度耗尽轮转: %s... → %s... (pool=%d)", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))], poolSize)
		p.syncCurrentAPIKeyToClient(rotatedKey)
	} else {
		p.log("★ 额度耗尽但无可轮转 key (pool=%d)", poolSize)
	}
	// 异步触发 App 层刷新耗尽 key 的额度 → 更新 store → syncMitmPoolKeys 移除
	if cb != nil {
		p.log("★ 触发 onKeyExhausted 回调: key=%s...", usedKey[:minStr(12, len(usedKey))])
		go cb(usedKey)
	}
	return rotatedKey
}

func newQuotaStreamWatchBody(inner io.ReadCloser, onQuota func(detail string), onSuccess func()) *quotaStreamWatchBody {
	return &quotaStreamWatchBody{
		inner:     inner,
		onQuota:   onQuota,
		onSuccess: onSuccess,
	}
}

func (b *quotaStreamWatchBody) Read(p []byte) (int, error) {
	n, err := b.inner.Read(p)
	if n > 0 {
		b.scanChunk(p[:n])
	}
	if err == io.EOF {
		b.finalize()
	}
	return n, err
}

func (b *quotaStreamWatchBody) Close() error {
	err := b.inner.Close()
	b.finalize()
	return err
}

func (b *quotaStreamWatchBody) scanChunk(chunk []byte) {
	if len(chunk) == 0 || b.sawQuota {
		return
	}
	lower := strings.ToLower(string(chunk))
	combined := b.recentText + lower
	if len(combined) > streamQuotaWindow {
		combined = combined[len(combined)-streamQuotaWindow:]
	}
	b.recentText = combined
	// 诊断：流式 chunk 中 precondition/quota/exhaust 关键词出现时记录
	if strings.Contains(lower, "precondition") || strings.Contains(lower, "exhaust") || strings.Contains(lower, "quota") {
		trafficLog("  STREAM-SCAN hit: chunk[%d] matched keyword, combined[%d]", len(chunk), len(combined))
	}
	if !isQuotaExhaustedText(combined) {
		return
	}
	b.sawQuota = true
	if b.onQuota != nil {
		b.onQuota("stream-body=" + truncate(strings.TrimSpace(combined), 180))
	}
}

func (b *quotaStreamWatchBody) finalize() {
	if b.finalized {
		return
	}
	b.finalized = true
	if b.sawQuota || b.onSuccess == nil {
		return
	}
	b.onSuccess()
}

func (p *MitmProxy) log(format string, args ...interface{}) {
	msg := fmt.Sprintf("[MITM] "+format, args...)
	p.appendRecentEvent(msg)
	log.Println(msg)
	utils.DLog("%s", msg)
	if p.logFn != nil {
		p.logFn(msg)
	}
}

func classifyMitmEventTone(message string) string {
	text := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(text, "失败"), strings.Contains(text, "错误"), strings.Contains(text, "异常退出"):
		return "danger"
	case strings.Contains(text, "⚠️"), strings.Contains(text, "耗尽"), strings.Contains(text, "跳过"), strings.Contains(text, "超时"):
		return "warning"
	case strings.Contains(text, "✅"), strings.Contains(text, "成功"), strings.Contains(text, "启动"), strings.Contains(text, "已停止"):
		return "success"
	default:
		return "info"
	}
}

func (p *MitmProxy) appendRecentEvent(message string) {
	event := MitmProxyEvent{
		At:      time.Now().Format(time.RFC3339),
		Message: strings.TrimSpace(message),
		Tone:    classifyMitmEventTone(message),
	}
	p.eventsMu.Lock()
	defer p.eventsMu.Unlock()
	p.recentEvents = append(p.recentEvents, event)
	if len(p.recentEvents) > recentEventLimit {
		p.recentEvents = append([]MitmProxyEvent(nil), p.recentEvents[len(p.recentEvents)-recentEventLimit:]...)
	}
}

func (p *MitmProxy) recentEventsSnapshot() []MitmProxyEvent {
	p.eventsMu.RLock()
	defer p.eventsMu.RUnlock()
	if len(p.recentEvents) == 0 {
		return nil
	}
	out := make([]MitmProxyEvent, 0, len(p.recentEvents))
	for i := len(p.recentEvents) - 1; i >= 0; i-- {
		out = append(out, p.recentEvents[i])
	}
	return out
}

func (p *MitmProxy) SetWindsurfService(windsurfSvc *WindsurfService) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.windsurfSvc = windsurfSvc
}

func (p *MitmProxy) SetOutboundProxy(proxyURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.proxyURL = strings.TrimSpace(proxyURL)
}

// SetDebugDump 开启/关闭 proto dump（GetChatMessage 请求/响应字段树写入文件）
func (p *MitmProxy) SetDebugDump(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.debugDump = enabled
}

// DebugDumpEnabled 返回当前 debug dump 状态
func (p *MitmProxy) DebugDumpEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.debugDump
}

func (p *MitmProxy) recordUpstreamFailure(kind upstreamFailureKind, detail, apiKey string) {
	if kind == upstreamFailureNone {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastErrorKind = string(kind)
	p.lastErrorSummary = strings.TrimSpace(detail)
	p.lastErrorAt = time.Now().Format(time.RFC3339)
	if apiKey != "" {
		p.lastErrorKey = apiKey[:minStr(12, len(apiKey))]
	} else {
		p.lastErrorKey = ""
	}
}

// SetPoolKeys configures the account pool from API keys.
func (p *MitmProxy) SetPoolKeys(keys []string) {
	p.mu.Lock()
	currentKey := ""
	if len(p.poolKeys) > 0 && p.currentIdx >= 0 && p.currentIdx < len(p.poolKeys) {
		currentKey = p.poolKeys[p.currentIdx]
	}
	addedKeys := make([]string, 0, len(keys))
	previousCurrentKey := currentKey

	p.poolKeys = keys
	for _, k := range keys {
		if _, ok := p.keyStates[k]; !ok {
			p.keyStates[k] = newPoolKeyState(k)
			addedKeys = append(addedKeys, k)
		}
	}
	// Remove stale keys
	for k := range p.keyStates {
		found := false
		for _, pk := range keys {
			if pk == k {
				found = true
				break
			}
		}
		if !found {
			delete(p.keyStates, k)
		}
	}

	if currentKey != "" {
		for i, k := range keys {
			if k == currentKey {
				p.currentIdx = i
				running := p.running
				p.mu.Unlock()
				if running && len(addedKeys) > 0 {
					go p.prefetchSpecificJWTs(addedKeys, false)
				}
				return
			}
		}
	}
	if p.currentIdx < 0 || p.currentIdx >= len(keys) {
		p.currentIdx = 0
	}
	newCurrentKey := ""
	if len(keys) > 0 && p.currentIdx >= 0 && p.currentIdx < len(keys) {
		newCurrentKey = keys[p.currentIdx]
	}
	running := p.running
	p.mu.Unlock()
	if running && len(addedKeys) > 0 {
		go p.prefetchSpecificJWTs(addedKeys, false)
	}
	if running && newCurrentKey != "" && newCurrentKey != previousCurrentKey {
		p.syncCurrentAPIKeyToClient(newCurrentKey)
	}
}

// Start starts the MITM proxy.
func (p *MitmProxy) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("代理已在运行")
	}
	if len(p.poolKeys) == 0 {
		p.mu.Unlock()
		return fmt.Errorf("号池为空，请先导入带 API Key 的账号")
	}
	p.mu.Unlock()

	// 1. Generate certificates
	p.log("生成 TLS 证书...")
	hostCert, err := EnsureCA(TargetDomain)
	if err != nil {
		return fmt.Errorf("证书生成失败: %w", err)
	}

	// 2. Setup TLS listener
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*hostCert},
	}

	addr := fmt.Sprintf("127.0.0.1:%d", p.port)
	listener, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	p.mu.Lock()
	p.listener = listener
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	p.log("代理已启动: %s", addr)

	// 3. Start JWT prefetch (synchronous — wait for first JWT)
	p.jwtOnce = sync.Once{}
	p.jwtReady = make(chan struct{})
	go p.prefetchJWTs()

	// Wait up to 15s for at least one JWT
	select {
	case <-p.jwtReady:
		p.log("✅ JWT 就绪，开始接受请求")
	case <-time.After(15 * time.Second):
		p.log("⚠️ JWT 预取超时，先接受请求（不替换身份）")
	}

	// 4. Start JWT refresh loop
	go p.jwtRefreshLoop()

	// 5. Serve requests
	go p.serve()

	return nil
}

// Stop stops the MITM proxy.
func (p *MitmProxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	close(p.stopCh)
	if p.listener != nil {
		p.listener.Close()
	}
	p.running = false
	p.log("代理已停止")
	return nil
}

// Status returns the current proxy status.
func (p *MitmProxy) Status() MitmProxyStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := MitmProxyStatus{
		Running:          p.running,
		Port:             p.port,
		HostsMapped:      IsHostsMapped(TargetDomain),
		CAInstalled:      IsCAInstalled(),
		LastErrorKind:    p.lastErrorKind,
		LastErrorSummary: p.lastErrorSummary,
		LastErrorAt:      p.lastErrorAt,
		LastErrorKey:     p.lastErrorKey,
	}

	totalReqs := 0
	for i, k := range p.poolKeys {
		state := p.keyStates[k]
		if state == nil {
			continue
		}
		totalReqs += state.RequestCount

		short := k
		if len(k) > 16 {
			short = k[:16] + "..."
		}

		p.jwtLock.RLock()
		hasJWT := len(state.JWT) > 0
		p.jwtLock.RUnlock()

		info := PoolKeyInfo{
			KeyShort:         short,
			Healthy:          state.Healthy,
			RuntimeExhausted: state.RuntimeExhausted,
			CooldownUntil:    state.CooldownUntil.Format(time.RFC3339),
			HasJWT:           hasJWT,
			RequestCount:     state.RequestCount,
			SuccessCount:     state.SuccessCount,
			TotalExhausted:   state.TotalExhausted,
			IsCurrent:        i == p.currentIdx,
		}
		status.PoolStatus = append(status.PoolStatus, info)

		if info.IsCurrent {
			status.CurrentKey = short
		}
	}
	status.TotalReqs = totalReqs
	status.RecentEvents = p.recentEventsSnapshot()
	return status
}

func (p *MitmProxy) CurrentAPIKey() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.poolKeys) == 0 || p.currentIdx < 0 || p.currentIdx >= len(p.poolKeys) {
		return ""
	}
	return p.poolKeys[p.currentIdx]
}

// buildUpstreamTransport 构建出站 Transport，支持通过用户本地代理 (如 Clash) 访问上游
func (p *MitmProxy) buildUpstreamTransport() *http.Transport {
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         UpstreamHost,
			NextProtos:         []string{"h2", "http/1.1"},
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	if p.proxyURL != "" {
		if u, err := url.Parse(p.proxyURL); err == nil {
			t.Proxy = http.ProxyURL(u)
			p.log("出站代理: %s", p.proxyURL)
		}
	}
	return t
}

// retryTransport 包装上游 Transport，在检测到额度耗尽时自动切号并重试
type retryTransport struct {
	base     http.RoundTripper
	proxy    *MitmProxy
	maxRetry int
}

type upstreamFailureKind string

const (
	upstreamFailureNone       upstreamFailureKind = ""
	upstreamFailureQuota      upstreamFailureKind = "quota"
	upstreamFailureAuth       upstreamFailureKind = "auth"
	upstreamFailureInternal   upstreamFailureKind = "internal"
	upstreamFailurePermission upstreamFailureKind = "permission"
	upstreamFailureGRPC       upstreamFailureKind = "grpc"
)

func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 保存原始 body 以便重试时重放
	var savedBody []byte
	if req.Body != nil {
		var err error
		savedBody, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(savedBody))
		req.ContentLength = int64(len(savedBody))
	}

	for attempt := 0; attempt <= rt.maxRetry; attempt++ {
		resp, err := rt.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		// 只对小响应体检查额度错误（大的是正常流式数据）
		if resp.ContentLength > 5000 {
			return resp, nil
		}

		// 读取响应体检查是否为额度耗尽
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		// gRPC Trailers-Only 错误: body 为空但 grpc-status/grpc-message 在 header 或 trailer 中
		grpcStatus := resp.Header.Get("grpc-status")
		grpcMsg := resp.Header.Get("grpc-message")
		if grpcStatus == "" {
			grpcStatus = resp.Trailer.Get("grpc-status")
		}
		if grpcMsg == "" {
			grpcMsg = resp.Trailer.Get("grpc-message")
		}

		kind, detail := classifyUpstreamFailure(grpcStatus, grpcMsg, string(respBody))
		isExhausted := kind == upstreamFailureQuota
		isAuthFailure := kind == upstreamFailureAuth
		usedKey := req.Header.Get("X-Pool-Key-Used")

		if (!isExhausted && !isAuthFailure) || attempt >= rt.maxRetry {
			// 不是可重试的额度/认证错误，或已达最大重试次数，返回
			if kind != upstreamFailureNone && kind != upstreamFailureQuota && kind != upstreamFailureAuth {
				rt.proxy.recordUpstreamFailure(kind, detail, usedKey)
				rt.proxy.log("上游%s错误(不轮转): %s", kind.logLabel(), detail)
			}
			if kind == upstreamFailureAuth {
				rt.proxy.recordUpstreamFailure(kind, detail, usedKey)
				rt.proxy.log("上游%s错误(已达重试上限): %s", kind.logLabel(), detail)
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		if isAuthFailure {
			refreshedJWT := rt.proxy.refreshJWTForKey(usedKey)
			if len(refreshedJWT) > 0 {
				newBody, replaced := ReplaceIdentityInBody(savedBody, []byte(usedKey), refreshedJWT)
				if replaced {
					req.Body = io.NopCloser(bytes.NewReader(newBody))
					req.ContentLength = int64(len(newBody))
				} else {
					req.Body = io.NopCloser(bytes.NewReader(savedBody))
					req.ContentLength = int64(len(savedBody))
				}
				req.Header.Set("Authorization", "Bearer "+string(refreshedJWT))
				req.Header.Set("X-Pool-Key-Used", usedKey)
				rt.proxy.log("★ 认证失败自动刷新 JWT(%d/%d): %s...",
					attempt+1, rt.maxRetry,
					usedKey[:minStr(12, len(usedKey))])
				continue
			}

			rt.proxy.recordUpstreamFailure(kind, detail, usedKey)
			rt.proxy.log("JWT 刷新失败，尝试切到下一把 key: %s...", usedKey[:minStr(12, len(usedKey))])
			rt.proxy.mu.Lock()
			rotatedKey := rt.proxy.rotateKey()
			rt.proxy.mu.Unlock()
			if rotatedKey != "" {
				rt.proxy.syncCurrentAPIKeyToClient(rotatedKey)
			}
		} else {
			// ★ 检测到额度耗尽，切号 + 重试
			rt.proxy.markRuntimeExhaustedAndRotate(usedKey, detail)
		}

		// 用新号重新构造请求
		newKey, newJWT := rt.proxy.pickPoolKeyAndJWT()
		if newKey == "" || len(newJWT) == 0 {
			rt.proxy.log("重试失败: 无可用号池 key")
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		// 重新替换身份
		newBody, replaced := ReplaceIdentityInBody(savedBody, []byte(newKey), newJWT)
		if replaced {
			req.Body = io.NopCloser(bytes.NewReader(newBody))
			req.ContentLength = int64(len(newBody))
		} else {
			req.Body = io.NopCloser(bytes.NewReader(savedBody))
			req.ContentLength = int64(len(savedBody))
		}
		req.Header.Set("Authorization", "Bearer "+string(newJWT))
		req.Header.Set("X-Pool-Key-Used", newKey)

		if isAuthFailure {
			rt.proxy.log("★ 认证失败切换重试(%d/%d): %s... → %s...",
				attempt+1, rt.maxRetry,
				usedKey[:minStr(12, len(usedKey))],
				newKey[:minStr(12, len(newKey))])
		} else {
			rt.proxy.log("★ 额度耗尽自动重试(%d/%d): %s... → %s...",
				attempt+1, rt.maxRetry,
				usedKey[:minStr(12, len(usedKey))],
				newKey[:minStr(12, len(newKey))])
		}
	}

	return nil, fmt.Errorf("超过最大重试次数")
}

func (rt *retryTransport) checkExhausted(textLower string) bool {
	return isQuotaExhaustedText(textLower)
}

func (p *MitmProxy) serve() {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// ★ 保留原始 Host（可能是 server.self-serve.windsurf.com 或 server.codeium.com）
			origHost := req.Host
			if origHost == "" || origHost == "127.0.0.1" || origHost == "127.0.0.1:443" {
				origHost = UpstreamHost
			}
			// 去掉端口部分
			if h, _, err := net.SplitHostPort(origHost); err == nil {
				origHost = h
			}

			p.handleRequest(req, origHost)
			req.URL.Scheme = "https"
			req.URL.Host = ResolveUpstreamIP()
			req.Host = origHost // 用原始域名作为 Host 头
		},
		Transport: &retryTransport{
			base:     p.buildUpstreamTransport(),
			proxy:    p,
			maxRetry: 3,
		},
		ModifyResponse: func(resp *http.Response) error {
			p.handleResponse(resp)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			p.log("上游错误: %s %s: %v", req.Method, req.URL.Path, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	server := &http.Server{
		Handler: proxy,
	}

	if err := server.Serve(p.listener); err != nil {
		select {
		case <-p.stopCh:
			// normal shutdown
		default:
			p.log("服务异常退出: %v", err)
		}
	}
}

func (p *MitmProxy) handleRequest(req *http.Request, origHost string) {
	// 使用传入的原始域名设置 Host 头（可能是 server.self-serve.windsurf.com 或 server.codeium.com）
	req.Host = origHost
	req.Header.Set("Host", origHost)

	path := req.URL.Path
	pathTail := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		pathTail = path[idx+1:]
	}

	ct := req.Header.Get("Content-Type")
	isProto := strings.Contains(strings.ToLower(ct), "proto") || strings.Contains(strings.ToLower(ct), "grpc")

	// ★ 全量流量记录
	trafficLog("REQ  %s %s (host=%s ct=%s cl=%d)", req.Method, path, origHost, ct, req.ContentLength)
	// 记录 Authorization header（截断）用于诊断身份替换问题
	if strings.Contains(path, "GetUserStatus") || strings.Contains(path, "GetUserJwt") {
		auth := req.Header.Get("Authorization")
		if len(auth) > 50 {
			auth = auth[:50] + "..."
		}
		trafficLog("  AUTH: %s", auth)
	}

	if !isProto {
		// Non-protobuf requests: just forward
		return
	}

	// Read body
	if req.Body == nil {
		return
	}
	bodyBytes, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil || len(bodyBytes) == 0 {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return
	}

	// Pick pool key + JWT
	poolKey, poolJWT := p.pickPoolKeyAndJWT()
	if poolKey == "" || len(poolJWT) == 0 {
		// ★ 核心安全逻辑：没有 JWT 绝不替换身份，直接透传原始请求
		if poolKey == "" {
			p.log("无可用号池 key")
		} else {
			p.log("跳过身份替换: %s (JWT 未就绪)", pathTail)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return
	}

	// Replace identity in protobuf body
	newBody, replaced := ReplaceIdentityInBody(bodyBytes, []byte(poolKey), poolJWT)
	if replaced {
		req.Body = io.NopCloser(bytes.NewReader(newBody))
		req.ContentLength = int64(len(newBody))
		p.log("身份替换: %s key=%s...%s", pathTail, poolKey[:minStr(12, len(poolKey))], suffix3(poolKey))
	} else {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Debug dump: GetChatMessage 请求
	if p.DebugDumpEnabled() && strings.Contains(path, "GetChatMessage") {
		if dumpPath, err := WriteProtoDump("req_"+pathTail, bodyBytes); err == nil {
			p.log("📝 dump 请求: %s", dumpPath)
		}
	}

	// Force Authorization header
	req.Header.Set("Authorization", "Bearer "+string(poolJWT))

	// Store pool key in request context via header (for response tracking)
	req.Header.Set("X-Pool-Key-Used", poolKey)
}

func (p *MitmProxy) handleResponse(resp *http.Response) {
	path := resp.Request.URL.Path
	pathTail := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		pathTail = path[idx+1:]
	}

	// ★ 全量流量记录（所有响应都记）
	respCT := resp.Header.Get("Content-Type")
	grpcSt := resp.Header.Get("grpc-status")
	trafficLog("RESP %s %s → %d (ct=%s cl=%d grpc-status=%s)", resp.Request.Method, path, resp.StatusCode, respCT, resp.ContentLength, grpcSt)

	// 对额度/套餐相关路径 dump 响应体
	isQuotaPath := strings.Contains(path, "PlanStatus") || strings.Contains(path, "UserStatus") ||
		strings.Contains(path, "GetUser") || strings.Contains(path, "RegisterUser") ||
		strings.Contains(path, "Subscription") || strings.Contains(path, "quota") ||
		strings.Contains(path, "Credit") || strings.Contains(path, "Billing") ||
		strings.Contains(path, "SeatManagement") || strings.Contains(path, "CurrentUser")
	if isQuotaPath && resp.Body != nil && resp.ContentLength < 500000 {
		bodySnap, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && len(bodySnap) > 0 {
			trafficLogMu.Lock()
			seq := trafficSeq
			trafficLogMu.Unlock()
			dumpPath := TrafficDumpBody(seq, sanitizePathForFile(pathTail), bodySnap)
			trafficLog("  DUMP %s (%d bytes) → %s", pathTail, len(bodySnap), dumpPath)
		}
		resp.Body = io.NopCloser(bytes.NewReader(bodySnap))
	}

	usedKey := resp.Request.Header.Get("X-Pool-Key-Used")
	resp.Request.Header.Del("X-Pool-Key-Used") // clean up internal header

	if usedKey == "" {
		return
	}

	// 优先检查响应 Content-Type；某些 gRPC 上游不返回 CT 时回退到请求 CT
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = resp.Request.Header.Get("Content-Type")
	}
	isProto := strings.Contains(strings.ToLower(ct), "proto") || strings.Contains(strings.ToLower(ct), "grpc")
	isBilling := strings.Contains(path, "GetChatMessage") || strings.Contains(path, "GetCompletions")

	// Check for exhaustion/quota errors in ALL protobuf responses
	isExhausted := false
	isSuccess := false
	exhaustedDetail := ""

	if isProto && resp.Body != nil {
		// 对小响应体直接完整读取；对对话/补全的大流式响应改为边转发边检测配额错误。
		// Trailers-Only gRPC 错误可能 ContentLength=-1 但 grpc-status 已在 header 中
		hasGRPCStatusHeader := resp.Header.Get("grpc-status") != "" && resp.Header.Get("grpc-status") != "0"
		shouldCheckBuffered := (resp.ContentLength >= 0 && resp.ContentLength < 5000) || resp.StatusCode >= 400 || hasGRPCStatusHeader
		shouldWatchStream := isBilling && resp.StatusCode == 200 && !shouldCheckBuffered
		if isBilling {
			trafficLog("  BILLING-PATH: path=%s isProto=%v buffered=%v stream=%v cl=%d status=%d grpcSt=%s key=%s...",
				pathTail, isProto, shouldCheckBuffered, shouldWatchStream, resp.ContentLength, resp.StatusCode, grpcSt, usedKey[:minStr(12, len(usedKey))])
		}

		if shouldCheckBuffered {
			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err == nil {
				// 同 retryTransport: 优先 header，回退 trailer（Trailers-Only 场景）
				gs := resp.Header.Get("grpc-status")
				gm := resp.Header.Get("grpc-message")
				if gs == "" {
					gs = resp.Trailer.Get("grpc-status")
				}
				if gm == "" {
					gm = resp.Trailer.Get("grpc-message")
				}
				kind, detail := classifyUpstreamFailure(gs, gm, string(bodyBytes))
				if kind == upstreamFailureQuota {
					isExhausted = true
					exhaustedDetail = detail
					p.log("额度耗尽: %s key=%s...", pathTail, usedKey[:minStr(12, len(usedKey))])
				} else if kind != upstreamFailureNone && isBilling {
					p.recordUpstreamFailure(kind, detail, usedKey)
					p.log("上游%s错误: %s key=%s...", kind.logLabel(), detail, usedKey[:minStr(12, len(usedKey))])
				}
				// Debug dump: GetChatMessage 响应（小包）
				if p.DebugDumpEnabled() && strings.Contains(path, "GetChatMessage") {
					if dumpPath, err := WriteProtoDump("resp_small_"+pathTail, bodyBytes); err == nil {
						p.log("📝 dump 响应(buffered): %s", dumpPath)
					}
				}
			}
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		} else if shouldWatchStream {
			// Debug dump: 对流式响应包装一个 tee 来捕获前几个 chunk
			var dumpBody io.ReadCloser
			if p.DebugDumpEnabled() && strings.Contains(path, "GetChatMessage") {
				dumpBody = newDumpTeeBody(resp.Body, "resp_stream_"+pathTail, p)
			}

			baseBody := resp.Body
			if dumpBody != nil {
				baseBody = dumpBody
			}
			resp.Body = newQuotaStreamWatchBody(baseBody, func(detail string) {
				p.log("流式额度耗尽: %s key=%s...", pathTail, usedKey[:minStr(12, len(usedKey))])
				rotatedKey := p.markRuntimeExhaustedAndRotate(usedKey, detail)
				if rotatedKey != "" {
					p.log("★ 流式额度耗尽立即轮转: %s... → %s...", usedKey[:minStr(12, len(usedKey))], rotatedKey[:minStr(12, len(rotatedKey))])
				}
			}, func() {
				p.mu.Lock()
				if state := p.keyStates[usedKey]; state != nil {
					state.recordSuccess()
				}
				p.mu.Unlock()
			})
		} else if isBilling && resp.StatusCode == 200 {
			isSuccess = true
		}
	}

	// Capture JWT from GetUserJwt response
	if strings.Contains(path, "GetUserJwt") && resp.StatusCode == 200 && resp.Body != nil {
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && len(bodyBytes) > 0 {
			jwt := ExtractJWTFromBody(bodyBytes)
			if jwt != "" && usedKey != "" {
				p.updateJWT(usedKey, []byte(jwt))
				p.log("捕获 JWT: key=%s... (%dB)", usedKey[:minStr(12, len(usedKey))], len(jwt))
			}
		}
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Update key state
	rotatedKey := ""
	if isExhausted {
		p.log("★ key 额度耗尽，立即轮转: %s...", usedKey[:minStr(12, len(usedKey))])
		rotatedKey = p.markRuntimeExhaustedAndRotate(usedKey, exhaustedDetail)
	} else {
		p.mu.Lock()
		state := p.keyStates[usedKey]
		if state != nil && isSuccess && isBilling {
			state.recordSuccess()
		}
		p.mu.Unlock()
	}
	if rotatedKey != "" {
		p.log("★ 额度耗尽已切换到: %s...", rotatedKey[:minStr(12, len(rotatedKey))])
	}
}

// ── Pool key selection ──

func (p *MitmProxy) pickPoolKeyAndJWT() (string, []byte) {
	p.mu.Lock()
	if len(p.poolKeys) == 0 {
		p.mu.Unlock()
		return "", nil
	}

	// Check if current key is still available
	currentKey := p.poolKeys[p.currentIdx]
	state := p.keyStates[currentKey]
	rotatedKey := ""
	if state != nil && !state.isAvailable() {
		// Current key cooling down, rotate
		rotatedKey = p.rotateKey()
		currentKey = p.poolKeys[p.currentIdx]
	}
	currentIdx := p.currentIdx
	keys := make([]string, len(p.poolKeys))
	copy(keys, p.poolKeys)
	p.mu.Unlock()
	if rotatedKey != "" {
		p.syncCurrentAPIKeyToClient(rotatedKey)
	}

	jwt := p.jwtBytesForKey(currentKey)
	if len(jwt) == 0 {
		jwt = p.ensureJWTForKey(currentKey)
	}

	// If current key has no JWT, find one that does
	if len(jwt) == 0 {
		for i := 0; i < len(keys); i++ {
			idx := (currentIdx + i) % len(keys)
			k := keys[idx]
			j := p.jwtBytesForKey(k)
			if len(j) == 0 {
				j = p.ensureJWTForKey(k)
			}
			if len(j) > 0 {
				p.mu.Lock()
				for liveIdx, liveKey := range p.poolKeys {
					if liveKey == k {
						p.currentIdx = liveIdx
						break
					}
				}
				p.mu.Unlock()
				return k, j
			}
		}
	}

	return currentKey, jwt
}

func (p *MitmProxy) rotateKey() string {
	if len(p.poolKeys) <= 1 {
		if len(p.poolKeys) == 1 {
			return p.poolKeys[p.currentIdx]
		}
		return ""
	}
	oldKey := p.poolKeys[p.currentIdx]

	// Find next available key
	for i := 1; i < len(p.poolKeys); i++ {
		idx := (p.currentIdx + i) % len(p.poolKeys)
		state := p.keyStates[p.poolKeys[idx]]
		if state != nil && state.isAvailable() {
			p.currentIdx = idx
			p.log("轮转: %s... → %s...", oldKey[:minStr(12, len(oldKey))],
				p.poolKeys[idx][:minStr(12, len(p.poolKeys[idx]))])
			return p.poolKeys[idx]
		}
	}

	// All exhausted: pick the one with shortest cooldown
	bestIdx := (p.currentIdx + 1) % len(p.poolKeys)
	bestCooldown := time.Duration(1<<63 - 1)
	for i, k := range p.poolKeys {
		state := p.keyStates[k]
		if state != nil {
			cd := time.Until(state.CooldownUntil)
			if cd < bestCooldown {
				bestCooldown = cd
				bestIdx = i
			}
		}
	}
	p.currentIdx = bestIdx
	p.log("所有 key 耗尽，选最短冷却: %s...", p.poolKeys[bestIdx][:minStr(12, len(p.poolKeys[bestIdx]))])
	return p.poolKeys[bestIdx]
}

// SwitchToKey 手动切换 MITM 代理到指定 API Key（前端「切换到此账号」「下一席位」调用）
func (p *MitmProxy) SwitchToKey(apiKey string) bool {
	p.mu.Lock()
	switchedKey := ""

	for i, k := range p.poolKeys {
		if k == apiKey {
			p.currentIdx = i
			// 重置该 key 状态为健康
			if state := p.keyStates[k]; state != nil {
				state.Healthy = true
				state.ConsecutiveErrs = 0
			}
			p.log("手动切换: → %s...", apiKey[:minStr(12, len(apiKey))])
			switchedKey = k
			break
		}
	}
	p.mu.Unlock()
	if switchedKey == "" {
		return false
	}
	p.syncCurrentAPIKeyToClient(switchedKey)
	return true
}

// ── JWT management ──

func (p *MitmProxy) updateJWT(apiKey string, jwt []byte) {
	p.mu.Lock()
	state := p.keyStates[apiKey]
	p.mu.Unlock()
	if state == nil {
		return
	}
	p.jwtLock.Lock()
	state.JWT = jwt
	p.jwtLock.Unlock()
}

func (p *MitmProxy) clearJWT(apiKey string) {
	p.mu.RLock()
	state := p.keyStates[apiKey]
	p.mu.RUnlock()
	if state == nil {
		return
	}
	p.jwtLock.Lock()
	state.JWT = nil
	p.jwtLock.Unlock()
}

func (p *MitmProxy) jwtBytesForKey(apiKey string) []byte {
	p.mu.RLock()
	state := p.keyStates[apiKey]
	p.mu.RUnlock()
	if state == nil {
		return nil
	}
	p.jwtLock.RLock()
	defer p.jwtLock.RUnlock()
	if len(state.JWT) == 0 {
		return nil
	}
	jwt := make([]byte, len(state.JWT))
	copy(jwt, state.JWT)
	return jwt
}

func (p *MitmProxy) ensureJWTForKey(apiKey string) []byte {
	if apiKey == "" || p.windsurfSvc == nil || !strings.HasPrefix(apiKey, "sk-ws-") {
		return nil
	}
	if jwt := p.jwtBytesForKey(apiKey); len(jwt) > 0 {
		return jwt
	}
	jwt, err := getJWTByAPIKeyFn(p.windsurfSvc, apiKey)
	if err != nil {
		p.log("JWT 按需获取失败: %s... (%v)", apiKey[:minStr(12, len(apiKey))], err)
		return nil
	}
	out := []byte(jwt)
	p.updateJWT(apiKey, out)
	p.jwtOnce.Do(func() {
		close(p.jwtReady)
	})
	p.log("JWT 按需获取成功: %s... (%dB)", apiKey[:minStr(12, len(apiKey))], len(out))
	return out
}

func (p *MitmProxy) refreshJWTForKey(apiKey string) []byte {
	if apiKey == "" || p.windsurfSvc == nil || !strings.HasPrefix(apiKey, "sk-ws-") {
		return nil
	}
	p.clearJWT(apiKey)
	jwt, err := getJWTByAPIKeyFn(p.windsurfSvc, apiKey)
	if err != nil {
		p.log("JWT 强制刷新失败: %s... (%v)", apiKey[:minStr(12, len(apiKey))], err)
		return nil
	}
	out := []byte(jwt)
	p.updateJWT(apiKey, out)
	p.jwtOnce.Do(func() {
		close(p.jwtReady)
	})
	p.log("JWT 强制刷新成功: %s... (%dB)", apiKey[:minStr(12, len(apiKey))], len(out))
	return out
}

func (p *MitmProxy) prefetchSpecificJWTs(keys []string, force bool) {
	if force {
		p.log("开始强制刷新 %d 个 key 的 JWT...", len(keys))
	} else {
		p.log("开始预取 %d 个 key 的 JWT...", len(keys))
	}
	for _, key := range keys {
		if !force && len(p.jwtBytesForKey(key)) > 0 {
			continue
		}
		if force {
			_ = p.refreshJWTForKey(key)
			continue
		}
		_ = p.ensureJWTForKey(key)
	}
}

func (p *MitmProxy) prefetchJWTs() {
	p.mu.RLock()
	keys := make([]string, len(p.poolKeys))
	copy(keys, p.poolKeys)
	p.mu.RUnlock()
	p.prefetchSpecificJWTs(keys, false)
}

func (p *MitmProxy) jwtRefreshLoop() {
	ticker := time.NewTicker(jwtRefreshMinutes * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.log("定时刷新 JWT...")
			p.mu.RLock()
			var keys []string
			for _, k := range p.poolKeys {
				if state := p.keyStates[k]; state != nil && state.RuntimeExhausted {
					continue // 跳过已耗尽的 key，节省 API 调用
				}
				keys = append(keys, k)
			}
			p.mu.RUnlock()
			if len(keys) > 0 {
				p.prefetchSpecificJWTs(keys, true)
			}
		}
	}
}

// ── Helpers ──

func minStr(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func suffix3(s string) string {
	if len(s) < 6 {
		return ""
	}
	return s[len(s)-3:]
}

func (k upstreamFailureKind) logLabel() string {
	switch k {
	case upstreamFailureQuota:
		return "额度"
	case upstreamFailureAuth:
		return "认证"
	case upstreamFailureInternal:
		return "内部"
	case upstreamFailurePermission:
		return "权限"
	case upstreamFailureGRPC:
		return "gRPC"
	default:
		return "未知"
	}
}

func decodeGRPCMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if decoded, err := url.QueryUnescape(raw); err == nil && decoded != "" {
		return decoded
	}
	return raw
}

func classifyUpstreamFailure(grpcStatus, grpcMessage, bodyText string) (upstreamFailureKind, string) {
	status := strings.TrimSpace(grpcStatus)
	msg := decodeGRPCMessage(grpcMessage)
	msgLower := strings.ToLower(msg)
	bodyLower := strings.ToLower(bodyText)
	combined := strings.TrimSpace(bodyLower + "\n" + msgLower)

	// gRPC 8=RESOURCE_EXHAUSTED, 9=FAILED_PRECONDITION (Windsurf 额度耗尽常返回 9)
	if status == "8" || isQuotaExhaustedText(combined) {
		return upstreamFailureQuota, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "9" && (strings.Contains(combined, "quota") || strings.Contains(combined, "usage") || strings.Contains(combined, "credits")) {
		return upstreamFailureQuota, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "16" || strings.Contains(combined, "unauthenticated") || strings.Contains(combined, "authentication credentials") {
		return upstreamFailureAuth, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if strings.Contains(combined, `"code":"permission_denied"`) ||
		strings.Contains(combined, "'code':'permission_denied'") ||
		strings.Contains(combined, "[permission_denied]") ||
		strings.Contains(combined, "api server wire error: permission denied") ||
		strings.Contains(combined, "permission_denied") {
		return upstreamFailureAuth, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "13" || strings.Contains(combined, "internal server error") || strings.Contains(combined, "error number 13") {
		return upstreamFailureInternal, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status == "7" || strings.Contains(combined, "permission denied") || strings.Contains(combined, "unauthorized") || strings.Contains(combined, "forbidden") {
		return upstreamFailurePermission, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	if status != "" && status != "0" {
		return upstreamFailureGRPC, formatUpstreamFailureDetail(status, msg, bodyText)
	}
	return upstreamFailureNone, ""
}

func formatUpstreamFailureDetail(grpcStatus, grpcMessage, bodyText string) string {
	parts := make([]string, 0, 3)
	if s := strings.TrimSpace(grpcStatus); s != "" {
		parts = append(parts, "grpc-status="+s)
	}
	if s := strings.TrimSpace(grpcMessage); s != "" {
		parts = append(parts, "grpc-message="+truncate(s, 140))
	}
	body := strings.TrimSpace(bodyText)
	if body != "" {
		parts = append(parts, "body="+truncate(body, 180))
	}
	if len(parts) == 0 {
		return "无上游细节"
	}
	return strings.Join(parts, " ")
}

func isQuotaExhaustedText(textLower string) bool {
	patterns := []string{
		"resource_exhausted",
		"resource exhausted",
		"not enough credits",
		"daily usage quota has been exhausted",
		"weekly usage quota has been exhausted",
		"usage quota has been exhausted",
		"usage quota is exhausted",
		"included usage quota is exhausted",
		"quota has been exhausted",
		"quota is exhausted",
		"quota exhausted",
		"daily_quota_exhausted",
		"weekly_quota_exhausted",
		"purchase extra usage",
	}
	for _, pat := range patterns {
		if strings.Contains(textLower, pat) {
			return true
		}
	}
	return (strings.Contains(textLower, "failed_precondition") || strings.Contains(textLower, "failed precondition")) &&
		(strings.Contains(textLower, "quota") || strings.Contains(textLower, "usage") || strings.Contains(textLower, "credits"))
}
