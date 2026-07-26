export default {
  pricingPage: {
    pageTitle: 'Pricing',
    primaryNavigation: 'Primary navigation',
    mobileNavigation: 'Mobile navigation',
    openNavigation: 'Open navigation',
    closeNavigation: 'Close navigation',
    docs: 'Docs',
    pricing: 'Pricing',
    models: 'Models',
    dashboard: 'Dashboard',
    login: 'Sign in',
    hero: {
      title: 'Simple, transparent pricing',
      description: 'High-performance AI gateway infrastructure built for applications at any scale. Pay as you go and upgrade when you need to.'
    },
    tiersLabel: 'Pricing plans',
    recommended: 'Recommended',
    tiers: {
      payg: {
        name: 'Pay as you go',
        price: '100 Credits',
        unit: '/ CNY',
        description: 'Flexible usage-based billing for applications of every size.',
        features: {
          usage: 'Credits deducted from actual usage',
          exchange: '1 Credit = 0.01 USD of API usage',
          availability: 'Credits available immediately after payment'
        },
        action: 'Get started'
      },
      pro: {
        name: 'Pro',
        price: '10,000 Credits',
        unit: '/ month',
        description: 'For developers with sustained usage, multiple projects, and higher request limits.',
        features: {
          credits: '10,000 Credits issued each month',
          limits: 'Higher RPM and concurrency limits',
          priorityScheduling: 'Priority scheduling during peak traffic',
          records: '90-day request history and billing exports',
          controls: 'Budget alerts and IP allowlists',
          support: 'Priority ticket support'
        },
        action: 'Upgrade to Pro'
      },
      enterprise: {
        name: 'Enterprise',
        price: 'Custom plan',
        description: 'For teams, high-volume workloads, and businesses that need fine-grained cost control.',
        features: {
          credits: 'Custom Credit packages with volume discounts',
          limits: 'Higher RPM, TPM and concurrency ceilings',
          scheduling: 'Dedicated groups and custom scheduling policies',
          team: 'Member, project and API key management',
          reports: 'Usage reports, budget controls and billing exports',
          security: 'IP allowlists and action audit logs',
          onboarding: 'Onboarding and migration assistance',
          support: 'Priority technical support'
        },
        action: 'Contact sales'
      }
    },
    comparison: {
      title: 'Feature comparison',
      feature: 'Feature',
      included: 'Included',
      rows: {
        credits: {
          label: 'Credits',
          payg: 'Top up as needed',
          pro: 'Monthly plan allowance',
          enterprise: 'Custom allowance with volume discounts'
        },
        throughput: {
          label: 'RPM / TPM',
          payg: 'Standard ceiling',
          pro: 'Higher ceiling',
          enterprise: 'Sized to your workload'
        },
        concurrency: {
          label: 'Concurrency',
          payg: 'Standard',
          pro: 'Raised',
          enterprise: 'Custom'
        },
        scheduling: {
          label: 'Scheduling priority',
          payg: 'Standard',
          pro: 'Priority scheduling',
          enterprise: 'Dedicated groups and policies'
        },
        keyCount: {
          label: 'API keys',
          payg: 'Basic quota',
          pro: 'More keys',
          enterprise: 'Managed per project and member'
        },
        keyLimit: 'Per-key limits',
        ipAllowlist: 'IP allowlist',
        teamMembers: 'Team member management',
        usageReports: {
          label: 'Usage reports',
          payg: 'Basic stats',
          pro: 'Detailed reports',
          enterprise: 'Project-level reports and export'
        },
        budget: {
          label: 'Budget controls',
          enterprise: 'Team and project budgets'
        },
        requestLogs: {
          label: 'Request history',
          payg: 'Standard retention',
          pro: 'Extended retention',
          enterprise: 'Custom retention'
        },
        support: {
          label: 'Support',
          payg: 'Community',
          pro: 'Priority tickets',
          enterprise: 'Onboarding help and priority support'
        },
        contract: {
          label: 'Contracts and invoices',
          payg: 'Case by case',
          pro: 'Case by case',
          enterprise: 'Commercial contracts and invoicing'
        }
      }
    },
    systemStatus: 'System status',
    footerDescription: 'High-performance AI gateway'
  }
}
