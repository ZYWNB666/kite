# 快速测试指南

## 测试环境
- **端口**: 18088
- **API Key**: `kite2-99nc4ckd94mzplhkjmv9g2rjscjh74k9`
- **访问地址**: http://localhost:18088

---

## 🚀 一键测试

```bash
# 给脚本添加执行权限
chmod +x test-server.sh

# 运行测试脚本
./test-server.sh
```

---

## 📝 手动测试步骤

### 1. 测试 Kubeconfig API

```bash
# 基本测试
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer kite2-99nc4ckd94mzplhkjmv9g2rjscjh74k9" \
  | jq .

# 预期返回：
# {
#   "clusters": [
#     {
#       "name": "cluster-name",
#       "kubeconfig": "apiVersion: v1\nkind: Config\n..."
#     }
#   ]
# }
```

### 2. 测试获取特定集群

```bash
# 替换 your-cluster-name 为实际集群名称
curl "http://localhost:18088/api/v1/proxy/kubeconfig?cluster=your-cluster-name" \
  -H "Authorization: Bearer kite2-99nc4ckd94mzplhkjmv9g2rjscjh74k9" \
  | jq .
```

### 3. 测试安全性（应该失败）

```bash
# 无效 API Key
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer invalid-key" \
  | jq .

# 预期返回：{"error":"unauthorized"} 或类似错误
```

---

## 🎨 前端 UI 测试

1. **打开浏览器**: http://localhost:18088
2. **登录管理员账号**
3. **进入 Settings → RBAC Management**
4. **点击 Add Role**

### ✅ 检查点

#### Permissions 部分
- [ ] Clusters 输入框
- [ ] Namespaces 输入框
- [ ] Resources 输入框
- [ ] **Resource Names (Optional)** 输入框 ⬅️ 新增
- [ ] Verbs 输入框

#### Proxy Permissions 部分 ⬅️ 新增
- [ ] **Allow proxy access via kite-proxy** 复选框
- [ ] 勾选后显示 **Proxy Namespaces (Optional)** 输入框

---

## 🧪 功能测试

### 测试 1：创建带 ResourceNames 的角色

1. **创建角色**:
   - Name: `test-specific-pods`
   - Clusters: `*`
   - Namespaces: `default`
   - Resources: `pods`
   - **Resource Names**: `nginx`, `app-pod` ⬅️ 输入具体 Pod 名称
   - Verbs: `get`

2. **保存并刷新页面**
3. **编辑该角色，验证 Resource Names 正确回显**

### 测试 2：创建带 Proxy 权限的角色

1. **创建角色**:
   - Name: `test-proxy-role`
   - Clusters: `*`
   - Namespaces: `*`
   - Resources: `*`
   - Verbs: `get`, `list`
   - **Allow Proxy**: ✅ 勾选
   - **Proxy Namespaces**: `default`, `kube-system`

2. **保存并验证**

### 测试 3：验证 API Key 的角色分配

1. **Settings → API Keys**
2. **找到名为 `kite2-99nc...` 的 API Key**
3. **检查分配的角色**（应该有 admin 或其他带 AllowProxy 的角色）

---

## ❗ 常见问题

### Q1: Kubeconfig API 返回空列表

**返回**: `{"error":"no clusters available for proxy or proxy not permitted"}`

**原因**:
- API Key 用户没有分配角色
- 角色没有勾选 AllowProxy
- 没有配置集群或集群使用 in-cluster 配置

**解决**:
1. Settings → RBAC Management → 找到 admin 角色
2. 编辑角色，确保勾选 **Allow proxy access**
3. Settings → API Keys → 检查 API Key 是否分配了该角色

### Q2: 前端不显示新字段

**原因**: 前端未重新编译

**解决**:
```bash
cd ui
npm run build
cd ..
# 重启应用或硬刷新浏览器 Ctrl+Shift+R
```

### Q3: 无法访问应用

**检查**:
```bash
# 检查应用是否运行
curl http://localhost:18088/api/v1/healthz

# 检查端口是否被占用
lsof -i :18088  # Linux/Mac
# 或
netstat -ano | findstr 18088  # Windows
```

---

## 📊 验证清单

### 后端功能
- [ ] 应用正常运行（端口 18088）
- [ ] Kubeconfig API 返回数据
- [ ] 无效 API Key 被拒绝
- [ ] admin 角色有 AllowProxy 权限

### 前端功能
- [ ] RBAC 对话框显示 Resource Names 字段
- [ ] RBAC 对话框显示 Proxy Permissions 部分
- [ ] 创建角色时可以配置三个新字段
- [ ] 编辑角色时三个新字段正确回显

### 功能验证
- [ ] ResourceNames 权限过滤正常工作
- [ ] AllowProxy 权限检查正常工作
- [ ] ProxyNamespaces 配置正确保存

---

## 🔍 详细测试

完整的测试计划请参考：
- **[SERVER_TEST_PLAN.md](./SERVER_TEST_PLAN.md)** - 详细的服务端测试
- **[CODE_REVIEW_SUMMARY.md](./CODE_REVIEW_SUMMARY.md)** - 修复总结

---

## 💡 测试技巧

### 使用 jq 美化输出
```bash
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer kite2-99nc4ckd94mzplhkjmv9g2rjscjh74k9" \
  | jq '.'
```

### 查看详细错误信息
```bash
curl -v "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer kite2-99nc4ckd94mzplhkjmv9g2rjscjh74k9"
```

### 保存 kubeconfig 到文件
```bash
curl "http://localhost:18088/api/v1/proxy/kubeconfig" \
  -H "Authorization: Bearer kite2-99nc4ckd94mzplhkjmv9g2rjscjh74k9" \
  | jq -r '.clusters[0].kubeconfig' > test-kubeconfig.yaml
```

---

**测试时间**: 约 15-20 分钟
**必需工具**: curl, jq, 浏览器
**测试顺序**: 后端 API → 前端 UI → 功能验证
