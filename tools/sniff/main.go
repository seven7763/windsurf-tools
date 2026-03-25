// sniff — 轻量 TLS 反向代理，拦截 Windsurf IDE 到 server.codeium.com / server.self-serve.windsurf.com 的所有请求并 dump
// 用法：
//  1. go run tools/sniff/main.go
//  2. 确保 hosts 已映射 (127.0.0.1 server.codeium.com server.self-serve.windsurf.com)
//  3. 启动/重启 Windsurf IDE，观察控制台输出
//  4. Ctrl+C 退出
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"windsurf-tools-wails/backend/services"
)

const (
	listenAddr = "127.0.0.1:443"
	proxyURL   = "http://127.0.0.1:7890"
)

var dumpDir string
var logFile *os.File

const baseDir = `E:\windsurf切号\windsurf-tools-wails`

func init() {
	dumpDir = filepath.Join(baseDir, "sniff_dumps")
	os.MkdirAll(dumpDir, 0755)
	logPath := filepath.Join(baseDir, "sniff_log.txt")
	var err error
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("无法创建日志文件 %s: %v", logPath, err)
	}
	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
}

func main() {
	// 生成 CA + 域名证书
	hostCert, err := services.EnsureCA(services.TargetDomain)
	if err != nil {
		log.Fatalf("证书生成失败: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*hostCert},
	}

	listener, err := tls.Listen("tcp", listenAddr, tlsConfig)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", listenAddr, err)
	}
	log.Printf("🔍 Sniff 代理启动: %s → %s (proxy: %s)", listenAddr, services.ResolveUpstreamIP(), proxyURL)
	log.Printf("📁 Dump 目录: %s", dumpDir)
	log.Println("等待 Windsurf IDE 请求...")

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         services.UpstreamHost,
			NextProtos:         []string{"h2", "http/1.1"},
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	reqCounter := 0

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			origHost := req.Host
			if origHost == "" || strings.HasPrefix(origHost, "127.0.0.1") {
				origHost = services.UpstreamHost
			}
			if h, _, err := net.SplitHostPort(origHost); err == nil {
				origHost = h
			}
			req.URL.Scheme = "https"
			req.URL.Host = services.ResolveUpstreamIP()
			req.Host = origHost
			req.Header.Set("Host", origHost)
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			reqCounter++
			req := resp.Request
			path := req.URL.Path
			method := req.Method
			status := resp.StatusCode
			ct := resp.Header.Get("Content-Type")

			// 基础日志
			grpcStatus := resp.Header.Get("grpc-status")
			tag := ""
			if grpcStatus != "" && grpcStatus != "0" {
				tag = fmt.Sprintf(" [grpc-status=%s]", grpcStatus)
			}

			log.Printf("#%03d %s %s → %d (ct=%s, cl=%d)%s",
				reqCounter, method, path, status, shortCT(ct), resp.ContentLength, tag)

			// 对额度/套餐相关路径做详细 dump
			isInteresting := isQuotaOrPlanPath(path)
			isSmall := resp.ContentLength >= 0 && resp.ContentLength < 500000

			if isInteresting && isSmall && resp.Body != nil {
				bodyBytes, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err == nil {
					resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

					// 解压 gzip
					payload := bodyBytes
					if len(payload) > 5 {
						// gRPC: skip 5-byte frame header
						inner := payload[5:]
						if len(inner) >= 2 && inner[0] == 0x1f && inner[1] == 0x8b {
							if r, err := gzip.NewReader(bytes.NewReader(inner)); err == nil {
								if d, err := io.ReadAll(r); err == nil {
									inner = d
								}
								r.Close()
							}
						}

						// 尝试 JSON 格式化
						if json.Valid(payload) {
							var pretty bytes.Buffer
							json.Indent(&pretty, payload, "", "  ")
							dumpToFile(reqCounter, path, pretty.Bytes())
							log.Printf("    📋 JSON 响应 (%d bytes), 已 dump", len(payload))
						} else {
							// protobuf: dump raw + decompressed
							dumpToFile(reqCounter, path, payload)
							if len(inner) > 10 {
								dumpToFile(reqCounter, path+"_decoded", inner)
								log.Printf("    📋 Proto 响应 (%d→%d bytes), dumped", len(payload), len(inner))
							}
						}
					}
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			log.Printf("❌ 上游错误: %s %s: %v", req.Method, req.URL.Path, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	server := &http.Server{Handler: proxy}
	if err := server.Serve(listener); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}

func isQuotaOrPlanPath(path string) bool {
	lower := strings.ToLower(path)
	keywords := []string{
		"planstatus", "plan_status", "getplan",
		"userstatus", "user_status", "getuser",
		"quota", "billing", "credit", "subscription",
		"seat_management", "auth",
		"currentuser", "current_user",
		"registeruser", "register_user",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return true // dump ALL for discovery
}

func shortCT(ct string) string {
	if ct == "" {
		return "-"
	}
	if len(ct) > 40 {
		return ct[:40] + "…"
	}
	return ct
}

func dumpToFile(counter int, path string, data []byte) string {
	// Sanitize path for filename
	name := strings.ReplaceAll(path, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if len(name) > 80 {
		name = name[:80]
	}
	fname := fmt.Sprintf("%03d_%s.bin", counter, name)
	fpath := filepath.Join(dumpDir, fname)
	os.WriteFile(fpath, data, 0644)
	return fpath
}
