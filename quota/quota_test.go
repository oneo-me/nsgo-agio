package quota

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0700)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// server answers each path with a status and body and records the headers
// of the last request.
func server(t *testing.T, routes map[string]func(w http.ResponseWriter)) (string, *http.Header) {
	t.Helper()
	var last http.Header
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := routes[r.URL.Path]
		if route == nil {
			http.NotFound(w, r)
			return
		}
		last = r.Header.Clone()
		route(w)
	}))
	t.Cleanup(s.Close)
	return s.URL, &last
}

func reply(status int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}
}

func source(t *testing.T, p Provider, base string) Source {
	return Source{Home: t.TempDir(), Now: func() time.Time { return now }, BaseURLs: map[Provider]string{p: base}}
}

// claudeBody is trimmed from a live response captured on 2026-08-07.
const claudeBody = `{
	"five_hour": {"utilization": 41.0, "resets_at": "2026-08-07T14:59:59.729061+00:00"},
	"limits": [
		{"kind": "session", "percent": 41, "resets_at": "2026-08-07T14:59:59.729061+00:00", "scope": null},
		{"kind": "weekly_all", "percent": 20, "resets_at": "2026-08-13T11:59:59.729091+00:00", "scope": null},
		{"kind": "weekly_scoped", "percent": 38, "resets_at": "2026-08-13T11:59:59.729307+00:00",
		 "scope": {"model": {"id": null, "display_name": "Fable"}, "surface": null}},
		{"kind": "overage", "percent": 3}
	]
}`

func TestClaudeReadsKeychainLimitsAndProfilePlan(t *testing.T) {
	base, headers := server(t, map[string]func(http.ResponseWriter){
		"/api/oauth/usage":   reply(200, claudeBody),
		"/api/oauth/profile": reply(200, `{"organization":{"organization_type":"claude_max","rate_limit_tier":"default_claude_max_20x"}}`),
	})
	s := source(t, Claude, base)
	s.Keychain = func(service string) (string, error) {
		if service != "Claude Code-credentials" {
			return "", errors.New("missing")
		}
		return `{"claudeAiOauth":{"accessToken":"key-token","expiresAt":` + jsonMillis(now.Add(time.Hour)) +
			`,"subscriptionType":"max","rateLimitTier":"default_claude_max_5x"}}`, nil
	}
	u, err := s.Fetch(context.Background(), Claude)
	if err != nil {
		t.Fatal(err)
	}
	if got := headers.Get("Authorization"); got != "Bearer key-token" {
		t.Fatalf("authorization %q", got)
	}
	if headers.Get("anthropic-beta") != oauthBeta {
		t.Fatal("missing beta header")
	}
	if u.Plan != "Max (20x)" {
		t.Fatalf("plan %q, want the profile's", u.Plan)
	}
	want := []Window{
		{Minutes: fiveHours, Percent: 41},
		{Minutes: week, Percent: 20},
		{Minutes: week, Scope: "Fable", Percent: 38},
	}
	if len(u.Windows) != len(want) {
		t.Fatalf("windows %+v", u.Windows)
	}
	for i, w := range want {
		got := u.Windows[i]
		if got.Minutes != w.Minutes || got.Scope != w.Scope || got.Percent != w.Percent || got.ResetsAt.IsZero() {
			t.Fatalf("window %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestClaudeFallsBackToFlatFieldsAndPiCredential(t *testing.T) {
	base, headers := server(t, map[string]func(http.ResponseWriter){
		"/api/oauth/usage": reply(200, `{"five_hour":{"utilization":12.5,"resets_at":"2026-08-07T14:59:59+00:00"},
			"seven_day":{"utilization":3},"seven_day_opus":{"utilization":7}}`),
		"/api/oauth/profile": reply(500, ``),
	})
	s := source(t, Claude, base)
	// An expired Claude Code credential gives way to pi's.
	write(t, filepath.Join(s.Home, ".claude", ".credentials.json"),
		`{"claudeAiOauth":{"accessToken":"old","expiresAt":`+jsonMillis(now.Add(-time.Minute))+`}}`)
	write(t, filepath.Join(s.Home, ".pi", "agent", "auth.json"),
		`{"anthropic":{"type":"oauth","access":"pi-token","expires":`+jsonMillis(now.Add(time.Hour))+`}}`)
	u, err := s.Fetch(context.Background(), Claude)
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Authorization") != "Bearer pi-token" {
		t.Fatalf("authorization %q", headers.Get("Authorization"))
	}
	if len(u.Windows) != 3 || u.Windows[2].Scope != "Opus" || u.Windows[0].Percent != 12.5 {
		t.Fatalf("windows %+v", u.Windows)
	}
}

func TestClaudeReportsMissingExpiredAndRateLimitedAccounts(t *testing.T) {
	base, _ := server(t, map[string]func(http.ResponseWriter){
		"/api/oauth/usage": func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "3340")
			reply(429, `{"error":{"type":"rate_limit_error","message":"Rate limited. Please try again later."}}`)(w)
		},
	})
	s := source(t, Claude, base)
	if _, err := s.Fetch(context.Background(), Claude); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v, want no credentials", err)
	}
	path := filepath.Join(s.Home, ".claude", ".credentials.json")
	write(t, path, `{"claudeAiOauth":{"accessToken":"old","expiresAt":`+jsonMillis(now.Add(-time.Minute))+`}}`)
	if _, err := s.Fetch(context.Background(), Claude); !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want expired", err)
	}
	write(t, path, `{"claudeAiOauth":{"accessToken":"token"}}`)
	_, err := s.Fetch(context.Background(), Claude)
	var status *StatusError
	if !errors.As(err, &status) || status.Status != 429 || status.RetryAfter != 3340*time.Second ||
		status.Message != "Rate limited. Please try again later." {
		t.Fatalf("err = %#v", err)
	}
}

