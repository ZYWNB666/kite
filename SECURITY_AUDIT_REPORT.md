# Kite 安全审计报告

> 审计时间: 2026-05-26  
> 更新审计时间: 2026-06-14  
> 审计范围: 认证/授权、API路由、数据加密、输入验证、SSRF/XSS/CSRF、K8s安全、配置与密钥管理  
> 原始状态: ✅ 全部 17 项漏洞已逐行确认，均未修复  
> 本次更新: 新增 20 项漏洞发现，总计 37 项

---

## ⚡ 外部可利用漏洞（无需登录）

> 以下漏洞**无需平台账户**即可从外部利用，是最高优先级修复项。

| # | 漏洞 | 严重程度 | 利用条件 |
|---|------|---------|---------|
| 1 | `CreateSuperUser`/`ImportClusters` 无认证 | 🔴 严重 | 首次部署时直接调用 |
| 2 | 硬编码默认密钥（JWT + 加密） | 🔴 严重 | 未设置环境变量时直接伪造JWT/解密数据 |
| 3 | OAuth 重定向 URL 被 `X-Forwarded-*` 操纵 | 🔴 严重 | `HOST` 未设置时构造请求头 |
| 18 | Helm Chart 默认使用已知加密密钥 | 🔴 严重 | Helm 默认部署时数据等同于明文 |
| 19 | Helm/install.yaml 默认 cluster-admin RBAC | 🔴 严重 | 默认部署即授予全集群控制权 |
| 20 | 无服务端 TLS 选项 | 🔴 严重 | 无反向代理时凭据明文传输 |
| 4 | 匿名用户拥有管理员权限 | 🟠 高危 | `ANONYMOUS_USER_ENABLED=true` 时 |
| 5 | 飞书回调签名验证可选 | 🟠 高危 | `VerificationToken` 未配置时 |
| 6 | API Key 非常量时间比较 | 🟠 高危 | 向认证端点发送请求测量时间 |
| 21 | 无 CSRF 保护 | 🟠 高危 | 诱导用户点击恶意链接 |
| 8 | 登录接口无速率限制 | 🟡 中危 | 直接对登录接口暴力破解 |
| 12 | Cookie Secure 依赖可伪造头 | 🟡 中危 | 需网络层中间人位置 |
| 28 | 缺少 HTTP 安全响应头 | 🟡 中危 | 直接访问 |
| 14 | `/metrics` 端点无认证 | 🟢 低危 | 直接访问 |
| 16 | 登录错误信息泄露用户名 | 🟢 低危 | 直接对登录接口探测 |

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

- `POST /api/v1/admin/users/create_super_user` — 在数据库为空时创建超级管理员账户
- `POST /api/v1/admin/clusters/import` — 导入任意 kubeconfig 集群配置

**缓解因素**: [`CreateSuperUser`](pkg/handlers/user_handler.go:33) 有 `if uc > 0` 检查，[`ImportClustersFromKubeconfig`](pkg/cluster/cluster_handler.go:253) 有 `if cc > 0` 检查。但这仅将攻击窗口限制在**首次部署阶段**，攻击者若抢先到达系统仍可完全接管。此外并发请求存在竞态条件，可能创建多个超级用户。

**修复建议**: 将这两个路由移到 `adminAPI.Use(authHandler.RequireAuth(), authHandler.RequireAdmin())` 之后，或在 handler 内部添加独立的认证逻辑。

---

### 2. 硬编码的默认密钥

**文件**: [`pkg/common/common.go`](pkg/common/common.go:13,28,37)

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
- **加密**: 任何人都可以解密数据库中用 `EncryptString()` 加密的敏感字段（OAuth ClientSecret、LDAP BindPassword、集群 kubeconfig、API Key 等）

**JWT 密钥部分缓解**: [`ensureJWTSecret()`](pkg/model/general_setting.go:174-184) 会在首次运行时自动生成随机密钥，但如果数据库写入失败则回退到默认值。

**加密密钥无任何自动修复机制**。

