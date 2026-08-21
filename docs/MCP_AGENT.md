# 用 MCP Agent 打牌

PokerNode 在站点的 `/mcp` 提供无状态 Streamable HTTP MCP。每名用户只有一把独立 MCP Key；Agent 持有哪名用户的 Key，就只能以该玩家身份读取牌桌和行动。

发牌、私牌可见性、行动顺序、下注范围和结算仍由 Go 服务端统一校验。MCP Key 不是 New API Token，也不能查看其他玩家未公开的底牌。

## 生成玩家 Key

1. 使用准备交给 Agent 的玩家账号登录 PokerNode。
2. 打开用户菜单中的“Agent MCP”。
3. 生成 Key 并立即复制。完整 Key 只显示一次，服务器只保存 SHA-256 哈希和末四位。
4. 打开“Agent 托管”开关后，MCP 才能入座、准备、行动或离桌；开启期间网页端只能观察，玩家可随时关闭开关收回控制。
5. 轮换会立即废止旧 Key；撤销会立即取消该 Agent 的授权并自动收回人工控制。

玩家账号还需要先加入频道、完成 New API 玩家绑定并具有足够余额。一个 PokerNode 账号在所有频道和游戏中只能同时坐一桌。多个 Agent 对局时，应为每个 Agent 建立独立 PokerNode 用户并分别生成 Key。

Key 也可以通过登录会话调用以下 API 管理：

- `GET /api/me/mcp-key`：读取是否已生成、末四位和生成时间。
- `POST /api/me/mcp-key`：生成或轮换，并且只在本次响应返回完整 `mcp_key`。
- `DELETE /api/me/mcp-key`：撤销。

## 连接 HTTP MCP

MCP 地址为 `https://你的域名/mcp`。客户端必须在每个请求中发送：

```http
Authorization: Bearer pnmcp_你的玩家Key
```

不同 MCP 客户端的配置字段和环境变量语法略有不同。若客户端采用通用 `mcpServers` JSON 且不支持变量展开，可以使用以下配置，并在本地替换示例 Key：

```json
{
  "mcpServers": {
    "pokernode": {
      "type": "http",
      "url": "https://poker.example.com/mcp",
      "headers": {
        "Authorization": "Bearer pnmcp_replace-with-player-key"
      }
    }
  }
}
```

不要将包含真实 Key 的配置提交到版本库。支持环境变量的客户端应优先使用下方对应格式。

### Codex

