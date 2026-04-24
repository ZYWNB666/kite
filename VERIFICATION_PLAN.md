# kite RBAC 和 Kubeconfig API 功能验证计划

## 📋 改动总结

本次改动实现了以下功能：

### 1. RBAC 精确到资源名 (ResourceNames)
- **数据库模型**：Role 添加 `ResourceNames` 字段
- **后端逻辑**：CanAccess 函数支持资源名称过滤
- **前端界面**：RBAC 对话框添加 Resource Names 输入框
- **用途**：限制用户只能访问特定名称的资源（如特定的 Pod、Deployment 等）

### 2. API Key 拉取 Kubeconfig
- **新增接口**：`GET /api/v1/proxy/kubeconfig`
- **权限控制**：仅限 API Key 用户访问
- **安全设计**：返回的 kubeconfig 必须仅保存在内存中
- **用途**：为 kite-proxy 等工具提供动态 kubeconfig

### 3. Proxy 转发权限控制
- **数据库模型**：Role 添加 `AllowProxy` 和 `ProxyNamespaces` 字段
- **后端逻辑**：新增 CanProxy 函数检查转发权限
- **前端界面**：RBAC 对话框添加 Proxy Permissions 部分
- **用途**：控制哪些用户可以使用 kite-proxy 进行端口转发

---

## ✅ 修复内容

1. ✅ 前端 Role 类型定义添加三个新字段
2. ✅ 前端 RBAC 对话框添加三个字段的输入UI
3. ✅ 默认 admin 角色添加 AllowProxy 权限

---

## 🧪 验证计划

### 准备工作

1. **启动应用**
```powershell
# 确保数据库迁移已运行（如使用SQLite，会自动创建表）
# 启动 kite 应用
go run main.go

# 或使用已编译的二进制
.\kite.exe
```

2. **确认管理员账号**
- 如果是首次启动，创建超级管理员账号
- 登录到 Web 界面：http://localhost:8080

---

### 测试 1：RBAC ResourceNames（精确到资源名）

#### 1.1 创建测试角色

1. 登录 kite Web 界面
2. 进入 **Settings → RBAC Management**
3. 点击 **Add Role**，创建角色：
   - Name: `test-pod-reader`
   - Description: `Only read specific pods`
   - Clusters: `*`
   - Namespaces: `default`
   - Resources: `pods`
   - **Resource Names**: `nginx`, `app-pod` (添加两个资源名)
   - Verbs: `get`
   - Allow Proxy: 不勾选

4. 保存并验证角色创建成功

#### 1.2 创建测试用户并分配角色

1. 进入 **Settings → User Management**
2. 创建用户：
   - Username: `testuser`
   - Password: `Test123456`
   - Provider: Password

3. 为该用户分配角色：
   - 在 RBAC Management 中找到 `test-pod-reader` 角色
   - 点击 **Assign**
   - Subject Type: `user`
   - Subject: `testuser`

#### 1.3 验证资源名过滤

1. 以 `testuser` 身份登录
2. 选择集群，进入 `default` 命名空间
3. 尝试查看 Pod 列表：
   - ✅ 应该能看到名为 `nginx` 和 `app-pod` 的 Pod
   - ❌ 不应该看到其他名称的 Pod
4. 尝试查看不在列表中的 Pod：
   - 访问其他 Pod 应该返回 403 Forbidden

**预期结果**：
- 用户只能访问 ResourceNames 列表中的资源
- 访问其他资源会被拒绝

---

### 测试 2：API Key 拉取 Kubeconfig

#### 2.1 创建 API Key

1. 以管理员身份登录
2. 进入 **Settings → API Keys**
3. 点击 **Create API Key**
   - Name: `test-proxy-key`
4. 复制生成的 API Key（格式：`kite<id>-<random>`）

#### 2.2 为 API Key 分配 Proxy 权限

1. 进入 **Settings → RBAC Management**
2. 找到 `admin` 角色（或创建新角色）
3. 确保该角色配置：
   - Allow Proxy: ✅ 勾选
   - Proxy Namespaces: `*`（或留空使用角色的 Namespaces）
4. 为 API Key 用户分配该角色：
   - Subject Type: `user`
   - Subject: `test-proxy-key`（API Key 的名称）

#### 2.3 测试 Kubeconfig API