func TestClaudePlanNames(t *testing.T) {
	for _, c := range []struct{ subscription, tier, want string }{
		{"max", "default_claude_max_20x", "Max (20x)"},
		{"", "default_claude_max_5x", "Max (5x)"},
		{"pro", "", "Pro"},
		{"", "", ""},
	} {
		if got := claudePlan(c.subscription, c.tier); got != c.want {
			t.Errorf("claudePlan(%q, %q) = %q, want %q", c.subscription, c.tier, got, c.want)
		}
	}
}

// codexBody is the shape of a live response captured on 2026-10-11 with a
// model-scoped lane added.
const codexBody = `{
	"plan_type": "prolite",
	"rate_limit": {
		"primary_window": {"used_percent": 89, "limit_window_seconds": 604800, "reset_after_seconds": 577009, "reset_at": 1792226600},
		"secondary_window": null
	},
	"additional_rate_limits": [
		{"limit_name": "GPT-5.3-Codex-Spark", "rate_limit": {"primary_window": {"used_percent": 0, "limit_window_seconds": 604800}}},
		{"limit_name": "GPT-5.3-Codex", "rate_limit": {"primary_window": {"used_percent": 12, "limit_window_seconds": 18000}}}
	]
}`

func TestCodexReadsChatGPTWindows(t *testing.T) {
	base, headers := server(t, map[string]func(http.ResponseWriter){"/backend-api/wham/usage": reply(200, codexBody)})
	s := source(t, Codex, base)
	s.Env = []string{"CODEX_HOME=~/custom-codex"}
	write(t, filepath.Join(s.Home, "custom-codex", "auth.json"),
		`{"tokens":{"access_token":"`+jwt(map[string]any{"exp": now.Add(time.Hour).Unix()})+`","account_id":"acct"}}`)
	u, err := s.Fetch(context.Background(), Codex)
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("ChatGPT-Account-Id") != "acct" {
		t.Fatalf("account header %q", headers.Get("ChatGPT-Account-Id"))
	}
	if u.Plan != "Pro (5x)" || len(u.Windows) != 2 {
		t.Fatalf("usage %+v", u)
	}
	if w := u.Windows[0]; w.Minutes != week || w.Percent != 89 || w.ResetsAt.Unix() != 1792226600 {
		t.Fatalf("account window %+v", w)
	}
	if w := u.Windows[1]; w.Minutes != fiveHours || w.Scope != "GPT-5.3-Codex" || w.Percent != 12 {
		t.Fatalf("scoped window %+v", w)
	}
}

func TestPiCodexReadsOnlyPiTokenWithAccountClaim(t *testing.T) {
	base, headers := server(t, map[string]func(http.ResponseWriter){"/backend-api/wham/usage": reply(200, codexBody)})
	s := source(t, PiCodex, base)
	s.BaseURLs[Codex] = base
	// Codex's own login is a different account.
	write(t, filepath.Join(s.Home, ".codex", "auth.json"), `{"tokens":{"access_token":"codex-token","account_id":"codex"}}`)
	if _, err := s.Fetch(context.Background(), PiCodex); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v, want no credentials", err)
	}
	access := jwt(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "from-claim"}})
	write(t, filepath.Join(s.Home, ".pi", "agent", "auth.json"),
		`{"openai-codex":{"type":"oauth","access":"`+access+`","expires":`+jsonMillis(now.Add(time.Hour))+`}}`)
	if _, err := s.Fetch(context.Background(), PiCodex); err != nil {
		t.Fatal(err)
	}
	if headers.Get("ChatGPT-Account-Id") != "from-claim" {
		t.Fatalf("account header %q", headers.Get("ChatGPT-Account-Id"))
	}
	if _, err := s.Fetch(context.Background(), Codex); err != nil || headers.Get("ChatGPT-Account-Id") != "codex" {
		t.Fatalf("err = %v, Codex account header %q", err, headers.Get("ChatGPT-Account-Id"))
	}
}

