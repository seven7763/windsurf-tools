package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"windsurf-tools-wails/backend/paths"

	"github.com/kardianos/service"
)

const managedServiceName = "WindsurfTools"
const (
	backgroundServiceLogName     = "background-service.log"
	desktopRuntimeLogName        = "desktop-runtime.log"
	backgroundServiceLogArchive  = "background-service.log.1"
	desktopRuntimeLogArchive     = "desktop-runtime.log.1"
	backgroundServiceLogMaxBytes = 2 * 1024 * 1024
	backgroundServiceTailLines   = 8
)

type BackgroundServiceStatus struct {
	Name             string   `json:"name"`
	Platform         string   `json:"platform"`
	Supported        bool     `json:"supported"`
	Installed        bool     `json:"installed"`
	Running          bool     `json:"running"`
	Status           string   `json:"status"`
	Detail           string   `json:"detail"`
	AutostartMitm    bool     `json:"autostart_mitm"`
	LogPath          string   `json:"log_path"`
	RecentLogs       []string `json:"recent_logs"`
	LastLogAt        string   `json:"last_log_at"`
	LastLogLine      string   `json:"last_log_line"`
	LastLogTone      string   `json:"last_log_tone"`
	LastErrorAt      string   `json:"last_error_at"`
	LastErrorLine    string   `json:"last_error_line"`
	RecentErrorCount int      `json:"recent_error_count"`
}

type DesktopRuntimeStatus struct {
	Status           string   `json:"status"`
	Detail           string   `json:"detail"`
	LogPath          string   `json:"log_path"`
	RecentLogs       []string `json:"recent_logs"`
	LastLogAt        string   `json:"last_log_at"`
	LastLogLine      string   `json:"last_log_line"`
	LastLogTone      string   `json:"last_log_tone"`
	LastErrorAt      string   `json:"last_error_at"`
	LastErrorLine    string   `json:"last_error_line"`
	RecentErrorCount int      `json:"recent_error_count"`
}

type backgroundServiceLogSummary struct {
	LastLogAt        string
	LastLogLine      string
	LastLogTone      string
	LastErrorAt      string
	LastErrorLine    string
	RecentErrorCount int
}

type serviceHandle interface {
	Start() error
	Stop() error
	Restart() error
	Install() error
	Uninstall() error
	Platform() string
	String() string
	Status() (service.Status, error)
}

var managedServiceFactory = func() (serviceHandle, error) {
	return newManagedService()
}

var backgroundServiceLogPathFn = defaultBackgroundServiceLogPath
var desktopRuntimeLogPathFn = defaultDesktopRuntimeLogPath

func defaultBackgroundServiceLogPath() (string, error) {
	dir, err := paths.ResolveAppConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, backgroundServiceLogName), nil
}

func defaultDesktopRuntimeLogPath() (string, error) {
	dir, err := paths.ResolveAppConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, desktopRuntimeLogName), nil
}

func trimRecentLogLines(raw []byte, maxLines int) []string {
	if maxLines <= 0 || len(raw) == 0 {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	out := make([]string, 0, maxLines)
	for i := len(lines) - 1; i >= 0 && len(out) < maxLines; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func splitBackgroundServiceLogLine(line string) (string, string) {
	clean := strings.TrimSpace(line)
	if clean == "" {
		return "", ""
	}
	parts := strings.Fields(clean)
	if len(parts) >= 3 && strings.Contains(parts[0], "/") && strings.Contains(parts[1], ":") {
		ts := parts[0] + " " + parts[1]
		msg := strings.TrimSpace(strings.TrimPrefix(clean, ts))
		return ts, msg
	}
	return "", clean
}

func classifyBackgroundServiceLogTone(line string) string {
	lower := strings.ToLower(strings.TrimSpace(line))
	switch {
	case lower == "":
		return "info"
	case strings.Contains(lower, "panic"),
		strings.Contains(lower, " fatal"),
		strings.Contains(lower, " failed"),
		strings.Contains(lower, " error"),
		strings.Contains(lower, " denied"),
		strings.Contains(lower, " address already in use"),
		strings.Contains(lower, " bind:"),
		strings.Contains(lower, " unavailable"):
		return "error"
	case strings.Contains(lower, " warning"),
		strings.Contains(lower, " warn"),
		strings.Contains(lower, " disabled"),
		strings.Contains(lower, " stop requested"),
		strings.Contains(lower, " canceled"),
		strings.Contains(lower, " cancelled"):
		return "warning"
	default:
		return "info"
	}
}

func summarizeBackgroundServiceLogs(lines []string) backgroundServiceLogSummary {
	var out backgroundServiceLogSummary
	for _, line := range lines {
		at, msg := splitBackgroundServiceLogLine(line)
		if msg == "" {
			continue
		}
		tone := classifyBackgroundServiceLogTone(msg)
		out.LastLogAt = at
		out.LastLogLine = msg
		out.LastLogTone = tone
		if tone == "error" {
			out.LastErrorAt = at
			out.LastErrorLine = msg
			out.RecentErrorCount++
		}
	}
	return out
}

func desktopRuntimeStatusFromLogs(recentLogs []string, logPath string, logErr error) DesktopRuntimeStatus {
	summary := summarizeBackgroundServiceLogs(recentLogs)
	info := DesktopRuntimeStatus{
		Status:           "待采样",
		Detail:           "当前桌面会话还没有写出新的诊断日志。",
		LogPath:          logPath,
		RecentLogs:       recentLogs,
		LastLogAt:        summary.LastLogAt,
		LastLogLine:      summary.LastLogLine,
		LastLogTone:      summary.LastLogTone,
		LastErrorAt:      summary.LastErrorAt,
		LastErrorLine:    summary.LastErrorLine,
		RecentErrorCount: summary.RecentErrorCount,
	}
	if logErr != nil {
		info.Status = "不可用"
		info.Detail = fmt.Sprintf("读取桌面日志失败: %v", logErr)
		return info
	}
	if len(recentLogs) == 0 {
		return info
	}
	switch {
	case summary.LastErrorLine != "":
		info.Status = "最近有错误"
		info.Detail = "当前桌面会话最近日志里检测到了异常，请先看下方错误摘要。"
	case summary.LastLogTone == "warning":
		info.Status = "最近有提示"
		info.Detail = "当前桌面会话最近有提示类事件，通常不影响使用，但值得留意。"
	default:
		info.Status = "活动中"
		info.Detail = "当前桌面会话最近日志正常。"
	}
	return info
}

func readRecentLogsFrom(pathFn func() (string, error), maxLines int) ([]string, string, error) {
	path, err := pathFn()
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, path, nil
		}
		return nil, path, err
	}
	return trimRecentLogLines(data, maxLines), path, nil
}

