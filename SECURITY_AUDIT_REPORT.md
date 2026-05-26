# Kite 安全审计报告

> 审计时间: 2026-05-26  
> 审计范围: 认证/授权、API路由、数据加密、输入验证、SSRF/XSS/CSRF等

---

## 🔴 严重 (Critical)

### 1. `CreateSuperUser` 和 `ImportClustersFromKubeconfig` 端点无认证保护

**文件**: [`routes.go`](routes.go:74-76)

```go
adminAPI := r.Group("/api/v1/admin")
adminAPI.POST("/users/create_super_user", handlers.CreateSuperUser)   // ← 无认证!
adminAPI.POST("/clusters/import", cm.ImportClustersFromKubeconfig)    // ← 无认证!
adminAPI.Use(authHandler.RequireAuth(), authHandler.RequireAdmin())   // ← 中间件在后面才注册
```

这两个路由注册在 `RequireAuth()` + `RequireAdmin()` 中间件**之前**，因此**任何人**都可以无认证调用：

- `POST /api/v1/admin/users/create_super_user` — 在数据库为空时（如全新部署）创建超级管理员账户
- `POST /api/v1/admin/clusters/import` — 导入任意 kubeconfig 集群配置

**攻击场景**: 攻击者在 Kite 首次部署、数据库尚未初始化时，抢先调用 `create_super_user` 创建自己的管理员账户，从而接管整个系统。

**修复建议**: 将这两个路由移到 `adminAPI.Use(authHandler.RequireAuth(), authHandler.RequireAdmin())` 之后，或在 handler 内部添加独立的认证逻辑。

---

### 2. 硬编码的默认密钥

**文件**: [`pkg/common/common.go`](pkg/common/common.go:13)

```go
const (
    DefaultJWTSecret  = "kite-default-jwt-secret-key-change-in-production"  // 第13行
)
var (
    JwtSecret         = DefaultJWTSecret           // 第28行
    KiteEncryptKey    = "kite-default-encryption-key-change-in-production"  // 第37行
)
```

如果生产环境未设置 `JWT_SECRET` 和 `KITE_ENCRYPT_KEY` 环境变量：
- **JWT**: 任何人都可以用已知的默认密钥伪造合法的 JWT token，冒充任意用户（包括管理员）
- **加密**: 任何人都可以解密数据库中用 `EncryptString()` 加密的敏感字段（如 OAuth ClientSecret、LDAP BindPassword、集群 kubeconfig 等）

虽然代码中有 warning 日志，但**不会阻止启动**。

**修复建议**: 
1. 如果未设置密钥，在生产模式下拒绝启动（`klog.Fatal`）
2. 或在首次启动时自动生成随机密钥并持久化

---

### 3. OAuth 重定向 URL 可被 `X-Forwarded-*` 头操纵

**文件**: [`pkg/auth/oauth_manager.go`](pkg/auth/oauth_manager.go:30-48)

```go
func getRequestHost(c *gin.Context) string {
    if common.Host != "" {
        return common.Host
    }
    proto := c.Request.Header.Get("X-Forwarded-Proto")  // ← 可伪造
    host := c.Request.Header.Get("X-Forwarded-Host")    // ← 可伪造
    if proto != "" && host != "" {
        return proto + "://" + host
    }
    ...
}
```

当 `HOST` 环境变量未设置时，`getRequestHost()` 直接信任 `X-Forwarded-Proto` 和 `X-Forwarded-Host` 请求头。攻击者可以：

1. 构造恶意请求，设置 `X-Forwarded-Host: evil.com`
2. OAuth 回调 URL 被设为 `https://evil.com/api/auth/callback`
3. 用户完成 OAuth 登录后，授权码被发送到攻击者的服务器
4. 攻击者用授权码换取 token，接管用户账户

**修复建议**: 
1. 强制要求设置 `HOST` 环境变量
2. 或对 `X-Forwarded-*` 头做白名单校验
3. 在反向代理层剥离不可信的 `X-Forwarded-*` 头

---

## 🟠 高危 (High)

### 4. 匿名用户拥有完全管理员权限

**文件**: [`pkg/model/user.go`](pkg/model/user.go:455-470)

