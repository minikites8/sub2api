# GPT/OpenAI 智能分层调度最终独立验收

## 1. 执行摘要

结论：**CONDITIONAL PASS — 有条件灰度**。仅允许下述已验证范围，不能将本报告理解为全功能验收通过。

- 审计分支 `main`，开始 Commit `f11fe920c3cb1813d10f5136eb72a230e4f8f0e3`；第 1–8 节的受测源代码提交为 `d8075aa6afb5ed3f8e49740bdc5d2b9011b200a7`；后续账号标签保存、排队超时及分组观测补丁的独立回归见第 9–11 节。旧版本对照为 QoS 引入前的 `21f675138`，从 Git archive 独立构建。
- 时间：2026-10-26；macOS arm64；Go 1.27.3；Docker Desktop；真实 PostgreSQL `18.1-alpine3.23`、Redis `8.4-alpine`；两个独立 `cmd/server` OS 进程，共用临时数据库和 Redis。
- 标准余额计费模式，31 个合成用户、3 个服务组、Strong 两账号共 20 槽、Basic 两账号共 30 槽；账号类型均为本地模拟 APIKey。没有调用真实付费模型，没有读取/修改生产余额或账号，没有部署。
- 每个测试服务显式配置数据库最大连接 20、空闲 10；测试 Redis 前置可控 TCP 转发器，用于真实断连。全部模型地址为 loopback 模拟上游；定价使用仓库 fallback 文件。仅临时数据库关闭同 IP 合成多用户触发的独立反滥用策略。
- 已修复 4 个 P1，并保留失败及复测证据；未解决 P0=0、P1=0。P2 及验证缺口见第 5、7 节。没有把 Mock/miniredis 测试等同于双实例服务验证。
- 受测服务二进制 SHA-256：`795f3f5c21cfd13870bf40bae4897c0f1855415c12117eb8791bb8db8b325566`（构建时 HEAD 为审计开始版本，工作区已包含四个修复；构建与测试均在本次任务内执行）。
- 本地原始证据目录：`artifacts/gpt-qos-final-20261026/`（忽略提交，包含请求明细、上游明细、服务日志、CPU/RSS、Redis 计数和失败日志）。可用 `scripts/test-gpt-qos-acceptance.sh` 从源码重建。证据 SHA-256 清单为 `manifest.json`；二进制只在临时目录构建。

灰度范围：显式质量标签、质量路由、有限正并发账号上的用户公平队列；经过人工确认的共享池静态限流可以单独验证后开启。首次灰度保持 `peak.enabled=false`。不宣称实时 VM2API 容量感知、借用模式的即时保障、Redis Cluster、显式多代理池、超大候选集或真实 OAuth 上游已验收。

## 2. 需求—代码—运行证据矩阵

PASS 必须在本列所述范围内理解；部分执行的模块整体仍标 PARTIAL。

