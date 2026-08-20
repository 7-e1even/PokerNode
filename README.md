# PokerNode

PokerNode 是一个面向桌面、平板和手机的朋友局游戏平台，现支持最多 8 人的德州扑克和 3 人斗地主：Go 服务端负责发牌、行动校验和结算，React 前端负责实时牌桌交互，并通过 New API 管理玩家余额。

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
- 每位玩家独立 MCP Key，支持外部 Agent 托管和网页一键收回控制
- 账号、角色、加密凭证和资金记录
- PostgreSQL 持久化、WebSocket 实时同步和响应式触屏牌桌

详细规则见 [德州扑克规则](docs/POKER_RULES.md) 与 [斗地主规则](docs/LANDLORD_RULES.md)。

## Docker 部署

服务器只需要 Docker Compose。PokerNode 默认从 GHCR 拉取 `linux/amd64`、`linux/arm64` 镜像，不在服务器本地构建。

先创建配置文件：

```bash
cp .env.example .env
```

两种部署方式都需要手动填写：

```dotenv
POKERNODE_SESSION_SECRET=至少32字符的随机值
POKERNODE_ENCRYPTION_KEY=Base64编码的32字节随机值
```

可以使用 `openssl rand -hex 48` 生成 Session Secret，使用 `openssl rand -base64 32` 生成加密密钥。加密密钥投入使用后不要随意更换，否则已保存的 New API 凭证将无法解密；迁移已有 PokerNode 数据库时必须沿用原密钥。

### 使用已有 PostgreSQL

在 `.env` 填写现有数据库连接：

```dotenv
DATABASE_URL=postgres://用户名:密码@数据库地址:5432/pokernode?sslmode=require
```

启动：

```bash
docker compose up -d
```

### 自动搭建 PostgreSQL

在 `.env` 填写一个 URL 安全的数据库密码，例如 `openssl rand -hex 32` 的输出：

```dotenv
POSTGRES_PASSWORD=随机数据库密码
```

使用内置数据库配置启动：

```bash
docker compose -f compose.postgres.yaml up -d
```

PostgreSQL 不开放宿主机端口，数据保存在 `pokernode_data` 持久卷中。执行普通的 `docker compose -f compose.postgres.yaml down` 会保留数据；增加 `-v` 会永久删除数据库卷。

打开 [http://localhost:8080](http://localhost:8080)。升级时运行：

```bash
docker compose pull
docker compose up -d
```

使用内置数据库时，在上述命令中增加 `-f compose.postgres.yaml`。端口、监听地址和可信来源可通过 `.env` 中的 `POKERNODE_PORT`、`POKERNODE_BIND_ADDRESS`、`POKERNODE_TRUSTED_ORIGINS` 调整。

## 首次使用

1. 第一个注册账号会成为超级管理员。
2. 创建频道，填写 New API 地址和管理员 **System Access Token**。
3. 创建牌桌并把频道邀请码发给玩家。
4. 玩家加入频道、买入、准备，随后自动开局。

管理员 Token 必须有用户管理和 quota 调整权限，普通模型调用密钥不能代替它。

## MCP Agent

站点在 `/mcp` 提供带每用户独立 Key 的 Streamable HTTP MCP。玩家在用户菜单的“Agent MCP”中生成 Key 并开启“Agent 托管”后，外部 Agent 可以读取牌局、入座、准备、等待轮次并执行合法动作；网页端可随时关闭托管并收回控制。

MCP 地址是 `https://你的域名/mcp`，请求使用玩家自己的 Key：

```http
Authorization: Bearer pnmcp_你的玩家Key
```

Codex、Claude Code 和 Cursor 建议先把 Key 放入环境变量：

```powershell
$env:POKERNODE_MCP_KEY = "pnmcp_你的玩家Key"
```

```bash
export POKERNODE_MCP_KEY="pnmcp_你的玩家Key"
```

常用客户端配置如下。请先将 `poker.example.com` 替换为实际域名。

### Codex

加入 `~/.codex/config.toml`，也可放在受信任项目的 `.codex/config.toml`：

```toml
[mcp_servers.pokernode]
url = "https://poker.example.com/mcp"
bearer_token_env_var = "POKERNODE_MCP_KEY"
```

### Claude Code / 通用 JSON

Claude Code 可写入项目根目录 `.mcp.json`；其他采用 `mcpServers` 格式并支持环境变量展开的客户端也可参考：

```json
{
  "mcpServers": {
    "pokernode": {
      "type": "http",
      "url": "https://poker.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ${POKERNODE_MCP_KEY}"
      }
    }
  }
}
```

### Cursor

写入项目的 `.cursor/mcp.json` 或用户目录的 `~/.cursor/mcp.json`：

```json
{
  "mcpServers": {
    "pokernode": {
      "url": "https://poker.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ${env:POKERNODE_MCP_KEY}"
      }
    }
  }
}
```

### VS Code / GitHub Copilot

写入 `.vscode/mcp.json`。首次启动时，VS Code 会安全地提示输入玩家 Key：

```json
{
  "inputs": [
    {
      "type": "promptString",
      "id": "pokernode-mcp-key",
      "description": "PokerNode 玩家 MCP Key",
      "password": true
    }
  ],
  "servers": {
    "pokernode": {
      "type": "http",
      "url": "https://poker.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ${input:pokernode-mcp-key}"
      }
    }
  }
}
```

保存后重启客户端并检查 MCP 连接状态。本地 `stdio` 模式、更多配置方式、工具列表和安全边界见 [用 MCP Agent 打牌](docs/MCP_AGENT.md)。生产环境必须使用 HTTPS，并且不要把完整 Key 提交到配置仓库。

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