**加密实现额外问题**: 密钥通过 `sha256.Sum256()` 直接派生（[`pkg/utils/secure.go:26`](pkg/utils/secure.go:26)），无 salt、无迭代，低熵密钥空间可被暴力破解。且**无密钥轮转机制**，更换密钥后旧数据不可解密。

**修复建议**: 
1. 如果未设置密钥，在生产模式下拒绝启动（`klog.Fatal`）
2. 使用 PBKDF2/scrypt/Argon2 进行密钥派生，或要求密钥为 32 字节随机数据
3. 实现密钥轮转迁移机制

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
2. OAuth 回调 URL 被设为 `https://evil.com/api/auth/callback`（[第55行](pkg/auth/oauth_manager.go:55)）
3. 用户完成 OAuth 登录后，授权码被发送到攻击者的服务器
4. 攻击者用授权码换取 token，接管用户账户

**修复建议**: 
1. 强制要求设置 `HOST` 环境变量
2. 或对 `X-Forwarded-*` 头做白名单校验
3. 在反向代理层剥离不可信的 `X-Forwarded-*` 头

---

### 18. Helm Chart 默认使用已知加密密钥

**文件**: [`charts/kite/values.yaml`](charts/kite/values.yaml:59)

```yaml
encryptKey: "kite-default-encryption-key-change-in-production"
```

Helm Chart 默认值与源码中硬编码的默认密钥相同。用户通过 Helm 部署时如果没有显式设置 `encryptKey`，所有数据库中的加密字段等同于明文存储。

**修复建议**: 将默认值设为空字符串，在模板中检测到空值时要求用户显式提供。

---

### 19. Helm/install.yaml 默认 cluster-admin RBAC

**文件**: [`charts/kite/values.yaml`](charts/kite/values.yaml:148-153), [`deploy/install.yaml`](deploy/install.yaml:1-12)

Helm Chart 默认 ClusterRole 授予 `["*"]` 全部权限：

```yaml
rbac:
  rules:
    - apiGroups: ["*"]
      resources: ["*"]
      verbs: ["*"]
    - nonResourceURLs: ["*"]
      verbs: ["*"]
```

`deploy/install.yaml` 更直接绑定内置 `cluster-admin` ClusterRole。

如果 kite Pod 被攻破，攻击者获得全集群控制权。`nonResourceURLs: ["*"]` 还额外授予 `/healthz`、`/metrics`、`/logs` 等集群级端点访问。

**修复建议**: 缩小默认 RBAC 范围至实际需要的资源和动词，移除 `nonResourceURLs` 通配。

---

### 20. 无服务端 TLS 选项

**文件**: [`main.go`](main.go:39)

```go
if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
```

应用只使用 `ListenAndServe()`，无 `ListenAndServeTLS()` 选项。TLS 完全依赖外部反向代理。如果部署时未配置 TLS 终止（如默认 Helm Ingress 未启用 TLS），所有流量包括 JWT cookie、密码、API Key 均以明文传输。

**修复建议**: 
1. 文档中强调必须使用 TLS 终止代理
2. Helm Chart 默认启用 Ingress TLS
3. 或添加服务端 TLS 支持

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

如果 `ANONYMOUS_USER_ENABLED=true`（[common.go:101](pkg/common/common.go:101)），所有未认证请求在 `RequireAuth()` 中直接放行（[middleware.go:55-64](pkg/auth/middleware.go:55-64)），跳过 JWT/API Key 验证，自动获得**全集群、全资源、全操作**的管理员权限。

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

此外，对于 v1 格式回调，即使配置了 `VerificationToken`，token 不匹配时仅记录警告**不拒绝请求**（[第123-127行](pkg/handlers/feishu_callback_handler.go:123-127)），攻击者可伪造 v1 格式回调绕过验证。

飞书回调端点 `POST /api/feishu/card-callback`（[routes.go:59](routes.go:59)）无 `RequireAuth()` 中间件，结合上述验证缺失，任何人可伪造批准/拒绝访问申请的回调，获取临时通配符权限（见 #11）。

