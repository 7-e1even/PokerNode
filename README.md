# PokerNode

一个以 Go 为权威后端的 6 人桌德州扑克系统。PokerNode 拥有独立账号与权限体系：玩家先进入频道，再从频道内选择牌桌。每个频道连接一套 New API，创建或加入频道时会自动创建并绑定独立的 New API 普通用户。

## 已实现

- 独立用户注册、登录、HttpOnly 会话；用户可自助修改登录账号和密码，微信首次登录自动注册，老账号可绑定微信
- `超级管理员 / 运营 / 玩家` 平台角色与接口级权限校验
- 频道创建、邀请码加入、管理员连接设置；一个频道可创建多张牌桌
- 频道管理员凭证与玩家个人凭证分离；自动创建普通用户和 System Access Token，凭证使用 AES-256-GCM 加密落库，界面只显示末四位
- New API 身份验证、余额读取、买入扣款、离桌结算
- 频道主可调整本频道成员余额；平台管理员可跨频道管理，所有人工调整记录操作者与原因
- 8-max No-Limit Hold'em：盲注、四轮下注、全下、边池、摊牌与牌型比较
- 全员准备后自动开始下一手；开桌时可设置 5–300 秒行动时限，超时由服务端自动过牌或弃牌
- 每张牌桌独立状态与资金流水，PostgreSQL 持久化，WebSocket 个性化实时视图
- React + Vite + Tailwind CSS + shadcn/ui（Radix Nova）响应式界面，适配桌面、小窗口、平板和手机牌桌
- 资金操作日志；上游结果不确定时进入人工核对，不盲目重试

PokerNode 里的 `$` 是 New API quota 换算后的显示单位，不是银行卡资金或法币支付。默认换算为 `500,000 quota = $1.00`，频道管理员可修改。

用户界面采用“大厅 → 频道 → 牌桌”结构。代码与数据库为兼容早期版本，仍使用 `spaces` 作为频道的内部名称。

玩家大厅与运营后台使用独立页面和登录入口。后台登录地址为 `/admin/login`，运营路由包括 `/admin`、`/admin/users`、`/admin/channels`、`/admin/balances`、`/admin/settings`；玩家侧使用 `/channels/:channelID`、`/channels/:channelID/balances` 和 `/channels/:channelID/tables/:tableID`。直接访问或刷新这些地址不会丢失当前位置。

德州扑克规则基线、随机性说明和专业边界见 [`docs/POKER_RULES.md`](docs/POKER_RULES.md)。

## 界面预览

![PokerNode 四人演示牌局](docs/assets/pokernode-table-demo.png)

演示账号、频道名称和账户余额均已脱敏。图中展示了多人下注记录、服务端行动倒计时与当前行动玩家状态。

## 一键部署：GHCR + Docker Compose

`main` 分支和 `v*` 标签会由 GitHub Actions 自动测试并构建 `linux/amd64`、`linux/arm64` 双镜像：

- `ghcr.io/7-e1even/pokernode-web`：Nginx 托管 React 静态资源，并反向代理 API 与 WebSocket。
- `ghcr.io/7-e1even/pokernode-server`：只运行 Go API，通过 Compose 内网访问，不直接暴露宿主机端口。

服务器不需要源码、Go 或 Node.js，只需要 Docker Compose。对外仍只有一个 Web 入口。

1. 复制环境文件：

   ```powershell
   Copy-Item .env.example .env
   ```

2. 将 PostgreSQL 连接串填入 `.env`。生产环境应使用专用账号和 TLS：

   ```dotenv
   DATABASE_URL=postgres://pokernode:数据库密码@postgres.example:5432/pokernode?sslmode=require
   ```

3. 生成两个不同的随机值，分别填入 `.env`：

   ```powershell
   $bytes = New-Object byte[] 32
   [Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
   [Convert]::ToBase64String($bytes)

   $sessionBytes = New-Object byte[] 48
   [Security.Cryptography.RandomNumberGenerator]::Fill($sessionBytes)
   [Convert]::ToBase64String($sessionBytes)
   ```

   第一个结果用于 `POKERNODE_ENCRYPTION_KEY`，第二个结果用于 `POKERNODE_SESSION_SECRET`。密钥一旦用于生产数据库，不要随意更换，否则已保存的 New API 凭证将无法解密。

4. 启动。Compose 会直接拉取 GitHub Container Registry 中的生产镜像：

   ```powershell
   docker compose up -d
   ```