func TestDeepSeekReadsBalanceWithPiOrEnvironmentKey(t *testing.T) {
	base, headers := server(t, map[string]func(http.ResponseWriter){
		"/user/balance": reply(200, `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"86.6","granted_balance":"0.00","topped_up_balance":"86.6"}]}`),
	})
	s := source(t, DeepSeek, base)
	if _, err := s.Fetch(context.Background(), DeepSeek); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v, want no credentials", err)
	}
	s.Env = []string{"DEEPSEEK_API_KEY=env-key"}
	u, err := s.Fetch(context.Background(), DeepSeek)
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Authorization") != "Bearer env-key" {
		t.Fatalf("authorization %q", headers.Get("Authorization"))
	}
	if len(u.Balances) != 1 || u.Balances[0] != (Balance{Currency: "CNY", Amount: "86.60"}) {
		t.Fatalf("balances %+v", u.Balances)
	}
	// A stored key wins; one naming a variable reads it.
	write(t, filepath.Join(s.Home, ".pi", "agent", "auth.json"), `{"deepseek":{"type":"api_key","key":"MY_DEEPSEEK"}}`)
	s.Env = append(s.Env, "MY_DEEPSEEK=named-key")
	if _, err := s.Fetch(context.Background(), DeepSeek); err != nil || headers.Get("Authorization") != "Bearer named-key" {
		t.Fatalf("err = %v, authorization %q", err, headers.Get("Authorization"))
	}
	// Command keys are never run.
	write(t, filepath.Join(s.Home, ".pi", "agent", "auth.json"), `{"deepseek":{"type":"api_key","key":"!echo secret"}}`)
	if _, err := s.Fetch(context.Background(), DeepSeek); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v, want no credentials", err)
	}
}

// xaiBody is trimmed from a live response captured on 2026-10-13.
const xaiBody = `{"config":{"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","start":"2026-08-06T11:39:26.268+00:00",
	"end":"2026-08-13T11:39:26.268+00:00"},"creditUsagePercent":37.5,"productUsage":[{"product":"GrokBuild","usagePercent":37.5}],
	"isUnifiedBillingUser":true,"prepaidBalance":{"val":0}}}`

func TestPiXAIReadsOnlyPiCredential(t *testing.T) {
	base, headers := server(t, map[string]func(http.ResponseWriter){
		"/v1/billing":  reply(200, xaiBody),
		"/v1/settings": reply(200, `{"subscription_tier_display":"SuperGrok"}`),
	})
	s := source(t, PiXAI, base)
	if _, err := s.Fetch(context.Background(), PiXAI); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v, want no credentials", err)
	}
	write(t, filepath.Join(s.Home, ".pi", "agent", "auth.json"),
		`{"xai":{"type":"oauth","access":"pi-xai-token","expires":`+jsonMillis(now.Add(time.Hour))+`}}`)
	u, err := s.Fetch(context.Background(), PiXAI)
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Authorization") != "Bearer pi-xai-token" {
		t.Fatalf("authorization %q", headers.Get("Authorization"))
	}
	if u.Plan != "SuperGrok" || len(u.Windows) != 1 || u.Windows[0].Percent != 37.5 {
		t.Fatalf("usage %+v", u)
	}
}

func TestErrorMessageFallback(t *testing.T) {
	body := `{"code":"unauthenticated","message":"Not logged in"}`
	if got := errorMessage([]byte(body)); got != "Not logged in" {
		t.Fatalf("message = %q", got)
	}
}

func jsonMillis(t time.Time) string {
	data, _ := json.Marshal(t.UnixMilli())
	return string(data)
}

func jwt(claims map[string]any) string {
	payload, _ := json.Marshal(claims)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func TestUsagePages(t *testing.T) {
	for _, p := range Providers {
		if !strings.HasPrefix(p.UsagePage(), "https://") {
			t.Errorf("%s usage page = %q", p, p.UsagePage())
		}
	}
}
