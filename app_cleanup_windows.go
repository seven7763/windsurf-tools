//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

// getWindsurfProcesses 获取 Windsurf 相关进程的内存占用
func getWindsurfProcesses() []map[string]interface{} {
	// tasklist /FI "IMAGENAME eq Windsurf.exe" /FO CSV /NH
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil
	}

	var results []map[string]interface{}
	var totalMem int64

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "windsurf") {
			continue
		}
		// CSV: "Image Name","PID","Session Name","Session#","Mem Usage"
		fields := parseCSVLine(line)
		if len(fields) < 5 {
			continue
		}
		name := fields[0]
		pid := fields[1]
		memStr := strings.ReplaceAll(fields[4], ",", "")
		memStr = strings.ReplaceAll(memStr, ".", "")
		memStr = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(memStr), "K"))
		memKB, _ := strconv.ParseInt(memStr, 10, 64)

		totalMem += memKB * 1024
		results = append(results, map[string]interface{}{
			"name":       name,
			"pid":        pid,
			"memory_kb":  memKB,
			"memory_mb":  float64(memKB) / 1024,
			"memory_str": humanSize(memKB * 1024),
		})
	}

	if len(results) > 0 {
		// 添加汇总
		results = append(results, map[string]interface{}{
			"name":       "== 合计 ==",
			"pid":        "",
			"memory_kb":  totalMem / 1024,
			"memory_mb":  float64(totalMem) / 1024 / 1024,
			"memory_str": humanSize(totalMem),
		})
	}
	return results
}

func parseCSVLine(line string) []string {
	var fields []string
	inQuote := false
	var current strings.Builder
	for _, ch := range line {
		switch {
		case ch == '"':
			inQuote = !inQuote
		case ch == ',' && !inQuote:
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}
	fields = append(fields, current.String())
	return fields
}