**修复建议**: 
1. 当 `VerificationToken` 未配置时，拒绝处理非 `url_verification` 类型的回调
2. v1 格式回调 token 不匹配时应拒绝请求

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

### 7. Resource Apply 仅检查 "create" 权限但可执行 "update" 操作

**文件**: [`pkg/handlers/resource_apply_handler.go`](pkg/handlers/resource_apply_handler.go:54)

```go
// 第54行：仅检查 "create" 权限
if !rbac.CanAccess(user, resource, "create", cs.Name, obj.GetNamespace()) {
    // 拒绝
}

// 第96-110行：但实际可能执行 Update 操作
case err == nil:
    obj.SetResourceVersion(existingObj.GetResourceVersion())
    if err := cs.K8sClient.Update(ctx, obj); err != nil {  // Update! 但未检查 "update" 权限
```

拥有 "create" 但没有 "update" 权限的用户可以修改已有资源，构成**权限提升**。

**修复建议**: 资源已存在时额外检查 `"update"` 权限。

---

### 21. 无 CSRF 保护

**文件**: [`routes.go`](routes.go), [`pkg/auth/login_handler.go`](pkg/auth/login_handler.go)

所有状态变更 API 端点无 CSRF token 机制。认证 cookie 使用 `SameSite=Lax`，提供部分保护但：
- 老浏览器不支持 SameSite
- OAuth 回调是 GET 请求执行认证操作
- `POST /api/auth/refresh` 可被跨站请求触发，延长攻击者访问窗口

**修复建议**: 对状态变更端点添加 CSRF token 或自定义请求头验证。

---

### 22. 无服务端会话撤销机制

**文件**: [`pkg/auth/middleware.go`](pkg/auth/middleware.go), [`pkg/auth/login_handler.go`](pkg/auth/login_handler.go)

认证完全依赖 JWT，无服务端会话存储。用户登出仅清除 cookie，已提取的 JWT 在过期前（24小时）始终有效。后果：
- 被盗 token 无法作废
- 修改密码不会使已有 session 失效
- `RefreshJWT`（[oauth_manager.go:143](pkg/auth/oauth_manager.go:143)）使用 `jwt.WithoutClaimsValidation()` 接受过期 token 并签发新 token，密码用户的 token 可无限续期
- 无法强制全局登出

**修复建议**: 实现服务端 session 跟踪或 token 黑名单机制。

---

### 23. kubectl terminal 创建 cluster-admin ServiceAccount 且不清理

**文件**: [`pkg/handlers/kubectl_terminal_handler.go`](pkg/handlers/kubectl_terminal_handler.go:107-147)

kubectl 终端功能自动在 `kube-system` 命名空间创建 ServiceAccount 并绑定 `cluster-admin` ClusterRoleBinding。此 SA 在会话结束后**不会清理**，持续保留在集群中。

**修复建议**: 会话结束时清理创建的 ServiceAccount 和 ClusterRoleBinding，或使用临时 token 替代。

---

### 24. Node terminal 创建特权 Pod

**文件**: [`pkg/handlers/node_terminal_handler.go`](pkg/handlers/node_terminal_handler.go:115-157)

Node 终端 Pod 配置：
- `HostNetwork: true` — 共享主机网络命名空间
- `HostPID: true` — 可见主机进程
- `HostIPC: true` — 共享主机 IPC 命名空间
- `Privileged: true` — 完全设备访问权限
- 挂载根文件系统至 `/host` 并 `chroot`

等效于节点 root 访问，但 RBAC 仅检查 `exec` 权限。

**修复建议**: 审计 node terminal 使用场景，考虑限制为特定管理员角色。

---

### 25. Helm Chart 无 Pod Security Context

**文件**: [`charts/kite/values.yaml`](charts/kite/values.yaml:162-173)

```yaml
podSecurityContext: {}
securityContext: {}
```

