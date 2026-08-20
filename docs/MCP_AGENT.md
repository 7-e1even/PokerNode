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

不同 MCP 客户端的配置字段略有不同，核心配置等价于：

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

生产环境必须使用 HTTPS，因为 MCP Key 等同于该玩家的牌局操作权限。仓库提供的 Nginx 配置已将 `/mcp` 反向代理到 Go 服务端。

## 可选：本地 stdio

需要在本机以子进程方式连接时，原有 stdio 服务仍可使用：

```powershell
go build -o .\bin\pokernode-mcp.exe .\cmd\pokernode-mcp
```

配置 `POKERNODE_BASE_URL` 和 `POKERNODE_MCP_KEY`。为兼容旧的本地人工会话，也仍支持 `POKERNODE_USERNAME`/`POKERNODE_PASSWORD` 或 `POKERNODE_SESSION_TOKEN`，但 Agent 托管写操作必须使用 MCP Key；后端 Docker 镜像也包含 `/app/pokernode-mcp`。

## 工具

- `pokernode_list_channels`：列出账号已加入的频道。
- `pokernode_get_current_game`：直接返回该账号当前唯一的牌局；未入座时返回 `active: false`。
- `pokernode_list_tables`：列出频道牌桌和当前座位。
- `pokernode_get_table`：读取当前玩家视角的完整牌桌状态。
- `pokernode_wait_for_turn`：等待轮到自己、牌局结束或超时。
- `pokernode_join_table`：买入并入座，金额单位是整数美分。
- `pokernode_ready`：准备；全员准备后自动开局。
- `pokernode_act`：执行当前游戏允许的动作；德州可弃牌、过牌、跟注、下注、加注或全下，斗地主可叫分、出牌或不出。
- `pokernode_leave_table`：仅在两手牌之间离桌并结算余额。

Agent 每次行动前应先读取 `game_type`、`allowed_actions` 和 `turn_id`。调用 `pokernode_act` 时必须原样提交最新的 `expected_turn_id`；轮次已经变化时服务端会拒绝陈旧动作，Agent 应重新读取状态再决策。德州的 `bet` 和 `raise` 使用 `amount_cents` 表示本轮下注总目标，不是增加量；目标必须位于 `min_raise_to_cents` 与 `max_raise_to_cents` 之间。

例如，读取到 `turn_id: 42` 后执行德州跟注：

```json
{"space_id":"space-id","table_id":"table-id","action":"call","expected_turn_id":42}
```

斗地主叫分使用 `{"action":"bid","bid":1,"expected_turn_id":42}`（`bid: 0` 表示不叫），出牌使用 `{"action":"play","cards":["3c","3d"],"expected_turn_id":42}`，不出也必须携带 `expected_turn_id`。普通牌沿用 `As`、`Td`、`2c` 等短码，小王和大王分别为 `SJ`、`BJ`。服务端仍会校验手牌归属、牌型以及能否压过上一手。

两种游戏默认行动时限都是 25 秒。`action_deadline_at` 是毫秒级 Unix 截止时间，`pokernode_wait_for_turn` 会在轮到当前玩家时返回；Agent 应预留网络与推理时间，不要等到截止瞬间才提交动作。

MCP 不提供创建账号、绑定资金凭证、开启托管或管理牌桌的能力。开启和收回托管只能由网页登录用户完成。禁用用户、轮换 Key 或撤销 Key 后，后续 HTTP MCP 请求会立即失去授权。
