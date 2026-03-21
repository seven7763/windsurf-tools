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
)

const (
	TargetDomain = "server.self-serve.windsurf.com"
	UpstreamIP   = "34.49.14.144"
	UpstreamHost = "server.self-serve.windsurf.com"

	defaultProxyPort  = 443
	jwtRefreshMinutes = 14
	maxConsecErrors   = 1
	keyCooldownSec    = 600
)

// PoolKeyState tracks the runtime state of each pool key.
type PoolKeyState struct {
	APIKey           string
	JWT              []byte
	Healthy          bool
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
	s.CooldownUntil = time.Now().Add(keyCooldownSec * time.Second)
	s.ConsecutiveErrs = 0
	s.TotalExhausted++
}

func (s *PoolKeyState) isAvailable() bool {
	if s.Healthy {
		return true
	}
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
	s.ConsecutiveErrs = 0
}

func (s *PoolKeyState) recordError() bool {
	s.RequestCount++
	s.ConsecutiveErrs++
	return s.ConsecutiveErrs >= maxConsecErrors
}

// MitmProxy is the core MITM reverse proxy that handles identity replacement.
type MitmProxy struct {
	mu          sync.RWMutex
	listener    net.Listener
	running     bool
	port        int
	proxyURL    string             // 出站代理 (如 http://127.0.0.1:7890)

	poolKeys    []string           // ordered list of api keys
	keyStates   map[string]*PoolKeyState
	currentIdx  int
	jwtLock     sync.RWMutex

	windsurfSvc *WindsurfService   // for JWT refresh
	logFn       func(string)       // log callback for UI

	jwtReady    chan struct{}       // closed when at least one JWT is available
	jwtOnce     sync.Once
	stopCh      chan struct{}
}

// MitmProxyStatus is exposed to the frontend.
type MitmProxyStatus struct {
	Running     bool              `json:"running"`
	Port        int               `json:"port"`
	HostsMapped bool              `json:"hosts_mapped"`
	CAInstalled bool              `json:"ca_installed"`
	CurrentKey  string            `json:"current_key"`
	PoolStatus  []PoolKeyInfo     `json:"pool_status"`
	TotalReqs   int               `json:"total_requests"`
}

