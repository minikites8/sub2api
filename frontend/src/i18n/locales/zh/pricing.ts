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
        price: '定制方案',
        description: '面向团队协作、大规模调用及需要精细化成本管理的业务场景。',
        features: {
          credits: '定制 Credits 套餐与阶梯折扣',
          limits: '更高 RPM、TPM 与并发上限',
          scheduling: '专属分组及自定义调度策略',
          team: '团队成员、项目与 API Key 管理',
          reports: '用量报表、预算控制及账单导出',
          security: 'IP 白名单与操作审计',
          onboarding: '接入配置与迁移协助',
          support: '优先技术支持'
        },
        action: '联系销售'
      }
    },
    comparison: {
      title: '功能对比',
      feature: '功能',
      included: '已包含',
      rows: {
        credits: {
          label: 'Credits',
          payg: '按需充值',
          pro: '每月套餐额度',
          enterprise: '定制额度与阶梯折扣'
        },
        throughput: {
          label: 'RPM / TPM',
          payg: '标准上限',
          pro: '更高上限',
          enterprise: '按业务需求定制'
        },
        concurrency: {
          label: '并发数',
          payg: '标准',
          pro: '提升',
          enterprise: '定制'
        },
        scheduling: {
          label: '调度优先级',
          payg: '普通',
          pro: '优先调度',
          enterprise: '专属分组与调度策略'
        },
        keyCount: {
          label: 'API Key 数量',
          payg: '基础数量',
          pro: '更多密钥',
          enterprise: '按项目及成员管理'
        },
        keyLimit: '密钥独立限额',
        ipAllowlist: 'IP 白名单',
        teamMembers: '团队成员管理',
        usageReports: {
          label: '用量报表',
          payg: '基础统计',
          pro: '详细报表',
          enterprise: '项目级报表与导出'
        },
        budget: {
          label: '预算控制',
          enterprise: '团队及项目级预算'
        },
        requestLogs: {
          label: '请求日志',
          payg: '基础期限',
          pro: '延长保存',
          enterprise: '自定义保存周期'
        },
        support: {
          label: '技术支持',
          payg: '社区支持',
          pro: '优先工单',
          enterprise: '接入协助与优先支持'
        },
        contract: {
          label: '合同与发票',
          payg: '按实际情况',
          pro: '按实际情况',
          enterprise: '支持商务合同及开票'
        }
      }
    },
    systemStatus: '系统状态',
    footerDescription: '高性能 AI 网关'
  }
}
