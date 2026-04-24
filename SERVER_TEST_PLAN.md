# Kite 服务端功能测试计划

## 📋 改动总结

本次实现了三个核心服务端功能：

### 1. ✅ RBAC 精确到资源名 (ResourceNames)
- **后端**：Role 模型添加 `ResourceNames` 字段，CanAccess 函数支持资源名过滤
- **前端**：RBAC 对话框添加 Resource Names 输入框
- **用途**：限制用户只能访问特定名称的资源（如特定的 Pod）

### 2. ✅ API Key 拉取 Kubeconfig
- **新接口**：`GET /api/v1/proxy/kubeconfig`
- **权限控制**：仅限 API Key 用户访问，需要 AllowProxy 权限
- **用途**：供 kite-proxy 等工具动态获取 kubeconfig

### 3. ✅ Proxy 转发权限控制
- **后端**：Role 模型添加 `AllowProxy` 和 `ProxyNamespaces` 字段，新增 CanProxy 函数
- **前端**：RBAC 对话框添加 Proxy Permissions 部分
- **用途**：控制用户是否可以通过 kite-proxy 转发

---

## 🔧 已修复问题

1. ✅ 前端 Role 类型定义添加三个新字段（resourceNames, allowProxy, proxyNamespaces）
2. ✅ 前端 RBAC 对话框添加三个字段的输入UI
3. ✅ 默认 admin 角色添加 AllowProxy = true

---

## 🧪 服务端测试方案

### 准备工作

```bash
# 1. 重新构建前端（修改了UI）
cd ui
npm install
npm run build
cd ..

# 2. 启动应用
go run main.go
# 或使用已编译的二进制
# ./kite

# 3. 打开浏览器
xdg-open http://localhost:18088
# 或 macOS: open http://localhost:18088
```

---

## 测试 1：RBAC ResourceNames（精确到资源名）

### 1.1 通过 Web UI 创建测试角色

1. **登录管理员账号**
2. **进入 Settings → RBAC Management**
3. **点击 Add Role**，创建：
   ```
   Name: test-specific-pods
   Clusters: *
   Namespaces: default
   Resources: pods
   Resource Names: nginx, app-pod  ⬅️ 新增字段
   Verbs: get
   Allow Proxy: 不勾选
   ```

### 1.2 创建测试用户并分配角色

1. **Settings → User Management → Create User**
   ```
   Username: testuser
   Password: Test123456
   ```

2. **Settings → RBAC Management → 找到 test-specific-pods 角色**
3. **点击 Assign 按钮**
   ```
   Subject Type: user
   Subject: testuser
   ```

### 1.3 验证资源名过滤

**方式 1：通过 Web UI**
1. 退出登录
2. 以 `testuser` / `Test123456` 登录
3. 选择集群 → default 命名空间 → Pods
4. **预期结果**：
   - ✅ 只能看到名为 `nginx` 和 `app-pod` 的 Pod
   - ❌ 看不到其他名称的 Pod
   - 尝试访问其他 Pod 应返回 403 错误

**方式 2：通过 API（需要先有测试 Pod）**
```bash
# 在集群中创建测试 Pod
kubectl run nginx --image=nginx
kubectl run test-pod --image=nginx
kubectl run app-pod --image=nginx

# 以 testuser 登录获取 token（需要从浏览器开发者工具获取）
# 或者使用 password login API
response=$(curl -X POST http://localhost:18088/api/auth/login/password \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"Test123456"}' \
  --silent)

token=$(echo $response | jq -r '.token')

# 测试访问允许的 Pod（应该成功）
curl "http://localhost:18088/api/v1/pods/default/nginx" \
  -H "Authorization: Bearer $token"

# 测试访问不允许的 Pod（应该失败 403）
curl "http://localhost:18088/api/v1/pods/default/test-pod" \
  -H "Authorization: Bearer $token"
```

**✅ 测试通过标准**：
- 用户只能访问 ResourceNames 列表中的资源
- 访问其他资源返回 403 Forbidden

---

