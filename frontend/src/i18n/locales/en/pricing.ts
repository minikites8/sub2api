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
        price: 'Custom',
        description: 'For large deployments that need tailored infrastructure and SLAs.',
        features: {
          discount: 'Custom Credit discounts',
          sla: '99.99% uptime SLA',
          support: 'Dedicated 24/7 support',
          infrastructure: 'Custom infrastructure'
        },
        action: 'Contact sales'
      }
    },
    comparison: {
      title: 'Feature comparison',
      feature: 'Feature',
      included: 'Included',
      rows: {
        requests: {
          label: 'API requests',
          payg: 'Usage-based Credits',
          pro: '10,000 Credits monthly',
          enterprise: 'Custom allowance'
        },
        creditPrice: {
          label: 'Credit pricing',
          payg: 'Standard',
          pro: 'Plan discount',
          enterprise: 'Custom discount'
        },
        limits: {
          label: 'RPM and concurrency',
          payg: 'Standard',
          pro: 'Higher limits',
          enterprise: 'Custom'
        },
        scheduling: {
          label: 'Peak scheduling',
          payg: 'Standard',
          pro: 'Priority',
          enterprise: 'Dedicated policy'
        },
        keyLimit: 'Per-key limits',
        ipAllowlist: 'IP allowlist',
        requestLogs: {
          label: 'Request history',
          payg: '7 days',
          pro: '90 days',
          enterprise: 'Custom'
        },
        billingReports: {
          label: 'Usage reports',
          payg: 'Basic',
          pro: 'Full export',
          enterprise: 'Full export'
        },
        customModels: 'Custom models',
        support: {
          label: 'Support',
          payg: 'Community',
          pro: 'Priority tickets',
          enterprise: 'Dedicated support'
        }
      }
    },
    systemStatus: 'System status',
    footerDescription: 'High-performance AI gateway'
  }
}
