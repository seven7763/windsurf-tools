package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DebugLogger 调试日志落盘：开启后将关键操作写入 debug.log
type DebugLogger struct {
	mu      sync.Mutex
	enabled bool
	file    *os.File
	logger  *log.Logger
	dir     string
}

var (
	globalDebugLogger *DebugLogger
	debugLogOnce      sync.Once
)

// InitDebugLogger 初始化全局调试日志（在 App 初始化时调用，可多次调用切换开关）
func InitDebugLogger(dataDir string, enabled bool) {
	debugLogOnce.Do(func() {
		globalDebugLogger = &DebugLogger{dir: dataDir}
	})
	if globalDebugLogger.dir == "" {
		globalDebugLogger.dir = dataDir
	}
	globalDebugLogger.SetEnabled(enabled)
}

// DLog 写一条调试日志（同时写 stdout 和文件）
func DLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	if globalDebugLogger != nil {
		globalDebugLogger.write(msg)
	}
}

func (d *DebugLogger) SetEnabled(on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if on == d.enabled {
		return
	}
	d.enabled = on
	if on {
		d.openFile()
	} else {
		d.closeFile()
	}
}

func (d *DebugLogger) openFile() {
	logPath := filepath.Join(d.dir, "debug.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[debuglog] 无法打开日志文件 %s: %v", logPath, err)
		return
	}
	d.file = f
	d.logger = log.New(f, "", 0)
	d.logger.Printf("=== debug log started at %s ===", time.Now().Format(time.RFC3339))
}

func (d *DebugLogger) closeFile() {
	if d.file != nil {
		_ = d.file.Close()
		d.file = nil
		d.logger = nil
	}
}

func (d *DebugLogger) write(msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.enabled || d.logger == nil {
		return
	}
	d.logger.Printf("%s %s", time.Now().Format("15:04:05.000"), msg)
}
