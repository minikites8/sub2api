# GPT/OpenAI 质量与 QoS 调度实施报告

日期：2026-10-26。基线：`main` / `21f675138`。本次未部署生产、未发送真实模型请求。用户后续授权在测试完成后提交并推送主分支，提交结果见最终交付。仓库原有发布工具修改及测试数据未纳入本功能。

## 1. 审计结论与实施范围

实际调用链为 OpenAI Handler → `SelectAccountWithSchedulerForCapability` / `selectAccountWithSchedulerOnce` → 旧普通选号或 `defaultOpenAIAccountScheduler.Select` → 账号终检与原子槽位 → 原有 RPM/额度/发送准入 → HTTP/SSE/WS → 原有受控重试和 Failover → 释放。

- 普通调度默认按账号优先级和可用性选择，并处理粘性等待；高级调度由已有配置开启，使用负载、错误率、TTFT 等组成 Top-K 顺序。独立的优先调度设置可让匹配分组/模型的文本请求进入高级选号，并应用体验/利润评分、容量档位、OAuth 额度偏好和公平选号。
- 原有订阅优先、BPS 优先、OAuth 5h/周额度排序均属于账号偏好，不能承担用户排队优先权。本次将它们限制在已确定的质量池内部。
- 分组权限、模型映射/白名单、协议/端点能力、隐私、父账号状态、利润门和额度自动暂停继续在候选资格及发送前检查中生效。
- Redis 原子账号槽使用 ZSET，已具备跨实例一致的并发限制；原有账号评分与进程内统计本身不构成跨实例用户公平队列。本次新增 Redis 全局队列并直接原子获取同一账号槽。
- `previous_response_id` 和不可迁移父会话先维护账号所有权；完整上下文可重建的普通会话使用稳定会话种子和池内负载选号，可以避开拥塞软粘性。
- 原有账号 `extra` JSONB 与 settings 配置足以存储质量、资源池和分组 QoS，无需增加账务字段或改动余额、销售倍率、Token 价格及扣费公式。
- 未找到可依赖的 VM2API 实时管理容量协议。本次没有添加假定的 HTTP 管理接口或额度探测请求。
- 管理前端实际为 Vue 3，沿用现有 Vue 表单、路由、认证与 API 客户端，未引入 React。

实施按四阶段落地：质量标签及强池优先；全局用户公平准入；聚合强池和共享物理池保护；有界观测、配置 UI 与兼容性测试。各阶段通过配置独立灰度，所有新开关默认关闭。范围边界见第 11 节，不能据此宣称已完成真实 VM2API 生产压测。

## 2. 主要代码位置

| 位置 | 职责 |
| --- | --- |
| `service/openai_quality_config.go` | 显式质量、分组 QoS、配置校验/缓存和灰度范围 |
| `service/openai_quality_router.go` | 质量硬分池、池内复用现有调度、决策状态 |
| `service/openai_qos_admission.go` | 真实用户准入、统一队列预算、最终状态复查、租约续期与释放 |
| `repository/lua/gpt_qos.lua` | 跨实例加权公平队列与账号/分组/资源池原子占槽 |
| `repository/gpt_qos_capacity.go`、原有 `concurrency_cache.go` | 共享池去重、普通选号共同遵守容量限制、聚合强池保护 |
| `service/openai_qos_{websocket,cancellation,observation,metrics}.go` | 每请求/每 WS turn 的预算、实际提交次数、有界观测 |
| `handler/gpt_qos_request.go`、OpenAI handlers、`failover_loop.go` | 预算继承、取消分类、失败后受控重选、输出后不重放 |
| `handler/admin/gpt_qos_handler.go`、admin 路由 | 管理设置和筛选后的观测快照 |
| `frontend/src/views/admin/GPTQoSSchedulingView.vue` | 开关、权重、阈值、容量池、灰度与观测页面 |
| `frontend/src/components/account/AccountQualityFields.vue` | 新建/编辑/批量质量、共享池与显式来源 |
| `frontend/src/components/group/GroupQoSEditor.vue` | 与价格独立的分组 QoS 编辑 |

## 3. 数据与迁移

迁移 `backend/migrations/283_gpt_quality_audit.sql` 新建 `account_quality_audit` 及账号 extra 更新触发器。字段为账号 ID、管理员 ID、旧等级、新等级、数据库时间；旧值默认为 unknown。标签写入与审计记录在同一个数据库事务中完成。管理员 ID 来自已认证的 admin middleware，服务层覆盖外部伪造的 actor 值；直接 SQL 维护需自行设置合法 actor，未知 actor 为 NULL/0。