```powershell
# 方式 1：获取所有集群的 kubeconfig
curl http://localhost:8080/api/v1/proxy/kubeconfig `
  -H "Authorization: Bearer YOUR_API_KEY" `
  | ConvertFrom-Json | ConvertTo-Json -Depth 10

# 方式 2：获取特定集群的 kubeconfig
curl "http://localhost:8080/api/v1/proxy/kubeconfig?cluster=cluster-name" `
  -H "Authorization: Bearer YOUR_API_KEY" `
  | ConvertFrom-Json | ConvertTo-Json -Depth 10
```

**预期结果**：
```json
{
  "clusters": [
    {
      "name": "cluster-1",
      "kubeconfig": "apiVersion: v1\nkind: Config\n..."
    }
  ]
}
```

#### 2.4 测试无权限场景

```powershell
# 测试 1：使用浏览器 Session（应该失败）
# 在浏览器控制台执行：
fetch('/api/v1/proxy/kubeconfig', {
  credentials: 'include'
}).then(r => r.json())

# 预期：{"error":"this endpoint is only available to API-key users"}

# 测试 2：使用无 Proxy 权限的 API Key（应该失败）
curl http://localhost:8080/api/v1/proxy/kubeconfig `
  -H "Authorization: Bearer API_KEY_WITHOUT_PROXY_PERMISSION"

# 预期：{"error":"no clusters available for proxy or proxy not permitted"}

# 测试 3：无效的 API Key（应该失败）
curl http://localhost:8080/api/v1/proxy/kubeconfig `
  -H "Authorization: Bearer invalid-key"

# 预期：401 Unauthorized
```

---

### 测试 3：Proxy 权限控制

#### 3.1 创建不同 Proxy 权限的角色

1. **全量 Proxy 角色**：
   - Name: `proxy-admin`
   - Allow Proxy: ✅
   - Proxy Namespaces: `*`

2. **受限 Proxy 角色**：
   - Name: `proxy-limited`
   - Allow Proxy: ✅
   - Proxy Namespaces: `default`, `kube-system`

3. **无 Proxy 角色**：
   - Name: `no-proxy`
   - Allow Proxy: ❌

#### 3.2 验证 Proxy 权限

为三个不同的 API Key 分配上述角色，然后测试：

```powershell
# 全量 Proxy 用户 - 应该获取所有集群
curl http://localhost:8080/api/v1/proxy/kubeconfig `
  -H "Authorization: Bearer PROXY_ADMIN_KEY"
# 预期：返回所有集群的 kubeconfig

# 受限 Proxy 用户 - 应该获取集群但命名空间受限
curl http://localhost:8080/api/v1/proxy/kubeconfig `
  -H "Authorization: Bearer PROXY_LIMITED_KEY"
# 预期：返回集群 kubeconfig，但 kite-proxy 使用时会被限制在指定命名空间

# 无 Proxy 用户 - 应该拒绝
curl http://localhost:8080/api/v1/proxy/kubeconfig `
  -H "Authorization: Bearer NO_PROXY_KEY"