5. 确认容器健康并打开 [http://localhost:8080](http://localhost:8080)：

   ```powershell
   docker compose ps
   Invoke-RestMethod http://localhost:8080/readyz
   ```

后续升级只需要：

```powershell
docker compose pull
docker compose up -d
```

默认监听所有网卡的 `8080` 端口。可在 `.env` 中通过 `POKERNODE_BIND_ADDRESS`、`POKERNODE_PORT` 修改宿主机监听地址和端口；通过 `POKERNODE_WEB_IMAGE`、`POKERNODE_SERVER_IMAGE` 可以分别固定到发布标签或 `sha-<完整提交哈希>` 镜像。

通过反向代理或内网穿透使用公网域名时，默认无需增加配置：WebSocket 可用时保持实时推送，不可用时牌桌会自动降级为短轮询同步。若代理改写了 Host 且希望恢复 WebSocket，可选设置 `POKERNODE_TRUSTED_ORIGINS=https://poker.example.com`，多个来源用逗号分隔；代理同时需要转发 `Upgrade` 与 `Connection` 请求头。

如需从当前源码本地构建，使用开发覆盖文件，不会改变默认的 GHCR 部署路径：

```powershell
docker compose -f compose.yaml -f compose.build.yaml up -d --build
```

镜像发布工作流位于 [`.github/workflows/container.yml`](.github/workflows/container.yml)。它使用临时 `GITHUB_TOKEN` 写入 GHCR，不需要在仓库中保存 Registry 密码。

### 可选：微信登录

在微信开放平台创建并审核“网站应用”，将授权回调域配置为 PokerNode 的 HTTPS 域名，然后在 `.env` 中同时填写：

```dotenv
WECHAT_APP_ID=你的网站应用AppID
WECHAT_APP_SECRET=你的网站应用AppSecret
WECHAT_REDIRECT_URI=https://你的域名/api/auth/wechat/callback
```

缺少任意一项时，微信登录会保持禁用，账号密码登录不受影响。首次微信授权会自动创建 PokerNode 账号；已有账号应先用账号密码登录，再从右上角头像菜单选择“绑定微信”。同一个微信只能绑定一个 PokerNode 账号，AppSecret 只能放在服务端环境变量中。

## 首次使用

1. 首次部署没有默认账号密码；首个通过密码注册或微信自动注册的账号会成为超级管理员。
2. 创建频道，填写 New API 根地址和管理员的 **System Access Token**。填写 `/profile` 页面地址也可以，后端会自动归一化为实例根地址。
3. 在频道中创建多张牌桌，并将频道邀请码发给玩家。
4. 每名玩家使用自己的 PokerNode 账号加入频道；PokerNode 会自动创建 New API 普通用户、生成 **System Access Token** 并完成绑定。若目标 New API 关闭了密码登录或接口不兼容，页面会提示玩家手动绑定作为兜底。
5. 玩家坐下时，PokerNode 使用频道管理员凭证扣减其 New API quota；离桌时按牌桌余额加回。

超级管理员可在独立运营后台查看平台统计、频道/New API 节点、成员绑定、牌桌和资金流水，关闭自助注册，并增删改查账号与调整角色；运营可查看运营数据、创建及启停玩家，但不能提升角色或修改注册策略。玩家账号会被后台登录接口直接拒绝。

管理员 Token 必须拥有调整成员 quota 的权限。普通模型调用密钥（通常为 `sk-...`）不能代替 System Access Token。

## 本地开发

需要 Go 1.25、Node.js 24 和 pnpm。

Windows 上可直接双击 `start-dev.bat`。脚本会在首次运行时生成 `.env` 随机密钥、安装前端依赖、分别启动前后端，并打开 [http://127.0.0.1:5173](http://127.0.0.1:5173)。

也可以手动运行：

```powershell
pnpm --dir web install
pnpm --dir web build

$env:POKERNODE_ADDR = "127.0.0.1:8080"
$env:DATABASE_URL = "postgres://pokernode:数据库密码@127.0.0.1:5432/pokernode?sslmode=disable"
$env:POKERNODE_SESSION_SECRET = "替换为至少32字符的随机值"
$env:POKERNODE_ENCRYPTION_KEY = "替换为Base64编码的32字节随机值"
go run ./cmd/pokernode
```

前后端分开开发时运行 `pnpm --dir web dev`；Vite 会把 `/api` 和 WebSocket 代理到 `127.0.0.1:8080`。

## 验证

```powershell
go test ./...
pnpm --dir web typecheck
pnpm --dir web build
```

涉及存储的测试使用独立 PostgreSQL schema，运行前设置测试数据库连接；测试结束后会删除该 schema，不会写入正式业务表：

```powershell
$env:POKERNODE_TEST_DATABASE_URL = $env:DATABASE_URL
go test ./...
```

测试包含完全本地的模拟微信与 New API，覆盖微信自动注册/登录/老账号绑定、角色边界、关闭注册、频道多牌桌、个人凭证绑定、双方买入、开局、弃牌、离桌和最终余额守恒，不会修改真实微信或 New API 数据。

本地开发时可按需在 `reference/` 下浅克隆 go-admin、poker-engine 和 go-poker-evaluator，分别用于研究 RBAC、对照牌局状态机和牌型评估差分测试。该目录不参与 PokerNode 生产运行，也不会提交到源码仓库。

## 结算安全边界

New API 现有 quota 调整接口没有业务幂等键。如果请求发生超时，PokerNode 会把操作标记为 `manual_review` 并停止自动重试，防止重复扣款或重复返还。频道管理员应在“资金记录”中取得操作 ID，再对照 New API 用户余额人工核对。高并发或公开运营前，建议为 New API 增加接受 PokerNode 操作 ID 的幂等调整接口。

生产部署还应使用 HTTPS、为 PostgreSQL 启用加密连接并定期备份、限制谁能创建频道，并为 PokerNode 配置专用的低权限 New API 管理账号。任何曾经在聊天、日志或截图中暴露过的 Token 都应轮换。