账号元数据：

- `extra.quality_tier`：strong / basic / unknown；缺失等价 unknown。旧账号不回填 strong。
- `extra.qos_resource_pool`：可选显式物理池 ID（1–64 个字母、数字、下划线、短横线），相同底层资源使用相同 ID。
- `extra.qos_upstream_kind`：unknown / vm2api / direct，仅用于来源观测，不参与质量推断。OAuth 缺失来源时可统计为 direct，质量仍只读显式标签。

调度配置保存于 settings 的 `gpt_qos_scheduling_v1`，含按真实分组 ID 索引的 `groups`、资源池表和配置版本。用户无法通过请求参数选择 QoS。配置沿用已有 settings 存取机制；本实例保存后发布新快照，其余实例按约 1 秒的按需后台刷新获取。不在请求热路径增加同步配置 SQL。刷新失败保留上一份有效配置；冷启动无法加载策略时保守拒绝 OpenAI 调度并等待恢复。

账号管理变更后同步刷新容量注册。新增/批量/筛选/导出都传递质量字段。独立审计表不删除历史账号的审计记录，也没有自动清理任务；部署方需按审计保留政策归档。

## 4. 实际策略优先级

1. 认证、授权分组、模型和协议/端点兼容、隐私、额度/利润等原有安全准入。
2. 不可迁移的 response / 父会话所有权；原账号不可用时拒绝，不偷偷换号。原账号可以是 basic 或 unknown，该续链不记为质量降级。
3. 对自由选号实施真实用户 QoS 公平队列、分组并发和高峰保护。
4. strong 优先；只有分组显式允许且强池等待预算耗尽/无合格 strong 时才尝试 basic。unknown 从自由选号池排除。
5. 同一质量池内复用原高级/优先调度的健康、TTFT、负载、账号优先级、成本、订阅/BPS/OAuth 额度偏好。
6. 原子占用现有账号槽，并在发送前继续执行最新账号状态/资源绑定、模型、权限、利润、RPM、额度等检查。

质量路由关闭时走原有选号分支。质量路由单独开启时复用原选号器在强池中做有界抢槽；全局 QoS 开启后由 Redis 同时决定公平准入和实际账号槽，不产生第二个无主容量预留。

软粘性保留稳定会话种子和已有缓存上下文，取消无条件优先。此版没有新增基于缓存命中概率的学习模型。

## 5. QoS 算法与 Redis 结构

采用分层加权公平队列（与按服务机会计量的 WDRR 等价公平目标）：tier 虚拟完成时间按权重递增，同 tier 的真实用户按虚拟服务量轮转，同一用户内部 FIFO。默认 Premium/Standard/Economy 权重 6/3/1；分组权重相对该等级默认权重调整份额。API Key ID 不参与公平身份，增加 Key 不增加用户份额。持续有合格容量时低等级也有服务机会；长期占槽的请求无法被抢占，公平性按准入次数计量，不是 GPU 时间或 Token 数。

一次原子 Lua 最多分配一个 grant，可以为其他正在等待的请求授予；grant 持久化，调用方轮询读取，不依赖进程内通知。每等级扫描最多 64 个用户；没有合格用户时轮转扫描偏移。每个请求至多 256 个候选；超过上限拒绝而不截断强约束。候选筛选沿用原批量缓存，最新数据库终检仅在实际授予槽位后执行，复查失败释放并在剩余预算内换候选。

Redis 前缀 `gptqos:{v1}:`：

| 后缀 | 类型与用途 |
| --- | --- |
| `requests` | HASH：安全 ID、候选容量、截止时间与已分配 grant；不存请求正文/凭据 |
| `waiting` / `leases` / `closed` | ZSET：队列截止时间、运行租约、取消/超时后短期封锁迟到轮询 |
| `users:<tier>` / `user:<tier>:<user>` | ZSET：真实用户轮转与用户 FIFO |
| `tierwait:*` / `groupwait:*` / `userwait:*` | ZSET：背压与监控计数，单用户跨 Key 限制 |
| `fair` | HASH：虚拟完成时间、轮转及增量清理游标 |
| `group:<id>` / `pool:<pool>` / `strong:<pool>` | ZSET：分组并发、去重物理池并发与其中强账号并发 |
| `pool_tier:*` / `all_strong` / `all_strong_premium` | ZSET：等级占用、聚合强池占用与受保护 Premium 占用 |
| `capacity_policy` / `account_pools` / `pool_limits` / `strong_limits` / `account_quality` | HASH：显式映射与有限容量注册，配置时原子刷新 |
| `slot_pool:<request>` | 带 TTL 的 STRING：普通调度槽位到共享资源池的释放关联 |
| `pools` / `global_peak` | HASH / STRING：物理池及聚合强池的高峰状态和滞回时间 |
| `metrics` | LIST：近 24 小时至多 10000 个完成请求的有界观测窗口 |

