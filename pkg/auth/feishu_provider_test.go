package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zxh326/kite/pkg/model"
)

func TestIsFeishuProvider(t *testing.T) {
	tests := []struct {
		name string
		op   model.OAuthProvider
		want bool
	}{
		{
			name: "feishu issuer",
			op:   model.OAuthProvider{Issuer: "https://open.feishu.cn"},
			want: true,
		},
		{
			name: "lark issuer",
			op:   model.OAuthProvider{Issuer: "https://open.larksuite.com"},
			want: true,
		},
		{
			name: "feishu auth URL",
			op:   model.OAuthProvider{AuthURL: "https://open.feishu.cn/open-apis/authen/v1/authorize"},
			want: true,
		},
		{
			name: "lark auth URL",
			op:   model.OAuthProvider{AuthURL: "https://open.larksuite.com/open-apis/authen/v1/authorize"},
			want: true,
		},
		{
			name: "generic provider",
			op:   model.OAuthProvider{Issuer: "https://accounts.google.com", AuthURL: "https://accounts.google.com/o/oauth2/v2/auth"},
			want: false,
		},
		{
			name: "empty issuer and auth URL",
			op:   model.OAuthProvider{},
			want: false,
		},
		{
			name: "feishu in mixed case issuer",
			op:   model.OAuthProvider{Issuer: "https://open.Feishu.cn"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFeishuProvider(tt.op); got != tt.want {
				t.Fatalf("isFeishuProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewFeishuProvider(t *testing.T) {
	op := model.OAuthProvider{
		Name:          "feishu",
		ClientID:      "cli_test123",
		ClientSecret:  "testsecret",
		RedirectURL:   "http://localhost:8080/api/auth/callback",
		Issuer:        "https://open.feishu.cn",
		UsernameClaim: "email",
		AllowedGroups: "group1,group2",
	}

	provider, err := NewFeishuProvider(op)
	if err != nil {
		t.Fatalf("NewFeishuProvider() error = %v", err)
	}
	if provider.AppID != "cli_test123" {
		t.Fatalf("AppID = %q, want %q", provider.AppID, "cli_test123")
	}
	if provider.AppSecret != "testsecret" {
		t.Fatalf("AppSecret = %q, want %q", provider.AppSecret, "testsecret")
	}
	if provider.isLark {
		t.Fatalf("isLark = true, want false for feishu.cn")
	}
	if provider.UsernameClaim != "email" {
		t.Fatalf("UsernameClaim = %q, want %q", provider.UsernameClaim, "email")
	}
	if len(provider.AllowedGroups) != 2 || provider.AllowedGroups[0] != "group1" || provider.AllowedGroups[1] != "group2" {
		t.Fatalf("AllowedGroups = %v, want [group1 group2]", provider.AllowedGroups)
	}
}

func TestNewFeishuProviderLark(t *testing.T) {
	op := model.OAuthProvider{
		Name:         "lark",
		ClientID:     "cli_test456",
		ClientSecret: "larksecret",
		AuthURL:      "https://open.larksuite.com/open-apis/authen/v1/authorize",
	}

	provider, err := NewFeishuProvider(op)
	if err != nil {
		t.Fatalf("NewFeishuProvider() error = %v", err)
	}
	if !provider.isLark {
		t.Fatalf("isLark = false, want true for larksuite.com")
	}
}

func TestFeishuProviderGetAuthURL(t *testing.T) {
	tests := []struct {
		name     string
		isLark   bool
		wantBase string
	}{
		{
			name:     "feishu auth URL",
			isLark:   false,
			wantBase: "https://open.feishu.cn/open-apis/authen/v1/index",
		},
		{
			name:     "lark auth URL",
			isLark:   true,
			wantBase: "https://open.larksuite.com/open-apis/authen/v1/index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &FeishuProvider{
				AppID:       "cli_test",
				RedirectURL: "http://localhost:8080/api/auth/callback",
				isLark:      tt.isLark,
			}
			authURL := provider.GetAuthURL("test-state")
			if !strings.HasPrefix(authURL, tt.wantBase+"?") {
				t.Fatalf("authURL should start with %q, got %q", tt.wantBase+"?", authURL)
			}
			// Check app_id parameter instead of client_id
			if !strings.Contains(authURL, "app_id=cli_test") {
				t.Fatalf("authURL should contain app_id=cli_test, got %q", authURL)
			}
			// Should NOT contain client_id
			if strings.Contains(authURL, "client_id") {
				t.Fatalf("authURL should not contain client_id, got %q", authURL)
			}
		})
	}
}

func TestFeishuGetAppAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
		}

		// Verify request body
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body["app_id"] != "cli_test" || body["app_secret"] != "testsecret" {
			t.Errorf("unexpected app_id/app_secret: %q/%q", body["app_id"], body["app_secret"])
		}

		resp := feishuAPIResponse{Code: 0, Msg: "ok"}
		data, _ := json.Marshal(feishuAppTokenData{
			AppAccessToken: "test-app-token",
			Expire:         7200,
		})
		resp.Data = data
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Test that the provider correctly caches the token
	provider := &FeishuProvider{
		AppID:     "cli_test",
		AppSecret: "testsecret",
	}

	// The getAppAccessToken method uses const URLs, so we can't easily test it
	// with a mock server. Instead, verify the caching logic works.
	// First call should set the cache
	provider.appAccessToken = "cached-token"
	provider.appAccessTokenExp = time.Now().Add(3600 * time.Second)

	// This should return the cached token without making an HTTP call
	token, err := provider.getAppAccessToken()
	if err != nil {
		t.Fatalf("getAppAccessToken() error = %v", err)
	}
	if token != "cached-token" {
		t.Fatalf("getAppAccessToken() = %q, want %q", token, "cached-token")
	}
}

func TestCleanMobileNumber(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"China +86", "+8617550585613", "17550585613"},
		{"China +86 another", "+8613800138000", "13800138000"},
		{"Hong Kong +852", "+85298765432", "98765432"},
		{"Macau +853", "+85388888888", "88888888"},
		{"Taiwan +886", "+886912345678", "912345678"},
		{"No prefix", "13800138000", "13800138000"},
		{"Empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanMobileNumber(tt.input)
			if got != tt.want {
				t.Errorf("cleanMobileNumber(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFeishuProviderResolveUsername(t *testing.T) {
	provider := &FeishuProvider{
		AppID: "cli_test",
		Name:  "feishu",
	}

	userInfoData := feishuUserInfoData{
		OpenID:    "ou_test123",
		Name:      "张三",
		EnName:    "Zhang San",
		Email:     "zhangsan@example.com",
		Mobile:    "+8613800138000",
		AvatarURL: "https://avatar.example.com/zhangsan.png",
	}

	tests := []struct {
		name          string
		usernameClaim string
		want          string
	}{
		{"default uses email", "", "zhangsan@example.com"},
		{"email claim", "email", "zhangsan@example.com"},
		{"mobile claim", "mobile", "13800138000"},                  // +86 prefix removed
		{"phone claim (alias for mobile)", "phone", "13800138000"}, // +86 prefix removed
		{"name claim", "name", "张三"},
		{"en_name claim", "en_name", "Zhang San"},
		{"open_id claim", "open_id", "ou_test123"},
		{"user_id claim (empty, falls back)", "user_id", "zhangsan@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider.UsernameClaim = tt.usernameClaim
			got := provider.resolveUsername(userInfoData)
			if got != tt.want {
				t.Fatalf("resolveUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFeishuProviderResolveUsernameFallbacks(t *testing.T) {
	provider := &FeishuProvider{
		AppID: "cli_test",
		Name:  "feishu",
	}

	// No email → should use mobile
	userInfoData := feishuUserInfoData{
		OpenID: "ou_test456",
		Name:   "李四",
		Mobile: "+8613800138001",
	}
	got := provider.resolveUsername(userInfoData)
	if got != "13800138001" { // +86 prefix removed
		t.Fatalf("resolveUsername() = %q, want %q", got, "13800138001")
	}

	// No email, no mobile → should use name
	userInfoData.Mobile = ""
	got = provider.resolveUsername(userInfoData)
	if got != "李四" {
		t.Fatalf("resolveUsername() = %q, want %q", got, "李四")
	}

	// No email, no mobile, no name → should use en_name
	userInfoData.EnName = "Li Si"
	userInfoData.Name = ""
	got = provider.resolveUsername(userInfoData)
	if got != "Li Si" {
		t.Fatalf("resolveUsername() = %q, want %q", got, "Li Si")
	}

	// No email, no mobile, no name, no en_name → should use open_id
	userInfoData.EnName = ""
	got = provider.resolveUsername(userInfoData)
	if got != "ou_test456" {
		t.Fatalf("resolveUsername() = %q, want %q", got, "ou_test456")
	}
}

func TestFeishuProviderAllowedGroups(t *testing.T) {
	provider := &FeishuProvider{
		AppID:         "cli_test",
		Name:          "feishu",
		AllowedGroups: []string{"admin", "dev"},
	}

	// Test with user in allowed group
	user := &model.User{
		Provider:   "feishu",
		Username:   "testuser",
		OIDCGroups: []string{"dev", "ops"},
	}
	if !isAllowedGroup(user.OIDCGroups, provider.AllowedGroups) {
		t.Fatalf("user with 'dev' group should be allowed")
	}

	// Test with user not in allowed group
	user.OIDCGroups = []string{"ops", "qa"}
	if isAllowedGroup(user.OIDCGroups, provider.AllowedGroups) {
		t.Fatalf("user without admin/dev group should not be allowed")
	}
}

func TestFeishuAPIResponseParsing(t *testing.T) {
	// Test that we correctly parse Feishu's nested response format
	raw := `{"code":0,"msg":"ok","data":{"open_id":"ou_123","name":"Test","email":"test@example.com"}}`

	var apiResp feishuAPIResponse
	if err := json.Unmarshal([]byte(raw), &apiResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if apiResp.Code != 0 {
		t.Fatalf("code = %d, want 0", apiResp.Code)
	}
	if apiResp.Msg != "ok" {
		t.Fatalf("msg = %q, want %q", apiResp.Msg, "ok")
	}

	var data feishuUserInfoData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal data: %v", err)
	}
	if data.OpenID != "ou_123" {
		t.Fatalf("open_id = %q, want %q", data.OpenID, "ou_123")
	}
	if data.Name != "Test" {
		t.Fatalf("name = %q, want %q", data.Name, "Test")
	}
	if data.Email != "test@example.com" {
		t.Fatalf("email = %q, want %q", data.Email, "test@example.com")
	}
}

func TestFeishuAPIErrorResponse(t *testing.T) {
	raw := `{"code":99991668,"msg":"invalid app_access_token","data":{}}`

	var apiResp feishuAPIResponse
	if err := json.Unmarshal([]byte(raw), &apiResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if apiResp.Code == 0 {
		t.Fatalf("code should be non-zero for error response")
	}
	if apiResp.Msg != "invalid app_access_token" {
		t.Fatalf("msg = %q, want %q", apiResp.Msg, "invalid app_access_token")
	}
}

func TestDecodeFeishuResponseWrapped(t *testing.T) {
	raw := `{"code":0,"msg":"ok","data":{"access_token":"u-test","refresh_token":"r-test","token_type":"Bearer","expires_in":7200}}`
	var data feishuUserTokenData
	if err := decodeFeishuResponse(bytes.NewBufferString(raw), "token exchange", &data); err != nil {
		t.Fatalf("decodeFeishuResponse() error = %v", err)
	}
	if data.AccessToken != "u-test" {
		t.Fatalf("AccessToken = %q, want %q", data.AccessToken, "u-test")
	}
	if data.RefreshToken != "r-test" {
		t.Fatalf("RefreshToken = %q, want %q", data.RefreshToken, "r-test")
	}
}

func TestDecodeFeishuResponseFlat(t *testing.T) {
	raw := `{"code":0,"msg":"ok","app_access_token":"a-test","expire":7200}`
	var data feishuAppTokenData
	if err := decodeFeishuResponse(bytes.NewBufferString(raw), "app_access_token", &data); err != nil {
		t.Fatalf("decodeFeishuResponse() error = %v", err)
	}
	if data.AppAccessToken != "a-test" {
		t.Fatalf("AppAccessToken = %q, want %q", data.AppAccessToken, "a-test")
	}
	if data.Expire != 7200 {
		t.Fatalf("Expire = %d, want %d", data.Expire, 7200)
	}
}

func TestDecodeFeishuResponseError(t *testing.T) {
	raw := `{"code":99991668,"msg":"invalid app_access_token","data":{}}`
	var data feishuAppTokenData
	err := decodeFeishuResponse(bytes.NewBufferString(raw), "app_access_token", &data)
	if err == nil {
		t.Fatalf("decodeFeishuResponse() should fail for non-zero code")
	}
	if !strings.Contains(err.Error(), "code=99991668") {
		t.Fatalf("error = %q, want contains %q", err.Error(), "code=99991668")
	}
}

func TestDecodeFeishuResponseErrorMessageField(t *testing.T) {
	raw := `{"code":99991663,"message":"invalid code","data":{}}`
	var data feishuUserTokenData
	err := decodeFeishuResponse(bytes.NewBufferString(raw), "token exchange", &data)
	if err == nil {
		t.Fatalf("decodeFeishuResponse() should fail for non-zero code")
	}
	if !strings.Contains(err.Error(), "invalid code") {
		t.Fatalf("error = %q, want contains %q", err.Error(), "invalid code")
	}
}
