# PokerNode

PokerNode 是一个最多 8 人的朋友局德州扑克系统：Go 服务端负责发牌、行动校验和结算，React 前端负责牌桌交互，并通过 New API 管理玩家余额。

> 页面中的 `$` 是 New API quota 的换算显示，不代表法币。默认 `500,000 quota = $1.00`。

## 预览

![PokerNode 四人演示牌局](docs/assets/pokernode-table-demo.png)

_演示账号、频道名称和账户余额已脱敏。_

## 主要功能

- 大厅、频道、邀请码和多牌桌
- No-Limit Hold'em、全下、边池和摊牌结算
- 全员准备后自动开局，行动时限可设为 5–300 秒
- New API 自动绑定、买入扣款和离桌返还
- 账号、角色、加密凭证和资金记录
- PostgreSQL 持久化、WebSocket 实时同步和移动端适配

详细规则见 [德州扑克规则](docs/POKER_RULES.md)。

## Docker 部署

服务器只需要 Docker Compose。默认从 GHCR 拉取 `linux/amd64`、`linux/arm64` 镜像。

1. 创建环境文件：

   ```powershell
   Copy-Item .env.example .env
   ```

2. 至少填写以下三项：

   ```dotenv
   DATABASE_URL=postgres://pokernode:数据库密码@postgres.example:5432/pokernode?sslmode=require
   POKERNODE_SESSION_SECRET=至少32字符的随机值
   POKERNODE_ENCRYPTION_KEY=Base64编码的32字节随机值
   ```

   加密密钥投入使用后不要随意更换，否则已保存的 New API 凭证将无法解密。

3. 启动并检查：

   ```powershell
   docker compose up -d
   docker compose ps
   Invoke-RestMethod http://localhost:8080/readyz
   ```

打开 [http://localhost:8080](http://localhost:8080)。升级时运行：

```powershell
docker compose pull
docker compose up -d
```

端口、监听地址和可信来源可通过 `.env` 中的 `POKERNODE_PORT`、`POKERNODE_BIND_ADDRESS`、`POKERNODE_TRUSTED_ORIGINS` 调整。

## 首次使用

1. 第一个注册账号会成为超级管理员。
2. 创建频道，填写 New API 地址和管理员 **System Access Token**。
3. 创建牌桌并把频道邀请码发给玩家。
4. 玩家加入频道、买入、准备，随后自动开局。

管理员 Token 必须有用户管理和 quota 调整权限，普通模型调用密钥不能代替它。

## 可选：微信登录

需要在 `.env` 中同时设置：

```dotenv
WECHAT_APP_ID=网站应用AppID
WECHAT_APP_SECRET=网站应用AppSecret
WECHAT_REDIRECT_URI=https://你的域名/api/auth/wechat/callback
```

不设置时，账号密码登录仍可正常使用。

## 本地开发

需要 Go 1.25、Node.js 24 和 pnpm。Windows 可直接运行：

```powershell
.\start-dev.bat
```

常用检查：

```powershell
go test ./...
pnpm --dir web build
```

从当前源码构建 Docker 镜像：

```powershell
docker compose -f compose.yaml -f compose.build.yaml up -d --build
```

## 生产提醒

- 使用 HTTPS、PostgreSQL TLS 和定期备份。
- 妥善保存 Session Secret、Encryption Key 和 System Access Token。
- 上游 quota 调整结果不确定时会进入 `manual_review`，请在资金记录中人工核对。