实际并发仍写入 `concurrency:account:<slotID>`；Live 的原槽位也参与普通账号占用检查。64 位账号/代理槽/用户/分组 ID 以字符串传给 Lua，避免负数代理槽被 double 精度改变。普通单代理使用原有 `AccountProxySlotID`；显式多代理池暂不纳入 QoS。

队列默认全局 1000、分组 200、单用户 4，可调整且上限有校验。过期等待/孤儿运行租约每次增量清理最多各 32 条；空用户队列及历史池元数据有清理。正常取消立即尝试关闭队列/租约。账号槽 TTL 复用已有设置，活动请求每 10 秒续约；释放用 once，重复调用幂等。续约失败取消请求，不在转发尚未停止时提前放回容量；Redis 失联的遗留状态按 TTL 保守过期。

Redis 故障时 QoS 返回服务不可用并记录 `gpt_qos_redis_unavailable` / `gpt_qos_release_failed`。不启用进程内替代队列，不宣称此时仍有严格保障。现有原子并发也不会因 QoS 故障被绕过。

## 6. 高峰保护和 VM2API 容量

- 聚合强池容量按资源池标识去重：每个共享池最多计入配置物理 cap 与活跃 strong 成员并发总和的较小值；独立强账号计一次。多个 API Key 不累计成多个物理池。
- 同时维护聚合强池和物理池保护。默认强占用率 ≥80% 持续 5 秒进入；≤65% 持续 15 秒退出，使用 Redis 时间；状态按调度活动观察更新。
- 高峰保留 `floor(capacity * ratio)`，容量 ≥2 且 ratio>0 时至少 1、至多 capacity-1；全池只有 1 槽则预留 0。聚合 20 槽预留 6，包含 20 个独立单槽账号的场景。物理池局部保护可因整数取整而比总体 30% 更保守。
- 严格模式阻止非受保护 Premium 的新请求占用保障容量；旧调度和非灰度调用也遵守启用的物理池/聚合保护。已运行 SSE 不驱逐，峰值刚切入时可能需要等已运行请求自然释放。
- 共享池中 basic 请求也受物理根池保护，不能通过另一个 Key 消耗同一物理预留。低峰不预留；借用模式在没有 Premium 等待时可共享，但不能承诺即时回收容量。
- 新启用容量策略时先从既有普通账号槽引导占用，再原子发布池限制。配置 cap 不是上游已证实的实际物理容量，管理员必须填写保守有限值。
- `UpstreamCapacityProvider` 和带来源时间的 snapshot 为扩展接口；可靠性检查拒绝超过 30 秒、未来时间或无界容量。当前未接入实际 VM2API provider，也未实现自适应容量学习。使用有限配置和原有成功/429 冷却/TTFT 健康策略；不会额外发模型探测。

## 7. 统一预算、SSE 与 Failover

QoS 开启的请求在 API Key/用户排队前创建统一上下文预算：普通默认 300000ms，high/xhigh/max 默认 900000ms，均可调。继承客户端更早截止时间；强池等待在首次自由选号开始后固定，换号不重置。允许降级的分组可另设 basic 等待预算（默认 1000ms），仍受总预算限制。默认强等待分别为 Premium 5000 / Standard 2000 / Economy 500ms。

HTTP、SSE、Responses/Chat Completions/Messages/Embeddings/alpha search 的新预算串联现有等待和受控发送。WS 的预算按 response.create turn 计算，成功结束停止 timer，不因连接空闲而误取消。取消信号不从 QoS 上游上下文脱离。

