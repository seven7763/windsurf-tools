package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// trafficLogger 记录 MITM 代理经过的所有 HTTP 请求/响应到文件
var (
	trafficLogMu   sync.Mutex
	trafficLogFile *os.File
	trafficLogPath string
	trafficSeq     int
)

// TrafficLogPath 返回流量日志文件路径
func TrafficLogPath() string {
	trafficLogMu.Lock()
	defer trafficLogMu.Unlock()
	return trafficLogPath
}

func initTrafficLog() {
	trafficLogMu.Lock()
	defer trafficLogMu.Unlock()
	if trafficLogFile != nil {
		return
	}
	dir, _ := os.UserConfigDir()
	trafficLogPath = filepath.Join(dir, "WindsurfTools", "traffic.log")
	os.MkdirAll(filepath.Dir(trafficLogPath), 0755)
	f, err := os.OpenFile(trafficLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	trafficLogFile = f
	fmt.Fprintf(f, "=== Traffic Log Started %s ===\n", time.Now().Format(time.RFC3339))
}

func trafficLog(format string, args ...interface{}) {
	initTrafficLog()
	trafficLogMu.Lock()
	defer trafficLogMu.Unlock()
	if trafficLogFile == nil {
		return
	}
	trafficSeq++
	line := fmt.Sprintf("#%04d [%s] %s\n", trafficSeq, time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	trafficLogFile.WriteString(line)
	trafficLogFile.Sync()
}

func sanitizePathForFile(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, "?", "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// TrafficDumpBody dump 响应体到文件，返回文件路径
func TrafficDumpBody(seq int, suffix string, data []byte) string {
	dir, _ := os.UserConfigDir()
	dumpDir := filepath.Join(dir, "WindsurfTools", "traffic_dumps")
	os.MkdirAll(dumpDir, 0755)
	fname := fmt.Sprintf("%04d_%s.bin", seq, suffix)
	fpath := filepath.Join(dumpDir, fname)
	os.WriteFile(fpath, data, 0644)
	return fpath
}
