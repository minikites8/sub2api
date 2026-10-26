# GPT/OpenAI 质量与 QoS 调度验收报告

日期：2026-10-26。基线 `main` / `21f675138`。本地实现与测试覆盖四阶段核心文本调度；不表示生产部署验收完成。所有新开关默认关闭。本次没有付费模型调用、生产数据库修改或生产部署。

## 1. 完成情况

| 阶段 | 交付 | 状态 |
| --- | --- | --- |
| Phase 1 | 显式质量标签、事务审计、账号 UI/筛选/批量、强池优先、降级许可、基本日志 | 完成，本地测试通过 |
| Phase 2 | Redis 分层加权公平准入、真实用户轮转、跨 Key 背压、统一等待/请求预算 | 完成，本地测试通过 |
| Phase 3 | 强池聚合去重、物理共享 cap、滞回、严格/借用预留、小池取整、容量 provider 扩展接口 | 完成有限配置模式；真实 VM2API provider 未接入 |
| Phase 4 | 配置/观测 UI、可控灰度、回滚、兼容性与并发测试、报告 | 完成本地验收；生产规模压力/故障切换及人工容量核验待办 |

范围：OpenAI 文本 HTTP、SSE、Responses、Chat Completions、Messages 转换及 Responses WS turn；显式多代理池与映射资源的 Live 音频不在本版保障范围。详见实施报告限制部分。

## 2. 已实际执行的自动化验证

下述命令均已执行，不使用未运行测试作为证据。后端命令从 `backend/` 执行；前端直接调用本地 node_modules 的工具入口，等价于项目脚本中的检查/构建子命令。未安装新依赖。原有测试断言保留。

### 完整后端单元回归

```sh
go test -tags=unit ./internal/service ./internal/handler ./internal/handler/admin ./internal/repository ./internal/handler/dto ./internal/server/routes ./internal/server/middleware ./migrations -count=1
```

退出码 0，八个包全部通过（最后完整运行）：

| 包 | 测试时间 |
| --- | ---: |
| internal/service | 269.148s |
| internal/handler | 124.011s |
| internal/handler/admin | 17.404s |
| internal/repository | 34.149s |
| internal/handler/dto | 4.075s |
| internal/server/routes | 22.973s |
| internal/server/middleware | 8.721s |
| migrations | 0.680s |

涵盖原有高级/优先调度、OAuth 额度、RPM/并发、分组权限、模型映射、reasoning effort、账号终检和原有 SSE/Failover 相关测试。此命令是相关八包完整回归，不是整个 Go 仓库的全平台测试。

### 新功能竞态检测

```sh
go test -race -tags=unit ./internal/service ./internal/repository ./internal/handler -run '^TestGPT(QoS|Quality)' -count=1
```

退出码 0：service 13.527s、repository 84.852s、handler 7.054s，未报告 data race。完整回归后对中性默认分组权重与新增 Failover 降级许可断言额外执行 service 同范围 `-race` 测试，通过，3.013s，无竞态报告。

### 真实 Redis / PostgreSQL 集成

```sh
go test -tags=integration ./internal/repository -run '^TestGPT(QoS|Quality)' -count=1 -v
```

最后单独执行退出码 0，repository 18.066s。测试基础设施在本机创建并清理隔离 Docker Redis 8.4 / PostgreSQL 18.1，执行真实 Lua 和迁移，不使用生产连接。

- 32 个同时到达请求共用 1 槽，精确授予 1 个，最终并发为 1，全部关闭后为 0。
- 审计迁移真实持久化：管理员 42 的 unknown → basic、管理员 43 的 basic → strong，旧值/新值/actor 精确匹配。
- 与普通并发键共存、共享物理池、聚合预留等用例一并通过。
- 一轮同时进行多项大型编译时，真实 Redis 并发用例触发原设 2 秒超时而失败关闭。保留失败记录；停止资源竞争后原命令、原 2 秒超时和原并发断言单独重跑通过。没有放宽超时或删除断言。需要预发布环境进一步验证高负载时超时率。

### 前端

```sh
node node_modules/vitest/vitest.mjs run src/views/admin/__tests__/GPTQoSSchedulingView.spec.ts src/views/admin/__tests__/AccountsView.selectAllResults.spec.ts src/components/account/__tests__ src/i18n/__tests__/localeKeyCompleteness.spec.ts
node node_modules/vue-tsc/bin/vue-tsc.js --noEmit
node node_modules/vite/bin/vite.js build
```