注释中建议的 `runAsNonRoot: true`、`readOnlyRootFilesystem: true`、`capabilities: drop: ALL` 未作为默认值。Pod 以 root 运行、全 capabilities、可写根文件系统。

**修复建议**: 将安全上下文设为默认值。

---

### 26. OAuth Refresh Token 嵌入 JWT

**文件**: [`pkg/auth/oauth_manager.go`](pkg/auth/oauth_manager.go:89)

```go
claims := Claims{
    UserID:       user.ID,
    RefreshToken: refreshToken,  // ← OAuth refresh token 嵌入 JWT
}
```

OAuth Refresh Token 存储在 JWT cookie 中。JWT 被泄露时（XSS、默认密钥、网络拦截），攻击者同时获得 refresh token，可长期维持对身份提供者的访问。

**修复建议**: 将 refresh token 存储在服务端，JWT 中只存引用 ID。

---

## 🟡 中危 (Medium)

### 8. 登录接口无速率限制

**文件**: [`routes.go`](routes.go:45-46)

密码登录和 LDAP 登录端点无速率限制，易遭**暴力破解**。bcrypt 对比约 100ms/次，限制约 10 次/秒/连接，但无法防御分布式攻击。

**修复建议**: 添加基于 IP 的速率限制中间件。

---

### 9. SSRF — 镜像标签查询

**文件**: [`pkg/handlers/image_tags_handler.go`](pkg/handlers/image_tags_handler.go:66)

```go
url := fmt.Sprintf("https://%s/v2/%s/tags/list", d.baseURL, d.repo)
resp, err := http.Get(url)
```

`baseURL` 来自用户输入的 `image` 查询参数。攻击者可构造特殊镜像名使服务器向内网发起请求。

**修复建议**: 对 `baseURL` 做白名单校验，或拒绝私有 IP 段和集群内部域名。

---

### 10. RBAC 正则表达式 ReDoS 风险

**文件**: [`pkg/rbac/rbac.go`](pkg/rbac/rbac.go:207)

```go
re, err := regexp.Compile("^(?:" + v + ")$")
```

RBAC 规则支持正则匹配，但每次请求重新编译且无缓存。恶意正则可导致 ReDoS（CPU 100%）。

**修复建议**: 
1. 缓存编译后的正则对象
2. 限制正则复杂度或使用 glob 模式替代

---

### 11. 临时访问授权角色权限过大

**文件**: [`pkg/handlers/access_request_handler.go`](pkg/handlers/access_request_handler.go:62-69)

```go
role := &model.Role{
    Clusters:    []string{"*"},    // ← 所有集群!
    Resources:   []string{"*"},    // ← 所有资源!
    Namespaces:  []string{req.Namespace},
    Verbs:       []string{"*"},    // ← 所有操作!
}
```

临时角色授予所有集群所有资源的所有操作权限，仅限制命名空间。结合 #5（飞书回调可伪造），攻击者可自行批准获取通配符临时角色。

**修复建议**: 临时角色应限制到特定集群，并根据申请场景限制资源和操作类型。

---

### 12. Cookie Secure 标志依赖可伪造的请求头

**文件**: [`pkg/auth/login_handler.go`](pkg/auth/login_handler.go:349)

当 `HOST` 未设置时，`secure` 标志依赖 `X-Forwarded-Proto` 头。攻击者可注入此头使 cookie 在 HTTP 连接上发送。

**修复建议**: 通过配置明确指定是否为 HTTPS 部署，不依赖请求头。

---

### 13. JWT 中嵌入 OAuth Refresh Token

> 见 #26（已提升为高危）

---

### 27. WebSocket 无 Origin 验证

**文件**: [`pkg/handlers/terminal_handler.go`](pkg/handlers/terminal_handler.go:42-59)

使用 `golang.org/x/net/websocket`，默认不校验 Origin。攻击者可在恶意页面中打开 WebSocket 连接到终端端点，若受害用户有活跃 session cookie，可在 Pod 中执行命令。

**修复建议**: 添加 Origin 头白名单校验。