| 模块 | 状态 | 实际路径与证据 |
|---|---|---|
| 显式账号质量、默认 Unknown、审计迁移 | PASS | `service/openai_quality_config.go`，迁移 `283_gpt_quality_audit.sql`；`TestGPTQualityLabelsAreExplicitAndValidated`、真实 PG `TestGPTQualityAuditMigrationPersistsActorAndOldNew` |
| Strong 优先及备用 Strong | PASS | `openai_account_scheduler.go → openai_quality_router.go → openai_qos_admission.go`；实际 HTTP `RealStrongStandbyPrecedesBasic`、质量优先 40 请求、旧订阅/优先级单测 |
| 模型/分组/能力先于 QoS | PASS | `gptQoSCandidateIneligibleReason`、完整快照批量读取和最终 DB 复核；真实 `RealModelEligibilityBeforeQualityAndQueue`，缓存代理槽回归 |
| 用户 QoS 授权来源 | PASS | Handler 从认证 APIKey 授权组解析配置，真实 UserID 注入 ctx；`ClientCannotPromoteQoSTier`、权限/中间件测试；客户端 QoS 字段无效 |
| 加权公平调度及用户公平 | PASS | Redis `lua/gpt_qos.lua` 分层虚拟完成时间算法；60 秒真实双实例饱和与 1/10/3 Key 三用户测试；不是账号评分或进程内 FIFO |
| 取消、排队背压、账号原子槽 | PASS | Redis 同一 Lua 原子选请求并取得既有账号槽；用户/组/总队列上限；真实混合取消、断连拒绝恢复、无残留队列检查及 Race 回归 |
| 高峰容量保障 | PARTIAL | 已验证 20 槽/6 预留的真实服务状态及 Premium 准入、整数取整单测、运行态冷却容量刷新；未完成所有容量规模、长 SSE、模型异质性及闲置状态退出的真实组合验证 |
| 共享物理池静态容量 | PASS | `repository/gpt_qos_capacity.go` 与既有 `concurrency_cache.go`；实际两 Key 合并为 10 槽，真实并发不超过 10；旧选号/单代理同槽测试 |
| VM2API 实时容量、自适应控制 | PARTIAL | 只有 `UpstreamCapacityProvider` 扩展接口及快照有效性函数；没有已验证 VM2API 管理接口、自动探测或自适应 cap。当前使用显式有限静态容量 |
| Responses 不可迁移续链 | PASS | 真实 `RealPreviousResponseBindingAcrossInstances` 保持原账号；`TestGPTQualityForcedContinuationRetainsBasicOwner`、父账号不可逃逸单测 |
| 软粘性/缓存收益综合决策 | PARTIAL | 质量模式自由选号绕过原无条件软粘性，按质量/负载选；没有完成缓存收益、预估等待、QoS 的综合效用模型，真实缓存命中收益未测 |
| SSE 与 Failover | PARTIAL | 真实流/非流、语义输出后 EOF 只提交一次、取消释放、429 无违规降级已测；500/502/503/网络断开/长 TTFT 的全部 QoS HTTP 组合未全部真实注入，已有 Handler 单测覆盖部分 |
| 统一预算/尝试上限 | PASS | `PrepareGPTQoSRequest` 冻结上下文和配置，`MarkGPTQoSSubmission` 在提交处计数；`TestGPTQoSRequestBudgetAndAttemptsSurviveFailover`、取消/本地超时分类测试 |
| 多实例与崩溃租约 | PASS | 独立 PID 实际请求、全局 Redis 槽、真实 Redis 断连保守拒绝；20 槽持有进程 SIGKILL 后由存活实例等待 TTL 恢复；限定单 Redis、有限账号槽的测试范围 |
| 管理设置持久化及权限 | PASS | 实际 JWT 管理 API 保存、跨实例版本刷新、非法参数 400、APIKey 不得管理；Vue 质量字段/QoS/组配置测试；完整点击式浏览器验收不包含在此 PASS 范围 |
| 批量质量管理全链路 | PARTIAL | 管理路由及服务有字段和校验，复用批量更新；没有对真实管理 HTTP 批量选择/过滤误更新做完整端到端测试 |
| 指标与日志 | PARTIAL | 有界异步通道、10,000 条/24h Redis 窗口，区分请求和提交次数、取消；日志扫描见下；丢弃/时间窗口及闲置峰值状态不代表完整账本，未证明所有维度长期准确 |
| 计费不变量 | PASS | 标准模式实际 SQL：重复 request_id、Token、销售倍率、余额和 usage 对账；相同 Token 正常结算费率一致；没有改动计费/扣费实现；中断流精确 token 定价仍沿用旧逻辑且未做所有组合对照 |
| 默认关闭、全部关闭后基线 | PASS | 四开关默认 false；当前与旧二进制各 200 次请求，固定 Basic 更高原优先级，均选 Basic、HTTP 200；只证明所列 Responses/并发范围，不泛化为全部端点/额度差分 |
| 带等待队列时热回滚完整差分 | NOT VERIFIED | 请求冻结进入时策略，旧请求可在预算内继续；没有在全部负载阶段对每个开关独立切换做真实差分，回滚先排空 |
| Redis 长故障超过租约、网络分区/旧持有者复活 | NOT VERIFIED | 已做短断连与 SIGKILL 到期回收；没有证明进程暂停超过 TTL 后继续向上游执行的完整 fencing 组合 |
| 性能和长期稳定性 | PARTIAL | 有真实请求延迟、CPU/RSS/Redis 命令量及局部 Redis 延迟；缺少长期 soak、全端点旧基线、高精度 DB query 计数及完整资源剖析 |

矩阵共 22 项：PASS 13、PARTIAL 7、NOT VERIFIED 2、FAIL 0。未把后两类计为 PASS。

## 3. 真实调用链与安全约束

`Responses / ChatCompletions / Messages兼容 / Embeddings 等 Handler → prepareGPTQoSRequest → 授权分组/模型映射 → SelectAccountWithSchedulerForCapability → 原权限/模型/协议/隐私/额度/RPM/利润条件 → 不可迁移续链 → selectQualityPools → 完整候选快照再过滤 → GPTQoSPoll → Redis 原子公平准入及账号槽 → 新鲜账号/绑定/额度/RPM终检 → Forward → 原结算和资源释放`。

普通/高级/优先调度继续存在。质量路由开启后，订阅、OAuth 额度、BPS、成本与原账号优先级不能越过 Strong/Basic 约束。Same-quality 普通账号由实时账号与物理池占用率选择；显式备用账号在主账号可用时保留备用语义，主账号无容量时仍早于 Basic。协议类型、Base URL、是否 VM2API 不决定质量。Unknown 仅可用于本来不可迁移的所有者续链，不能冒充 Strong。

Lua 以真实用户 ID 做 tier 内公平性主体，APIKey ID 不参与加权。tier 虚拟完成时间按配置权重推进，用户也有独立服务进度，避免持续插队与 Key 数量增益；不是按完成 Token 或运行时间计费的资源公平算法。请求长短不同，不要求最终完成数严格 6:3:1。