```go
AnonymousUser = User{
    Username: "anonymous",
    Provider: "Anonymous",
    Roles: []common.Role{
        {
            Name:       "admin",
            Clusters:   []string{"*"},
            Resources:  []string{"*"},
            Namespaces: []string{"*"},
            Verbs:      []string{"*"},
        },
    },
}
```

如果 `ANONYMOUS_USER_ENABLED=true`，所有未认证用户自动获得**全集群、全资源、全操作**的管理员权限。虽然代码中有 warning，但这在生产环境中极其危险。

**修复建议**: 匿名用户应默认为最小权限（如只读），而非管理员。

---

### 5. 飞书回调签名验证可选

**文件**: [`pkg/handlers/feishu_callback_handler.go`](pkg/handlers/feishu_callback_handler.go:50-60)

```go
if token := string(setting.VerificationToken); token != "" {
    // 签名验证...
}
// 如果 token 为空，直接跳过验证！
```

如果未配置 `VerificationToken`，飞书回调端点 `POST /api/feishu/card-callback` **完全无认证**。任何人都可以伪造回调请求来**批准或拒绝**访问申请。

**攻击场景**: 攻击者构造恶意请求批准自己的访问申请，获取临时管理员角色。

**修复建议**: 当 `VerificationToken` 未配置时，拒绝处理非 `url_verification` 类型的回调。

---

### 6. API Key 比较未使用常量时间比较

**文件**: [`pkg/auth/middleware.go`](pkg/auth/middleware.go:41)

```go
if key != string(apikey.APIKey) {
    // 拒绝
}
```

使用 `!=` 比较 API Key 会产生**计时侧信道**。攻击者可以通过测量响应时间逐字节猜测 API Key。

**修复建议**: 使用 `crypto/subtle.ConstantTimeCompare()` 替代。

---

## 🟡 中危 (Medium)

### 7. 登录接口无速率限制

**文件**: [`routes.go`](routes.go:45-46)

```go
authGroup.POST("/login/password", authHandler.PasswordLogin)
authGroup.POST("/login/ldap", authHandler.LDAPLogin)
```

密码登录和 LDAP 登录端点没有任何速率限制，容易遭受**暴力破解攻击**。

**修复建议**: 添加基于 IP 的速率限制中间件（如 `golang.org/x/time/rate` 或 `github.com/ulule/limiter`）。

---

### 8. SSRF — 镜像标签查询

**文件**: [`pkg/handlers/image_tags_handler.go`](pkg/handlers/image_tags_handler.go:66)

```go
func (d containerRegistryV2) GetTags(ctx context.Context) ([]ImageTagInfo, error) {
    url := fmt.Sprintf("https://%s/v2/%s/tags/list", d.baseURL, d.repo)
    resp, err := http.Get(url)
```

`baseURL` 来自用户输入的 `image` 查询参数。攻击者可以构造特殊的镜像名使服务器向内网服务发起请求（SSRF）。

**修复建议**: 对 `baseURL` 做白名单校验，或限制只能访问已知的容器镜像仓库域名。

---

### 9. RBAC 正则表达式 ReDoS 风险

**文件**: [`pkg/rbac/rbac.go`](pkg/rbac/rbac.go:207)

```go
re, err := regexp.Compile("^(?:" + v + ")$")
```

RBAC 规则中的 `Clusters`、`Namespaces` 等字段支持正则匹配，但正则表达式**每次调用都重新编译**且**无缓存**。恶意或不当的正则模式可能导致：
- **ReDoS（正则拒绝服务）**: 恶意正则导致 CPU 100%
- **性能问题**: 每次请求都编译正则

**修复建议**: 
1. 缓存编译后的正则对象
2. 限制正则复杂度或使用 glob 模式替代

---

### 10. 临时访问授权角色权限过大

**文件**: [`pkg/handlers/access_request_handler.go`](pkg/handlers/access_request_handler.go:62-69)

```go
role := &model.Role{
    Name:       roleName,
    Clusters:   []string{"*"},    // ← 所有集群!
    Resources:  []string{"*"},    // ← 所有资源!
    Namespaces: []string{req.Namespace},
    Verbs:      []string{"*"},    // ← 所有操作!
}
```

