package models

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID                    string `json:"id"`
	Email                 string `json:"email"`
	Password              string `json:"password,omitempty"`
	Nickname              string `json:"nickname"`
	Token                 string `json:"token,omitempty"`
	RefreshToken          string `json:"refresh_token,omitempty"`
	WindsurfAPIKey        string `json:"windsurf_api_key,omitempty"`
	PlanName              string `json:"plan_name"`
	UsedQuota             int    `json:"used_quota"`
	TotalQuota            int    `json:"total_quota"`
	DailyRemaining        string `json:"daily_remaining"`
	WeeklyRemaining       string `json:"weekly_remaining"`
	DailyResetAt          string `json:"daily_reset_at"`
	WeeklyResetAt         string `json:"weekly_reset_at"`
	SubscriptionExpiresAt string `json:"subscription_expires_at"`
	TokenExpiresAt        string `json:"token_expires_at"`
	Status                string `json:"status"`
	Tags                  string `json:"tags"`
	Remark                string `json:"remark"`
	LastLoginAt           string `json:"last_login_at"`
	LastQuotaUpdate       string `json:"last_quota_update"`
	CreatedAt             string `json:"created_at"`
}

func NewAccount(email string) Account {
	return Account{
		ID:        uuid.NewString(),
		Email:     email,
		PlanName:  "unknown",
		Status:    "active",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}