原账号并发 ZSET 与 QoS grant 在同一原子操作取得，无第二份容量预占。lease 关闭幂等，取消与终检失败关闭排队/已获槽。租约 10 秒续期，丢失时取消受控上游上下文；本次崩溃实验将槽 TTL 明确设为 60 秒。该 TTL 回收有可用性等待代价，不是即时容灾。

高峰阈值有进入/退出 hold 与滞回，保留容量采用 `capacity>=2` 时 `min(capacity-1,max(1,floor(capacity*ratio)))`，容量 1 不预留，至少保留一个共享槽。不会抢占已运行 SSE。借用模式只能承诺有机会回收，不能保证 Premium 即时得到槽位。当前有效注册容量每 5 秒由后台在有策略读取时合并刷新，存在有限新鲜度窗口，并非 VM2API 物理实时容量。

## 4. 实际测试与性能证据

后端最终 `-tags=unit` 全集：7 包全部通过；按 Go JSON 事件计（含父测试及嵌套子测试）通过 26,647、跳过 9、失败 0。Race：4 包全部通过，158 项测试事件通过、失败 0，未出现 data race。前端 3 文件 24 测试通过。真实 Redis/PG 专项通过（命令筛选 `TestGPT(QoSRealRedis|QoSGroupConfig|QualityAudit)`，包含旧版本精度失败复测）。

实际双服务测试 `TestGPTQoSAcceptanceTwoProcesses`：13 子场景全部 PASS，运行 100.51s；日志 `process-certified-2.log`。另两个独立场景：SSE 内容后 EOF 不重放 PASS、进程崩溃/TTL 回收 PASS。

**持续饱和公平性**：30 用户、120 闭环 worker，执行 60s；同为 40ms 模拟上游成本，禁用降级和预留，强池 20。去除开始 5s 和末端排空后观察 54s，共 4322 次实际上游准入（80.04 次/s），不是将排队轮询或重试当作完成请求。

| QoS | 稳态准入 | 份额 | 对目标相对误差 | 发送到上游 P50/P95/P99 ms |
|---|---:|---:|---:|---|
| premium | 2562 | 59.28% | -1.20% | 569.6 / 941.4 / 1092.8 |
| standard | 1320 | 30.54% | +1.80% | 1377.9 / 1687.9 / 1820.3 |
| economy | 440 | 10.18% | +1.80% | 4588.2 / 5089.7 / 5193.0 |

三档均满足初始 ±10% 相对误差断言。若仅用独立伯努利假设作参考，Economy 的 Wilson 95% 区间约 9.31%–11.12%；公平调度的准入相关，不把它当作独立随机样本的严格置信保证。混合负载使用每 worker 固定种子 1337 的派生 RNG，进程调度仍存在正常非确定性。

**同等级真实用户公平**：持有 1/10/3 个 Key，稳态服务机会 {'0': 265, '1': 264, '2': 262}，总 791；每用户 18 worker。Key 多的用户没有取得额外份额；全部 1149 次 HTTP 200。

**故障混合场景**：持续 Premium/Standard、突发式 Economy 的闭环负载，1/7 左右慢请求、主动取消与一次性 429；另有跨实例强制续链独立子场景。强池 20、普通池 30，保护比 30%。此阶段不能拿最终完成数去比较 6:3:1。

| QoS | 原始请求 | HTTP 200 | 503 | 客户端取消返回0 | 实际上游尝试 | Strong/Basic 尝试 |
|---|---:|---:|---:|---:|---:|---:|
| premium | 1281 | 126 | 1025 | 129 | 128 | 128 / 0 |
| standard | 1012 | 10 | 911 | 91 | 10 | 10 / 0 |
| economy | 641 | 597 | 0 | 44 | 597 | 12 / 585 |

按实际尝试统计，Strong 150/735（20.41%）、Basic 585/735（79.59%）。Premium 违规降级 0。此阶段实际 Strong/Basic 并发峰值为 {'basic': 18, 'strong': 20}；不超过 20/30。Economy 有进展且没有无限等待，全部排队/运行计数最终排空。429 使 Strong 冷却后 Premium/Standard 出现大量明确 503；不能据此宣称高峰成功率已达到生产目标。成功准入样本中 Premium 排队分位较 Standard 低，但发生整池不可用时保障容量无法变成可用模型。

VM2API/直接登录真实请求占比：**NOT VERIFIED**；本实验是通用 APIKey 假上游，不能把其协议当成真实 VM2API 或 OAuth；管理来源标签未知时应展示 unknown。

**高峰与物理池**：真实管理快照中 Strong 总容量 20、预留 6；Premium 准入成功；候选冷却后数据库有效容量和注册表在后台刷新窗口内一致。两 Strong Key 配同一静态根 cap10 后，上游峰值 Strong=10。小池取整/严格保护/Legacy 槽互相约束另有单测，不能称每种小池规模均做过双服务实验。

