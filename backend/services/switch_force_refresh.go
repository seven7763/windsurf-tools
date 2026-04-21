package services

import (
	"fmt"
	"strings"
)

// SwitchAccountForceRefresh writes auth files and aggressively refreshes the
// local Windsurf session cache so an already-open client can pick up the new
// account immediately.
func (s *SwitchService) SwitchAccountForceRefresh(token, email string) error {
	cands := s.windsurfAuthPathCandidates()
	if len(cands) == 0 {
		return fmt.Errorf("获取auth路径失败: 无法解析用户主目录")
	}

	var writeErrs []string
	for _, authPath := range cands {
		if err := WriteAuthFile(authPath, token, email); err != nil {
			writeErrs = append(writeErrs, fmt.Sprintf("%s: %v", authPath, err))
		}
	}
	if len(writeErrs) > 0 {
		return fmt.Errorf("写入auth文件失败: %s", strings.Join(writeErrs, "; "))
	}

	if err := s.clearWindsurfSessionCache(email); err != nil {
		return fmt.Errorf("清理 Windsurf 本地会话缓存失败: %w", err)
	}
	s.TryOpenWindsurfRefreshURIs()
	return nil
}