type PoolKeyInfo struct {
	KeyShort       string `json:"key_short"`
	Healthy        bool   `json:"healthy"`
	HasJWT         bool   `json:"has_jwt"`
	RequestCount   int    `json:"request_count"`
	SuccessCount   int    `json:"success_count"`
	TotalExhausted int    `json:"total_exhausted"`
	IsCurrent      bool   `json:"is_current"`
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

func (p *MitmProxy) log(format string, args ...interface{}) {
	msg := fmt.Sprintf("[MITM] "+format, args...)
	log.Println(msg)
	if p.logFn != nil {
		p.logFn(msg)
	}
}

// SetPoolKeys configures the account pool from API keys.
func (p *MitmProxy) SetPoolKeys(keys []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.poolKeys = keys
	for _, k := range keys {
		if _, ok := p.keyStates[k]; !ok {
			p.keyStates[k] = newPoolKeyState(k)
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

	if p.currentIdx >= len(keys) {
		p.currentIdx = 0
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
		Running:     p.running,
		Port:        p.port,
		HostsMapped: IsHostsMapped(TargetDomain),
		CAInstalled: IsCAInstalled(),
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
			KeyShort:       short,
			Healthy:        state.Healthy,
			HasJWT:         hasJWT,
			RequestCount:   state.RequestCount,
			SuccessCount:   state.SuccessCount,
			TotalExhausted: state.TotalExhausted,
			IsCurrent:      i == p.currentIdx,
		}
		status.PoolStatus = append(status.PoolStatus, info)

		if info.IsCurrent {
			status.CurrentKey = short
		}
	}
	status.TotalReqs = totalReqs
	return status
}

// buildUpstreamTransport 构建出站 Transport，支持通过用户本地代理 (如 Clash) 访问上游
func (p *MitmProxy) buildUpstreamTransport() *http.Transport {
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:        UpstreamHost,
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
	base    http.RoundTripper
	proxy   *MitmProxy
	maxRetry int
}

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
		if err != nil || len(respBody) == 0 {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		textLower := strings.ToLower(string(respBody))
		isExhausted := rt.checkExhausted(textLower)

		if !isExhausted || attempt >= rt.maxRetry {
			// 不是额度错误，或已达最大重试次数，返回
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			return resp, nil
		}

		// ★ 检测到额度耗尽，切号 + 重试
		usedKey := req.Header.Get("X-Pool-Key-Used")
		rt.proxy.mu.Lock()
		if state := rt.proxy.keyStates[usedKey]; state != nil {
			state.markExhausted()
		}
		rt.proxy.rotateKey()
		rt.proxy.mu.Unlock()

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

		rt.proxy.log("★ 额度耗尽自动重试(%d/%d): %s... → %s...",
			attempt+1, rt.maxRetry,
			usedKey[:minStr(12, len(usedKey))],
			newKey[:minStr(12, len(newKey))])
	}

	return nil, fmt.Errorf("超过最大重试次数")
}

func (rt *retryTransport) checkExhausted(textLower string) bool {
	patterns := []string{
		"resource_exhausted", "resource exhausted",
		"not enough credits",
		"daily usage quota has been exhausted",
		"weekly usage quota has been exhausted",
		"usage quota has been exhausted",
		"quota has been exhausted",
		"daily_quota_exhausted", "weekly_quota_exhausted",
		"permission denied",
	}
	for _, pat := range patterns {
		if strings.Contains(textLower, pat) {
			return true
		}
	}
	if (strings.Contains(textLower, "failed_precondition") || strings.Contains(textLower, "failed precondition")) &&
		(strings.Contains(textLower, "quota") || strings.Contains(textLower, "usage") || strings.Contains(textLower, "credits")) {
		return true
	}
	return false
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
			req.URL.Host = UpstreamIP
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

	usedKey := resp.Request.Header.Get("X-Pool-Key-Used")
	resp.Request.Header.Del("X-Pool-Key-Used") // clean up internal header

	if usedKey == "" {
		return
	}

	ct := resp.Request.Header.Get("Content-Type")
	isProto := strings.Contains(strings.ToLower(ct), "proto") || strings.Contains(strings.ToLower(ct), "grpc")
	isBilling := strings.Contains(path, "GetChatMessage") || strings.Contains(path, "GetCompletions")

	// Check for exhaustion/quota errors in ALL protobuf responses
	isExhausted := false
	isSuccess := false

	if isProto && resp.Body != nil {
		// ★ 检查所有非流式响应体：gRPC 额度错误可能是 HTTP 200 + chunked
		shouldCheck := resp.ContentLength < 5000 || resp.StatusCode >= 400
		shouldMarkSuccess := resp.ContentLength > 5000 && resp.StatusCode == 200

		if shouldCheck {
			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err == nil && len(bodyBytes) > 0 {
				textLower := strings.ToLower(string(bodyBytes))

				// ★ Exhaustion patterns — covers all known quota error messages
				exhaustionPatterns := []string{
					"resource_exhausted",
					"resource exhausted",
					"not enough credits",
					"daily usage quota has been exhausted",
					"weekly usage quota has been exhausted",
					"usage quota has been exhausted",
					"quota has been exhausted",
					"daily_quota_exhausted",
					"weekly_quota_exhausted",
					"permission denied",
				}
				for _, pat := range exhaustionPatterns {
					if strings.Contains(textLower, pat) {
						isExhausted = true
						p.log("额度耗尽: %s key=%s... [%s]", pathTail, usedKey[:minStr(12, len(usedKey))], pat)
						break
					}
				}

				// ★ FAILED_PRECONDITION / Failed precondition with quota/usage message
				if !isExhausted &&
					(strings.Contains(textLower, "failed_precondition") || strings.Contains(textLower, "failed precondition")) &&
					(strings.Contains(textLower, "quota") || strings.Contains(textLower, "usage") || strings.Contains(textLower, "credits")) {
					isExhausted = true
					p.log("额度耗尽(precondition): %s key=%s...", pathTail, usedKey[:minStr(12, len(usedKey))])
				}
			}
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		} else if shouldMarkSuccess && isBilling {
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
	p.mu.Lock()
	state := p.keyStates[usedKey]
	if state != nil {
		if isSuccess && isBilling {
			state.recordSuccess()
		} else if isExhausted {
			// ★ 额度耗尽 = 立即标记 + 轮转，不等连续错误
			p.log("★ key 额度耗尽，立即轮转: %s...", usedKey[:minStr(12, len(usedKey))])
			state.markExhausted()
			p.rotateKey()
		}
	}
	p.mu.Unlock()
}

// ── Pool key selection ──

func (p *MitmProxy) pickPoolKeyAndJWT() (string, []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.poolKeys) == 0 {
		return "", nil
	}

	// Check if current key is still available
	currentKey := p.poolKeys[p.currentIdx]
	state := p.keyStates[currentKey]
	if state != nil && !state.isAvailable() {
		// Current key cooling down, rotate
		p.rotateKey()
		currentKey = p.poolKeys[p.currentIdx]
	}

	p.jwtLock.RLock()
	jwt := p.keyStates[currentKey].JWT
	p.jwtLock.RUnlock()

	// If current key has no JWT, find one that does
	if len(jwt) == 0 {
		for i := 0; i < len(p.poolKeys); i++ {
			idx := (p.currentIdx + i) % len(p.poolKeys)
			k := p.poolKeys[idx]
			p.jwtLock.RLock()
			j := p.keyStates[k].JWT
			p.jwtLock.RUnlock()
			if len(j) > 0 {
				p.currentIdx = idx
				return k, j
			}
		}
	}

	return currentKey, jwt
}

func (p *MitmProxy) rotateKey() {
	if len(p.poolKeys) <= 1 {
		return
	}

	oldKey := p.poolKeys[p.currentIdx]
	if state := p.keyStates[oldKey]; state != nil {
		state.markExhausted()
	}

	// Find next available key
	for i := 1; i < len(p.poolKeys); i++ {
		idx := (p.currentIdx + i) % len(p.poolKeys)
		state := p.keyStates[p.poolKeys[idx]]
		if state != nil && state.isAvailable() {
			p.currentIdx = idx
			p.log("轮转: %s... → %s...", oldKey[:minStr(12, len(oldKey))],
				p.poolKeys[idx][:minStr(12, len(p.poolKeys[idx]))])
			return
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
}

// SwitchToKey 手动切换 MITM 代理到指定 API Key（前端「切换到此账号」「下一席位」调用）
func (p *MitmProxy) SwitchToKey(apiKey string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, k := range p.poolKeys {
		if k == apiKey {
			p.currentIdx = i
			// 重置该 key 状态为健康
			if state := p.keyStates[k]; state != nil {
				state.Healthy = true
				state.ConsecutiveErrs = 0
			}
			p.log("手动切换: → %s...", apiKey[:minStr(12, len(apiKey))])
			return true
		}
	}
	return false
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

func (p *MitmProxy) prefetchJWTs() {
	p.mu.RLock()
	keys := make([]string, len(p.poolKeys))
	copy(keys, p.poolKeys)
	p.mu.RUnlock()

	p.log("开始预取 %d 个 key 的 JWT...", len(keys))

	for _, key := range keys {
		if !strings.HasPrefix(key, "sk-ws-") {
			continue
		}
		jwt, err := p.windsurfSvc.GetJWTByAPIKey(key)
		if err != nil {
			p.log("JWT 获取失败: %s... (%v)", key[:minStr(12, len(key))], err)
			continue
		}
		p.updateJWT(key, []byte(jwt))
		p.log("JWT 获取成功: %s... (%dB)", key[:minStr(12, len(key))], len(jwt))

		// Signal that at least one JWT is ready
		p.jwtOnce.Do(func() {
			close(p.jwtReady)
		})
	}
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
			p.prefetchJWTs()
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