实际 transport 提交边界计数（默认上限 4），不把队列轮询算作模型尝试；所有选号与 Failover 重入仍受原次数限制和冻结的质量/分组许可。写出不可撤回流数据后保留原有禁止透明跨账号重试约束。预算耗尽返回 504、租约失效返回 503、真实客户端取消为 499/已提交流关闭，不将本地超时/取消计入账号质量失败。清理取消后恢复 Handler 父上下文，避免外层 defer 把本地容量拒绝误标为客户端断开。

## 8. 后台与观测

新增 `/api/v1/admin/gpt-qos/config` GET/PUT、`/api/v1/admin/gpt-qos/snapshot` GET；管理页 `/admin/gpt-qos`。账号新增/编辑/批量质量、池标识、来源，账号列表支持质量筛选、徽标与筛选后的批量/导出。分组编辑点击底部“更新分组”会同时保存已修改的 QoS；与销售倍率仍使用独立的调度配置存储。QoS 写入失败保留弹窗和修改，明确提示分组基本字段已保存、QoS 尚未保存，不显示全部成功。

独立开关：`quality_routing_enabled`、`enabled`（公平队列）、`capacity_enabled`、`peak.enabled`，全部 false；公平队列依赖质量路由，峰值保护依赖队列和容量控制。`group_ids` 可限定质量/QoS 灰度；共享物理 cap 是全局资源约束，启用后同池旧流量也受限，不能将它误解为仅灰度分组生效。

后台可调等级、权重、降级许可、强/basic 等待、队列上限、用户上限、分组并发、保护参与、普通/长推理总预算、提交次数、阈值/滞回/比例/严格模式、采样率、共享 cap。可查看当前有效顺序与配置版本，关闭全部新功能按钮保留标签/配置以便回滚再启用。

决策记录字段包含 request_id/user_id/group_id、等级、请求/生效模型与 effort、最终质量/账号/池、策略/粘性原因、排队和选号耗时、提交次数、降级原因、峰值状态、配置版本及结果。不记录密钥、OAuth token、正文或完整会话。固定长度标签、有界 1024 条后台通道，写日志/Redis 不阻塞模型路径；丢弃与写失败计数可见。决策日志默认采样 10%，沿用既有日志轮转/保留设置（默认 100MB、10 份备份、7 天），需运维配置实际日志输出与采集。

观测按完成请求计数，不把重试重复记为用户请求；展示各等级请求/提交/成功/首次成功率、强池命中率、平均等待与 P50/P95/P99、取消、降级及原因、实际 API/OAuth 请求占比、聚合/物理池并发和 Premium 预留使用、高峰触发原因。支持 group/model/effort 筛选；容量状态为全局资源事实，筛选只作用于请求统计。此窗口有界且可能丢弃，不是永久账务统计或审计账本。

## 9. 配置与回滚兼容性

旧高级调度/优先调度设置均保留；质量开关关闭时不改旧选号策略。关闭所有新开关不撤销质量标签、不删除审计、不变更计费，新的请求走旧路径，已开始的 QoS 请求保留被冻结策略并自然释放。不得为回滚直接清空并发键或强制中断 SSE。

本次完整回归触发已有模型目录缓存的 singleflight 迟到调用重复刷新问题；增加缓存内二次检查，保留原测试精确一次调用断言，并新增确定性回归。前端旧选择确认测试改为点击现有确认对话框，Grok 编辑测试等待加载完成再操作，未删除或降低业务断言。

## 10. 风险控制

最主要风险是错误的质量标签/共享物理池标识/并发 cap，而不是账号协议。启用前必须人工核验。配置保存为整个 settings 文档，多个管理员同时保存仍为最后写入者生效，不提供跨管理员字段级 CAS；避免并发编辑。配置存储和 Redis 注册不能跨系统事务提交，保存失败时应检查实际读取值并回滚后重试。

实际可用容量还受到 RPM、模型兼容、额度、冷却及上游隐形排队约束；保障的是有限并发资源的准入优先权，不能承诺任意模型或推理强度的固定 TTFT。容量注册随管理变更刷新，运行时冷却/额度变化可使注册容量暂时偏高，最终候选与发送门仍会拒绝不可用账号。高峰状态是活动触发观测，不是独立后台探测器。

## 11. 已知限制与人工验收边界

