package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

const (
	feishuAuthURL        = "https://open.feishu.cn/open-apis/authen/v1/index"
	feishuAuthURLOIDC    = "https://open.feishu.cn/open-apis/authen/v1/authorize"
	feishuAppTokenURL    = "https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal/"
	feishuTokenURL       = "https://open.feishu.cn/open-apis/authen/v1/access_token"
	feishuTokenURLOIDC   = "https://open.feishu.cn/open-apis/authen/v1/oidc/access_token"
	feishuUserInfoURL    = "https://open.feishu.cn/open-apis/authen/v1/user_info"
	feishuRefreshURL     = "https://open.feishu.cn/open-apis/authen/v1/refresh_access_token"
	feishuRefreshURLOIDC = "https://open.feishu.cn/open-apis/authen/v1/oidc/refresh_access_token"

	larkAuthURL        = "https://open.larksuite.com/open-apis/authen/v1/index"
	larkAuthURLOIDC    = "https://open.larksuite.com/open-apis/authen/v1/authorize"
	larkAppTokenURL    = "https://open.larksuite.com/open-apis/auth/v3/app_access_token/internal/"
	larkTokenURL       = "https://open.larksuite.com/open-apis/authen/v1/access_token"
	larkTokenURLOIDC   = "https://open.larksuite.com/open-apis/authen/v1/oidc/access_token"
	larkUserInfoURL    = "https://open.larksuite.com/open-apis/authen/v1/user_info"
	larkRefreshURL     = "https://open.larksuite.com/open-apis/authen/v1/refresh_access_token"
	larkRefreshURLOIDC = "https://open.larksuite.com/open-apis/authen/v1/oidc/refresh_access_token"
)

// FeishuProvider implements OAuthProvider for Feishu/Lark OAuth2.
// Feishu uses a non-standard OAuth2 flow that requires an app_access_token
// obtained separately before exchanging the authorization code or refreshing tokens.
type FeishuProvider struct {
	AppID         string
	AppSecret     string
	RedirectURL   string
	Name          string
	AuthURL       string
	TokenURL      string
	UserInfoURL   string
	RefreshURL    string
	UsernameClaim string
	AllowedGroups []string
	isLark        bool

	// Cached app_access_token with its expiry
	mu                sync.RWMutex
	appAccessToken    string
	appAccessTokenExp time.Time
}

// feishuAPIResponse is the common wrapper for all Feishu API responses
type feishuAPIResponse struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type feishuAppTokenData struct {
	AppAccessToken string `json:"app_access_token"`
	Expire         int    `json:"expire"`
}

type feishuUserTokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type feishuUserInfoData struct {
	OpenID    string `json:"open_id"`
	UnionID   string `json:"union_id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	EnName    string `json:"en_name"`
	Email     string `json:"email"`
	Mobile    string `json:"mobile"`
	AvatarURL string `json:"avatar_url"`
}

// decodeFeishuResponse decodes Feishu/Lark API response and supports both:
// 1) standard wrapper with data field: {"code":0,"msg":"ok","data":{...}}
// 2) flat payload at top level: {"code":0,"msg":"ok","access_token":"..."}
func decodeFeishuResponse(respBody io.Reader, apiName string, out interface{}) error {
	bodyBytes, err := io.ReadAll(respBody)
	if err != nil {
		return fmt.Errorf("failed to read %s response: %w", apiName, err)
	}

	var apiResp feishuAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("failed to decode %s response: %w, body=%s", apiName, err, strings.TrimSpace(string(bodyBytes)))
	}

	msg := apiResp.Msg
	if msg == "" {
		msg = apiResp.Message
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("feishu %s error: code=%d, msg=%s", apiName, apiResp.Code, msg)
	}

	// Prefer wrapped `data` payload when present.
	if len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, out); err == nil {
			return nil
		}
	}

	// Fallback for APIs that return top-level fields.
	if err := json.Unmarshal(bodyBytes, out); err != nil {
		return fmt.Errorf("failed to parse %s data: %w", apiName, err)
	}

	return nil
}

// NewFeishuProvider creates a new Feishu/Lark OAuth2 provider
func NewFeishuProvider(op model.OAuthProvider) (*FeishuProvider, error) {
	isLark := strings.Contains(strings.ToLower(op.Issuer), "larksuite.com") ||
		strings.Contains(strings.ToLower(op.AuthURL), "larksuite.com")

	var allowedGroups []string
	if op.AllowedGroups != "" {
		for _, g := range strings.Split(op.AllowedGroups, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				allowedGroups = append(allowedGroups, g)
			}
		}
	}

	return &FeishuProvider{
		AppID:         op.ClientID,
		AppSecret:     string(op.ClientSecret),
		RedirectURL:   op.RedirectURL,
		Name:          string(op.Name),
		AuthURL:       strings.TrimSpace(op.AuthURL),
		TokenURL:      strings.TrimSpace(op.TokenURL),
		UserInfoURL:   strings.TrimSpace(op.UserInfoURL),
		RefreshURL:    "",
		UsernameClaim: op.UsernameClaim,
		AllowedGroups: allowedGroups,
		isLark:        isLark,
	}, nil
}

func (f *FeishuProvider) GetProviderName() string {
	return f.Name
}

// GetAuthURL returns the authorization URL for Feishu/Lark.
// Unlike standard OAuth2, Feishu uses app_id instead of client_id.
func (f *FeishuProvider) GetAuthURL(state string) string {
	authURL := feishuAuthURL
	if f.AuthURL != "" {
		authURL = f.AuthURL
	}
	if f.isLark {
		authURL = larkAuthURL
		if f.AuthURL != "" {
			authURL = f.AuthURL
		}
	}
	params := url.Values{}
	params.Set("app_id", f.AppID)
	params.Set("redirect_uri", f.RedirectURL)
	params.Set("state", state)
	return authURL + "?" + params.Encode()
}

func (f *FeishuProvider) getTokenEndpoints() []string {
	if f.TokenURL != "" {
		return []string{f.TokenURL}
	}
	if f.isLark {
		return []string{larkTokenURL, larkTokenURLOIDC}
	}
	return []string{feishuTokenURL, feishuTokenURLOIDC}
}

func (f *FeishuProvider) getRefreshEndpoints() []string {
	if f.RefreshURL != "" {
		return []string{f.RefreshURL}
	}
	if f.isLark {
		return []string{larkRefreshURL, larkRefreshURLOIDC}
	}
	return []string{feishuRefreshURL, feishuRefreshURLOIDC}
}

func (f *FeishuProvider) postAuthenJSON(
	endpoint string,
	appAccessToken string,
	apiName string,
	reqBody map[string]string,
	out interface{},
) error {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal %s request: %w", apiName, err)
	}

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+appAccessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call %s endpoint %s: %w", apiName, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read %s response from %s: %w", apiName, endpoint, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu %s HTTP %d from %s: %s", apiName, resp.StatusCode, endpoint, strings.TrimSpace(string(respBytes)))
	}

	if err := decodeFeishuResponse(bytes.NewReader(respBytes), apiName, out); err != nil {
		return fmt.Errorf("%w (endpoint=%s)", err, endpoint)
	}

	return nil
}

// getAppAccessToken retrieves a valid app_access_token, using cache when possible.
// Feishu requires this token in the Authorization header for code exchange and refresh.
func (f *FeishuProvider) getAppAccessToken() (string, error) {
	f.mu.RLock()
	if f.appAccessToken != "" && time.Now().Before(f.appAccessTokenExp) {
		token := f.appAccessToken
		f.mu.RUnlock()
		return token, nil
	}
	f.mu.RUnlock()

	appTokenURL := feishuAppTokenURL
	if f.isLark {
		appTokenURL = larkAppTokenURL
	}

	body := map[string]string{
		"app_id":     f.AppID,
		"app_secret": f.AppSecret,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal app token request: %w", err)
	}

	req, err := http.NewRequest("POST", appTokenURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get app_access_token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var data feishuAppTokenData
	if err := decodeFeishuResponse(resp.Body, "app_access_token", &data); err != nil {
		return "", err
	}

	// Cache the token, expire 5 minutes early as safety margin
	f.mu.Lock()
	f.appAccessToken = data.AppAccessToken
	expiry := time.Duration(data.Expire-300) * time.Second
	if expiry < 60*time.Second {
		expiry = 60 * time.Second
	}
	f.appAccessTokenExp = time.Now().Add(expiry)
	f.mu.Unlock()

	if data.AppAccessToken == "" {
		return "", fmt.Errorf("feishu app_access_token is empty")
	}

	klog.V(1).Infof("Refreshed app_access_token for %s, expires in %ds", f.Name, data.Expire)
	return data.AppAccessToken, nil
}

// ExchangeCodeForToken exchanges the authorization code for user access token.
// Supports both:
// 1. Feishu open platform (open.feishu.cn): requires app_access_token in Authorization header
// 2. Feishu passport/OIDC (passport.feishu.cn): uses standard OAuth2 with client_id/client_secret
func (f *FeishuProvider) ExchangeCodeForToken(code string) (*TokenResponse, error) {
	var lastErr error
	errDetails := make([]string, 0, len(f.getTokenEndpoints()))

	for _, endpoint := range f.getTokenEndpoints() {
		// Detect endpoint type and use appropriate authentication method
		isPassportEndpoint := strings.Contains(endpoint, "passport.feishu.cn") ||
			strings.Contains(endpoint, "passport.larksuite.com")

		var data feishuUserTokenData
		var err error

		if isPassportEndpoint {
			// Standard OAuth2 flow for passport endpoints
			err = f.exchangeTokenStandardOAuth(endpoint, code, &data)
		} else {
			// Feishu open platform flow with app_access_token
			err = f.exchangeTokenWithAppToken(endpoint, code, &data)
		}

		if err == nil && data.AccessToken != "" {
			return &TokenResponse{
				AccessToken:  data.AccessToken,
				RefreshToken: data.RefreshToken,
				TokenType:    data.TokenType,
				ExpiresIn:    data.ExpiresIn,
			}, nil
		}
		if err == nil && data.AccessToken == "" {
			err = fmt.Errorf("token exchange succeeded but access_token is empty on endpoint %s", endpoint)
		}
		lastErr = err
		errDetails = append(errDetails, err.Error())
		klog.Warningf("Feishu token exchange failed on endpoint %s (provider=%s): %v", endpoint, f.Name, err)
	}

	if len(errDetails) > 0 {
		return nil, fmt.Errorf("all token endpoints failed: %s", strings.Join(errDetails, " | "))
	}
	return nil, lastErr
}

// exchangeTokenWithAppToken exchanges code using Feishu open platform flow (requires app_access_token)
func (f *FeishuProvider) exchangeTokenWithAppToken(endpoint string, code string, out *feishuUserTokenData) error {
	appAccessToken, err := f.getAppAccessToken()
	if err != nil {
		return err
	}

	body := map[string]string{
		"grant_type":   "authorization_code",
		"code":         code,
		"redirect_uri": f.RedirectURL,
	}

	return f.postAuthenJSON(endpoint, appAccessToken, "token exchange", body, out)
}

// exchangeTokenStandardOAuth exchanges code using standard OAuth2 flow (for passport endpoints)
func (f *FeishuProvider) exchangeTokenStandardOAuth(endpoint string, code string, out *feishuUserTokenData) error {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  f.RedirectURL,
		"client_id":     f.AppID,
		"client_secret": f.AppSecret,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal token exchange request: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call token exchange endpoint %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token exchange response from %s: %w", endpoint, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu token exchange HTTP %d from %s: %s", resp.StatusCode, endpoint, strings.TrimSpace(string(respBytes)))
	}

	if err := decodeFeishuResponse(bytes.NewReader(respBytes), "token exchange", out); err != nil {
		return fmt.Errorf("%w (endpoint=%s)", err, endpoint)
	}

	return nil
}

// RefreshToken refreshes the user access token using the refresh token.
// Supports both open platform and passport/OIDC endpoints.
func (f *FeishuProvider) RefreshToken(refreshToken string) (*TokenResponse, error) {
	var lastErr error
	for _, endpoint := range f.getRefreshEndpoints() {
		// Detect endpoint type and use appropriate authentication method
		isPassportEndpoint := strings.Contains(endpoint, "passport.feishu.cn") ||
			strings.Contains(endpoint, "passport.larksuite.com")

		var data feishuUserTokenData
		var err error

		if isPassportEndpoint {
			// Standard OAuth2 flow for passport endpoints
			err = f.refreshTokenStandardOAuth(endpoint, refreshToken, &data)
		} else {
			// Feishu open platform flow with app_access_token
			err = f.refreshTokenWithAppToken(endpoint, refreshToken, &data)
		}

		if err == nil {
			return &TokenResponse{
				AccessToken:  data.AccessToken,
				RefreshToken: data.RefreshToken,
				TokenType:    data.TokenType,
				ExpiresIn:    data.ExpiresIn,
			}, nil
		}
		lastErr = err
		klog.Warningf("Feishu token refresh failed on endpoint %s (provider=%s): %v", endpoint, f.Name, err)
	}

	return nil, lastErr
}

// refreshTokenWithAppToken refreshes token using Feishu open platform flow
func (f *FeishuProvider) refreshTokenWithAppToken(endpoint string, refreshToken string, out *feishuUserTokenData) error {
	appAccessToken, err := f.getAppAccessToken()
	if err != nil {
		return err
	}

	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}

	return f.postAuthenJSON(endpoint, appAccessToken, "refresh token", body, out)
}

// refreshTokenStandardOAuth refreshes token using standard OAuth2 flow
func (f *FeishuProvider) refreshTokenStandardOAuth(endpoint string, refreshToken string, out *feishuUserTokenData) error {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     f.AppID,
		"client_secret": f.AppSecret,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal refresh token request: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call refresh token endpoint %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read refresh token response from %s: %w", endpoint, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu refresh token HTTP %d from %s: %s", resp.StatusCode, endpoint, strings.TrimSpace(string(respBytes)))
	}

	if err := decodeFeishuResponse(bytes.NewReader(respBytes), "refresh token", out); err != nil {
		return fmt.Errorf("%w (endpoint=%s)", err, endpoint)
	}

	return nil
}

// GetUserInfo retrieves user information using the user access token.
func (f *FeishuProvider) GetUserInfo(accessToken string) (*model.User, error) {
	userInfoURL := feishuUserInfoURL
	if f.UserInfoURL != "" {
		userInfoURL = f.UserInfoURL
	}
	if f.isLark {
		userInfoURL = larkUserInfoURL
		if f.UserInfoURL != "" {
			userInfoURL = f.UserInfoURL
		}
	}

	req, err := http.NewRequest("GET", userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var data feishuUserInfoData
	if err := decodeFeishuResponse(resp.Body, "user info", &data); err != nil {
		return nil, err
	}

	klog.V(1).Infof("Feishu user info: open_id=%s, name=%s, email=%s, mobile=%s", data.OpenID, data.Name, data.Email, data.Mobile)

	// Determine username: custom claim → email → mobile → name → open_id
	username := f.resolveUsername(data)

	user := &model.User{
		Provider:  f.Name,
		Sub:       data.OpenID,
		Username:  username,
		Name:      data.Name,
		AvatarURL: data.AvatarURL,
	}

	// Feishu's standard user info API does not return group/department information.
	// If allowed_groups is configured, log a warning and skip group validation.
	if len(f.AllowedGroups) > 0 {
		klog.Warningf("Feishu provider %s has allowed_groups configured %v, but Feishu user info API does not provide group data. Skipping group validation for user %s. To enable group validation, you would need to implement department API calls.",
			f.Name, f.AllowedGroups, user.Username)
	}

	return user, nil
}

// cleanMobileNumber removes country code prefix from mobile number
// e.g., "+8617550585613" → "17550585613"
func cleanMobileNumber(mobile string) string {
	if mobile == "" {
		return mobile
	}
	// Remove common country code prefixes
	mobile = strings.TrimPrefix(mobile, "+86")  // China
	mobile = strings.TrimPrefix(mobile, "+852") // Hong Kong
	mobile = strings.TrimPrefix(mobile, "+853") // Macau
	mobile = strings.TrimPrefix(mobile, "+886") // Taiwan
	return mobile
}

// resolveUsername determines the username from Feishu user info data.
// Priority: custom claim → email → mobile → name → en_name → open_id
func (f *FeishuProvider) resolveUsername(data feishuUserInfoData) string {
	if f.UsernameClaim != "" {
		switch f.UsernameClaim {
		case "email":
			if data.Email != "" {
				return data.Email
			}
		case "mobile", "phone": // phone is alias for mobile
			if data.Mobile != "" {
				return cleanMobileNumber(data.Mobile)
			}
		case "name":
			if data.Name != "" {
				return data.Name
			}
		case "en_name":
			if data.EnName != "" {
				return data.EnName
			}
		case "user_id":
			if data.UserID != "" {
				return data.UserID
			}
		case "open_id":
			return data.OpenID
		case "union_id":
			if data.UnionID != "" {
				return data.UnionID
			}
		default:
			// Unknown claim, fall through to default
			klog.Warningf("Unknown username_claim %q for Feishu provider %s, using default", f.UsernameClaim, f.Name)
		}
	}

	// Default priority
	if data.Email != "" {
		return data.Email
	}
	if data.Mobile != "" {
		return cleanMobileNumber(data.Mobile)
	}
	if data.Name != "" {
		return data.Name
	}
	if data.EnName != "" {
		return data.EnName
	}
	return data.OpenID
}

// isFeishuProvider detects whether an OAuth provider configuration is for Feishu/Lark
// by checking the Issuer or AuthURL for feishu.cn or larksuite.com domains.
func isFeishuProvider(op model.OAuthProvider) bool {
	lowerIssuer := strings.ToLower(op.Issuer)
	lowerAuthURL := strings.ToLower(op.AuthURL)
	return strings.Contains(lowerIssuer, "feishu.cn") ||
		strings.Contains(lowerIssuer, "larksuite.com") ||
		strings.Contains(lowerAuthURL, "feishu.cn") ||
		strings.Contains(lowerAuthURL, "larksuite.com")
}