**崩溃与 SSE**：`process-crash-final/process-crash.json`：实例持有20槽后 SIGKILL，存活实例先保守503，60025ms 后恢复（TTL60s），最终租约0。`process-sse/partial-sse.json`：有效 SSE token 后 EOF，实际上游提交1次，无重复输出/重复账单观察；未完成全部中断定价组合。

**计费**：最终正常结算 7609 行，Strong/Basic 每次100输入+50输出Token，正常费用均0.000875；最大逐用户余额对账差额 0。提前检查的 7207 行中重复request_id=0、Token异常=0、倍率变化=0。异步入账必须排空再比较，故障期结算失败行不能被误当作新的质量费率。

**关闭开关旧二进制差分**：各200次固定 Basic 原优先级请求，HTTP200均为200且均选相同Basic；当前 P50/P95/P99={'p50': 73.867, 'p95': 93.04, 'p99': 99.802}ms，基线={'p50': 76.65, 'p95': 100.944, 'p99': 102.049}ms。单轮低负载数据只作观察，不报告改善百分比。

**资源与放大**：服务进程 ps CPU 采样范围 19.0%–37.5%，RSS 163.4–205.0MiB。该窗口 Redis total_commands_processed 增加 52,909,281（包含 Lua 内部 redis.call，不能当网络往返数）；EVALSHA 增加 103,892 次，累计执行时间增加 54.00s。120 竞争请求下单 Redis 脚本成本明显，首批只允许低流量验证，不能声称调度吞吐无回归。

局部真实 Redis 专项的 200 次空载 acquire/release、20次不同候选扫描延迟保留在 `real-redis-final.log`；与60s压力口径不同，不用它覆盖压力下的Redis成本。

调度决策日志扫描：9,435 条 `gpt_qos_decision` 中，测试 APIKey、上游凭据及请求输入标记命中为 0（`logscan.json`）。这里只能证明本次固定场景；没有使用真实 OAuth/Cookie，不能冒称已遍历所有日志路径。

全集跳过的 9 项（均未计 PASS）：`TestAPIKeyConcurrencyUpgradePaths`（未设置独立升级测试 PG URL）、`TestHarvestPostgresPersistenceAndReset`（未设置 harvest PG DSN）、`TestControlledExperimentPostgresBudgetAndRecovery`（未设置其独立 PG DSN）、`TestDingTalkOAuthStart_Disabled`（代码中的 sentinel）、`TestAuthPendingIdentityService_UpsertAdoptionDecision_ClearsLegacyNullSessionReference`（SQLite 不支持历史 NULL 夹具）、`TestContentModerationTypeSafeLive`（缺显式 live 授权/密钥）、`TestGPTAutoLiveJevBenchmark`（缺 live key）、`TestEstimateOpenAIInputTokens_CompareWithOpenAIAPI`（缺 OpenAI key，遵守不付费探测）、`TestPluginRuntimeIntegration`（未提供插件测试包）。这些与本次真实 PG/Redis/QoS 专项不是同一测试，不能互相替代。

### 命令与复现

```bash
# 完整隔离服务验收（需要 Docker 正在运行、本地 Go 及依赖）
bash scripts/test-gpt-qos-acceptance.sh

# 本次还单独执行了相关后端全集，日志为 backend-suite*.jsonl
cd backend
go test -tags=unit ./internal/service ./internal/repository ./internal/handler \
  ./internal/handler/admin ./internal/server/middleware ./internal/server/routes \
  ./internal/config -count=1 -json

# Race：此命令检查 Go 路径；独立服务二进制为正常构建，不能称其已全程带 Race
 go test -race -tags=unit ./internal/service ./internal/repository ./internal/handler \
  ./internal/handler/admin -run 'Test(GPTQoS|GPTQuality|UpdateAccount|StreamWrittenGuard|FailoverRetryWaitBudget|FailoverClientGone)' -count=1 -json

# 前端仅执行已安装依赖，无安装/网络更新
cd ../frontend
./node_modules/.bin/vitest run \
 src/components/account/__tests__/AccountQualityFields.spec.ts \
 src/views/admin/__tests__/GPTQoSSchedulingView.spec.ts \
 src/views/admin/__tests__/GroupsView.qos.spec.ts
```

性能口径：`admission_wait_ms` 为客户端发送至模拟上游实际开始，包含认证/其它等待，不能冒称纯 QoS queue；`http_duration_ms` 含执行及返回。纯调度日志中的 queue_wait/selection_duration 可另外从 `gpt_qos_decision` 分析。Redis commandstats 为隔离 Redis 的整个服务窗口，不是精确到某一算法的增量。`ps %CPU` 是进程生命周期平均，RSS 是采样时驻留内存；没有测 Redis/PG 全部 CPU，DB `pg_stat_statements` 未启用，查询次数 NOT VERIFIED。不用局部样本虚构生产改善百分比。

## 5. 缺陷与修复证据