---

### 28. 缺少 HTTP 安全响应头

**文件**: 全局中间件

应用未设置任何安全响应头：
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY` / `SAMEORIGIN`
- `Content-Security-Policy`
- `Strict-Transport-Security`
- `Referrer-Policy`
- `Permissions-Policy`

SPA fallback（[`static.go:49`](static.go:49)）仅设置 `Content-Type`。

**修复建议**: 添加全局安全头中间件。

---

### 29. K8s proxy 仅检查 `get` 权限

**文件**: [`pkg/handlers/proxy_handler.go`](pkg/handlers/proxy_handler.go:34)

```go
rbac.CanAccess(user, kind, "get", cs.Name, namespace)
```

代理请求只检查 `get` 权限，但代理可访问 Pod/Service 的任意 HTTP API 端点（含写操作）。

**修复建议**: 根据代理请求的 HTTP 方法映射到对应 RBAC 动词。

---

### 30. Pod 文件操作路径未消毒

**文件**: [`pkg/handlers/resources/pod_handler.go`](pkg/handlers/resources/pod_handler.go:297-498)

`path` 查询参数直接传入 `ls`、`cat`、`tar`、`tee` 等 Pod 内命令。虽然目标是 Pod 文件系统而非宿主机，但路径穿越（如 `../../etc/shadow`）可在 Pod 内读写任意文件。上传操作验证了文件名但未验证目标路径。

**修复建议**: 验证路径不包含 `..` 组件。

---

### 31. AI 会话删除无所有权检查

**文件**: [`pkg/ai/handler.go`](pkg/ai/handler.go:368-371)

`HandleDeleteConversationSession` 接受 URL 中的 session ID 直接删除，不验证请求用户是否为会话所有者。任何已认证用户可删除他人 AI 会话。

**修复建议**: 删除前验证 session 归属。

---

### 32. 用户 API 返回密码/敏感字段

**文件**: [`pkg/handlers/user_handler.go`](pkg/handlers/user_handler.go:49,76)

`CreateSuperUser` 和 `CreatePasswordUser` 返回完整 user 对象。虽然 `Password` 字段有 `json:"-"` 标签，但 `APIKey` 字段（`json:"apiKey,omitempty"`）会被暴露，且未来新增字段若无正确标签也会泄露。

**修复建议**: 使用独立的响应结构体，仅返回必要字段。

---

### 33. RBAC 角色缓存导致权限变更延迟

**文件**: [`pkg/model/user.go`](pkg/model/user.go:66-94), [`pkg/rbac/rbac.go`](pkg/rbac/rbac.go:144-148)

用户缓存 30 秒 + RBAC 配置同步间隔 1 分钟，权限变更最多需 90 秒生效。被降权用户在此窗口内仍保留旧权限。

**修复建议**: 权限变更时主动清除相关用户缓存。

---

### 34. LDAP 允许明文连接

**文件**: [`pkg/auth/ldap.go`](pkg/auth/ldap.go:60-69)

支持 StartTLS 且正确设置 TLS 1.2+（[第60-66行](pkg/auth/ldap.go:60-66)），但不强制要求。使用 `ldap://` 且不启用 StartTLS 时，绑定密码和用户密码以明文传输。

**修复建议**: 警告或拒绝 `ldap://` 且未启用 StartTLS 的配置。

---

### 35. 飞书 v1 回调 token 不匹配时仅警告不拒绝

**文件**: [`pkg/handlers/feishu_callback_handler.go`](pkg/handlers/feishu_callback_handler.go:123-127)

对于非 schema "2.0" 的回调，token 不匹配时仅记录 warning 但不拒绝请求。攻击者可伪造 v1 格式回调绕过签名验证。

**修复建议**: token 不匹配时应拒绝请求。

---

### 36. 多种输入无长度/字符验证

**文件**: 多个 handler