51 个测试文件 / 775 个测试通过，类型检查退出码 0，最终构建成功（16m43s，受并行编译和本机负载影响，不能据此评估构建回归）。构建仍有项目原有大 chunk、混合动态/静态导入、Browserslist 数据过期与 Node shell 弃用提示；未为本次任务升级依赖或改写打包策略。

新增 UI 测试验证默认关闭、显式启用保存、回滚关闭全部新功能且保留分组配置、Premium 默认禁止降级及不写价格字段。旧账号选择测试操作现有确认对话框，Grok 编辑测试等待异步加载完成；保留原业务断言。

### 必要回归修复

```sh
go test -tags=unit ./internal/service -run 'TestPinnedOpenAIModelsListMixedAccountsShareColdCacheAcrossGroups|TestOpenAIModelsLateSingleflightReusesCompletedRefresh' -count=20
```

通过，7.665s。完整回归暴露模型目录缓存 singleflight 的迟到调用可重复刷新，修复增加 flight 内缓存二次检查；原“每上游精确一次”断言未降低，新测试确定性覆盖迟到 flight。

`git diff --check` 通过。未运行全仓 lint 或其他平台集成套件，未宣称这些检查通过。

## 3. 并发、公平性与质量结果

| 用例 | 已验证结果 |
| --- | --- |
| strong/basic 都空闲、basic 账号优先级更高 | strong 胜出，协议类型及订阅偏好不覆盖质量分池 |
| strong 满载，分组禁止降级 | 不调用 basic/unknown，返回容量不可用 |
| strong 满载，显式允许降级 | 有界等待后 basic 可接收 |
| strong 模型不兼容 | 不阻塞合格且获准的 basic；未知质量不冒充 strong |
| response/父会话硬续链 | 保留 basic 原账号；不可用父账号不逃逸自由选号 |
| 持续积压下 100 次服务机会 | Premium 60 / Standard 30 / Economy 10 |
| 同等级两用户 | Premium 各 30、Standard 各 15、Economy 各 5 |
| 同用户多个 API Key | 以 user_id 计同一队列与上限，不增加轮转份额 |
| 两个 cache 客户端、40 个并发请求、单槽 | 无超卖；跨客户端重复释放幂等 |
| 全池 20 槽 / 30% | 高峰预留 6；20 个独立单槽账号聚合后也预留 6 |
| 物理共享池 strong/basic 混合 | basic 和旧调度不能消耗同一物理 Premium 预留 |
| 单槽物理/聚合池 | 预留 0，不因取整无限闲置 |
| 低峰和高峰退出 | 不无故预留；满足退出阈值稳定窗口后共享 |
| 单代理 | 复用现有负数 64 位槽 ID，与旧并发互斥；ID 字符串传 Lua 无精度丢失 |
| 取消/超时/迟到轮询 | 正常 Redis 下队列清理，closed fence 阻止复活；关闭后不继续续租 |
| Redis 故障 | 拒绝准入，无进程内队列替代，不突破并发 |
| 共享 VM Key | 使用同一个显式物理 cap，不能相加重复计算 |

公平性按服务机会计量，无法保证长/短请求消耗相同资源；无限期不释放的上游工作不产生可分配槽。借用模式和已经开始的 SSE 均不能瞬时抢占回收。

## 4. SSE / Failover / 会话兼容性

新增真实 Handler + miniredis + 假上游用例，在 Responses streaming=true/false 两种模式各验证强池胜出、禁降级拒绝、显式降级、全开关关闭保留旧优先级，共 8 个路径；并验证无队列/租约残留。假上游在进程内返回固定响应，不发网络模型调用。

新增取消分类测试：QoS 总预算为 504、租约失效为 503、客户端取消为 499；已提交流内容不被追加 JSON 或重放。统一预算继承更早的父截止时间，提交次数跨 Failover 累计；WS 成功 turn 停止计时器，连接空闲不误判超时。

原有八包回归覆盖发送前账号状态变化、RPM、429 冷却、同账号重试、模型/分组约束、流式提交后的禁止重放。最新补充测试对排除 strong 后的重新选号验证降级许可持续有效。真实上游长 SSE/WS 网络分区和极长推理没有在本次向生产服务发起验证。