# 预期：{"error":"no clusters available for proxy or proxy not permitted"}
```

---

### 测试 4：前端 UI 验证

#### 4.1 RBAC 对话框字段验证

1. 打开 RBAC 管理界面
2. 点击 **Add Role** 或编辑现有角色
3. 验证以下字段存在且可用：

**基本权限部分**：
- ✅ Clusters
- ✅ Namespaces
- ✅ Resources
- ✅ **Resource Names** (新增，可选)
- ✅ Verbs

**Proxy 权限部分**：
- ✅ **Allow proxy access via kite-proxy** (复选框)
- ✅ **Proxy Namespaces** (勾选 Allow proxy 后显示)

#### 4.2 角色创建和保存

1. 创建一个包含所有新字段的角色
2. 保存后刷新页面
3. 编辑该角色，验证所有字段值正确保存和显示

---

### 测试 5：端到端场景测试

#### 场景 1：只读用户尝试转发端口

1. 创建只读角色（Verbs: `get`, `log`，不勾选 Allow Proxy）
2. 为用户分配该角色
3. 该用户尝试获取 kubeconfig → 应该失败

#### 场景 2：特定 Pod 访问权限

1. 创建角色仅允许访问特定 Pod 名称
2. 用户尝试访问：
   - ✅ 列表中的 Pod → 成功
   - ❌ 其他 Pod → 403 Forbidden

#### 场景 3：多集群 Proxy 访问

1. 配置多个 Kubernetes 集群
2. 创建角色限制只能代理特定集群
3. 使用 API Key 获取 kubeconfig
4. 验证只返回有权限的集群

---

## 🐛 已知问题和注意事项

### 1. 数据库迁移
如果是从旧版本升级，需要确保数据库表已更新：
- `roles` 表应该有 `resource_names`、`allow_proxy`、`proxy_namespaces` 字段
- 可能需要手动运行迁移或重新初始化数据库

### 2. 缓存刷新
修改角色后，可能需要：
- 重新登录用户
- 清除应用缓存
- 或者等待缓存过期（默认 30 秒）

### 3. In-cluster 配置
如果 kite 本身运行在 Kubernetes 集群中，且使用 in-cluster 配置：
- Kubeconfig API 会跳过该集群（因为没有 kubeconfig 文件可返回）
- 这是预期行为

### 4. 前端构建
修改前端代码后需要重新构建：
```powershell
cd ui
npm install
npm run build
cd ..
```

---

## 📝 测试检查清单

### 后端功能
- [ ] RBAC 支持 ResourceNames 字段
- [ ] ResourceNames 为空时允许所有资源名
- [ ] ResourceNames 配置后正确过滤资源
- [ ] Kubeconfig API 正常返回
- [ ] Kubeconfig API 仅允许 API Key 访问
- [ ] AllowProxy 权限检查正常工作
- [ ] ProxyNamespaces 正确限制命名空间
- [ ] 无权限用户被正确拒绝

### 前端功能
- [ ] RBAC 对话框显示 Resource Names 输入框
- [ ] RBAC 对话框显示 Allow Proxy 复选框
- [ ] RBAC 对话框显示 Proxy Namespaces 输入框
- [ ] 勾选 Allow Proxy 后才显示 Proxy Namespaces
- [ ] 所有字段正确保存到数据库
- [ ] 编辑角色时所有字段正确回显

### 安全性
- [ ] 浏览器 Session 无法访问 Kubeconfig API
- [ ] 无效 API Key 被拒绝
- [ ] 无 Proxy 权限的 API Key 被拒绝
- [ ] ResourceNames 限制无法被绕过

---

## 🚀 快速验证脚本

```powershell
# 1. 创建测试 API Key（需要先通过 Web UI 创建并复制）
$apiKey = "kite<YOUR_API_KEY>"

# 2. 测试 Kubeconfig API
Write-Host "Testing Kubeconfig API..." -ForegroundColor Cyan
$response = curl http://localhost:8080/api/v1/proxy/kubeconfig `
  -H "Authorization: Bearer $apiKey" `
  --silent

if ($response) {
    Write-Host "✅ Kubeconfig API works" -ForegroundColor Green
    $response | ConvertFrom-Json | ConvertTo-Json -Depth 5
} else {
    Write-Host "❌ Kubeconfig API failed" -ForegroundColor Red
}

# 3. 测试无效访问（应该失败）
Write-Host "`nTesting invalid access..." -ForegroundColor Cyan
curl http://localhost:8080/api/v1/proxy/kubeconfig `
  -H "Authorization: Bearer invalid-key"

Write-Host "`n✅ If you see 401 or error above, security works" -ForegroundColor Green
```

---

## 📊 测试结果记录表

| 测试项 | 状态 | 备注 |
|--------|------|------|
| ResourceNames 数据库字段 | ⬜ |  |
| ResourceNames 后端逻辑 | ⬜ |  |
| ResourceNames 前端 UI | ⬜ |  |
| Kubeconfig API 端点 | ⬜ |  |
| API Key 认证检查 | ⬜ |  |
| AllowProxy 字段 | ⬜ |  |
| ProxyNamespaces 字段 | ⬜ |  |
| CanProxy 权限检查 | ⬜ |  |
| 前端 Proxy UI | ⬜ |  |
| 安全性验证 | ⬜ |  |

---

## 💡 故障排除

### 问题 1：Kubeconfig API 返回空列表

**可能原因**：
1. API Key 用户没有分配角色
2. 角色没有勾选 AllowProxy
3. 集群使用 in-cluster 配置（无 kubeconfig 可返回）

**解决方法**：
1. 检查 API Key 用户的角色分配
2. 编辑角色，确保 AllowProxy 勾选
3. 使用外部 kubeconfig 导入集群

### 问题 2：前端不显示新字段

**可能原因**：
1. 前端代码未重新编译
2. 浏览器缓存

**解决方法**：
```powershell
cd ui
npm run build
cd ..
# 硬刷新浏览器：Ctrl+Shift+R
```

### 问题 3：ResourceNames 不生效

**可能原因**：
1. 数据库表未更新
2. ResourceNames 字段为空（允许所有）

**解决方法**：
1. 检查数据库 `roles` 表是否有 `resource_names` 字段
2. 确认 ResourceNames 已正确保存（不是空数组）

---

**测试新增的 proxy 权限字段**

```bash
# 1. 检查 roles 配置文件是否支持新字段
cat config/roles.yaml  # 或您的 roles 配置文件路径
```

配置示例（确认包含以下字段）：
```yaml
roles:
  - name: proxy-user
    clusters: ["*"]
    namespaces: ["*"]
    resources: ["*"]
    verbs: ["get", "list"]
    allowProxy: true           # 新增
    proxyNamespaces: ["*"]     # 新增
    resourceNames: []          # 新增（可选）