| 编号/严重度 | 原因和复现 | 本次修复 | 回归证据 |
|---|---|---|---|
| F1 / P1 / 已修复 | `qualityPoolOrder` 删除所有备用账号：主 Strong 满载时，备用 Strong 空闲却拒绝/选 Basic | 保留备用池成员；Go 与 Lua 都在同质量内保留主/备用次序，质量仍早于备用偏好 | `standby-before.log` 原两项失败；`TestGPTQualityStrongStandbyBeforeBasicOnPrimarySaturation`、`TestGPTQoSStrongStandbyBeforeBasic`、真实 `RealStrongStandbyPrecedesBasic` 通过 |
| F2 / P1 / 已修复 | 顶层共享池/质量字段被合并成新的完整 `extra`，清掉旧质量及透传配置；无关编辑也会重置 Strong | 独立字段更新先保留完整现有 extra；无关 extra 编辑保留显式质量、资源池、来源，显式 Unknown/空池仍可清除 | `metadata-before.log`、早期共享池真实失败；新增服务/管理 Handler 回归及真实 `SharedKeysUseOnePhysicalCapacity` 通过 |
| F3 / P1 / 已修复 | Redis Lua `tostring(UnixNano)` 精度舍入，把同一版本判为旧版本，账号池变更被丢弃 | Redis 注册版本用精确十进制字符串；相同版本可刷新账号状态，旧版本不能覆盖；兼容旧科学计数法 | `capacity-version-before.log` 真实 Redis 期待 10 实为 20；`TestGPTQoSRealRedisCapacitySameRevisionAccountMutation` 含旧格式升级/旧版本拒绝通过 |
| F4 / P1 / 已修复 | 仅政策版本变化才刷新容量，429/冷却/到期不改变版本，注册有效容量永久陈旧 | 峰值保护开启时后台每实例合并刷新，最长约 5 秒加配置轮询/查询耗时；不在请求协程扫描 DB | `peak-refresh-before.log` 调用次数期待 1 实为 0；`TestGPTQoSPeakCapacityRefreshesWithoutPolicyChange`，真实混合场景对照 DB 可调度容量通过 |

P2 尚存 5 项，不修改既有计费语义去掩盖问题：

1. 峰值状态主要由准入评估推进，完全无请求时界面可能保留旧 peak/容量显示。保护模式暂不纳入首批承诺保障灰度。
2. 配置先写 DB、后同步 Redis；同步失败返回错误但 DB 可能已经保存，UI 与生效状态需要再次读取确认。代码确认，未覆盖所有跨实例故障时序。
3. 压测发现单 Redis 脚本成本明显（第4节）；QoS 限制单次候选不超过 256，最大总等待 2,000；大池每轮扫描和 25ms 轮询有热 key/放大开销。超大池不能按本报告直接启用。
4. 异步观测可丢弃，Redis 只保留最近 10,000 条/24h；统计不具备财务账本的完整性，也没有完成所有维度与地面真值的长期核对。
5. 软粘性没有完整缓存收益/预计排队效用模型，可能损失缓存命中收益；续链正确性测试不能证明缓存收益不变。

环境/夹具问题单列，不当作调度代码修复：早期测试 HTTP allowlist、JWT 指纹、隔离管理员确认记录未配置；同 loopback IP 多用户触发原风控；并行编译/全集测试干扰压力样本；数据库连接总预算超出 PG 默认 100 导致 `too many clients` 和原结算失败记录；切换进程前未排空异步结算。原始日志均保留，没有通过删断言获得通过。数据库连接预算是实际部署必须核对的条件。

P3：增加分模型/资源池公平性解释、跨组配置并发编辑保护、VM2API provider 契约与新鲜度告警、按运行成本公平性的可选策略。

## 6. 本次修改文件与数据库/Redis兼容

- `.gitignore`
- `backend/internal/handler/admin/account_handler.go`
- `backend/internal/handler/admin/gpt_qos_account_update_test.go`
- `backend/internal/repository/gpt_qos_acceptance_crash_test.go`
- `backend/internal/repository/gpt_qos_acceptance_faults_test.go`
- `backend/internal/repository/gpt_qos_acceptance_process_test.go`
- `backend/internal/repository/gpt_qos_acceptance_quality_cases_test.go`
- `backend/internal/repository/gpt_qos_acceptance_redis_proxy_test.go`
- `backend/internal/repository/gpt_qos_acceptance_scenarios_test.go`
- `backend/internal/repository/gpt_qos_acceptance_sse_test.go`
- `backend/internal/repository/gpt_qos_acceptance_user_fairness_test.go`
- `backend/internal/repository/gpt_qos_cache_test.go`
- `backend/internal/repository/gpt_qos_capacity.go`
- `backend/internal/repository/gpt_qos_integration_test.go`
- `backend/internal/repository/integration_harness_test.go`
- `backend/internal/repository/lua/gpt_qos.lua`
- `backend/internal/service/admin_account.go`
- `backend/internal/service/openai_qos_admission.go`
- `backend/internal/service/openai_qos_capacity_refresh_test.go`
- `backend/internal/service/openai_quality_account_update_test.go`
- `backend/internal/service/openai_quality_config.go`
- `backend/internal/service/openai_quality_router.go`
- `backend/internal/service/openai_quality_router_test.go`
- `scripts/test-gpt-qos-acceptance.sh`