以下输入缺乏长度或字符验证：
- `CreateAccessRequest.Reason` — 无最大长度
- `CreateTemplateRequest.Name` 和 `YAML` — 无最大长度
- `UpdateUser.AvatarURL` — 可能为 `javascript:` URI
- `CreateRole` 各字段 — 无长度限制，通配符数组无校验
- `Cluster.PrometheusURL` — 无 URL 格式验证
- `sortBy` 参数 — 未做白名单校验，潜在 GORM Order 注入

**修复建议**: 对各输入添加长度限制和格式校验。

---

## 🟢 低危 (Low)

### 14. `/metrics` 端点无认证

**文件**: [`routes.go`](routes.go:31-34)

Prometheus metrics 端点无需认证即可访问，可能泄露内部运行指标。

**修复建议**: 添加认证或将 metrics 端点绑定到内部端口。

---

### 15. 无密码复杂度要求

**文件**: [`pkg/handlers/user_handler.go`](pkg/handlers/user_handler.go:52-77)

创建用户和重置密码时无密码长度/复杂度校验。可设置 "1" 这样的弱密码。

**修复建议**: 添加最小密码长度和复杂度校验。

---

### 16. 登录错误信息泄露用户名

**文件**: [`pkg/auth/login_handler.go`](pkg/auth/login_handler.go:107-110)

```go
errMsg := fmt.Sprintf("%s login failed for %s: %v", strings.ToUpper(provider), username, err)
c.JSON(http.StatusUnauthorized, gin.H{"error": errMsg})
```

错误信息包含用户名和错误详情，可枚举用户并区分"用户不存在"与"密码错误"。

**修复建议**: HTTP 响应使用通用错误信息（如 "Invalid credentials"）。

---

### 17. `ListUsers` 搜索参数 LIKE 通配符未转义

**文件**: [`pkg/model/user.go`](pkg/model/user.go:252-258)

```go
likeQuery := "%" + search + "%"
```

`%` 和 `_` 通配符未转义，攻击者可构造 `search=%` 匹配所有记录。

**修复建议**: 对 `search` 参数中的 `%` 和 `_` 进行转义。

---

### 37. 其他低危发现

| 问题 | 文件 | 说明 |
|------|------|------|
| 加密错误返回字符串而非 error 类型 | [`pkg/utils/secure.go`](pkg/utils/secure.go:29) | 可能将错误字符串误存为加密数据 |
| bcrypt DefaultCost (10) | [`pkg/utils/secure.go`](pkg/utils/secure.go:17) | OWASP 建议新系统使用 cost 12+ |
| OAuth 回调 URL 泄露内部错误 | [`pkg/auth/login_handler.go`](pkg/auth/login_handler.go:234) | 重定向 URL 含内部错误详情 |
| Cookie 过期时间长于 JWT | [`pkg/common/common.go`](pkg/common/common.go:43) | 48h cookie vs 24h JWT，造成无效刷新 |
| RBAC URL 解析子资源列表不完整 | [`pkg/middleware/rbac.go`](pkg/middleware/rbac.go:73-97) | 新子资源可能被误解析 |
| SQLite 文件无权限限制 | [`pkg/model/model.go`](pkg/model/model.go:52) | 默认 0644，本地用户可读 |
| Docker 容器默认以 root 运行 | [`Dockerfile`](Dockerfile:38-46) | 无 `USER` 指令 |
| JWT 缺少 aud/sub 声明 | [`pkg/auth/oauth_manager.go`](pkg/auth/oauth_manager.go:85-96) | 不符合 JWT 最佳实践 |
| OAuth state 存客户端 cookie，无 PKCE | [`pkg/auth/login_handler.go`](pkg/auth/login_handler.go:65-70) | 弱于服务端存储 + PKCE |
| OAuth discovery 用未验证 HTTP | [`pkg/auth/oauth_provider.go`](pkg/auth/oauth_provider.go:71-118) | MITM 可注入恶意端点 |
| 加密密钥无法轮转 | [`pkg/utils/secure.go`](pkg/utils/secure.go) | 换密钥后旧数据不可解密 |
| Ingress TLS 默认关闭 | [`charts/kite/values.yaml`](charts/kite/values.yaml:183-195) | 启用 Ingress 不开 TLS 则明文传输 |

