package services

import (
	"bytes"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ══════════════════════════════════════════════════════════════
// 系统信息探测 —— 用真实运行时值代替硬编码假数据。
// macOS M-series (M1/M2/M3/M4/M5 Pro/Max/Ultra) 必须用 sysctl
// 读出真实的 CPU brand、核心数、内存，否则服务端容易识别为可疑客户端。
// ══════════════════════════════════════════════════════════════

type systemOSInfo struct {
	ProductName string // e.g. "macOS", "Windows 10 Pro"
	Version     string // e.g. "26.0", "10.0"
	Build       string // e.g. "25A286", "26200"
	MajorVer    uint64
	MinorVer    uint64
}

type systemCPUInfo struct {
	NumSockets int
	NumCores   int
	NumThreads int
	VendorID   string
	Family     string
	ModelName  string
	Memory     uint64
}

var (
	osInfoOnce  sync.Once
	cpuInfoOnce sync.Once
	cachedOS    systemOSInfo
	cachedCPU   systemCPUInfo
)

func getSystemOSInfo() systemOSInfo {
	osInfoOnce.Do(func() {
		cachedOS = probeSystemOSInfo()
	})
	return cachedOS
}

func getSystemCPUInfo() systemCPUInfo {
	cpuInfoOnce.Do(func() {
		cachedCPU = probeSystemCPUInfo()
	})
	return cachedCPU
}

func probeSystemOSInfo() systemOSInfo {
	switch runtime.GOOS {
	case "darwin":
		info := systemOSInfo{ProductName: "macOS", Version: "26.0", Build: "25A286", MajorVer: 26, MinorVer: 0}
		if v := sysctlOrSwVers("-productVersion"); v != "" {
			info.Version = v
			maj, min := splitVersion(v)
			info.MajorVer = maj
			info.MinorVer = min
		}
		if b := sysctlOrSwVers("-buildVersion"); b != "" {
			info.Build = b
		}
		return info
	case "windows":
		// 读真实 Windows 版本需要调用 GetVersionEx / RtlGetVersion，复杂度高。
		// 保留原有 Windows 10/11 合理默认（服务端对 windows 没那么严格）。
		return systemOSInfo{
			ProductName: "Windows 10 Pro",
			Version:     "10.0",
			Build:       "26200",
			MajorVer:    10,
			MinorVer:    0,
		}
	default:
		return systemOSInfo{
			ProductName: "Linux",
			Version:     "6.5",
			Build:       "0",
			MajorVer:    6,
			MinorVer:    5,
		}
	}
}

func probeSystemCPUInfo() systemCPUInfo {
	threads := runtime.NumCPU()
	if threads <= 0 {
		threads = 8
	}
	// cores 在 macOS 上区分 performance+efficiency，总物理核心由 sysctl hw.physicalcpu 给出。
	info := systemCPUInfo{
		NumSockets: 1,
		NumCores:   threads,
		NumThreads: threads,
	}

	switch runtime.GOOS {
	case "darwin":
		if b := sysctl("machdep.cpu.brand_string"); b != "" {
			info.ModelName = b
			info.VendorID = "Apple"
			info.Family = "0"
		}
		if n, ok := sysctlInt("hw.physicalcpu"); ok && n > 0 {
			info.NumCores = n
		}
		if n, ok := sysctlInt("hw.logicalcpu"); ok && n > 0 {
			info.NumThreads = n
		}
		if mem, ok := sysctlUint("hw.memsize"); ok && mem > 0 {
			info.Memory = mem
		}
		if info.ModelName == "" {
			info.ModelName = "Apple Silicon"
			info.VendorID = "Apple"
			info.Family = "0"
		}
	case "windows":
		// 保留现有合理默认；真正读 Windows CPU 信息要调 IsProcessorFeaturePresent 等。
		info.NumSockets = 1
		info.NumCores = threads
		info.NumThreads = threads
		if runtime.GOARCH == "arm64" {
			info.VendorID = "ARM"
			info.Family = "0"
			info.ModelName = "ARM64 CPU"
		} else {
			info.VendorID = "AuthenticAMD"
			info.Family = "107"
			info.ModelName = "AMD Ryzen 7 7745HX with Radeon Graphics"
		}
		if info.Memory == 0 {
			info.Memory = 33475293184 // 32GB 合理默认
		}
	default:
		info.VendorID = "GenuineIntel"
		info.Family = "6"
		info.ModelName = "Linux CPU"
		info.Memory = 33475293184
	}
	return info
}

// sysctl 调用 `sysctl -n <key>` 并返回 trimmed 输出；失败或非 darwin 返回空串。
func sysctl(key string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("sysctl", "-n", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sysctlInt(key string) (int, bool) {
	s := sysctl(key)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func sysctlUint(key string) (uint64, bool) {
	s := sysctl(key)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// sysctlOrSwVers 优先用 sw_vers（macOS 专用），sysctl 作兜底。
func sysctlOrSwVers(flag string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("sw_vers", flag).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(bytes.TrimSuffix(out, []byte{'\n'})))
}

func splitVersion(v string) (uint64, uint64) {
	parts := strings.SplitN(v, ".", 3)
	var maj, min uint64
	if len(parts) > 0 {
		maj, _ = strconv.ParseUint(parts[0], 10, 64)
	}
	if len(parts) > 1 {
		min, _ = strconv.ParseUint(parts[1], 10, 64)
	}
	return maj, min
}
