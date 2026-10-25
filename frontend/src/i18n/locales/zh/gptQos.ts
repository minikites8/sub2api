export default {
  aggregateStrong: '去重后强池总容量',
  queued: '等待请求', active: '执行并发 / 有效容量', strongCapacity: '强池并发 / 有效容量', reservedUsed: 'Premium 保障使用 / 预留', premiumActive: 'Premium 在用', peakReason: '高峰状态原因', downgradeReasons: '质量降级原因', upstreamShares: '上游实际请求占比', firstSuccessRate: '首次成功率 %',
  basicWait: 'Basic 降级池等待预算（毫秒）',
  strongHitRate: '强池命中率 %',
  rolloutGroups: '灰度分组 ID（逗号分隔，留空应用于全部分组）',
  upstreamKind: '上游来源', direct: '直接登录', vmShare: 'VM2API 请求数', directShare: '直接登录请求数', metricsDropped: '丢弃的观测事件数',
  title: 'GPT 质量与 QoS 调度', accountQuality: '账号质量与共享容量', qualityTier: '质量等级', strong: '高质量 Strong', basic: '普通质量 Basic', unknown: '未验证 Unknown',
  sharedPool: '共享资源池标识（独立容量留空）', accountHint: '质量由管理员显式标记。共享同一 VM2API 底层资源的账号填写相同池标识，并在调度页配置该池的总并发上限。', bulkQuality: '批量修改质量与共享池',
  qualityRouting: '质量优先路由', qosEnabled: '全局公平排队', capacityEnabled: '共享资源池容量控制', peakEnabled: '高峰保护', strictReserve: '严格预留（允许借用时无法保证立即获得槽位）',
  enter: '高峰进入占用率', exit: '高峰退出占用率', ratio: 'Premium 预留比例', enterHold: '进入稳定窗口（毫秒）', exitHold: '退出稳定窗口（毫秒）',
  premiumWeight: 'Premium 默认权重', standardWeight: 'Standard 默认权重', economyWeight: 'Economy 默认权重', maxWaiting: '全局最大排队数', requestBudget: '普通请求总预算（毫秒）', highBudget: '长推理总预算（毫秒）', maxAttempts: '最大上游提交次数', sampleRate: '决策日志采样率',
  save: '保存', refresh: '刷新观测', rollback: '关闭全部新调度功能', pools: '共享资源池', poolID: '池标识', concurrency: '并发上限', addPool: '添加资源池', remove: '移除',
  effective: '实际执行顺序', effectiveHint: '权限/模型/硬续链 → QoS 与容量 → 质量池/降级许可 → 池内原调度 → 原子槽位 / RPM / 发送前复核', defaultsOff: '新功能默认关闭。QoS 依赖质量路由，高峰保护依赖 QoS 和容量控制。未标记账号不进入强池。',
  saved: '配置已保存', failed: '操作失败', groupQos: '分组 QoS（独立于销售倍率）', tier: '服务等级', weight: '分组权重', fallback: '允许降级至 Basic', strongWait: '强池最大等待（毫秒）', groupMaxWaiting: '分组最大排队数', userMaxWaiting: '单用户最大排队数（跨 API Key）', protection: '参与 Premium 容量保障', groupConcurrency: '分组并发限制（0 为不限制）',
  groupHint: 'Premium 和 Standard 默认禁止降级。硬续链保留原账号。此处单独保存调度配置。', metrics: '调度观测', redisUnavailable: 'Redis 不可用：QoS 请求失败关闭，容量保障不可用。', filters: '观测筛选', model: '请求模型', effort: '推理强度', groupID: '分组 ID',
  requests: '请求数', attempts: '尝试次数', successes: '成功请求', firstSuccess: '首次成功', canceled: '客户端取消', downgrades: '质量降级', strongHits: '强池请求', average: '平均等待 ms', p50: 'P50 ms', p95: 'P95 ms', p99: 'P99 ms', capacity: '全局队列与资源池状态',
  metricsWindow: '近 24 小时最多 10000 个完成请求；请求与重试分开统计。', qualityFilter: '全部质量等级', createFirst: '创建分组后可在编辑界面配置 QoS。'
}