- `docs/GPT_QOS_SCHEDULING_FINAL_ACCEPTANCE.md`

没有新增数据库迁移，没有修改用户余额、销售倍率、Token 定价或扣费实现。复核了既有质量审计迁移；质量/QoS 当前保存在 `accounts.extra`、settings JSON，新账号默认 Unknown。修复只是防止无关编辑抹掉这些新增元数据。

Redis 继续使用 `gptqos:{v1}:` 下 requests/waiting/leases/fair、tier/user/group 队列、物理池/Strong/保留容量计数、capacity_policy 注册表与有界 metrics；原子账号槽沿用 `concurrency:account:*`。配置版本改为精确十进制字符串，脚本兼容旧科学计数法。本次没有引入无法跨实例同步的本地准入锁；本地 mutex 仅用于缓存刷新合并。

## 7. 未验证范围与上线风险

- 真实 VM2API 内部池/容量协议与过期数据行为、真实 OAuth 额度/订阅/计费、真实上游 TTFT/429/费用均未进行付费探测。静态 cap 不能替代物理容量证明，别名必须管理员显式去重。
- 没有完成 Redis Cluster/主从切换/长网络分区、超过租约的进程暂停及旧持有者复活 fencing。不能宣称任意故障下绝不超卖。
- 所有新开关关闭的真实差分仅覆盖固定模型、固定优先级、Responses 流/非流、现有账号并发和账单；RPM/额度、全部端点、随机策略分布和繁忙时热切换没有完整旧二进制矩阵。相关单测不是这些真实矩阵的替代。
- 没有跑长时间真实 SSE/WS、长推理、真实缓存恢复、每种 HTTP 故障与所有 QoS/粘性组合；没有完整浏览器逐项操作录像、真实批量管理权限误更新验证。
- 跨主机时钟偏差、同时管理写入及 DB/Redis 跨存储发布顺序未做完整故障验证；配置变更先采用同一管理入口串行操作。
- 单 Redis Lua 串行、全局热 key、候选扫描、控制面后台账号刷新有成本。长期内存/Goroutine/连接泄漏和生产吞吐 NOT VERIFIED。测试的 CPU/内存和低负载旧版本对照不构成生产容量规划结论。
- 默认 PG 连接池 256/实例与数据库连接上限必须匹配。两实例对默认 100 连接 PG 的失配在本次真实产生结算失败日志，不能带着这种配置灰度。

## 8. 可执行灰度与回滚

1. 部署前人工核对：数据库备份和迁移完成；所有新开关保持 false；Redis 独立实例与访问权限正确；DB 总连接预算有余量；账号并发有限且已确认；Strong 为人工显式验证，Unknown 不算 Strong；共享 Key 的池 ID 和有限 cap 不重复累计；排除显式多代理池/未验证 Live。
2. 第一批仅选择 `group_ids` 中一个测试组，`quality_routing_enabled=true`，其余新开关 false。Premium/Standard 默认不允许 Basic。确认选择原因、模型映射、续链与 503 明确原因。
3. 第二批开启 `enabled=true`，权重 6/3/1，设置队列/每用户上限和统一预算，`peak.enabled=false`。先限定内部验证组和低流量（建议初始<=10请求/s，远低于压测饱和点），从少量合成或获准真实流量观察，确保无违规 Basic、无持续队列残留、PG 无结算错误、Redis 无脚本超时。
4. 只有手动去重并确认静态容量的资源池，才启用 `capacity_enabled=true`；不宣称自动感知 VM2API。首批不启用峰值容量承诺；完成第 7 节相关峰值验证后单独评估。
5. 观察：每 QoS 请求/提交/成功/取消/质量降级、成功准入份额及排队 P95/P99、503 原因、Strong 实际/注册容量差、metrics_dropped、Redis 脚本延迟、数据库连接和结算失败。全 Strong 不可用时 Premium 拒绝属于质量约束，但持续高失败率仍应停止灰度。
配置操作先集中到同一管理入口、逐次串行保存；跨主机时钟偏差/并发管理写入的版本时序尚未完整验证。

6. 立即停止灰度条件：Premium 违规降级、实际槽超限、重复扣费/对账异常、未知的续链/流重放、Redis 故障仍提交上游、容量注册迟迟不收敛，或明显服务回归。
7. 回滚顺序：停止新灰度流量 → 等待在途/排队排空（不要强制中断 SSE）→ `peak.enabled=false`、`enabled=false` → `quality_routing_enabled=false`；读回配置并等两实例配置缓存刷新。共享别名仍使用中时先保留容量控制，直到确认原并发配置可以独立保护物理容量，再关闭 `capacity_enabled`。全部关闭的模拟基线已验证，繁忙热切换未验证。
8. 不删除审计表、质量元数据或 financial 表；这些新增元数据不要求回滚扣费数据。必要时回滚二进制前排空队列并核对所有实例版本，禁止混合版本对未排空的容量注册表进行管理变更。