## 5. 性能数据与适用范围

真实本地 Redis 单线程顺序测试：预热 20 对，统计 200 对完整“获取 + 释放”。

| 路径 | 平均 | P50 | P95 | 顺序完成对/秒 |
| --- | ---: | ---: | ---: | ---: |
| 关闭开关、原账号槽 | 2.538ms | 2.215ms | 4.980ms | 394.1 |
| QoS 空闲准入 | 0.985ms | 0.867ms | 1.731ms | 1015.3 |

这不是请求全链路、上游 TTFT、生产吞吐或并发饱和延迟；旧路径包含活跃索引维护等额外命令，不能据此宣称“QoS 使模型调用加速”。新路径最少一轮原子 Poll + 一轮 Close，等待时约每 25ms 轮询，执行期间每 10 秒续约；满队列下 Redis 成本必须另做压力验证。

本机 Go/miniredis 微基准，100 次/路径：

| 路径 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| flags_off_account_slot | 651872 | 488276 | 2666 |
| qos_idle_fair_admission | 983182 | 1445907 | 5321 |

模拟器包含 Lua 解释和 Redis 模拟分配，QoS 在此项 CPU/分配更高（约 1.51 倍时间 / 2.96 倍分配字节），不能归因于生产 Go 热路径内存或当成网络吞吐。未对生产规模候选/排队数量做饱和基准。建议灰度先限制分组最大队列、用户等待数和候选池规模，监控 Redis 脚本耗时及队列超时。

## 6. 部署前检查清单

- [ ] 核验账号质量标签，所有原账号默认 unknown；未经验证的账号不填 strong。
- [ ] 核验真实 VM2API 底层共享关系；相同物理资源必须同 pool ID，cap 使用有限且保守值。
- [ ] 备份配置及数据库，在预发布应用 283 迁移并核对管理员审计。
- [ ] 确认当前 Redis 单主架构、时钟、持久性与内存，不直接用于跨 slot Redis Cluster。
- [ ] 先将所有实例更新到支持本功能的版本，再开启容量注册。禁止旧二进制绕过新共享容量脚本。
- [ ] 预发布验证所有实例读取同一配置版本，管理页可读且 Redis 故障告警可见。
- [ ] 排空映射资源的既有 Live 音频；显式多代理与 Live 使用独立未启用的新资源池。
- [ ] 明确 Premium 分组、保护参与和禁止降级，验证用户仅能使用实际授权分组。
- [ ] 验证普通/长推理预算符合真实 SSE 时长，避免部署时误截断正常长任务。
- [ ] 配置日志轮转/保留与审计归档，检查无密钥、token、正文；核对观测丢弃指标。
- [ ] 压测大量排队/短请求/长 SSE、实例被终止、Redis 故障恢复与主从切换；确认 CPU/Redis P95 达标。
- [ ] 核验发布自动化。现有 main 推送会触发 CI/打包及镜像发布；仓库工作流未包含本次主动部署生产的操作。不要将新镜像自动拉入生产灰度外实例。

## 7. 灰度策略

1. 部署代码与迁移，四个开关全部关闭，观察原路径错误率、账务与 RPM/并发。
2. 少量测试分组标记正确 strong/basic，仅开启质量路由。Premium/Standard 禁止降级；核对硬续链与模型兼容。
3. 对同一灰度分组开启全局 QoS，先以较小队列和用户上限运行。核对份额、排队分位数、取消清理和真实账号状态复查。
4. 核实物理池和 cap 后开启共享容量；注意它对同池所有流量全局生效，不受 group_ids 局部限制。
5. 先观察负载再启用峰值保护；严格预留会减少非 Premium 可使用容量。确认 80/65% 滞回、6/20 槽保障及 SSE 自然排空。
6. 人工确认生产指标后逐步扩组。此步骤为操作建议，本次未执行上线。

## 8. 回滚步骤