## 测试 2：API Key 拉取 Kubeconfig

### 2.1 创建 API Key

1. **以管理员登录**
2. **Settings → API Keys → Create API Key**
   ```
   Name: test-proxy-key
   ```
3. **复制生成的 API Key**（格式：`kite<id>-<random>`）
   ```
   例如：kite1-abc123def456...
   ```

### 2.2 验证 API Key 有 Proxy 权限

1. **Settings → RBAC Management**
2. **找到 admin 角色，点击编辑**
3. **确认配置**：
   ```
   Allow proxy access: ✅ 勾选
   Proxy Namespaces: *（或留空）
   ```
4. **确认 API Key 用户已分配 admin 角色**
   - API Key 用户名为创建时的 Name：`test-proxy-key`
   - 在 admin 角色的 Assignments 中应该能看到

### 2.3 测试 Kubeconfig API

```bash
# 设置 API Key 变量
apiKey="kite2-99nc4ckd94mzplhkjmv9g2rjscjh74k9"

# 测试 1：获取所有集群的 kubeconfig
echo "Testing Kubeconfig API..."
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer $apiKey" \
  --silent | jq .

# 预期返回：
# {
#   "clusters": [
#     {
#       "name": "cluster-1",
#       "kubeconfig": "apiVersion: v1\nkind: Config\n..."
#     }
#   ]
# }

# 测试 2：获取特定集群
curl "http://localhost:18088/api/v1/proxy/kubeconfig?cluster=your-cluster-name" \
  -H "Authorization: Bearer $apiKey" \
  --silent | jq .
```

### 2.4 测试权限拒绝场景

```bash
# 场景 1：浏览器 Session 访问（应该失败）
# 在浏览器控制台执行（登录后）：
fetch('/api/v1/proxy/kubeconfig', {
  credentials: 'include'
}).then(r => r.json()).then(console.log)

# 预期：{"error":"this endpoint is only available to API-key users"}

# 场景 2：无效 API Key（应该失败）
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer invalid-key-12345"

# 预期：401 Unauthorized 或 {"error":"unauthorized"}

# 场景 3：无 Proxy 权限的 API Key
# 创建新角色（不勾选 Allow Proxy）
# 创建新 API Key 并分配该角色
# 使用该 API Key 访问
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer NO_PROXY_PERMISSION_KEY"

# 预期：{"error":"no clusters available for proxy or proxy not permitted"}
```

**✅ 测试通过标准**：
- 有 Proxy 权限的 API Key 能获取 kubeconfig
- 浏览器 Session 被拒绝
- 无效或无权限的 API Key 被拒绝

---

## 测试 3：Proxy 权限控制

### 3.1 创建不同 Proxy 权限的角色

**角色 1：全量 Proxy**
```
Name: proxy-admin
Clusters: *
Namespaces: *
Resources: *
Verbs: get, list
Allow Proxy: ✅
Proxy Namespaces: *
```

**角色 2：受限 Proxy**
```
Name: proxy-limited
Clusters: *
Namespaces: default, kube-system
Resources: *
Verbs: get, list
Allow Proxy: ✅
Proxy Namespaces: default
```

**角色 3：无 Proxy**
```
Name: no-proxy
Clusters: *
Namespaces: *
Resources: *
Verbs: get
Allow Proxy: ❌（不勾选）
```

### 3.2 创建三个 API Key 并测试

```bash
# 创建三个 API Key，分别分配上述角色
# API Key 1: proxy-admin-key -> proxy-admin 角色
# API Key 2: proxy-limited-key -> proxy-limited 角色
# API Key 3: no-proxy-key -> no-proxy 角色

# 测试全量 Proxy（应该成功）
key1="kite2-..."
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer $key1"
# 预期：返回所有集群的 kubeconfig

# 测试受限 Proxy（应该成功但命名空间受限）
key2="kite3-..."
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer $key2"
# 预期：返回 kubeconfig，但实际使用时会被限制在 default 命名空间

# 测试无 Proxy（应该失败）
key3="kite4-..."
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer $key3"
# 预期：{"error":"no clusters available for proxy or proxy not permitted"}
```

