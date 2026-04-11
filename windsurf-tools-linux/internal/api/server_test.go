package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"windsurf-tools-linux/internal/models"
	"windsurf-tools-linux/internal/store"
)

func TestHandleStateReturnsSummary(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/state status = %d, want %d", rec.Code, http.StatusOK)
	}

	var state StateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Summary.TotalAccounts != 1 {
		t.Fatalf("total_accounts = %d, want 1", state.Summary.TotalAccounts)
	}
	if len(state.Accounts) != 1 {
		t.Fatalf("accounts len = %d, want 1", len(state.Accounts))
	}
}

func TestHandleAccountsCreatesRecord(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	payload := models.Account{Email: "new@example.com", PlanName: "trial", DailyRemaining: "10%"}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/accounts", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/accounts status = %d, want %d", rec.Code, http.StatusOK)
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	stateRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(stateRec, stateReq)

	var state StateResponse
	if err := json.Unmarshal(stateRec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Summary.TotalAccounts != 2 {
		t.Fatalf("total_accounts = %d, want 2", state.Summary.TotalAccounts)
	}
}

func TestHandleAccountDeleteRemovesRecord(t *testing.T) {
	t.Parallel()

	testStore := newTestStore(t)
	account, err := testStore.SaveAccount(models.Account{Email: "delete@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(testStore, false)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/accounts/"+account.ID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/accounts/:id status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if _, err := testStore.GetAccount(account.ID); err == nil {
		t.Fatal("expected account to be removed")
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	testStore := newTestStore(t)
	srv, err := New(testStore, false)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewInDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAccount(models.Account{
		Email:                 "alpha@example.com",
		PlanName:              "pro",
		DailyRemaining:        "62%",
		SubscriptionExpiresAt: "2099-01-01T00:00:00Z",
		WindsurfAPIKey:        "ws-live-demo",
	}); err != nil {
		t.Fatal(err)
	}
	return s
}