1. 管理页点击“关闭全部新调度功能”，确认 `enabled`、`quality_routing_enabled`、`capacity_enabled`、`peak.enabled` 全为 false，并读取已保存配置验证。
2. 等待各实例刷新与既有 QoS/SSE/WS turn 自然释放。不要清空并发/lease 键，不强制中断正常流。
3. 验证新请求回到原调度、账务与并发仍正确。保留标签和审计便于排查。
4. 如需回退二进制，先在新版本完成开关关闭和请求排空，再逐实例回退；保留新增审计表/trigger，避免破坏已有数据。
5. Redis 不可用时无法证明策略已全局回滚，不把面板失联当作已生效；先恢复控制面或停止新准入，再按上述步骤核验。

## 9. 未执行项目与原因

- 真实付费模型、VM2API 管理 API/额度探测：按用户要求禁止且没有可证实的 API 合同。
- 生产部署与真实流量灰度：按要求未自动执行。
- 多 OS/跨地域多进程灾难恢复、Redis Cluster、故障切换长期 soak：本地环境只覆盖两个独立 cache 客户端共享 Redis 和原子 Lua，不冒充生产多实例压测。
- 真实浏览器全链路人工点验：本次为 Vue 组件测试、类型检查与构建；上线前需对实际管理权限和输入校验再点验。
- 全仓所有包与所有协议集成、全仓 lint：本次选择调度调用链相关八包与新增真实 Redis/PostgreSQL 用例，未声称其他检查通过。
- 自适应上游 cap 学习、显式多代理池、Live 保障：本版没有实现，均有明确拒绝/隔离边界。

## 10. 验收判断

核心文本场景具备进入小范围预发布/受控灰度的代码条件；生产容量保障须完成第 6 节人工检查及高负载故障演练。不能宣称可直接全量上线或真实上游容量已经验证。计费公式、余额、价格倍率未改；新开关默认关闭。最终提交与远程状态在交付回复中记录。

本地证据目录：`artifacts/gpt-qos-20261026/`（测试日志，未将大日志提交 Git）。资源竞争时的失败记录为 `integration-contended.log`，最终通过记录为 `integration-final.log`；补充 service 验证为 `service-final.log`。


## 11. 分组 QoS 保存/回显修复补充验收（2026-10-26）

修复前新增复现测试：Premium/Economy 使用主“更新分组”保存时 QoS 请求次数为 0，两项重开验证和一项失败反馈验证失败。修复后保留这些断言并通过：

- 分组页面、全局调度页面和语言键回归：10 个文件 / 42 项测试全部通过（`GroupsView*.spec.ts`、`GPTQoSSchedulingView.spec.ts`、locale completeness）。
- 验证主保存后关闭重开仍为 Premium/Economy、保留其他分组、基本字段不变更 QoS、保存失败保留弹窗及明确提示。
- 验证旧全局页面保存不覆盖较新分组配置、迟到 GET 不覆盖当前分组、旧分组请求完成不写入或关闭新分组弹窗、服务端未确认时不显示成功。
- PostgreSQL 集成 `TestGPTQoSGroupConfigPersistsAcrossServiceInstances` 通过（repository 4.545s）；创建新的 SettingService 后读回 Premium，保存另一 Economy 后原 Premium 保持不变，功能开关不会被保存动作打开。
- 前端类型检查通过，生产构建通过（23.18s），`git diff --check` 通过；本次后端仅增加持久化验证测试，没有修改后端运行逻辑，未重复运行上文八包完整回归。

本次不访问生产数据库，不发送真实模型调用，不执行生产部署。旧版本的临时使用方法是点击 QoS 区域独立的“保存”；升级此修复后统一使用底部“更新分组”。日志保存在 `artifacts/gpt-qos-group-save-20261026/`。

## 12. 账号字段隐藏与 QoS 503 修复补充验收（2026-10-26）

本次只读查看了管理员账号编辑界面、错误详情和按请求 ID 过滤的系统日志，没有修改生产设置。用户提供的那条请求日志为 `QoS selected account changed`，未到达上游。旧日志无法区分其具体终检条件，因此没有把单一生产请求强行归因为共享池配置或代理绑定。

修复前的自动化复现：

- 账号质量组件默认挂载时应显示共享池及上游来源；新增测试实际失败，显式隐藏选项测试通过，确认是可选布尔属性默认值缺陷。
- 使用实际 `schedulerCache.SetSnapshot/GetSnapshot`、miniredis、Responses Handler 与进程内假上游：单代理 Strong 的非流式、流式两项均返回 503，期望 200。复现日志明确包含原来的泛化客户端错误，未发送外部模型请求。