---

## 漏洞统计

| 严重程度 | 数量 |
|---------|------|
| 🔴 严重  | 6    |
| 🟠 高危  | 9    |
| 🟡 中危  | 12   |
| 🟢 低危  | 10   |
| **合计** | **37** |

### 按利用条件分类

| 分类 | 数量 | 漏洞编号 |
|------|------|---------|
| ⚡ 外部可利用（无需登录） | 15 | #1, #2, #3, #4, #5, #6, #8, #12, #14, #16, #18, #19, #20, #21, #28 |
| 🔒 内部漏洞（需先登录） | 22 | #7, #9, #10, #11, #13, #15, #17, #22, #23, #24, #25, #26, #27, #29, #30, #31, #32, #33, #34, #35, #36, #37 |

---

## 优先修复建议

### 第一优先级 — 立即修复（可被外部直接利用接管系统）

1. **#1** 无认证端点 — 路由移到中间件之后
2. **#2** 硬编码默认密钥 — 未设置时拒绝启动
3. **#3** OAuth 重定向操纵 — 强制设置 `HOST` 或白名单校验
4. **#18** Helm 默认加密密钥 — 默认值改为空，要求显式配置
5. **#19** cluster-admin RBAC — 缩小默认权限
6. **#20** 无 TLS — 文档强调 + Helm 默认启用 TLS

### 第二优先级 — 尽快修复（可导致账户接管/权限提升）

7. **#4** 匿名用户权限 — 改为只读
8. **#5** + **#35** 飞书回调验证 — 未配置 token 时拒绝，v1 不匹配时拒绝
9. **#21** CSRF 保护 — 添加 CSRF token
10. **#22** 会话撤销 — 实现 token 黑名单
11. **#6** API Key 常量时间比较
12. **#7** Apply 权限检查 — 增加 update 权限校验

### 第三优先级 — 短期修复（降低攻击面）

13. **#8** 速率限制
14. **#9** SSRF 白名单
15. **#11** 临时角色权限范围
16. **#23** kubectl terminal SA 清理
17. **#26** Refresh Token 服务端存储
18. **#27** WebSocket Origin 校验
19. **#28** HTTP 安全头

### 第四优先级 — 中期修复

20. **#10** RBAC 正则缓存
21. **#12** Cookie Secure 配置化
22. **#24** Node terminal 权限审计
23. **#25** Pod Security Context
24. **#29-#37** 其余中低危漏洞

---

## 重新验证详情

