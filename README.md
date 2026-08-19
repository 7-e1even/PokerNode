# PokerNode

PokerNode 是一个朋友局游戏平台，现支持最多 8 人的德州扑克和 3 人斗地主：Go 服务端负责发牌、行动校验和结算，React 前端负责牌桌交互，并通过 New API 管理玩家余额。

> 页面中的 `$` 是 New API quota 的换算显示，不代表法币。默认 `500,000 quota = $1.00`。

## 预览

![PokerNode 四人演示牌局](docs/assets/pokernode-table-demo.png)

_演示账号、频道名称和账户余额已脱敏。_

## 主要功能

- 大厅、频道、邀请码和多牌桌
- No-Limit Hold'em、全下、边池和摊牌结算
- 3 人斗地主、叫分、常用牌型、炸弹/王炸与春天结算
- 全员准备后自动开局，行动时限可设为 5–300 秒
- New API 自动绑定、买入扣款和离桌返还
- 账号、角色、加密凭证和资金记录
- PostgreSQL 持久化、WebSocket 实时同步和移动端适配

详细规则见 [德州扑克规则](docs/POKER_RULES.md) 与 [斗地主规则](docs/LANDLORD_RULES.md)。

## Docker 部署

服务器只需要 Docker Compose。默认从 GHCR 拉取 `linux/amd64`、`linux/arm64` 镜像。

1. 创建环境文件：

   ```bash
   cp .env.example .env
   ```

2. 至少填写以下三项：

   ```dotenv
   DATABASE_URL=postgres://pokernode:数据库密码@postgres.example:5432/pokernode?sslmode=require
   POKERNODE_SESSION_SECRET=至少32字符的随机值
   POKERNODE_ENCRYPTION_KEY=Base64编码的32字节随机值
   ```

   加密密钥投入使用后不要随意更换，否则已保存的 New API 凭证将无法解密。

3. 启动并检查：

   ```bash
   docker compose up -d
   docker compose ps
   curl http://localhost:8080/readyz
   ```

打开 [http://localhost:8080](http://localhost:8080)。升级时运行：

```bash
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

## MCP Agent

站点在 `/mcp` 提供带每用户独立 Key 的 Streamable HTTP MCP，外部 Agent 可以用普通玩家身份读取牌桌、入座、准备、等待轮次并执行合法动作；本地 `stdio` 模式仍可选用。接入方式与安全边界见 [用 MCP Agent 打牌](docs/MCP_AGENT.md)。

## 可选：微信登录

需要在 `.env` 中同时设置：

```dotenv
WECHAT_APP_ID=网站应用AppID
WECHAT_APP_SECRET=网站应用AppSecret
WECHAT_REDIRECT_URI=https://你的域名/api/auth/wechat/callback
```

不设置时，账号密码登录仍可正常使用。

## 生产提醒

- 使用 HTTPS、PostgreSQL TLS 和定期备份。
- 妥善保存 Session Secret、Encryption Key 和 System Access Token。
- 上游 quota 调整结果不确定时会进入 `manual_review`，请在资金记录中人工核对。