首批允许灰度的结论以本报告明确范围和部署检查为前提。没有自动部署。

## 9. 账号配置保存的补充修复（2026-10-26）

用户继续反馈修改账号配置后 Strong 变回 Unknown。上一轮 F2 只保护了管理服务层，未完整覆盖底层通用更新与旧快照写入，本次补齐，不将上一轮测试当作这条遗漏路径已通过的证据。

新复现的两个问题：

- 账号刷新/配置流程持有旧的 `extra` 后，通用 `AccountRepository.Update` 仍会写回整个对象。真实 PostgreSQL 测试证明，旧快照中的缺失标签、Unknown、Basic 都会覆盖数据库里后来设置的 Strong。
- 编辑弹窗只读取 `extra.quality_tier`；接口仅在顶层返回 `quality_tier=strong`、extra 不完整时，会错误显示 Unknown。

修复：

1. 管理请求在补齐旧 extra 之前记录实际提交的质量字段；服务层保留这份修改意图。
2. 在已有数据库行锁内部，以当前行的质量、共享池、来源及审计管理员为准保留未显式修改的元数据。普通配置保存及后台旧对象更新不能改写这些标签。
3. 管理员主动设置 Strong/Basic/Unknown 或清空共享池仍可持久化；显式等级修改继续产生审计记录。
4. 前端使用接口顶层质量字段，兼容旧接口的 extra 回退；普通修改不提交质量等级。

回归与证据：

- `go test -tags=unit ./internal/service ./internal/repository ./internal/handler/admin -run 'Test(AccountQuality|AccountUpdate|GPTQualityMetadata|GPTQoSIndependentAccountField|UpdateAccount|LockAndMergeAccountProbeExtra)' -count=1 -json`：3 包通过，87 个含父/子测试的通过事件；最终代码的 Race 回归 3 包、92 个父/子测试事件通过，包含关联账号隔离和管理字段意图回归。
- `go test -tags=integration ./internal/repository -run '^Test(AccountUpdate(StaleConfigurationPreservesCurrentQuality|ExplicitQualityChangeStillPersists)|GPTQualityAuditMigrationPersistsActorAndOldNew)$' -count=1 -v`：3 个根测试通过，含 3 个旧快照场景、显式三级标签变更与真实 PG 审计检查。
- 前端所有 `EditAccountModal*.spec.ts` 与 `AccountQualityFields.spec.ts`：8 文件、189 项通过；`vue-tsc --noEmit` 与本次变更文件 ESLint 通过。
- 修复前后日志保存在 `artifacts/gpt-qos-final-quality-label-fix/`。没有修改生产账号或自动将 Unknown 提升为 Strong；已经被旧版本实际清空的标签需要管理员根据验证记录重新设置。

本节只补充账号标签保存回归，没有重新执行第 4 节完整压力场景，也没有改变其灰度限制。部署修复需同步更新后端与前端。

## 10. 排队到期被误报为 closed 的补充修复（2026-10-26）

复现用户反馈的 `Scheduling admission expired or was closed. Please retry.`：等待记录达到截止时间时，Lua 清理逻辑和主动取消共用 `closed` 标记。任一实例、后续请求或管理观测读先清理记录后，原请求再次轮询就只看到 closed，无法识别正常排队超时。

本次修复：

- 等待到期保留独立的 `gptqos:{v1}:timed_out` ZSET 原因索引，与原 closed 防复活围栏共存。原因和围栏均保留 120 秒、键空闲 TTL180 秒，并随脚本调用有界清理；重复 close 不抹掉 timeout 原因。没有数据库迁移，也不要求清空 Redis。
- 原请求即使不在本次批量清理的前 32 项中，也会检查自身等待截止时间。已经取得的 grant 仍由执行租约控制，不会因先前排队时间到期而被收回。
- Go 保留同一个等待预算，轮询等待按剩余时间裁剪；兼容旧实例留下的 closed 过期标记，映射为 `gpt_qos_queue_timeout`。真正提前关闭仍保留独立的 `gpt_qos_queue_closed`，不会通过重新排队重置预算或公平进度。
- 在轮询前后检查取消，已取消请求刚拿到的 grant 会释放，不再继续账号复核/提交。

验证：