- 未连接真实 VM2API 容量接口，没有主动探测、自适应 cap 学习或真实额度验证。
- 显式多代理绑定账号不进入新 QoS 保证池；普通单代理支持并复用真实槽 ID。需要该资源的强续链会失败关闭，不会绕过并发。
- Live 音频使用另一套槽转移生命周期。启用共享/峰值映射的资源暂时拒绝新的 Live 转移/准入；上线前必须排空既有 Live，并用独立账号池承载。文本 HTTP/SSE/Responses WS 继续支持。
- Redis Lua 使用与现有并发键同一实例原子执行。未实现 Redis Cluster 跨 slot 多键事务或跨地域调度；此版本面向现有单主 Redis 部署。
- 公平服务机会不等于按 Token 或运行秒数公平；不会驱逐长 SSE。严格预留形成的瞬间也不能抢回已借出的运行槽。
- 有界观测用于运营定位，不能作为精确计费或无限期历史报表。质量审计表保留期需运维制定。
- 真实多进程故障切换、Redis 主从切换、长时间网络分区、真实浏览器全链路及付费上游压测仍需部署前人工/预发布环境验证；本次只运行了本地 mock、真实本地 Redis/PostgreSQL 和构建测试。

## 12. 本次文件清单

以下为本功能与必要回归修复文件，不含工作区原有无关改动。`.gitignore` 另增加两份交付报告的跟踪例外：

- `backend/internal/handler/admin/account_data.go`
- `backend/internal/handler/admin/account_handler.go`
- `backend/internal/handler/admin/gpt_qos_handler.go`
- `backend/internal/handler/dto/mappers.go`
- `backend/internal/handler/dto/types.go`
- `backend/internal/handler/failover_loop.go`
- `backend/internal/handler/gpt_qos_handler_test.go`
- `backend/internal/handler/gpt_qos_request.go`
- `backend/internal/handler/openai_alpha_search.go`
- `backend/internal/handler/openai_chat_completions.go`
- `backend/internal/handler/openai_embeddings.go`
- `backend/internal/handler/openai_gateway_handler.go`
- `backend/internal/repository/account_repo.go`
- `backend/internal/repository/concurrency_cache.go`
- `backend/internal/repository/gpt_qos_benchmark_test.go`
- `backend/internal/repository/gpt_qos_cache.go`
- `backend/internal/repository/gpt_qos_cache_test.go`
- `backend/internal/repository/gpt_qos_capacity.go`
- `backend/internal/repository/gpt_qos_integration_test.go`
- `backend/internal/repository/lua/gpt_qos.lua`
- `backend/internal/repository/scheduler_cache.go`
- `backend/internal/server/routes/admin.go`
- `backend/internal/service/admin_account.go`
- `backend/internal/service/admin_service.go`
- `backend/internal/service/api_key_slot_lease.go`
- `backend/internal/service/controlled_experiment_transport.go`
- `backend/internal/service/openai_account_scheduler.go`
- `backend/internal/service/openai_codex_models_service.go`
- `backend/internal/service/openai_gateway_scheduling.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/openai_models_list_test.go`
- `backend/internal/service/openai_qos_admission.go`
- `backend/internal/service/openai_qos_budget_test.go`
- `backend/internal/service/openai_qos_cancellation.go`
- `backend/internal/service/openai_qos_capacity.go`
- `backend/internal/service/openai_qos_metrics.go`
- `backend/internal/service/openai_qos_observation.go`
- `backend/internal/service/openai_qos_websocket.go`
- `backend/internal/service/openai_quality_config.go`
- `backend/internal/service/openai_quality_router.go`
- `backend/internal/service/openai_quality_router_test.go`
- `backend/internal/service/openai_turn_admission.go`
- `backend/internal/service/openai_ws_forwarder_ingress.go`
- `backend/internal/service/openai_ws_forwarder_support.go`
- `backend/internal/service/openai_ws_forwarder_v2.go`
- `backend/internal/service/openai_ws_v2_passthrough_adapter.go`
- `backend/internal/service/setting_service.go`
- `backend/migrations/283_gpt_quality_audit.sql`
- `frontend/src/api/admin/accounts.ts`
- `frontend/src/api/admin/gptQos.ts`
- `frontend/src/components/account/AccountQualityFields.vue`
- `frontend/src/components/account/BulkEditAccountModal.vue`
- `frontend/src/components/account/CreateAccountModal.vue`
- `frontend/src/components/account/EditAccountModal.vue`
- `frontend/src/components/account/__tests__/EditAccountModal.grokMediaEligibility.spec.ts`
- `frontend/src/components/group/GroupQoSEditor.vue`
- `frontend/src/components/layout/AppSidebar.vue`
- `frontend/src/i18n/locales/en/gptQos.ts`
- `frontend/src/i18n/locales/en/index.ts`
- `frontend/src/i18n/locales/zh/gptQos.ts`
- `frontend/src/i18n/locales/zh/index.ts`
- `frontend/src/router/index.ts`
- `frontend/src/views/admin/AccountsView.vue`
- `frontend/src/views/admin/GPTQoSSchedulingView.vue`
- `frontend/src/views/admin/GroupsView.vue`
- `frontend/src/views/admin/__tests__/AccountsView.selectAllResults.spec.ts`
- `frontend/src/views/admin/__tests__/GPTQoSSchedulingView.spec.ts`
- `frontend/src/views/admin/__tests__/GroupsView.qos.spec.ts`