func readRecentBackgroundServiceLogs(maxLines int) ([]string, string, error) {
	return readRecentLogsFrom(backgroundServiceLogPathFn, maxLines)
}

func readRecentDesktopRuntimeLogs(maxLines int) ([]string, string, error) {
	return readRecentLogsFrom(desktopRuntimeLogPathFn, maxLines)
}

func rotateLogFile(path, archiveName string, maxBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if fi.Size() < maxBytes {
		return nil
	}
	archivePath := filepath.Join(filepath.Dir(path), archiveName)
	_ = os.Remove(archivePath)
	if err := os.Rename(path, archivePath); err != nil {
		return err
	}
	return nil
}

func setupRuntimeLogging(pathFn func() (string, error), archiveName string) (string, func(), error) {
	path, err := pathFn()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return path, nil, err
	}
	if err := rotateLogFile(path, archiveName, backgroundServiceLogMaxBytes); err != nil {
		return path, nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return path, nil, err
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	return path, func() { _ = file.Close() }, nil
}

func setupBackgroundServiceLogging() (string, func(), error) {
	return setupRuntimeLogging(backgroundServiceLogPathFn, backgroundServiceLogArchive)
}

func setupDesktopRuntimeLogging() (string, func(), error) {
	return setupRuntimeLogging(desktopRuntimeLogPathFn, desktopRuntimeLogArchive)
}

// headlessProgram 无 WebView / 无托盘，仅跑 initBackend 与可选 MITM（供系统服务 / systemd 等）。
type headlessProgram struct {
	app    *App
	cancel context.CancelFunc
}

func (p *headlessProgram) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.app = NewApp()
	p.app.ctx = ctx
	go func() {
		logPath, closeLog, logErr := setupBackgroundServiceLogging()
		if closeLog != nil {
			defer closeLog()
		}
		if logErr != nil {
			log.Printf("[WindsurfTools] service log setup: %v", logErr)
		} else {
			log.Printf("[WindsurfTools] service start: %s", logPath)
		}
		if err := p.app.initBackend(); err != nil {
			log.Printf("[WindsurfTools] service init: %v", err)
			cancel()
			return
		}
		log.Printf("[WindsurfTools] backend initialized")
		if p.app.store.GetSettings().MitmProxyEnabled {
			log.Printf("[WindsurfTools] MITM autostart enabled")
			if err := p.app.StartMitmProxy(); err != nil {
				log.Printf("[WindsurfTools] MITM start: %v", err)
			}
		} else {
			log.Printf("[WindsurfTools] MITM autostart disabled")
		}
		<-ctx.Done()
		log.Printf("[WindsurfTools] service context canceled")
	}()
	return nil
}

func (p *headlessProgram) Stop(s service.Service) error {
	log.Printf("[WindsurfTools] service stop requested")
	if p.cancel != nil {
		p.cancel()
	}
	if p.app != nil {
		p.app.shutdown(context.Background())
	}
	return nil
}

func runHeadlessDaemon() error {
	s, err := newManagedService()
	if err != nil {
		return err
	}
	return s.Run()
}