- 修复前 `TestGPTQoSExpiredWaitIsTimeoutAcrossPollers` 可复现：期待 timeout，实际 closed。失败日志保留。
- 普通相关单测 3 包、65 个父/子测试事件通过；最终 `-race -tags=unit` 的 QoS/质量/Handler 回归 3 包、94 个测试事件通过。原取消/超卖断言保留，排队到期断言更新为明确 timeout；新增已获槽位晚交接和原因索引到期清理测试。
- 真实 Redis `TestGPTQoSRealRedisWaitExpirationRemainsTimeout`、原子抢槽、60 次负载轮转通过，验证其他 owner 清理后返回 timeout，晚到轮询不能复活或占用下一请求槽位。
- 新增真实双服务 `TestGPTQoSAcceptanceTimeoutDiagnostics`，共用隔离 Redis/PG：配置 Strong 等待80ms、占满20个 Strong 槽，第二实例持续观测清理。Premium 约119ms端到端返回503及 `gpt_qos_queue_timeout`，上游调用/账单0；Economy 获准走 Basic 返回200；释放 Strong 后 Premium 返回200；最终仅2条成功用量、排队/运行计数排空。
- 复现命令：构建 `cmd/server` 后设置 `SUB2API_ACCEPTANCE_BINARY`，执行 `go test -tags=integration ./internal/repository -run '^TestGPTQoSAcceptanceTimeoutDiagnostics$' -count=1 -v`；该场景已加入 `scripts/test-gpt-qos-acceptance.sh`。原始证据在 `artifacts/gpt-qos-final-admission-fix/`。

这修复了已证实的误分类及截止时间边界处理；不表示容量不足时可以保证请求成功。真正等满预算仍会明确拒绝，Premium 的质量保证、原子并发、次数上限和计费语义不变。本节未重新运行第4节完整60秒压力场景，原灰度限制仍适用。需更新后端服务二进制使内嵌 Lua 生效。

## 11. API/OAuth 请求数与可折叠分组观测（2026-10-27）

基于 `41f282064` 增加观测能力，未改动调度准入、账号优先级、容量上限及扣费。原列按 `qos_upstream_kind=vm2api/direct` 统计，不能仅改名为 API/OAuth。本次从实际账号选择和提交边界记录账号类型，按一个完成请求的最后一次上游提交归类；旧事件保留未分类，未提交请求不计入两类请求数。

每个 QoS 的可折叠分组子行与汇总共用分组/模型/推理强度筛选、同一观测窗口。分组名称仅在管理快照读取，支持已停用分组，已删除分组或读取失败时保留 ID。组内平均数、成功率和分位数直接从对应请求计算；修改分组等级不会把历史请求迁移到新等级。

已执行验证：

| 命令 / 测试 | 实际结果 |
| --- | --- |
| `go test -race -tags=unit ./internal/service ./internal/handler/admin ./internal/repository -run 'TestGPT(QoS\|Quality)' -count=1 -json` | 3 包通过，89 个含父/子测试的通过事件，0 失败/跳过；涵盖新增协议计数、请求/尝试区分、分组聚合、名称读取失败、Redis 序列化/筛选以及既有 QoS/质量回归 |
| `vitest run src/views/admin/__tests__/GPTQoSSchedulingView.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts` | 2 文件、24 项通过；含各 QoS 独立折叠、分组各列数值、ID 回退、名称转义、筛选刷新、旧快照兼容和双语键完整性 |
| `vue-tsc --noEmit`、变更前端文件 ESLint、`git diff --check`、`bash -n scripts/test-gpt-qos-acceptance.sh` | 通过 |
| `go build -o ../artifacts/gpt-qos-final-observation-update/server ./cmd/server` | 通过 |
| `SUB2API_ACCEPTANCE_BINARY=.../server SUB2API_ACCEPTANCE_EVIDENCE=.../two-processes go test -tags=integration ./internal/repository -run '^TestGPTQoSAcceptanceObservationGroups$' -count=1 -v -timeout=3m` | 真实两个服务进程、隔离 PostgreSQL 18.1 和 Redis 8.4、本地模拟上游，根测试通过；测试本身约 4.69 秒，包含容器准备约 10.94 秒 |

双实例实测共 7 次请求（含 3 次 SSE）：初始 Premium/Standard/Economy 各 2 次，再将原 Premium 分组改成 Standard 并发送 1 次。两个实例均返回 Premium=2、Standard=3、Economy=2，API 请求数=7、OAuth=0；Standard 下同时显示原 Standard 分组与原 Premium 分组。按原 Premium 分组 ID、模型 `gpt-5.2`、`effort=high` 同时筛选后，两个实例都返回 Premium=2、Standard=1，API=3，分组名称和原始 QoS 归属一致。队列和运行计数排空。请求仅命中回环模拟上游。

证据位于 `artifacts/gpt-qos-final-observation-update/`：`go-race.json`、`integration.log`、`two-processes/observation-groups-instance-{0,1}.json` 与服务日志。测试源码新增 `openai_qos_metrics_test.go`、`gpt_qos_metrics_test.go`（admin/repository）、`gpt_qos_acceptance_metrics_test.go`；复现入口已加入验收脚本的 `ObservationGroups` 场景。首次运行测试时修正了一处测试代码引用的不存在状态常量，最终 Race 与真实双实例回归均已通过。

本节没有运行真实 OAuth/付费上游，OAuth 类型记录通过调度器/提交边界单测验证；也未重跑第 4 节 60 秒压力、旧二进制差分和完整浏览器视觉验收，不将此次有限的双实例观测检查视作这些场景通过。原有全功能 CONDITIONAL PASS 限制不变。需同步更新前后端；无需数据库迁移或清空观测记录，升级前未记录的账号类型只能显示未分类。