交付文档：本报告与 `GPT_QOS_SCHEDULING_ACCEPTANCE_REPORT.md`。

## 13. 分组 QoS 保存/回显修复（2026-10-26）

用户反馈修改分组 QoS 后重新打开仍显示 Standard。复现确认旧版“更新分组”按钮只调用原 groups.update，未保存 QoS；原独立保存按钮与主按钮的语义不一致。

修复将 QoS 编辑器接入分组主保存流程，移除嵌入场景中的第二个保存按钮，并在写入返回后核对服务端确认的字段。仅基本字段修改时不重写 QoS；QoS 保存失败保留修改并显示部分保存提示。加载时忽略此前分组的迟到响应，保存时固定原分组/编辑器引用，避免切换弹窗后写错分组或关闭新弹窗。全局调度页面在保存前读取最新 groups，保留该页面打开后从分组弹窗保存的配置。

相关分组回归同时发现原 Codex Manifest 表单缺失保存前校验及重开后的草稿重置，按现有精确断言补齐，未降低测试要求。没有改动调度算法、计费、账号质量或默认开关。

## 14. 账号容量字段与 QoS 503 修复（2026-10-26）

只读核对用户提供的请求后，系统日志中的实际错误为 `QoS selected account changed`，该请求没有进入上游。旧日志把多个终检条件合并为同一错误，不能据此确定该生产请求具体命中了哪个比较项；没有把它归因为共享容量开关，也没有修改生产配置或发送模型探测。

本地使用真实候选投影、完整 Redis 快照、Handler 和假上游复现了一个确定缺陷：`sched:meta` 不包含代理绑定，队列按正数账号 ID 抢槽；完整账号使用负数 `AccountProxySlotID`，因此终检拒绝。修复在入队前复用 `SchedulerSnapshotService.GetAccounts` 批量读取完整快照，再确定候选模型资格和真实槽位。Redis 使用现有分批 MGET；缓存缺失沿用有上限的批量数据库回退，没有新增逐候选 SQL 查询。

实际顺序保持为：模型映射及现有候选资格筛选 → 质量池内部排序 → 完整快照的模型/分组/隐私/端点/质量复核 → QoS 公平准入及原子账号槽 → 完整账号与数据库终检 → 原有 RPM、额度及发送前准入。完整快照中的质量若已变化，剔除该候选，不能借缓存变化把 Basic 放进 Strong 池；模型不兼容的 Strong 不占公平服务机会，也不延迟明确允许的 Basic 后备。

终检没有放宽。账号禁用、并发上限变化、代理槽绑定变化和资源池变化仍拒绝并释放原租约；增加了 `account_unavailable`、`concurrency_changed`、`slot_binding_changed`、`resource_pool_changed` 等服务端原因。无候选时记录排除原因计数，包括不支持的显式代理池、无限/未知并发、未启用或未配置的共享容量。日志仅含请求/分组/账号 ID 和固定原因，不包含令牌、密钥或正文。

增加 `GPTQoSSelectionError`，Responses、Chat Completions 和 Embeddings 的选号失败返回允许列表内的 `error.code` 及说明，例如 `gpt_qos_account_changed`、`gpt_qos_queue_timeout`、`gpt_qos_no_eligible_capacity`、`gpt_qos_unavailable`。已开始的 Responses SSE 使用 `response.failed` 终止事件，不追加普通 JSON，也不重放上游请求。原有非 QoS 错误保留原分类；客户端取消不会误记为 Redis 故障。