临时角色授予**所有集群**的**所有资源**的**所有操作**权限，仅限制了命名空间。用户申请某个命名空间的访问，却获得了所有集群中该命名空间的完全控制权。

**修复建议**: 临时角色应限制到特定集群，并根据申请场景限制资源和操作类型。

---

### 11. Cookie Secure 标志依赖可伪造的请求头

**文件**: [`pkg/auth/login_handler.go`](pkg/auth/login_handler.go:349)

```go
func setCookieSecure(c *gin.Context, name, value string, maxAge int) {
    secure := strings.HasPrefix(common.Host, "https://") || 
        (c.Request != nil && (c.Request.TLS != nil || 
            strings.EqualFold(c.Request.Header.Get("X-Forwarded-Proto"), "https")))
```

当 `HOST` 未设置时，`secure` 标志依赖 `X-Forwarded-Proto` 头。攻击者可以设置此头使 cookie 在 HTTP 连接上被发送。

**修复建议**: 不应仅依赖 `X-Forwarded-Proto`，应通过配置明确指定是否为 HTTPS 部署。

---

### 12. JWT 中嵌入 OAuth Refresh Token

**文件**: [`pkg/auth/oauth_manager.go`](pkg/auth/oauth_manager.go:89)

```go
claims := Claims{
    UserID:       user.ID,
    RefreshToken: refreshToken,  // ← OAuth refresh token 嵌入 JWT
}
```

OAuth Refresh Token 被嵌入 JWT 并存储在 cookie 中。如果 JWT 被泄露（如 XSS），攻击者同时获得 refresh token，可以长期维持访问。

**修复建议**: 将 refresh token 存储在服务端（数据库），JWT 中只存引用 ID。

---

## 🟢 低危 (Low)

### 13. `/metrics` 端点无认证

**文件**: [`routes.go`](routes.go:31-34)

```go
r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(...)))
```

Prometheus metrics 端点无需认证即可访问，可能泄露内部运行指标。

**修复建议**: 添加认证或将 metrics 端点绑定到内部端口。

---

### 14. 无密码复杂度要求

**文件**: [`pkg/handlers/user_handler.go`](pkg/handlers/user_handler.go:52-77)

创建用户和重置密码时，没有对密码长度、复杂度做任何校验。用户可以设置 "1" 这样的弱密码。

**修复建议**: 添加最小密码长度和复杂度校验。

---

### 15. 登录错误日志泄露用户名

**文件**: [`pkg/auth/login_handler.go`](pkg/auth/login_handler.go:107-108)

```go
errMsg := fmt.Sprintf("%s login failed for %s: %v", strings.ToUpper(provider), username, err)
klog.Warning(errMsg)
```

登录失败的日志包含用户名和错误详情，在日志被集中收集时可能造成信息泄露。

**修复建议**: 日志中不记录用户名，或对用户名做脱敏处理。

---

### 16. `ListUsers` 搜索参数存在潜在 SQL 注入

**文件**: [`pkg/model/user.go`](pkg/model/user.go:252-258)

```go
likeQuery := "%" + search + "%"
query = query.Where("users.username LIKE ? OR users.name LIKE ?", likeQuery, likeQuery)
```

虽然 GORM 的参数化查询可以防止传统 SQL 注入，但 `LIKE` 查询中的 `%` 和 `_` 通配符未被转义。攻击者可以构造 `search=%` 来匹配所有记录，或利用通配符进行信息探测。

**修复建议**: 对 `search` 参数中的 `%` 和 `_` 进行转义。

---

## 漏洞统计

| 严重程度 | 数量 |
|---------|------|
| 🔴 严重  | 3    |
| 🟠 高危  | 3    |
| 🟡 中危  | 6    |
| 🟢 低危  | 4    |
| **合计** | **16** |

---

## 优先修复建议

1. **立即修复** #1（无认证端点）和 #2（硬编码密钥）— 这两个漏洞可被直接利用接管系统
2. **尽快修复** #3（OAuth 重定向）和 #5（飞书回调）— 可导致账户接管
3. **短期修复** #4、#6、#7、#8 — 降低攻击面
4. **中期修复** 其余中低危漏洞
