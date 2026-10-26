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

观测按完成请求计数，不把重试重复记为用户请求；展示各等级请求/提交/成功/首次成功率、强池命中率、平均等待与 P50/P95/P99、取消、降级及原因、显式来源占比、聚合/物理池并发和 Premium 预留使用、高峰触发原因。支持 group/model/effort 筛选；容量状态为全局资源事实，筛选只作用于请求统计。此窗口有界且可能丢弃，不是永久账务统计或审计账本。

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
