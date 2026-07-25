export default {
  pricingPage: {
    pageTitle: '价格',
    primaryNavigation: '主导航',
    mobileNavigation: '移动端导航',
    openNavigation: '打开导航',
    closeNavigation: '关闭导航',
    docs: '文档',
    pricing: '价格',
    models: '模型广场',
    dashboard: '面板',
    login: '登录',
    hero: {
      title: '简单透明的定价',
      description: '为大规模应用设计的高性能 AI 网关基础设施。按需付费，随时升级。'
    },
    tiersLabel: '计费方案',
    recommended: '默认推荐',
    tiers: {
      payg: {
        name: '按量付费',
        price: '100 Credits',
        unit: '/ 元',
        description: '适合所有规模的应用，充值到账后按实际调用计费。',
        features: {
          usage: '按实际请求量扣除 Credits',
          exchange: '1 Credit = 0.01 USD 调用额度',
          availability: '充值到账后即时可用'
        },
        action: '开始使用'
      },
      pro: {
        name: '专业版',
        price: '10,000 Credits',
        unit: '/ 月',
        description: '面向持续调用、多项目管理及需要更高调用上限的开发者。',
        features: {
          credits: '每月发放 10,000 Credits',
          limits: '更高 RPM 与并发上限',
          priorityScheduling: '高峰期优先调度',
          records: '90 天请求记录及账单导出',
          controls: '预算告警与 IP 白名单',
          support: '优先工单支持'
        },
        action: '升级到专业版'
      },
      enterprise: {
        name: '企业版',
        price: '自定义',
        description: '面向需要定制 SLA 与基础设施的大规模部署。',
        features: {
          discount: '专属 Credits 折扣',
          sla: '99.99% 运行时间 SLA',
          support: '24/7 专属支持',
          infrastructure: '自定义基础设施'
        },
        action: '联系销售'
      }
    },
    comparison: {
      title: '功能对比',
      feature: '功能',
      included: '已包含',
      rows: {
        requests: {
          label: 'API 请求',
          payg: '按需扣除 Credits',
          pro: '每月 10,000 Credits',
          enterprise: '自定义额度'
        },
        creditPrice: {
          label: 'Credits 单价',
          payg: '标准',
          pro: '套餐优惠',
          enterprise: '专属折扣'
        },
        limits: {
          label: 'RPM 与并发',
          payg: '标准',
          pro: '更高上限',
          enterprise: '自定义'
        },
        scheduling: {
          label: '高峰期调度',
          payg: '普通',
          pro: '优先',
          enterprise: '专属策略'
        },
        keyLimit: 'Key 独立限额',
        ipAllowlist: 'IP 白名单',
        requestLogs: {
          label: '请求日志',
          payg: '7 天',
          pro: '90 天',
          enterprise: '自定义'
        },
        billingReports: {
          label: '消费报表',
          payg: '基础',
          pro: '完整导出',
          enterprise: '完整导出'
        },
        customModels: '自定义模型',
        support: {
          label: '支持',
          payg: '社区',
          pro: '优先工单',
          enterprise: '专属支持'
        }
      }
    },
    systemStatus: '系统状态',
    footerDescription: '高性能 AI 网关'
  }
}
