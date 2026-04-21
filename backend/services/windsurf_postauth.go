package services

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"windsurf-tools-wails/backend/utils"
)

// WindsurfPostAuth exchanges an auth1 token for a devin session token inside the
// same browser-style session Windsurf web uses after password login.
func (s *WindsurfService) WindsurfPostAuth(authToken string) (*WindsurfPostAuthResp, error) {
	authToken = strings.TrimSpace(authToken)
	if authToken == "" {
		return nil, fmt.Errorf("WindsurfPostAuth 缺少 auth token")
	}

	reqBody := utils.EncodeStringField(1, authToken)
	req, err := http.NewRequest("POST",
		"https://windsurf.com/_backend/exa.seat_management_pb.SeatManagementService/WindsurfPostAuth",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 WindsurfPostAuth 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Origin", "https://windsurf.com")
	req.Header.Set("Referer", "https://windsurf.com/account/login")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("WindsurfPostAuth 请求失败(网络): %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("WindsurfPostAuth 失败(%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	msg := decodeProtoMessage(respBody)
	out := &WindsurfPostAuthResp{Raw: respBody}
	for _, field := range msg {
		val := string(field.Bytes)
		switch {
		case strings.HasPrefix(val, "devin-session-token$"):
			out.SessionKey = val
		case strings.HasPrefix(val, "auth1_"):
			out.AuthToken = val
		case strings.HasPrefix(val, "account-"):
			out.AccountID = val
		case strings.HasPrefix(val, "org-"):
			out.OrgID = val
		}
	}
	if out.SessionKey == "" {
		return nil, fmt.Errorf("WindsurfPostAuth 未返回 session token")
	}
	return out, nil
}