```

**API 测试**：
```bash
# 2. 创建测试用 API key（需要有 admin 权限）
curl -X POST http://localhost:8080/api/v1/admin/api-keys \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "proxy-test-user",
    "roles": ["proxy-user"],
    "provider": "api_key"
  }'

# 记录返回的 API key
```

#### 1.2 Kubeconfig API 端点验证

```bash
# 3. 测试 kubeconfig API（使用上面创建的 API key）
curl -X GET "http://localhost:8080/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  | jq .

# 预期结果：返回用户有权限访问的集群列表及其 kubeconfig
# {
#   "clusters": [
#     {
#       "name": "cluster-1",
#       "kubeconfig": "apiVersion: v1\nkind: Config\n..."
#     }
#   ]
# }

# 4. 测试过滤特定集群
curl -X GET "http://localhost:8080/api/v1/proxy/kubeconfig?cluster=cluster-1" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  | jq .

# 5. 测试无权限用户（应返回 403）
curl -X GET "http://localhost:8080/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer INVALID_OR_NO_PROXY_PERMISSION_KEY"
# 预期结果：{"error": "no clusters available for proxy or proxy not permitted"}

# 6. 测试非 API key 用户访问（应拒绝）
curl -X GET "http://localhost:8080/api/v1/proxy/kubeconfig" \
  -H "Cookie: session=YOUR_BROWSER_SESSION_COOKIE"
# 预期结果：{"error": "this endpoint is only available to API-key users"}
```

#### 1.3 RBAC 资源名称过滤验证

```bash
# 7. 配置一个带资源名称限制的角色
# 编辑 roles.yaml 添加：
# - name: pod-reader-limited
#   clusters: ["test-cluster"]
#   namespaces: ["default"]
#   resources: ["pods"]
#   verbs: ["get"]
#   resourceNames: ["nginx-pod", "app-pod"]  # 仅允许访问这两个 pod

# 8. 重启 kite 使配置生效
# 重新加载或重启应用

# 9. 测试资源名称过滤
# 创建测试用户并分配 pod-reader-limited 角色
# 尝试访问允许的 pod（应成功）
# 尝试访问不在列表中的 pod（应失败）
```

---

### ✅ 第二阶段：kite-proxy 应用验证

#### 2.1 构建和运行 kite-proxy

```bash
# 1. 进入 kite-proxy 目录
cd kite-proxy

# 2. 构建前端
cd ui
npm install
npm run build
cd ..

# 3. 构建 Go 应用
go mod tidy
go build -o kite-proxy .

# 4. 运行 kite-proxy
./kite-proxy \
  --port 8090 \
  --kite-url http://localhost:8080 \
  --api-key YOUR_API_KEY_WITH_PROXY_PERMISSION

# 或使用环境变量
export PORT=8090
export KITE_URL=http://localhost:8080
export KITE_API_KEY=YOUR_API_KEY
./kite-proxy
```

#### 2.2 Web UI 验证

```bash
# 5. 打开浏览器访问 UI
start http://localhost:8090/ui/
```

**UI 功能检查**：
- [ ] **Configuration 页面**：可以输入/修改 kite URL 和 API key
- [ ] **Clusters 页面**：显示用户有权限访问的集群列表
- [ ] **Clusters 页面**：可以点击 "Pre-warm" 按钮预加载 kubeconfig
- [ ] **Usage 页面**：显示使用说明和 kubectl 示例命令
- [ ] **Usage 页面**：可以下载生成的 kubeconfig 文件

#### 2.3 API 功能验证

```bash
# 6. 测试配置 API
curl http://localhost:8090/api/config
# 预期：返回当前配置（kiteURL 可能为空，如果未通过 UI 设置）

