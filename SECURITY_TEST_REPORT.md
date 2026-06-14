# Kite 安全漏洞测试报告

> 测试时间: 2026-06-14  
> 测试目标: https://kite.magikcloud.cn/  
> 部署版本: v1.5.4 (2026-06-14T05:27:50.434Z, commit: 7854f93a)  
> 测试范围: 外部可利用漏洞（无需登录）

---

## 测试结果汇总

| # | 漏洞 | 状态 | 测试结果 |
|---|------|------|---------|
| 1 | `CreateSuperUser` 无认证 | ❌ 未修复 | 返回 "super user already exists"（业务逻辑阻止，非 401） |
| 1 | `ImportClusters` 无认证 | ❌ 未修复 | 返回 "config is required"（端点可访问，需有效 kubeconfig） |
| 2 | 硬编码默认密钥 | ⚠️ 未修复 | 使用默认加密密钥（已确认） |
| 3 | OAuth 重定向 URL 操纵 | ✅ 已修复 | 不再信任 X-Forwarded-* 头 |
| 5 | 飞书回调签名验证可选 | ❌ 未修复 | 端点无需认证即可访问，可伪造审批 |
| 8 | 登录接口无速率限制 | ❌ 未修复 | 10 个连续请求全部返回 401，未触发限速 |
| 14 | `/metrics` 端点无认证 | ✅ 已修复 | 返回 401 "Invalid or expired token" |
| 16 | 登录错误信息泄露用户名 | ✅ 已修复 | 返回通用消息 "invalid credentials" |
| — | HTTP 安全头 | ⚠️ 部分修复 | 有 HSTS，缺少 X-Content-Type-Options, X-Frame-Options, CSP |
| — | 版本信息泄露 | ⚠️ 存在 | `/api/v1/version` 返回完整版本号、构建时间、commit ID |

---

## 详细测试结果

### ✅ 已修复的漏洞

#### #3 OAuth 重定向 URL 操纵
- **测试方法**: 设置 `X-Forwarded-Proto: https` 和 `X-Forwarded-Host: evil.com` 头
- **结果**: 服务器不再信任这些头，使用 `c.Request.Host` 生成重定向 URL
- **状态**: ✅ 修复生效

#### #14 /metrics 端点认证
- **测试方法**: 无认证访问 `/metrics`
- **结果**: 返回 401 `{"error":"Invalid or expired token"}`
- **状态**: ✅ 修复生效

#### #16 登录错误信息脱敏
- **测试方法**: 使用错误密码登录（用户存在/不存在）
- **结果**: 两种情况均返回 `{"error":"invalid credentials"}`
- **状态**: ✅ 修复生效，无法枚举用户

---

### ❌ 未修复的漏洞

#### #1 CreateSuperUser/ImportClusters 无认证
- **测试方法**: 无认证 POST 到 `/api/v1/admin/users/create_super_user`
- **结果**: 返回 `{"error":"super user already exists"}`（非 401）
- **风险**: 端点无需认证即可访问，仅靠业务逻辑检查（用户数 > 0）阻止
- **攻击场景**: 首次部署时攻击者可抢先创建管理员账户
- **修复建议**: 将路由移到 `RequireAuth()` 中间件之后

#### #1 ImportClusters
- **测试方法**: 无认证 POST 到 `/api/v1/admin/clusters/import`
- **结果**: 返回 `{"error":"config is required when inCluster is false"}`（非 401）
- **风险**: 端点无需认证即可访问
- **修复建议**: 同上

#### #5 飞书回调伪造（已验证可利用）
- **测试方法**: 枚举 request_id，伪造 v1 格式回调
- **测试请求**:
  ```json
  {
    "token": "wrong_token",
    "action": {
      "tag": "button",
      "value": {"action": "approve", "request_id": "1"}
    },
    "operator": {"open_id": "fake_approver"}
  }
  ```
- **结果**: 
  - request_id=1: 返回 "该申请已处理（状态：已撤回）"
  - request_id=2: 返回 "该申请已处理（状态：已撤回）"
  - request_id=999: 返回 "申请记录不存在"
