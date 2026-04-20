package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

const (
	feishuAuthURL     = "https://open.feishu.cn/open-apis/authen/v1/authorize"
	feishuAppTokenURL = "https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal/"
	feishuTokenURL    = "https://open.feishu.cn/open-apis/authen/v1/oidc/access_token"
	feishuUserInfoURL = "https://open.feishu.cn/open-apis/authen/v1/user_info"
	feishuRefreshURL  = "https://open.feishu.cn/open-apis/authen/v1/oidc/refresh_access_token"

	larkAuthURL     = "https://open.larksuite.com/open-apis/authen/v1/authorize"
	larkAppTokenURL = "https://open.larksuite.com/open-apis/auth/v3/app_access_token/internal/"
	larkTokenURL    = "https://open.larksuite.com/open-apis/authen/v1/oidc/access_token"
	larkUserInfoURL = "https://open.larksuite.com/open-apis/authen/v1/user_info"
	larkRefreshURL  = "https://open.larksuite.com/open-apis/authen/v1/oidc/refresh_access_token"
)

// FeishuProvider implements OAuthProvider for Feishu/Lark OAuth2.
// Feishu uses a non-standard OAuth2 flow that requires an app_access_token
// obtained separately before exchanging the authorization code or refreshing tokens.
type FeishuProvider struct {
	AppID         string
	AppSecret     string
	RedirectURL   string
	Name          string
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
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
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
	if f.isLark {
		authURL = larkAuthURL
	}
	params := url.Values{}
	params.Set("app_id", f.AppID)
	params.Set("redirect_uri", f.RedirectURL)
	params.Set("state", state)
	return authURL + "?" + params.Encode()
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

	var apiResp feishuAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("failed to decode app_access_token response: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("feishu app_access_token error: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	var data feishuAppTokenData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return "", fmt.Errorf("failed to parse app_access_token data: %w", err)
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

	klog.V(1).Infof("Refreshed app_access_token for %s, expires in %ds", f.Name, data.Expire)
	return data.AppAccessToken, nil
}

// ExchangeCodeForToken exchanges the authorization code for user access token.
// Feishu requires app_access_token in the Authorization header (non-standard).
func (f *FeishuProvider) ExchangeCodeForToken(code string) (*TokenResponse, error) {
	appAccessToken, err := f.getAppAccessToken()
	if err != nil {
		return nil, err
	}

	tokenURL := feishuTokenURL
	if f.isLark {
		tokenURL = larkTokenURL
	}

	body := map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token request: %w", err)
	}

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+appAccessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var apiResp feishuAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("feishu token exchange error: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	var data feishuUserTokenData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse token data: %w", err)
	}

	return &TokenResponse{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		TokenType:    data.TokenType,
		ExpiresIn:    data.ExpiresIn,
	}, nil
}

// RefreshToken refreshes the user access token using the refresh token.
// Feishu requires app_access_token in the Authorization header (non-standard).
func (f *FeishuProvider) RefreshToken(refreshToken string) (*TokenResponse, error) {
	appAccessToken, err := f.getAppAccessToken()
	if err != nil {
		return nil, err
	}

	refreshURL := feishuRefreshURL
	if f.isLark {
		refreshURL = larkRefreshURL
	}

	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	req, err := http.NewRequest("POST", refreshURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+appAccessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var apiResp feishuAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("feishu refresh token error: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	var data feishuUserTokenData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse refresh data: %w", err)
	}

	return &TokenResponse{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		TokenType:    data.TokenType,
		ExpiresIn:    data.ExpiresIn,
	}, nil
}

// GetUserInfo retrieves user information using the user access token.
func (f *FeishuProvider) GetUserInfo(accessToken string) (*model.User, error) {
	userInfoURL := feishuUserInfoURL
	if f.isLark {
		userInfoURL = larkUserInfoURL
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

	var apiResp feishuAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode user info response: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("feishu user info error: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	var data feishuUserInfoData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse user info data: %w", err)
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

	if !isAllowedGroup(user.OIDCGroups, f.AllowedGroups) {
		klog.Warningf("User %s is not in any allowed groups %v (provider: %s)", user.Username, f.AllowedGroups, f.Name)
		return nil, ErrNotInAllowedGroups
	}

	return user, nil
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
		case "mobile":
			if data.Mobile != "" {
				return data.Mobile
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
		return data.Mobile
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