| # | 漏洞 | 验证文件 | 行号 | 2026-05-26 | 2026-06-14 |
|---|------|---------|------|-----------|-----------|
| 1 | 无认证端点 | `routes.go` | 74-77 | ✅ 未修复 | ✅ 未修复 |
| 2 | 硬编码默认密钥 | `pkg/common/common.go` | 13,28,37 | ✅ 未修复 | ✅ 未修复 |
| 3 | OAuth 重定向操纵 | `pkg/auth/oauth_manager.go` | 34-37 | ✅ 未修复 | ✅ 未修复 |
| 4 | 匿名用户管理员权限 | `pkg/model/user.go` | 455-470 | ✅ 未修复 | ✅ 未修复 |
| 5 | 飞书回调签名可选 | `pkg/handlers/feishu_callback_handler.go` | 50-60 | ✅ 未修复 | ✅ 未修复 |
| 6 | API Key 非常量时间比较 | `pkg/auth/middleware.go` | 41 | ✅ 未修复 | ✅ 未修复 |
| 7 | Apply 仅检查 create 权限 | `pkg/handlers/resource_apply_handler.go` | 54 vs 106 | ✅ 未修复 | ✅ 未修复 |
| 8 | 登录无速率限制 | `routes.go` | 45-46 | ✅ 未修复 | ✅ 未修复 |
| 9 | SSRF 镜像标签查询 | `pkg/handlers/image_tags_handler.go` | 66 | ✅ 未修复 | ✅ 未修复 |
| 10 | RBAC ReDoS | `pkg/rbac/rbac.go` | 207 | ✅ 未修复 | ✅ 未修复 |
| 11 | 临时角色权限过大 | `pkg/handlers/access_request_handler.go` | 62-69 | ✅ 未修复 | ✅ 未修复 |
| 12 | Cookie Secure 依赖伪造头 | `pkg/auth/login_handler.go` | 349 | ✅ 未修复 | ✅ 未修复 |
| 13 | JWT 嵌入 Refresh Token | `pkg/auth/oauth_manager.go` | 89 | ✅ 未修复 | ✅ 未修复 (→ #26) |
| 14 | /metrics 无认证 | `routes.go` | 31-34 | ✅ 未修复 | ✅ 未修复 |
| 15 | 无密码复杂度 | `pkg/handlers/user_handler.go` | 52-77 | ✅ 未修复 | ✅ 未修复 |
| 16 | 登录错误泄露用户名 | `pkg/auth/login_handler.go` | 107-110 | ✅ 未修复 | ✅ 未修复 |
| 17 | LIKE 通配符未转义 | `pkg/model/user.go` | 252-258 | ✅ 未修复 | ✅ 未修复 |
| 18 | Helm 默认加密密钥 | `charts/kite/values.yaml` | 59 | — | 🆕 新发现 |
| 19 | cluster-admin RBAC 默认 | `charts/kite/values.yaml`, `deploy/install.yaml` | 148-153 | — | 🆕 新发现 |
| 20 | 无服务端 TLS | `main.go` | 39 | — | 🆕 新发现 |
| 21 | 无 CSRF 保护 | `routes.go` | 全局 | — | 🆕 新发现 |
| 22 | 无会话撤销 | `pkg/auth/middleware.go` | 全局 | — | 🆕 新发现 |
| 23 | kubectl terminal SA 不清理 | `pkg/handlers/kubectl_terminal_handler.go` | 107-147 | — | 🆕 新发现 |
| 24 | Node terminal 特权 Pod | `pkg/handlers/node_terminal_handler.go` | 115-157 | — | 🆕 新发现 |
| 25 | 无 Pod Security Context | `charts/kite/values.yaml` | 162-173 | — | 🆕 新发现 |
| 26 | Refresh Token 嵌入 JWT | `pkg/auth/oauth_manager.go` | 89 | — | 🆕 由 #13 提升 |
| 27 | WebSocket 无 Origin 校验 | `pkg/handlers/terminal_handler.go` | 42-59 | — | 🆕 新发现 |
| 28 | 缺少 HTTP 安全头 | 全局中间件 | — | — | 🆕 新发现 |
| 29 | K8s proxy 仅检查 get | `pkg/handlers/proxy_handler.go` | 34 | — | 🆕 新发现 |
| 30 | Pod 文件路径未消毒 | `pkg/handlers/resources/pod_handler.go` | 297-498 | — | 🆕 新发现 |
| 31 | AI 会话删除无所有权检查 | `pkg/ai/handler.go` | 368-371 | — | 🆕 新发现 |
| 32 | 用户 API 返回敏感字段 | `pkg/handlers/user_handler.go` | 49,76 | — | 🆕 新发现 |
| 33 | RBAC 角色缓存延迟 | `pkg/model/user.go`, `pkg/rbac/rbac.go` | 66-94 | — | 🆕 新发现 |
| 34 | LDAP 明文连接 | `pkg/auth/ldap.go` | 60-69 | — | 🆕 新发现 |
| 35 | 飞书 v1 token 不拒绝 | `pkg/handlers/feishu_callback_handler.go` | 123-127 | — | 🆕 新发现 |
| 36 | 输入无长度/字符验证 | 多个 handler | — | — | 🆕 新发现 |
| 37 | 其他低危发现 | 多个文件 | — | — | 🆕 新发现 |