账号表单的 `showPool` / `showSource` 是可选布尔属性，Vue 会将缺省值转为 false，导致新增/编辑页只显示质量等级。显式默认 true 后恢复共享池及上游来源输入；批量编辑仍遵守显式隐藏选项。中英文提示说明：独立容量留空；确认共用底层资源才填写相同池标识，并在公平排队前配置池总并发、启用共享容量控制。Strong 不要求填写池标识。

主要改动文件：

- `backend/internal/service/openai_qos_admission.go`、`openai_qos_error.go`、`openai_qos_admission_test.go`
- `backend/internal/handler/no_account_error.go`、`openai_gateway_handler.go`、`openai_chat_completions.go`、`openai_embeddings.go`、`gpt_qos_handler_test.go`
- `backend/internal/repository/gpt_qos_integration_test.go`：真实 Redis 的完整快照/代理槽/另一 cache 实例竞争回归
- `frontend/src/components/account/AccountQualityFields.vue` 及对应组件回归测试
- `frontend/src/i18n/locales/zh/gptQos.ts`、`frontend/src/i18n/locales/en/gptQos.ts`

本修复不增加迁移，不改变 Redis 键或 Lua 原子槽协议，不修改计费。所有开关关闭时不执行新增完整候选读取。显式多代理池的独立生命周期仍未实现，继续拒绝参与公平容量保障；普通单代理账号使用原有代理绑定槽。性能新增成本为 QoS 启用时对合格候选的批量快照读取，未将先前准入微基准当作本修复的全链路性能结果。

## 15. 高峰保护开关与触发状态展示修复（2026-10-26）

用户反馈启用高峰保护后，资源池表格仍显示“已禁用”。只读刷新生产页面时，功能开关已勾选，强池观测为 0/26，原因为 `below_threshold_or_hysteresis_hold`。旧表格直接将 `pool.peak` 的 false 翻译为“已禁用”；该字段表示当前是否已触发保护，不能代表配置开关。

前端改为区分“已关闭”“已启用，待触发”“保护中”。没有强池容量的池、不足的观测、Redis 不可用以及配置版本不同步单独显示，不宣称已具备保护。Strong 池同时考虑总强池的全局保护状态，避免总池已触发、单池未触发时误显示待触发。原因字段增加中文/英文说明，并按已观测的配置显示进入/退出阈值及稳定窗口。

展示优先使用快照接口已有的 `config` 字段；旧响应缺少该字段时，使用独立保存的已确认配置。编辑草稿不改变运行状态。保存/回滚成功后自动刷新观测，并用请求序号忽略迟到的旧观测，防止保存前的结果覆盖保存后的状态。更高版本的实例配置仍作为最新观测，不被旧表单覆盖。

涉及 `frontend/src/views/admin/GPTQoSSchedulingView.vue`、对应页面测试、`frontend/src/api/admin/gptQos.ts`、中英文 `gptQos.ts`。没有修改后端调度、高峰阈值、容量预留、数据库、Redis 数据结构或计费；没有更改生产配置。

## 16. 同质量池按负载调度，不按来源集中分配（2026-10-26）

质量始终由 `quality_tier` 显式决定。正常 OAuth 可以配置为 Strong，与任何来源的 Strong 账号共同承接请求；API Key、OAuth、Base URL 和 `qos_upstream_kind` 均不用于推断质量。没有批量修改线上标签，也没有将全部 OAuth 自动归类为 Strong 或 Basic。

本次定位到的集中分配原因是队列脚本遇到“第一个仍有空位的候选”就返回，缺少同池实时负载比较。因此旧偏好排序靠前的账号可能承接全部串行短请求，即使其他 Strong 账号闲置。这是候选选择问题，不是 VM2API 质量规则。

新顺序为：已有模型/分组/端点/状态资格及硬续链 → QoS 用户公平与高峰保护 → 合格质量等级 → 同质量的实时归一化占用 → 空闲同负载池轮转 → 原候选顺序的软偏好。Lua 在原子抢槽事务内使用 `max(账号槽占用/账号上限, 物理资源池占用/有效池上限)` 比较，包含已有普通并发与 Live 占用。Strong 有可用容量时不会因 Basic 更空闲而降级；无降级许可时 Basic 仍不入队。