# 7. 设置配置
curl -X POST http://localhost:8090/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "kiteURL": "http://localhost:8080",
    "apiKey": "YOUR_API_KEY"
  }'

# 8. 测试集群列表 API
curl http://localhost:8090/api/clusters | jq .
# 预期：返回用户有权限访问的集群列表

# 9. 测试生成 kubeconfig
curl http://localhost:8090/api/kubeconfig > generated-kubeconfig.yaml
cat generated-kubeconfig.yaml
# 预期：生成的 kubeconfig，server 地址指向 kite-proxy

# 10. 测试缓存清理
curl -X DELETE http://localhost:8090/api/cache
# 预期：{"message": "cache cleared"}

# 11. 测试预热特定集群
curl -X POST http://localhost:8090/api/cache/cluster-1
# 预期：{"message": "cache warmed for cluster cluster-1"}

# 12. 测试健康检查
curl http://localhost:8090/healthz
# 预期：{"status": "ok"}
```

#### 2.4 Kubernetes API 代理验证

```bash
# 13. 使用生成的 kubeconfig 测试 kubectl
export KUBECONFIG=generated-kubeconfig.yaml

# 14. 测试基本 kubectl 命令
kubectl get namespaces
kubectl get pods -A
kubectl get nodes

# 15. 测试切换集群（如果有多个集群）
kubectl config get-contexts
kubectl config use-context cluster-2
kubectl get pods

# 16. 验证请求确实通过 kite-proxy
# 在 kite-proxy 终端应该看到请求日志
```

#### 2.5 内存安全性验证

```bash
# 17. 验证 kubeconfig 不会写入磁盘
# 检查 kite-proxy 工作目录
ls -la kite-proxy/
# 预期：没有 kubeconfig 文件或 .kube/ 目录

# 18. 验证进程内存使用（可选）
# 使用 pprof 或其他工具检查内存中是否保存了 kubeconfig
```

---

### ✅ 第三阶段：集成测试

#### 3.1 权限边界测试

```bash
# 19. 测试无 proxy 权限的 API key
# 创建一个 allowProxy: false 的角色，生成 API key
# 尝试启动 kite-proxy 应连接失败或无法获取集群列表

# 20. 测试 namespace 限制
# 配置 proxyNamespaces: ["default", "kube-system"]
# 尝试访问其他 namespace，应该在 kite 服务器端被拦截

# 21. 测试集群权限限制
# 配置 clusters: ["cluster-1"]
# 尝试通过 kite-proxy 访问 cluster-2，应该无法获取 kubeconfig
```

#### 3.2 并发和缓存测试

```bash
# 22. 并发访问同一集群
# 开启多个终端，同时执行 kubectl 命令
# 验证 kite-proxy 日志显示复用缓存而非重复请求

# 23. 缓存失效测试
# 清除缓存后，验证下次请求会重新从 kite 获取 kubeconfig
curl -X DELETE http://localhost:8090/api/cache
kubectl get pods  # 应该看到重新获取 kubeconfig 的日志
```

#### 3.3 错误处理测试

```bash
# 24. kite 服务器不可用
# 停止 kite 服务器
# 验证 kite-proxy 返回有意义的错误信息

# 25. 无效的 API key
# 修改配置使用无效的 API key
# 验证错误提示清晰

# 26. 网络超时
# 使用防火墙或网络工具模拟延迟
# 验证超时处理正确
```

---

### ✅ 第四阶段：回归测试

#### 4.1 现有功能验证

```bash
# 27. 验证主应用（kite）的现有功能未受影响
# - 用户登录（浏览器）
# - 集群管理
# - RBAC 管理
# - 资源查看/编辑
# - 日志查看
# - 终端 (exec)
# - AI 助手

# 28. 运行自动化测试
cd e2e
npm install
npm test

# 29. 检查测试覆盖率
# 确认所有现有测试通过
```

#### 4.2 性能测试

```bash
# 30. 基准测试（可选）
# 使用 hey 或 ab 工具测试 API 性能
hey -n 1000 -c 10 http://localhost:8090/api/clusters

