package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kardianos/service"
)

type fakeServiceHandle struct {
	name       string
	platform   string
	status     service.Status
	statusErr  error
	startErr   error
	stopErr    error
	restartErr error
	installErr error
	removeErr  error
	calls      []string
}

func (f *fakeServiceHandle) Start() error {
	f.calls = append(f.calls, "start")
	return f.startErr
}

func (f *fakeServiceHandle) Stop() error {
	f.calls = append(f.calls, "stop")
	return f.stopErr
}

func (f *fakeServiceHandle) Restart() error {
	f.calls = append(f.calls, "restart")
	return f.restartErr
}

func (f *fakeServiceHandle) Install() error {
	f.calls = append(f.calls, "install")
	return f.installErr
}

func (f *fakeServiceHandle) Uninstall() error {
	f.calls = append(f.calls, "uninstall")
	return f.removeErr
}

func (f *fakeServiceHandle) Platform() string { return f.platform }
func (f *fakeServiceHandle) String() string   { return f.name }
func (f *fakeServiceHandle) Status() (service.Status, error) {
	return f.status, f.statusErr
}

func TestNormalizeServiceAction(t *testing.T) {
	got, err := normalizeServiceAction(" Restart ")
	if err != nil {
		t.Fatalf("normalizeServiceAction() error = %v", err)
	}
	if got != "restart" {
		t.Fatalf("normalizeServiceAction() = %q, want %q", got, "restart")
	}
	if _, err := normalizeServiceAction("noop"); err == nil {
		t.Fatal("normalizeServiceAction() should reject unsupported action")
	}
}

func TestBackgroundServiceStatusFromNotInstalled(t *testing.T) {
	fake := &fakeServiceHandle{name: "Windsurf Tools", platform: "windows-service"}
	got := backgroundServiceStatusFrom(fake, service.StatusUnknown, service.ErrNotInstalled, true)
	if got.Installed {
		t.Fatal("Installed should be false for ErrNotInstalled")
	}
	if got.Status != "未安装" {
		t.Fatalf("Status = %q, want %q", got.Status, "未安装")
	}
	if !got.AutostartMitm {
		t.Fatal("AutostartMitm should be true")
	}
}

func TestBackgroundServiceStatusFromRunning(t *testing.T) {
	fake := &fakeServiceHandle{name: "Windsurf Tools", platform: "windows-service"}
	got := backgroundServiceStatusFrom(fake, service.StatusRunning, nil, false)
	if !got.Installed || !got.Running {
		t.Fatal("running service should be installed and running")
	}
	if got.Status != "运行中" {
		t.Fatalf("Status = %q, want %q", got.Status, "运行中")
	}
}

func TestControlManagedServiceWrapsErrors(t *testing.T) {
	fake := &fakeServiceHandle{startErr: errors.New("boom")}
	err := controlManagedService(fake, "start")
	if err == nil {
		t.Fatal("controlManagedService() expected error")
	}
	if len(fake.calls) != 1 || fake.calls[0] != "start" {
		t.Fatalf("calls = %#v, want start", fake.calls)
	}
}

func TestTrimRecentLogLinesKeepsNewestNonEmptyLines(t *testing.T) {
	raw := []byte("\r\nfirst\n\nsecond\nthird\n")
	got := trimRecentLogLines(raw, 2)
	want := []string{"second", "third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trimRecentLogLines() = %#v, want %#v", got, want)
	}
}

func TestReadRecentBackgroundServiceLogsReturnsTail(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "background-service.log")
	if err := os.WriteFile(logPath, []byte("alpha\nbeta\n\ngamma\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	original := backgroundServiceLogPathFn
	backgroundServiceLogPathFn = func() (string, error) { return logPath, nil }
	t.Cleanup(func() {
		backgroundServiceLogPathFn = original
	})

	got, gotPath, err := readRecentBackgroundServiceLogs(2)
	if err != nil {
		t.Fatalf("readRecentBackgroundServiceLogs() error = %v", err)
	}
	if gotPath != logPath {
		t.Fatalf("readRecentBackgroundServiceLogs() path = %q, want %q", gotPath, logPath)
	}
	want := []string{"beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readRecentBackgroundServiceLogs() = %#v, want %#v", got, want)
	}
}

func TestSummarizeBackgroundServiceLogsFindsLatestError(t *testing.T) {
	lines := []string{
		"2026/03/22 08:40:00.100000 [WindsurfTools] service start: C:\\temp\\background-service.log",
		"2026/03/22 08:40:01.100000 [WindsurfTools] backend initialized",
		"2026/03/22 08:40:02.100000 [WindsurfTools] MITM start: listen tcp 127.0.0.1:443: bind: Only one usage of each socket address is normally permitted",
	}

	got := summarizeBackgroundServiceLogs(lines)
	if got.LastLogTone != "error" {
		t.Fatalf("LastLogTone = %q, want %q", got.LastLogTone, "error")
	}
	if got.LastErrorAt == "" {
		t.Fatal("LastErrorAt should not be empty")
	}
	if got.RecentErrorCount != 1 {
		t.Fatalf("RecentErrorCount = %d, want %d", got.RecentErrorCount, 1)
	}
	if got.LastErrorLine == "" || got.LastLogLine == "" {
		t.Fatal("summary should keep last error and last log lines")
	}
}

func TestSummarizeBackgroundServiceLogsWithoutError(t *testing.T) {
	lines := []string{
		"2026/03/22 08:40:00.100000 [WindsurfTools] service start: C:\\temp\\background-service.log",
		"2026/03/22 08:40:01.100000 [WindsurfTools] MITM autostart disabled",
	}

	got := summarizeBackgroundServiceLogs(lines)
	if got.LastLogTone != "warning" {
		t.Fatalf("LastLogTone = %q, want %q", got.LastLogTone, "warning")
	}
	if got.LastErrorLine != "" || got.LastErrorAt != "" {
		t.Fatalf("summary should not report errors, got %#v", got)
	}
	if got.RecentErrorCount != 0 {
		t.Fatalf("RecentErrorCount = %d, want %d", got.RecentErrorCount, 0)
	}
}

func TestDesktopRuntimeStatusFromLogsWithError(t *testing.T) {
	lines := []string{
		"2026/03/22 09:00:00.100000 [WindsurfTools] desktop session start: C:\\temp\\desktop-runtime.log",
		"2026/03/22 09:00:01.100000 [WindsurfTools] desktop init: permission denied",
	}

	got := desktopRuntimeStatusFromLogs(lines, "C:\\temp\\desktop-runtime.log", nil)
	if got.Status != "最近有错误" {
		t.Fatalf("Status = %q, want %q", got.Status, "最近有错误")
	}
	if got.LastErrorLine == "" || got.RecentErrorCount != 1 {
		t.Fatalf("desktopRuntimeStatusFromLogs() = %#v", got)
	}
}

func TestDesktopRuntimeStatusFromLogsWithoutLogs(t *testing.T) {
	got := desktopRuntimeStatusFromLogs(nil, "C:\\temp\\desktop-runtime.log", nil)
	if got.Status != "待采样" {
		t.Fatalf("Status = %q, want %q", got.Status, "待采样")
	}
	if got.LogPath == "" {
		t.Fatal("LogPath should be preserved")
	}
}
