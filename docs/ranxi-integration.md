# Ranxi production 合并

本次合并来自 `ranxi2001/sub2api` 的 `production` 分支，基准提交为 `0ae36e501952000c5c910a2e616c6e0861f66a49`（v2.9.8）。

新增能力包括 Prism 代理及客户端工具桥接、TypeSafe / Jev System One、Astra 网关借用和自动调度、区域上游代理、API Key 并发队列、充值阶梯赠金与折扣、模型广场和功能搜索。原有 Kiro、远程打票、IP 管理、充值优惠券及首充活动继续使用。

## 打票方式

管理员打开“采集面板”，通过“采集方式”选择：

- **跟随部署配置**：沿用 `gateway.openai_codex_ticket.cloud_mint.enabled` 及后台远程 Relay 配置，兼容已有配置。
- **远程打票（Relay）**：使用现有远程地址、密钥、协议和网关策略，采集请求继续发送 `X-Mint-TTL: 0`。
- **本地打票**：使用本地采集代理，支持原生 SSE / WebSocket、Mihomo 节点租约、并行采集和出口绑定。

本地代理可以在“系统设置 → 网关转发”中保存，也可以配置 `gateway.openai_codex_ticket.harvest_proxy_url`。支持 HTTP、SOCKS5、SOCKS5h 和 `ippool://active`。后台保存会立即使代理缓存失效；代理密码在返回值及审计数据中显示为掩码。

本地代理用户名含 `-sid-<字母数字>` 时，每次探测自动生成同长度的新 SID，例如 `socks5://user-region-Rand-sid-Ab12Cd34-t-5:password@proxy.example:3000`。保存的代理模板保持原值，地区、时长和密码保持原值。路由 Cookie 按当次代理 SID 隔离，成功票据绑定当次代理地址，业务请求在该票据有效期内沿用同一 SID。采集日志显示当次 SID，出口 IP 由代理供应商分配。SID 代理直接进行会话轮换；Mihomo 节点记忆继续用于侧车代理，远程 Relay 继续使用原有配置。

采集方式保存为 `openai_codex_harvest_controls_v1` 中的 `mint_mode`，取值为空字符串、`remote` 或 `local`。历史远程票保留账户的生产代理路径；本地票携带采集节点和代理绑定。账户网关选择及黑名单继续生效。

## 部署

Prism 的运行方式和依赖见 [Prism adapter 文档](../prism-adapter/README.md)。网关和完整 `prism-adapter/` 目录共同升级，按文档配置浏览器会话、适配器密钥和 Prism 模型范围。

新增数据库迁移随服务启动执行。`241_add_typesafe_platform.sql` 兼容存量 Kiro 行，`265_preserve_kiro_typesafe_platforms.sql` 将配额与组合路由的平台约束收敛到 Kiro、TypeSafe 和其他已有平台的全集。已有主分支迁移文件保持原内容。

全局充值阶梯与优惠券、首充活动组合时，优惠券保持原有到账规则，阶梯赠金单独记录，阶梯折扣与优惠券共同计算实付。账单和余额预览使用相同的规则。

## 验证记录

- SID 自动轮换的 7 项测试通过，覆盖用户名参数和编码密码保留、并发会话生成、手动重试、Cookie 隔离、成功票据出口绑定，以及远程 Relay 和原生 292 打票。打票服务相关的 1352 项回归通过，统计包含子用例；分组模型清单及采集唤醒的历史失败组沿用显式跳过。
- SID 改动后的 Linux 嵌入资源构建和 183 项管理接口回归通过，Mihomo 定向采集相关的 9 项回归通过。补充的 `TestUseOnceLeasePersistsReservationAndSerializesProbes` 报告 `failed` / `used` 状态差异，该失败已在改动前主分支 `8fdd2be` 上复现。
- 前端类型检查、Vite 正式构建通过。设置、采集方式、模型广场入口和充值相关的 140 项回归通过。
- 打票、Prism、TypeSafe、Astra、区域代理、API Key 队列和充值阶梯的 1855 项后端回归通过，统计包含子用例。Prism 协议桥、区域路由、TypeSafe 客户端和 BPS 包测试通过。
- 后端所有包的 `-tags=unit` 单测编译通过。`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed ./cmd/server` 构建通过，前端资源已嵌入 Linux 服务。
- 存量 SQL 迁移逐文件核对通过。Docker Compose 安全、网关环境变量、运行资源、简单模式环境变量以及 Caddy 缓存配置检查通过。

全量回归仍报告失败，需要在后续维护中处理。前端剩余的 22 项失败已在合并前主分支复现。后端相关回归中，分组模型清单断言、GPT-6 定价别名断言、SQLite 测试夹具的 `gift_balance` 列，以及空 Relay 设置夹具仍有历史失败；上述测试组在通过的功能回归命令中显式跳过。全量后端检查还包含 Windows 下的进程、权限和代理脚本限制。

Prism Python 适配器在 Windows 上执行了 76 项离线测试，其中 42 项通过、34 项报错，报错涉及 POSIX 目录同步和子进程环境。模型选择的 4 项测试单独通过。完整适配器离线测试、数据库集成测试及 macOS 容器脚本测试由对应的 Linux / macOS CI 环境验证。