负载相同按物理池最近分配序号轮转，避免低并发短请求永远命中第一账号。同一共享池的多个 API Key 共用序号，不能靠增加 Key 数获得更多空闲轮转份额。只开启质量路由、未开启全局队列时，复用现有负载快照，按负载、等待数、最近使用时间稳定排序，再保留旧策略的同序偏好；该路径不宣称具有队列的原子全局轮转保证。已有手动备用资格规则保持不变。

Redis 不增加键：在 `gptqos:{v1}:fair` 增加 `dispatch_sequence` 字段，在已有池元数据增加 `last_dispatch` 字段；复用原有池元数据回收。旧记录缺字段按 0 兼容。WDRR/QoS 用户份额、原子槽获取/释放、RPM、额度、续链、SSE 和计费没有改写。新规则由现有质量路由/公平排队开关限定，全部关闭时不执行这些选择逻辑。

主要修改为 `openai_quality_router.go`、`repository/lua/gpt_qos.lua` 及对应 service/repository/真实 Redis 集成回归。管理界面来源字段改为“仅统计”，容量提示改为通用共享资源表述。Lua 需要比较全部有界候选，既有上限仍为 256，网络往返次数没有增加，但空闲场景 Redis 脚本工作量增加；候选规模耗时实测见验收补充。混合旧/新实例期间可能仍有旧实例采用首个可用规则，全部实例升级后才能获得一致的负载选择行为。无需迁移或清空 Redis；回滚继续关闭新开关并等待运行中租约自然释放。

## 17. 管理页池名称展示（2026-10-26）

资源池表格优先显示 `账号质量等级:账号名`，例如 `Strong:正常 OAuth`、`Basic:账号 B`、`Unknown:账号 C`。共享池在同一行显示成员名称，使用 ` / ` 分隔，不拆分容量行。内部 `account:id` / `shared:pool` 保留为稳定身份、表格 key 和诊断数据，不以可变名称作为调度或并发键。

快照接口增加可选的 `display_name`。只在管理快照请求中通过一次批量 SQL 投影获取可见账号/共享成员的 ID、名称、显式质量等级和共享池关系，不读取 Credentials/OAuth Token，不进入模型请求热路径。账号改名后刷新可读取新名称；查询有 2 秒上限，名称不可读或新前端连接旧后端时回退原标识，容量和峰值观测仍可使用。

新增 `backend/internal/service/openai_qos_pool_labels.go`、`backend/internal/repository/gpt_qos_pool_labels.go`，接入 `openai_qos_metrics.go`；更新前端 API 类型及 `GPTQoSSchedulingView.vue`。没有数据库迁移、Redis 键变化、账号写入、调度排序或计费变化。

## 18. API/OAuth 统计与 QoS 可折叠分组明细（2026-10-27）

- 观测表将 VM2API 请求数和直接登录请求数替换为 API 请求数、OAuth 请求数；下方请求占比也使用实际账号类型。调度选择记录账号 Type，实际 transport 提交边界冻结本次提交的类型，完成事件新增可选 `account_type`。每个用户请求仅计一次，多次尝试按最后一次实际提交的账号类型归类；未发送的请求不计入 API/OAuth 数量。后续选号但未发送不会覆盖之前的提交类型。原手动来源字段与旧统计响应仍保留兼容，不用于推断 API/OAuth。
- 历史 Redis 事件缺少账号类型时保留未分类，不根据 VM2API、direct 或质量标签反推。无需清空 Redis、补写历史或修改数据库；原 24 小时/10000 条窗口与异步有界采集保持不变。
- 快照增加 `groups: { [qos_tier]: [{ group_id, group_name?, ...metrics }] }` 和 `account_type_requests`。各 QoS 行默认收起，点击展开同列的分组统计，显示名称和 ID，支持分别展开/收起。汇总与明细使用同一筛选后的事件集；分组修改 QoS 后，历史请求保留原始等级。分组分位数直接从该组请求计算。
- 管理接口通过一次有界的元数据读取补齐包含已停用分组的名称；已删除/名称不可读时显示分组 ID。该查询仅发生在管理观测接口，不增加模型请求的同步数据库查询。旧后端缺少新字段时前端显示缺失提示，不将旧来源计数伪装成账号类型计数。
- 修改 `openai_qos_metrics.go`、`openai_quality_router.go`、`openai_qos_observation.go`、`openai_qos_websocket.go`、管理快照 Handler、前端 API/页面和中英文文案；新增 service、repository、admin 与真实双服务观测回归。测试证据见最终验收报告第 11 节。调度、容量、RPM 与计费规则没有变更。
