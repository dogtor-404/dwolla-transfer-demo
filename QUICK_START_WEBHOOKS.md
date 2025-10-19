# Quick Start Guide - Dwolla Webhooks

## 快速开始指南

### 1️⃣ 安装 ngrok

```bash
brew install ngrok/ngrok/ngrok
```

### 2️⃣ 配置 ngrok

访问 https://dashboard.ngrok.com/get-started/your-authtoken 获取 token

```bash
ngrok config add-authtoken YOUR_AUTHTOKEN_HERE
```

### 3️⃣ 生成 Webhook Secret

```bash
openssl rand -base64 32
```

复制输出的字符串

### 4️⃣ 更新 .env 文件

在 `dwolla-transfer-demo/.env` 中添加：

```bash
DWOLLA_WEBHOOK_SECRET=刚才生成的字符串
WEBHOOK_BASE_URL=https://your-ngrok-url.ngrok.io
```

### 5️⃣ 启动服务

**终端 1** - Plaid Service:
```bash
cd plaid-quickstart/go
go run server.go
```

**终端 2** - Dwolla Service:
```bash
cd dwolla-transfer-demo
go run server.go
```

### 6️⃣ 运行 Webhook 测试

**终端 3** - Run Test:
```bash
cd dwolla-transfer-demo
./test_webhook.sh
```

## 测试结果

测试脚本会：
- ✅ 自动启动 ngrok
- ✅ 自动注册 webhook
- ✅ 创建测试客户和账户
- ✅ 执行转账
- ✅ 等待 webhook 通知
- ✅ 自动清理资源

在 **终端 2** (Dwolla Service) 中，你会看到实时的 webhook 通知：

```
============================================================
🔔 WEBHOOK RECEIVED at 2024-10-19 22:30:45
============================================================
Event ID:  abc-123-def
Topic:     transfer_completed
Timestamp: 2024-10-19T22:30:45.000Z
Resource:  https://api-sandbox.dwolla.com/transfers/xxx
✅ Transfer completed successfully!
============================================================
```

## 手动测试

如果你想手动测试：

### 1. 启动 ngrok
```bash
ngrok http 8001
```

复制显示的 HTTPS URL（例如：`https://abc123.ngrok.io`）

### 2. 更新 .env
将复制的 URL 更新到 `.env` 文件中的 `WEBHOOK_BASE_URL`

### 3. 重启 Dwolla 服务
```bash
go run server.go
```

### 4. 注册 webhook
```bash
curl -X POST http://localhost:8001/api/dwolla/webhook-subscription \
  -H "Content-Type: application/json" \
  -d '{}'
```

### 5. 创建转账并观察 webhook
运行 `test_flow.sh` 或手动创建转账，然后在 Dwolla 服务终端查看 webhook 通知

## 常见问题

### Q: ngrok URL 每次都会变？
**A**: 免费版 ngrok 每次重启都会生成新的 URL。你需要：
- 每次重启 ngrok 后更新 `.env` 文件中的 `WEBHOOK_BASE_URL`
- 重启 Dwolla 服务
- 或者升级到 ngrok 付费版获取固定域名

### Q: 没有收到 webhook？
**A**: 检查：
1. ngrok 是否在运行：`curl http://localhost:4040/api/tunnels`
2. `.env` 中的 `WEBHOOK_BASE_URL` 是否正确
3. Dwolla 服务是否成功注册了 webhook
4. Sandbox 环境的 webhook 可能有几秒延迟

### Q: Signature verification failed？
**A**: 确保：
1. `DWOLLA_WEBHOOK_SECRET` 与注册时使用的一致
2. Secret 没有额外的空格或换行符
3. 重启了 Dwolla 服务来加载新的环境变量

## 查看当前的 Webhook 订阅

```bash
curl http://localhost:8001/api/dwolla/webhook-subscriptions
```

## 删除 Webhook 订阅

```bash
curl -X DELETE http://localhost:8001/api/dwolla/webhook-subscription/SUBSCRIPTION_ID
```

## 支持的事件类型

- `transfer_completed` - 转账成功 ✅
- `transfer_failed` - 转账失败 ❌
- `transfer_cancelled` - 转账取消 ⚠️
- `customer_created` - 客户创建 👤
- `customer_funding_source_added` - 银行账户添加 🏦
- 更多事件...

## 需要帮助？

查看完整文档：
- `README.md` - 完整的使用指南
- `WEBHOOK_IMPLEMENTATION.md` - 实现细节
- [Dwolla Webhooks 文档](https://developers.dwolla.com/docs/balance/webhooks)