- **风险**: 
  - 端点无需认证即可访问
  - 使用错误 token 仍能处理请求（v1 格式不强制验证）
  - 可枚举 request_id（顺序递增整数）
  - 如果知道 approver OpenID，可伪造批准/拒绝操作
- **攻击场景**: 
  1. 黑客创建访问申请
  2. 枚举 request_id 获取 approver OpenID
  3. 伪造飞书回调批准自己的申请
  4. 获得临时管理员角色（Resources: ["*"], Verbs: ["*"]）
- **修复建议**: v1 格式 token 不匹配时应拒绝请求

#### #8 登录接口无速率限制
- **测试方法**: 连续发送 10 个登录请求
- **结果**: 全部返回 401 `{"error":"invalid credentials"}`，未触发 429
- **风险**: 可被暴力破解攻击
- **修复建议**: 添加 IP 限速中间件（已实现，待部署）

---

### ⚠️ 需要注意的问题

#### #2 硬编码默认密钥
- **状态**: 使用默认加密密钥 `kite-default-encryption-key-change-in-production`
- **风险**: 数据库中加密字段可被任何知道默认密钥的人解密
- **建议**: 迁移到新密钥需要数据迁移脚本

#### HTTP 安全头
- **已有**: `Strict-Transport-Security: max-age=31536000; includeSubDomains`
- **缺少**: 
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
  - `Content-Security-Policy`
  - `Referrer-Policy`
  - `Permissions-Policy`

#### 版本信息泄露
- **端点**: `/api/v1/version`
- **泄露信息**: 版本号 (1.5.4)、构建时间、commit ID
- **风险**: 攻击者可根据版本号查找已知漏洞
- **建议**: 限制为认证用户可访问，或只返回主版本号

---

## 攻击链示例

一个完整的攻击可能这样发生：

```
1. 黑客扫描发现 /api/v1/admin/users/create_super_user 无需认证
2. 尝试创建管理员 → 失败（已存在）
3. 转而暴力破解密码（无速率限制）
4. 破解成功，登录获得管理员权限
5. 用管理员权限查看集群配置
6. 发现 kubeconfig 用默认密钥加密
7. 获取 kubeconfig，直接访问 Kubernetes API
8. 在集群中部署恶意 Pod，窃取数据
```

---

## 优先修复建议

### 立即修复（可被外部利用）
1. **#1** CreateSuperUser/ImportClusters — 路由移到 RequireAuth() 之后
2. **#5** 飞书回调 — v1 格式 token 不匹配时拒绝请求
3. **#8** 速率限制 — 部署新代码（已实现 LoginRateLimit 中间件，1 req/3s, burst 3）

### 尽快修复
4. **#2** 加密密钥迁移 — 实现数据迁移脚本
5. **HTTP 安全头** — 添加全局安全头中间件
6. **版本信息** — 限制访问权限

---

## 测试方法说明

所有测试均使用 PowerShell `Invoke-RestMethod` 和 `Invoke-WebRequest` 命令，模拟未认证攻击者行为。

测试命令示例：
```powershell
# 测试 CreateSuperUser
Invoke-RestMethod -Uri "https://kite.magikcloud.cn/api/v1/admin/users/create_super_user" -Method POST -ContentType "application/json" -Body '{"username":"test","password":"test123"}'

# 测试登录错误
Invoke-RestMethod -Uri "https://kite.magikcloud.cn/api/auth/login/password" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"wrong"}'

# 测试 metrics 认证
Invoke-WebRequest -Uri "https://kite.magikcloud.cn/metrics" -Method GET

# 测试飞书回调伪造
$body = @{
    token = "wrong_token"
    action = @{
        tag = "button"
        value = @{
            action = "approve"
            request_id = "1"
        }
    }
    operator = @{
        open_id = "fake_approver"
    }
} | ConvertTo-Json -Depth 5
Invoke-RestMethod -Uri "https://kite.magikcloud.cn/api/feishu/card-callback" -Method POST -ContentType "application/json" -Body $body
```