Codex 支持 Streamable HTTP MCP 和 Bearer Token 认证，配置字段见 [Codex MCP 官方文档](https://developers.openai.com/codex/mcp/)。先把玩家 Key 放入 `POKERNODE_MCP_KEY` 环境变量：

```powershell
$env:POKERNODE_MCP_KEY = "pnmcp_你的玩家Key"
```

Linux 或 macOS 使用：

```bash
export POKERNODE_MCP_KEY="pnmcp_你的玩家Key"
```

然后编辑用户级 `~/.codex/config.toml`，或受信任项目中的 `.codex/config.toml`：

```toml
[mcp_servers.pokernode]
url = "https://poker.example.com/mcp"
bearer_token_env_var = "POKERNODE_MCP_KEY"
```

将示例域名替换为 PokerNode 的实际 HTTPS 域名，保存配置并重启 Codex。可在 Codex 中输入 `/mcp` 查看连接状态。环境变量必须对启动 Codex 的进程可见；如果使用系统级环境变量，设置后应重新启动 Codex。

### Claude Code

Claude Code 的项目级配置位于项目根目录 `.mcp.json`，用户级配置也可通过 `claude mcp add --scope user` 添加。其 JSON 支持 `${VAR}` 环境变量展开，详见 [Claude Code MCP 官方文档](https://code.claude.com/docs/en/mcp)：

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

也可以直接通过命令添加；注意这会把当前 Key 写入客户端配置：

```bash
claude mcp add --transport http --scope user --header "Authorization: Bearer pnmcp_你的玩家Key" pokernode https://poker.example.com/mcp
```

使用 `claude mcp get pokernode` 或 Claude Code 中的 `/mcp` 检查连接。

### Cursor

Cursor 的项目级配置位于 `.cursor/mcp.json`，用户级配置位于 `~/.cursor/mcp.json`。远程服务使用 `url` 和 `headers`，桌面 IDE 的环境变量写法为 `${env:VAR}`，详见 [Cursor MCP 官方指南](https://cursor.com/guides/coding-agent-mcp)：

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

环境变量必须在 Cursor 启动前可见；通过桌面图标启动时，应设置用户或系统级环境变量并重启 Cursor。

### VS Code / GitHub Copilot

VS Code 的工作区配置位于 `.vscode/mcp.json`，顶层字段使用 `servers`。下例通过密码输入框收集 Key，避免将其写入版本库；字段定义见 [VS Code MCP 官方配置参考](https://code.visualstudio.com/docs/agents/reference/mcp-configuration)：

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

保存后从命令面板运行 `MCP: List Servers`，启动 `pokernode` 并确认信任提示。

生产环境必须使用 HTTPS，因为 MCP Key 等同于该玩家的牌局操作权限。仓库提供的 Nginx 配置已将 `/mcp` 反向代理到 Go 服务端。

## 可选：本地 stdio

需要在本机以子进程方式连接时，原有 stdio 服务仍可使用：

```powershell
go build -o .\bin\pokernode-mcp.exe .\cmd\pokernode-mcp
```

配置 `POKERNODE_BASE_URL` 和 `POKERNODE_MCP_KEY`。为兼容旧的本地人工会话，也仍支持 `POKERNODE_USERNAME`/`POKERNODE_PASSWORD` 或 `POKERNODE_SESSION_TOKEN`，但 Agent 托管写操作必须使用 MCP Key；后端 Docker 镜像也包含 `/app/pokernode-mcp`。

## 工具

- `pokernode_list_channels`：列出账号已加入的频道。
- `pokernode_get_current_game`：只返回当前唯一牌局的位置和托管状态；未入座时返回 `active: false`。
- `pokernode_list_tables`：列出频道牌桌摘要，不附带完整玩家名单。
- `pokernode_wait_for_turn`：等待轮到自己、需要准备、收到移出投票、离开座位或超时，返回精简决策状态。
- `pokernode_join_table`：买入并入座，金额单位是整数美分。
- `pokernode_ready`：准备；全员准备后自动开局。
- `pokernode_act`：执行当前游戏允许的动作；德州可弃牌、过牌、跟注、下注、加注或全下，斗地主可叫分、出牌或不出。
- `pokernode_leave_table`：仅在两手牌之间离桌并结算余额。

MCP 的权威金额使用整数美分，避免浮点误差。返回结果只声明一次 `money: {"currency":"USD","unit":"cent","scale":100}`；所有 `*_cents` 都除以 100 后才是美元。例如 `stack_cents: 10000` 表示 100.00 美元，`buy_in_cents: 2000` 表示买入 20.00 美元。

为减少上下文，正常托管循环只需先调用一次 `pokernode_get_current_game` 确认已入座，之后重复 `pokernode_wait_for_turn` → 决策 → `pokernode_act`。`wait_for_turn.state.legal_actions` 只列出当前合法动作，手牌和公共牌使用紧凑短码；`ready`、`act` 和 `leave_table` 会根据账号的全局唯一座位自动定位牌桌，不再重复提交 `space_id`、`table_id`。完整牌桌诊断不再作为 MCP 工具暴露，需要时使用网页牌桌或牌局历史。

Agent 每次行动前应读取 `game_type`、`legal_actions` 和 `turn_id`。调用 `pokernode_act` 时必须原样提交最新的 `expected_turn_id`；返回 `code: "stale_turn"` 时，按 `next_tool` 重新等待，不要用相同参数连续重试。德州的 `bet` 和 `raise` 使用 `amount_cents` 表示本轮下注总目标，不是增加量；目标必须位于 `min_raise_to_cents` 与 `max_raise_to_cents` 之间。

`wait_for_turn` 返回 `ready_required` 时立即调用 `pokernode_ready`；返回 `kick_vote` 时说明当前玩家正被发起移出投票，应在 `expires_at` 前准备；返回 `not_seated` 时停止等待并调用一次 `pokernode_get_current_game`。服务端会拒绝同一账号的并发等待，并通过 `retry_after_ms` 指示退避时间，客户端不得无间隔轮询。

例如，读取到 `turn_id: 42` 后执行德州跟注：

```json
{"action":"call","expected_turn_id":42}
```

斗地主叫分使用 `{"action":"bid","bid":1,"expected_turn_id":42}`（`bid: 0` 表示不叫），出牌使用 `{"action":"play","cards":["3c","3d"],"expected_turn_id":42}`，不出也必须携带 `expected_turn_id`。普通牌沿用 `As`、`Td`、`2c` 等短码，小王和大王分别为 `SJ`、`BJ`。服务端仍会校验手牌归属、牌型以及能否压过上一手。

两种游戏默认行动时限都是 25 秒。`action_deadline_at` 是毫秒级 Unix 截止时间，`pokernode_wait_for_turn` 会在轮到当前玩家时返回；Agent 应预留网络与推理时间，不要等到截止瞬间才提交动作。

MCP 不提供创建账号、绑定资金凭证、开启托管或管理牌桌的能力。开启和收回托管只能由网页登录用户完成。禁用用户、轮换 Key 或撤销 Key 后，后续 HTTP MCP 请求会立即失去授权。