# 31. 内存泄漏检查
# 长时间运行 kite-proxy，监控内存使用
# 确认缓存不会无限增长
```

---

## 验证结果记录

### 主应用（kite）
- [ ] RBAC 配置加载正确
- [ ] Kubeconfig API 正常工作
- [ ] API key 认证正确
- [ ] 权限检查符合预期
- [ ] 资源名称过滤生效

### kite-proxy 应用
- [ ] 构建成功（前端 + 后端）
- [ ] 应用启动正常
- [ ] Web UI 可访问且功能正常
- [ ] API 端点响应正确
- [ ] Kubernetes API 代理工作正常
- [ ] kubeconfig 仅存在于内存
- [ ] 缓存机制工作正常
- [ ] 错误处理清晰友好

### 集成测试
- [ ] 权限边界正确执行
- [ ] 并发访问正常
- [ ] 缓存复用有效
- [ ] 错误处理完善

### 回归测试
- [ ] 现有功能未受影响
- [ ] 自动化测试全部通过
- [ ] 性能符合预期

---

## 潜在问题和注意事项

### 1. 配置文件更新
确保 `config/roles.yaml` 中至少有一个角色配置了 `allowProxy: true`，否则无法使用 kite-proxy。

示例：
```yaml
roles:
  - name: admin
    clusters: ["*"]
    namespaces: ["*"]
    resources: ["*"]
    verbs: ["*"]
    allowProxy: true
    proxyNamespaces: ["*"]
```

### 2. API Key 管理
- API key 需要通过 kite 管理界面创建
- 确保 API key 对应的角色有 `allowProxy: true`
- API key 应该妥善保管，因为它能访问 kubeconfig

### 3. 安全考虑
- kite-proxy 应该运行在可信的本地环境
- 生成的 kubeconfig 文件包含敏感信息，不应该提交到版本控制
- 考虑在生产环境使用 HTTPS

### 4. 网络配置
- 确保 kite-proxy 能访问 kite 服务器
- 如果 kite 服务器使用自签名证书，可能需要配置 TLS 跳过验证

### 5. 依赖版本
- Go 版本：检查 go.mod 中的 go 版本要求
- Node.js 版本：前端构建可能需要特定版本的 Node.js

---

## 快速验证命令汇总

```bash
# 1. 验证主应用 kubeconfig API
curl -X GET "http://localhost:8080/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer YOUR_API_KEY" | jq .

# 2. 构建并运行 kite-proxy
cd kite-proxy/ui && npm install && npm run build && cd ..
go build -o kite-proxy .
./kite-proxy --port 8090 --kite-url http://localhost:8080 --api-key YOUR_API_KEY

# 3. 测试 kite-proxy UI
start http://localhost:8090/ui/

# 4. 生成并使用 kubeconfig
curl http://localhost:8090/api/kubeconfig > test-kubeconfig.yaml
export KUBECONFIG=test-kubeconfig.yaml
kubectl get pods -A

# 5. 运行自动化测试
cd e2e && npm test
```

---

## 验证完成标准

所有以下条件满足后，可认为合并的代码已验证通过：

1. ✅ kite 主应用的新 API 端点正常工作
2. ✅ RBAC 新字段正确加载和应用
3. ✅ kite-proxy 应用能成功构建和运行
4. ✅ kite-proxy Web UI 功能完整
5. ✅ kubectl 通过 kite-proxy 能正常访问 Kubernetes 集群
6. ✅ 权限控制符合预期（有权限能访问，无权限被拒绝）
7. ✅ kubeconfig 确实只存在于内存中
8. ✅ 所有自动化测试通过
9. ✅ 现有功能未受影响

---

## 文档更新检查

- [ ] README.md 是否需要更新
- [ ] kite-proxy/README.md 是否完整
- [ ] API 文档是否需要补充 `/api/v1/proxy/kubeconfig` 端点
- [ ] 配置文档是否说明新的 RBAC 字段
- [ ] 用户指南是否包含 kite-proxy 使用说明

---

## 报告问题

如果在验证过程中发现问题，请记录以下信息：
- 问题描述
- 复现步骤
- 预期行为
- 实际行为
- 错误日志（如果有）
- 环境信息（操作系统、Go 版本、Node.js 版本等）