已执行并通过：

- `go test -tags=unit ./internal/service ./internal/handler ./internal/repository -count=1`：三包通过，分别 238.231s / 102.489s / 15.763s。
- `go test -race -tags=unit ./internal/service ./internal/handler ./internal/repository -run 'TestGPT(QoS|Quality)' -count=1`：三包通过，分别 4.932s / 7.195s / 42.642s，无竞态报告。
- `go test -tags=integration ./internal/repository -run 'TestGPTQoSRealRedis(AtomicAdmission|CachedProxySelectionSharesLegacySlot)$' -count=1 -v`：通过（4.601s），日志确认临时 PostgreSQL 18.1 / Redis 8.4 实际启动，两个测试实际运行，没有因 Docker 不可用跳过。新增用例经过实际调度入口、候选元数据和完整快照；QoS 持有代理槽时，另一 cache 实例的普通抢槽被拒绝，重复释放无残留，释放后可重新获取。账号终检使用固定仓库 fixture，设置保存使用临时 PostgreSQL；不是生产多进程压测。
- 账号管理、QoS 页面、分组 QoS 及语言键前端回归：52 个文件 / 781 项测试通过；新增组件测试验证默认显示、显式隐藏和池/来源编辑事件。
- `vue-tsc --noEmit` 通过；调整中英文容量提示后的 Vite 生产构建通过（18.12s）。构建有既有大 chunk 提示，不是构建失败。
- `git diff --check` 通过。

关键断言：

- 模型筛选发生在公平队列之前；完整快照的模型白名单、分组模型限制、分组授权、质量标签和账号状态变化会在入队前剔除。明确允许降级时，不兼容 Strong 不阻塞合格 Basic；禁止降级时 Basic/Unknown 不进入队列。
- 保留映射模型支持；候选投影缺失代理信息时，完整快照决定负数代理绑定槽，不能误抢正数账号槽。非流式与流式均成功，已有槽满载时仍拒绝，完成后没有队列/租约残留。
- 抢槽后账号禁用、代理变化、并发变化、共享池变化仍拒绝，释放原准入并提供固定细分原因；没有为通过测试绕开终检。
- 队列满、超时、关闭、状态不可读、账号变化等返回固定安全错误码及说明；已开始的 Responses SSE 使用 `response.failed`，不追加普通 JSON 或错误重放。
- 三包竞态回归同时覆盖既有加权份额、用户公平、防饥饿、Premium 保障、取消清理和槽位安全测试。没有把重试计作多个用户请求或改变计费路径。

新增成本是 QoS 启用时批量读取完整候选快照。未重新测量生产规模全链路 P95/TTFT，也没有把旧的纯准入微基准当作本修复性能结果。没有执行付费模型、真实长 SSE/网络分区、生产部署或全量上线；显式多代理池仍是已有功能边界。应先更新所有实例及前端，再按第 7 节小范围灰度；回滚继续使用第 8 节关闭全部新开关并等待租约自然释放。

日志目录：`artifacts/gpt-qos-503-20261026/`。本修复不增加数据库迁移，不改变 Redis 键或 Lua 脚本，不需要清理已有并发键。

## 13. 高峰保护状态展示补充验收（2026-10-26）

修复前，“已启用但未触发”以及“保存后刷新观测”的新增复现测试失败。修复后执行页面/分组回归及语言键/消息编译：4 个文件、25 项测试全部通过；`vue-tsc --noEmit` 通过，最终 Vite 生产构建通过（39.05s）。构建仍有既有 chunk 体积提示，分组测试有 JSDOM XHR 诊断输出，测试进程退出码为 0；未隐藏这些输出。

覆盖已启用待触发、未保存的开关不影响运行状态、总强池保护对单池生效、无强池容量不参与、关闭配置时不采信旧的触发样本、Redis 不可用、旧实例配置不同步、新版本配置覆盖旧观测、保存后自动刷新，以及迟到的保存前观测不得覆盖新结果。保留分组主保存/重开不丢失 QoS 的回归断言。