func runServiceControl(action string) error {
	s, err := newManagedService()
	if err != nil {
		return err
	}
	return controlManagedService(s, action)
}

func newManagedService() (service.Service, error) {
	prg := &headlessProgram{}
	cfg := &service.Config{
		Name:        managedServiceName,
		DisplayName: "Windsurf Tools",
		Description: "Windsurf 号池、自动刷新与 MITM（无界面；配置见用户配置目录下 WindsurfTools/settings.json）",
	}
	return service.New(prg, cfg)
}

func normalizeServiceAction(action string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "install", "uninstall", "start", "stop", "restart":
		return strings.TrimSpace(strings.ToLower(action)), nil
	default:
		return "", fmt.Errorf("不支持的服务动作: %s", action)
	}
}

func controlManagedService(s serviceHandle, action string) error {
	normalized, err := normalizeServiceAction(action)
	if err != nil {
		return err
	}
	switch normalized {
	case "install":
		if err := s.Install(); err != nil {
			return fmt.Errorf("安装服务失败: %w", err)
		}
	case "uninstall":
		if err := s.Uninstall(); err != nil {
			return fmt.Errorf("卸载服务失败: %w", err)
		}
	case "start":
		if err := s.Start(); err != nil {
			return fmt.Errorf("启动服务失败: %w", err)
		}
	case "stop":
		if err := s.Stop(); err != nil {
			return fmt.Errorf("停止服务失败: %w", err)
		}
	case "restart":
		if err := s.Restart(); err != nil {
			return fmt.Errorf("重启服务失败: %w", err)
		}
	}
	return nil
}

func backgroundServiceStatusFrom(s serviceHandle, status service.Status, statusErr error, autostartMitm bool) BackgroundServiceStatus {
	info := BackgroundServiceStatus{
		Name:          managedServiceName,
		Supported:     s != nil,
		AutostartMitm: autostartMitm,
		Status:        "未知",
	}
	if s != nil {
		info.Name = s.String()
		info.Platform = s.Platform()
	}
	if errors.Is(statusErr, service.ErrNotInstalled) {
		info.Status = "未安装"
		info.Detail = "系统服务尚未安装"
		return info
	}
	if statusErr != nil {
		info.Detail = statusErr.Error()
		return info
	}
	info.Installed = true
	switch status {
	case service.StatusRunning:
		info.Running = true
		info.Status = "运行中"
		info.Detail = "服务正在后台运行"
	case service.StatusStopped:
		info.Status = "已停止"
		info.Detail = "服务已安装，但当前未运行"
	default:
		info.Status = "未知"
		info.Detail = "无法确定当前服务状态"
	}
	return info
}

func (a *App) GetBackgroundServiceStatus() (BackgroundServiceStatus, error) {
	autostartMitm := false
	recentLogs, logPath, logErr := readRecentBackgroundServiceLogs(backgroundServiceTailLines)
	logSummary := summarizeBackgroundServiceLogs(recentLogs)
	if a.store != nil {
		autostartMitm = a.store.GetSettings().MitmProxyEnabled
	}
	s, err := managedServiceFactory()
	if err != nil {
		return BackgroundServiceStatus{
			Name:             managedServiceName,
			Supported:        false,
			Status:           "不可用",
			Detail:           err.Error(),
			AutostartMitm:    autostartMitm,
			LogPath:          logPath,
			RecentLogs:       recentLogs,
			LastLogAt:        logSummary.LastLogAt,
			LastLogLine:      logSummary.LastLogLine,
			LastLogTone:      logSummary.LastLogTone,
			LastErrorAt:      logSummary.LastErrorAt,
			LastErrorLine:    logSummary.LastErrorLine,
			RecentErrorCount: logSummary.RecentErrorCount,
		}, nil
	}
	st, statusErr := s.Status()
	info := backgroundServiceStatusFrom(s, st, statusErr, autostartMitm)
	info.LogPath = logPath
	info.RecentLogs = recentLogs
	info.LastLogAt = logSummary.LastLogAt
	info.LastLogLine = logSummary.LastLogLine
	info.LastLogTone = logSummary.LastLogTone
	info.LastErrorAt = logSummary.LastErrorAt
	info.LastErrorLine = logSummary.LastErrorLine
	info.RecentErrorCount = logSummary.RecentErrorCount
	if logErr != nil && info.Detail == "" {
		info.Detail = fmt.Sprintf("读取日志失败: %v", logErr)
	}
	return info, nil
}

func (a *App) GetDesktopRuntimeStatus() (DesktopRuntimeStatus, error) {
	recentLogs, logPath, logErr := readRecentDesktopRuntimeLogs(backgroundServiceTailLines)
	return desktopRuntimeStatusFromLogs(recentLogs, logPath, logErr), nil
}

func (a *App) ControlBackgroundService(action string) error {
	s, err := managedServiceFactory()
	if err != nil {
		return err
	}
	return controlManagedService(s, action)
}
