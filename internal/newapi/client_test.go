package newapi

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeBaseURLDropsProfilePath(t *testing.T) {
	got, err := NormalizeBaseURL("http://192.168.1.20:3000/profile")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://192.168.1.20:3000" {
		t.Fatalf("unexpected normalized URL %q", got)
	}
}

func TestCentsToQuota(t *testing.T) {
	quota, err := CentsToQuota(123, 500_000)
	if err != nil {
		t.Fatal(err)
	}
	if quota != 615_000 {
		t.Fatalf("unexpected quota %d", quota)
	}
	if _, err := CentsToQuota(math.MaxInt64, 500_000); err == nil {
		t.Fatal("expected overflow to be rejected")
	}
}

func TestDeleteUserUsesAdminEndpoint(t *testing.T) {
	deletedUserID := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/user/42" || r.Header.Get("Authorization") != "Bearer admin-pat" {
			writeTestResponse(w, http.StatusBadRequest, false, "unexpected request", nil)
			return
		}
		deletedUserID <- "42"
		writeTestResponse(w, http.StatusOK, true, "", nil)
	}))
	defer upstream.Close()

	if err := NewClient().DeleteUser(context.Background(), upstream.URL, "admin-pat", 42); err != nil {
		t.Fatal(err)
	}
	if got := <-deletedUserID; got != "42" {
		t.Fatalf("New API user was not deleted, got %q", got)
	}
}

func TestProvisionUserCreatesTokenAndRecoversExistingAccount(t *testing.T) {
	var created bool
	var storedUsername, storedPassword string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/":
			if r.Header.Get("Authorization") != "Bearer admin-pat" {
				writeTestResponse(w, http.StatusUnauthorized, false, "unauthorized", nil)
				return
			}
			var input struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			if created {
				writeTestResponse(w, http.StatusOK, false, "用户已存在", nil)
				return
			}
			created = true
			storedUsername, storedPassword = input.Username, input.Password
			writeTestResponse(w, http.StatusOK, true, "", nil)
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/login":
			var input struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input.Username != storedUsername || input.Password != storedPassword {
				writeTestResponse(w, http.StatusUnauthorized, false, "bad credentials", nil)
				return
			}
			writeTestResponse(w, http.StatusOK, true, "", map[string]any{
				"access_token": "login-token",
				"user":         map[string]any{"id": 7, "username": storedUsername, "display_name": "Player", "role": 1, "status": 1},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/token":
			if r.Header.Get("Authorization") != "Bearer login-token" {
				writeTestResponse(w, http.StatusUnauthorized, false, "unauthorized", nil)
				return
			}
			writeTestResponse(w, http.StatusOK, true, "", "user-pat")
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/self":
			if r.Header.Get("Authorization") != "Bearer user-pat" {
				writeTestResponse(w, http.StatusUnauthorized, false, "unauthorized", nil)
				return
			}
			writeTestResponse(w, http.StatusOK, true, "", map[string]any{
				"id": 7, "username": storedUsername, "display_name": "Player", "role": 1, "status": 1, "quota": 500_000,
			})
		default:
			writeTestResponse(w, http.StatusNotFound, false, "not found", nil)
		}
	}))
	defer upstream.Close()

	client := NewClient()
	for attempt := 0; attempt < 2; attempt++ {
		user, token, err := client.ProvisionUser(context.Background(), upstream.URL, "admin-pat", "pn_1234567890abcdef", "strong-password", "Player")
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt+1, err)
		}
		if user.ID != 7 || user.Username != "pn_1234567890abcdef" || token != "user-pat" {
			t.Fatalf("attempt %d returned unexpected result: user=%#v token=%q", attempt+1, user, token)
		}
	}
}

func writeTestResponse(w http.ResponseWriter, status int, success bool, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": success, "message": message, "data": data})
}