**✅ 测试通过标准**：
- proxy-admin 角色能获取所有集群和命名空间
- proxy-limited 角色能获取 kubeconfig 但命名空间受限
- no-proxy 角色无法获取 kubeconfig

---

## 测试 4：前端 UI 验证

### 4.1 RBAC 对话框字段检查

1. **打开 Settings → RBAC Management**
2. **点击 Add Role**
3. **验证以下字段存在**：

**Permissions 部分**：
- ✅ Clusters
- ✅ Namespaces
- ✅ Resources
- ✅ **Resource Names (Optional)** ⬅️ 新增
- ✅ Verbs

**Proxy Permissions 部分**（新增）：
- ✅ **Allow proxy access via kite-proxy** (checkbox)
- ✅ **Proxy Namespaces (Optional)** (勾选上面后显示)

### 4.2 数据保存和回显验证

1. **创建包含所有新字段的角色**：
   ```
   Resource Names: test-pod-1, test-pod-2
   Allow Proxy: ✅
   Proxy Namespaces: default, production
   ```

2. **保存后刷新页面**
3. **编辑该角色**
4. **验证所有字段正确回显**

**✅ 测试通过标准**：
- 所有新字段正确显示在 UI 中
- 保存的值正确回显
- Proxy Namespaces 只在勾选 Allow Proxy 后显示

---

## 测试 5：数据库验证（可选）

```bash
# 如果使用 SQLite
sqlite3 kite.db

# 查看 roles 表结构
.schema roles

# 应该包含以下字段：
# resource_names TEXT
# allow_proxy BOOLEAN NOT NULL DEFAULT 0
# proxy_namespaces TEXT

# 查看实际数据
SELECT id, name, resource_names, allow_proxy, proxy_namespaces FROM roles;

# 验证 admin 角色的 allow_proxy 为 1（true）
SELECT name, allow_proxy FROM roles WHERE name='admin';

# 退出
.quit
```

---

## 📊 测试结果记录表

| 测试项 | 状态 | 备注 |
|--------|------|------|
| ResourceNames 字段显示在 UI | ⬜ |  |
| ResourceNames 正确保存到数据库 | ⬜ |  |
| ResourceNames 权限过滤生效 | ⬜ |  |
| Kubeconfig API 端点可访问 | ⬜ |  |
| API Key 认证检查 | ⬜ |  |
| 浏览器 Session 被拒绝 | ⬜ |  |
| AllowProxy 字段显示在 UI | ⬜ |  |
| ProxyNamespaces 字段显示在 UI | ⬜ |  |
| CanProxy 权限检查生效 | ⬜ |  |
| 无 Proxy 权限用户被拒绝 | ⬜ |  |
| admin 角色默认有 AllowProxy | ⬜ |  |

---

## 💡 常见问题排查

### 问题 1：前端不显示新字段

**原因**：前端代码未重新编译

**解决**：
```bash
cd ui
npm run build
cd ..
# 重启应用或硬刷新浏览器 Ctrl+Shift+R
```

### 问题 2：Kubeconfig API 返回空列表

**可能原因**：
1. API Key 用户没有分配角色
2. 角色的 AllowProxy 未勾选
3. 集群使用 in-cluster 配置（无 kubeconfig 可返回）

**排查步骤**：
```powershell
# 1. 检查 API Key 用户的角色
# 在 Web UI 中查看 API Key 名称对应的用户是否有角色分配

# 2. 检查角色配置
# 在 RBAC Management 中查看分配的角色是否勾选了 Allow Proxy

# 3. 检查集群配置
# Settings → Cluster Management → 查看集群的 kubeconfig 是否存在
```

### 问题 3：ResourceNames 不生效

**原因**：ResourceNames 字段为空或为 `["*"]` 时允许所有

**解决**：确保 ResourceNames 包含具体的资源名称，不要使用 `*`

### 问题 4：数据库字段缺失

**原因**：数据库未迁移或使用旧版本的表结构