生产只读刷新显示强池 0/26，默认阈值 80%/65%，进入/退出窗口 5 秒/15 秒，因此“尚未触发”符合当时负载。没有发送模型请求制造高峰，没有保存线上开关，也没有部署。本次仅修改前端展示/刷新与 API 类型，没有重复运行后端调度单元或 Redis 压测；高峰算法和账务路径保持原样。后端算法测试证据见前述章节，不将它们冒充本次重新运行的结果。

日志目录：`artifacts/gpt-qos-peak-ui-20261026/`。本修复部署前端即可生效；关闭全部新功能的回滚入口及后端行为不变。

## 14. 来源无关的同质量池负载分配补充验收（2026-10-26）

修复前实际复现：第一个 Strong 有占用但仍有空位、第二个 Strong 空闲时，返回第一个账号；两个 Key 共用的根池与一个独立池串行完成 60 次时，前者占 60 次，后者为 0。修复后保持精确断言，通过低负载选择及 30/30 空闲池轮转测试，没有通过修改账号标签或降低断言掩盖问题。

已执行的三包完整单元回归：`go test -tags=unit ./internal/service ./internal/handler ./internal/repository -count=1`，service 239.366s / handler 98.727s / repository 13.665s 全部通过。覆盖普通及高级/优先调度、既有 OAuth 额度偏好、模型/分组限制、硬续链、SSE、Failover、RPM、额度与 Feature Flag 关闭路径。

新回归还覆盖：正常 OAuth 与 API Key 都是 Strong 时，较低负载者胜出（包括旧优先级、订阅偏好相反的情况）；完整队列与只质量路由两种模式；2/10 占用优于 1/2；同一共享根下空闲 Key 不能掩盖 8/10 的根池拥塞；多个 Key 不增加物理池空闲轮转份额；Basic 更空闲也不能越过合格 Strong。原有 Premium/Standard/Economy 份额、用户公平、防饥饿、容量保护和跨 cache 实例并发断言保留并通过。

前端账号字段及中英文键/消息编译回归 3 个文件、8 项通过，类型检查通过，最终 Vite 生产构建通过（19.02s，保留既有 chunk 体积提示）。没有修改线上账号质量标签或生产开关，没有发送真实模型请求；正常 OAuth 是否标为 Strong 仍由管理员显式验证。来源字段只影响统计。

`go test -race -tags=unit ./internal/service ./internal/handler ./internal/repository -run 'TestGPT(QoS|Quality)' -count=1`：三包通过，分别 7.636s / 3.855s / 48.890s，无竞态报告。

真实 Redis 集成：`go test -tags=integration ./internal/repository -run 'TestGPTQoSRealRedis(AtomicAdmission|CachedProxySelectionSharesLegacySlot|LoadRouting|CandidateScanLatencySample)$' -count=1 -v` 通过（4.834s）。日志确认临时 PostgreSQL 18.1 / Redis 8.4 实际启动、用例实际执行，没有跳过。32 路单槽争抢、完整快照代理槽与另一 cache 的普通抢槽、60 次三池空闲分配 20/20/20、首池占用 6/10 时选择空闲第二池均通过，无队列/租约残留。

本机真实 Redis 顺序“准入 + 释放”小样本：各候选数预热 5 次后统计 20 次，最近秩分位数：

| 候选账号数 | P50 | P95 |
| --- | ---: | ---: |
| 1 | 0.340ms | 0.394ms |
| 16 | 0.616ms | 0.657ms |
| 64 | 1.241ms | 1.258ms |
| 256 | 3.724ms | 3.823ms |

这是修改后不同候选规模的本地顺序测量，不是改造前后对照，也不是全链路 P95、CPU 饱和吞吐或模型 TTFT。新路径比较全部有界候选，空闲时脚本工作量较原“首个可用”增加；没有新增网络往返。未测满 2000 等待请求、多模型阻塞及 Redis 故障切换时的生产规模吞吐，灰度仍应限制队列/候选规模并监控脚本耗时。未执行生产部署，未宣称流量必按固定比例分配：空闲同负载才轮转，运行中按占用率分配，并且必须先满足模型、状态与容量约束。

日志目录：`artifacts/gpt-qos-load-routing-20261026/`。没有数据库迁移或新 Redis 键；新增的哈希/池元数据字段兼容旧记录。全部实例更新后再验收一致的负载行为，回滚仍使用关闭新开关及自然排空步骤。