**解决**：
```bash
# 方式 1：删除数据库重新初始化（测试环境）
rm kite.db
go run main.go

# 方式 2：手动添加字段（生产环境）
sqlite3 kite.db << EOF
ALTER TABLE roles ADD COLUMN resource_names TEXT;
ALTER TABLE roles ADD COLUMN allow_proxy BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE roles ADD COLUMN proxy_namespaces TEXT;
.quit
EOF
```

---

## 🎯 快速验证脚本

```bash
#!/bin/bash

# 完整的快速验证脚本
echo -e "\033[32m=== Kite 服务端功能快速验证 ===\033[0m"

# 1. 检查应用是否运行
echo -e "\n\033[36m[1/4] 检查应用状态...\033[0m"
health=$(curl http://localhost:18088/api/v1/healthz --silent 2>/dev/null)
if echo "$health" | grep -q "ok"; then
    echo -e "\033[32m✅ 应用正常运行\033[0m"
else
    echo -e "\033[31m❌ 应用未启动，请先启动 kite\033[0m"
    exit 1
fi

# 2. 测试 Kubeconfig API
echo -e "\n\033[36m[2/4] 测试 Kubeconfig API...\033[0m"
apiKey="kite2-99nc4ckd94mzplhkjmv9g2rjscjh74k9"
echo -e "\033[33m使用 API Key: $apiKey\033[0m"

result=$(curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer $apiKey" \
  --silent 2>/dev/null)

if echo "$result" | jq -e '.clusters' >/dev/null 2>&1; then
    echo -e "\033[32m✅ Kubeconfig API 正常工作\033[0m"
    cluster_count=$(echo "$result" | jq '.clusters | length')
    echo -e "\033[90m可访问集群数量: $cluster_count\033[0m"
    echo "$result" | jq .
elif echo "$result" | jq -e '.error' >/dev/null 2>&1; then
    error_msg=$(echo "$result" | jq -r '.error')
    echo -e "\033[33m⚠️  返回错误: $error_msg\033[0m"
else
    echo -e "\033[31m❌ API 返回异常\033[0m"
    echo "$result"
fi

# 3. 检查前端界面
echo -e "\n\033[36m[3/4] 打开前端界面...\033[0m"
echo -e "\033[33m请手动验证:\033[0m"
echo -e "\033[90m  1. Settings → RBAC Management → Add Role\033[0m"
echo -e "\033[90m  2. 检查是否有 'Resource Names' 输入框\033[0m"
echo -e "\033[90m  3. 检查是否有 'Proxy Permissions' 部分\033[0m"

# 根据操作系统打开浏览器
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:18088
elif command -v open &> /dev/null; then
    open http://localhost:18088
else
    echo "请手动打开: http://localhost:18088"
fi

# 4. 总结
echo -e "\n\033[36m[4/4] 验证完成\033[0m"
echo -e "\033[33m请查看上述结果并手动测试 UI 功能\033[0m"
echo -e "\n\033[90m详细测试步骤请参考 SERVER_TEST_PLAN.md\033[0m"
```

---

## ✅ 验证完成标准

所有以下条件满足后，服务端功能验证通过：

1. ✅ 前端 RBAC 对话框显示三个新字段
2. ✅ 创建角色时可以配置 ResourceNames、AllowProxy、ProxyNamespaces
3. ✅ ResourceNames 权限过滤正确工作
4. ✅ Kubeconfig API 返回正确数据
5. ✅ API Key 认证和权限检查正常
6. ✅ 浏览器 Session 无法访问 Kubeconfig API
7. ✅ 无 Proxy 权限的用户被正确拒绝
8. ✅ admin 角色默认有 AllowProxy 权限
9. ✅ 所有字段正确保存和回显

---

## 📝 下一步

服务端验证通过后，可以开始开发和测试客户端（kite-proxy）。

客户端开发要求：
1. 使用 Kubeconfig API 获取配置
2. Kubeconfig 只保存在内存中
3. 多个连接复用同一个 kubeconfig
4. 提供简单的前端界面（可选）
