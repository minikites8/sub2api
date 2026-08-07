import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsageView from '../UsageView.vue'

const {
  query,
  getById,
  getStats,
  getDashboardModels,
  getDashboardSnapshotV2,
  list,
  getAvailable,
  showError,
  showWarning,
  showSuccess,
  showInfo,
} = vi.hoisted(() => ({
  query: vi.fn(),
  getById: vi.fn(),
  getStats: vi.fn(),
  getDashboardModels: vi.fn(),
  getDashboardSnapshotV2: vi.fn(),
  list: vi.fn(),
  getAvailable: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
}))

const messages: Record<string, string> = {
  'admin.dashboard.timeRange': 'Time range',
  'admin.dashboard.granularity': 'Granularity',
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.users.columnSettings': 'Columns',
  'admin.usage.group': 'Group',
  'admin.usage.billingType': 'Billing type',
  'admin.usage.billingMode': 'Billing mode',
  'admin.usage.allTypes': 'All types',
  'admin.usage.allBillingTypes': 'All billing types',
  'admin.usage.billingTypeBalance': 'Balance',
  'admin.usage.billingTypeSubscription': 'Subscription',
  'admin.usage.allBillingModes': 'All billing modes',
  'admin.usage.billingModeToken': 'Token',
  'admin.usage.billingModePerRequest': 'Per request',
  'admin.usage.billingModeImage': 'Image',
  'admin.usage.allGroups': 'All groups',
  'admin.usage.allModels': 'All models',
  'usage.allApiKeys': 'All API Keys',
  'usage.apiKeyFilter': 'API Key',
  'usage.model': 'Model',
  'usage.type': 'Type',
  'usage.ws': 'WS',
  'usage.stream': 'Stream',
  'usage.sync': 'Sync',
  'usage.async': 'Async',
  'usage.exporting': 'Exporting',
  'usage.exportCsv': 'Export CSV',
  'usage.failedToLoad': 'Failed to load',
  'usage.noDataToExport': 'No data',
  'usage.preparingExport': 'Preparing export',
  'usage.exportSuccess': 'Export success',
  'usage.exportFailed': 'Export failed',
  'usage.costDetails': 'Cost Breakdown',
  'usage.inputTokenPrice': 'Input price',
  'usage.outputTokenPrice': 'Output price',
  'usage.perMillionTokens': '/ 1M tokens',
  'usage.serviceTier': 'Service tier',
  'usage.serviceTierPriority': 'Fast',
  'usage.rate': 'Rate',
  'usage.time': 'Time',
  'usage.userAgent': 'User-Agent',
  'usage.reasoningEffort': 'Reasoning Effort',
  'usage.inboundEndpoint': 'Inbound Endpoint',
  'usage.requestDetails.action': 'Details',
  'usage.requestDetails.openAria': 'View request details',
  'usage.requestDetails.title': 'Request Details',
  'usage.requestDetails.request': 'Requested model',
  'usage.requestDetails.requestContext': 'Request Context',
  'usage.requestDetails.requestId': 'Request ID',
  'usage.requestDetails.routing': 'Routing & Model Options',
  'usage.requestDetails.transport': 'Transport',
  'usage.requestDetails.websocket': 'WebSocket',
  'usage.requestDetails.streaming': 'Streaming response',
  'usage.requestDetails.synchronous': 'Synchronous response',
  'usage.requestDetails.nodeId': 'Processing Node ID',
  'usage.requestDetails.client': 'Client',
  'usage.requestDetails.ipAddress': 'IP Address',
  'usage.requestDetails.imageMetadata': 'Image Parameters',
  'usage.requestDetails.mediaType': 'Media Type',
  'usage.requestDetails.asyncTask': 'Async Task',
  'usage.requestDetails.taskKind': 'Task Type',
  'usage.requestDetails.taskKindGrokVideo': 'Grok Video Generation',
  'usage.requestDetails.taskId': 'Task ID',
  'usage.requestDetails.taskStatus': 'Task Status',
  'usage.requestDetails.statusUrl': 'Status URL',
  'usage.requestDetails.resultUrl': 'Result URL {index}',
  'usage.requestDetails.expiresAt': 'Result Expires At',
  'usage.requestDetails.videoMetadata': 'Video Parameters',
  'usage.requestDetails.videoCount': 'Video Count',
  'usage.requestDetails.videoResolution': 'Resolution',
  'usage.requestDetails.videoDuration': 'Duration (seconds)',
  'usage.requestDetails.failedToLoad': 'Failed to load request details',
  'usage.analytics.firstTokenLatency': 'First token',
  'usage.analytics.totalLatency': 'Total latency',
  'admin.usage.inputCost': 'Input Cost',
  'admin.usage.outputCost': 'Output Cost',
  'admin.usage.cacheCreationCost': 'Cache Creation Cost',
  'admin.usage.cacheReadCost': 'Cache Read Cost',
  'common.refresh': 'Refresh',
  'common.reset': 'Reset',
}

vi.mock('@/api', () => ({
  usageAPI: {
    query,
    getById,
    getStats,
    getDashboardModels,
    getDashboardSnapshotV2,
  },
  keysAPI: {
    list,
  },
  userGroupsAPI: {
    getAvailable,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess, showInfo }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const simpleStub = { template: '<div><slot /></div>' }
const chartStub = { template: '<div />' }

const usageLog = {
  id: 1,
  request_id: 'req-user-export',
  node_id: 'edge-cn-01',
  actual_cost: 0.092883,
  total_cost: 0.092883,
  rate_multiplier: 1,
  service_tier: 'priority',
  input_cost: 0.020285,
  output_cost: 0.00303,
  cache_creation_cost: 0.000001,
  cache_read_cost: 0.069568,
  input_tokens: 4057,
  output_tokens: 101,
  cache_creation_tokens: 4,
  cache_read_tokens: 278272,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  image_count: 0,
  image_size: null,
  first_token_ms: 12,
  duration_ms: 345,
  created_at: '2026-03-08T00:00:00Z',
  model: 'gpt-5.4',
  reasoning_effort: null,
  inbound_endpoint: null,
  user_agent: 'openai-node/4.71.1',
  ip_address: '203.0.113.10',
  api_key: { name: 'demo-key' },
  group: { name: 'default' },
  billing_mode: 'token',
  request_type: 'sync',
  stream: false,
}

function mountUsageView() {
  return mount(UsageView, {
    global: {
      stubs: {
        AppLayout: simpleStub,
        Pagination: true,
        Select: true,
        DateRangePicker: true,
        Icon: true,
        UsageStatsCards: chartStub,
        UsageTable: chartStub,
        ModelDistributionChart: chartStub,
        GroupDistributionChart: chartStub,
        EndpointDistributionChart: chartStub,
        TokenUsageTrend: chartStub,
        UsageTokenBarChart: chartStub,
        UsageModelCreditChart: chartStub,
        UsageLatencyHeatmap: chartStub,
        LoadingSpinner: chartStub,
      },
    },
  })
}

describe('user UsageView', () => {
  beforeEach(() => {
    query.mockReset()
    getById.mockReset()
    getStats.mockReset()
    getDashboardModels.mockReset()
    getDashboardSnapshotV2.mockReset()
    list.mockReset()
    getAvailable.mockReset()
    showError.mockReset()
    showWarning.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()

    query.mockResolvedValue({ items: [usageLog], total: 1, pages: 1 })
    getById.mockResolvedValue(usageLog)
    getStats.mockResolvedValue({
      total_requests: 1,
      total_input_tokens: 10,
      total_output_tokens: 20,
      total_cache_tokens: 0,
      total_tokens: 30,
      total_cost: 0.1,
      total_actual_cost: 0.08,
      average_duration_ms: 12,
      endpoints: [],
      upstream_endpoints: [],
      endpoint_paths: [],
    })
    getDashboardModels.mockResolvedValue({
      models: [{ model: 'gpt-5.4', requests: 1, input_tokens: 10, output_tokens: 20, cache_creation_tokens: 0, cache_read_tokens: 0, total_tokens: 30, cost: 0.1, actual_cost: 0.08 }],
      start_date: '2026-03-08',
      end_date: '2026-03-08',
    })
    getDashboardSnapshotV2.mockResolvedValue({
      generated_at: '2026-03-08T00:00:00Z',
      start_date: '2026-03-08',
      end_date: '2026-03-08',
      granularity: 'hour',
      trend: [],
      groups: [],
    })
    list.mockResolvedValue({ items: [{ id: 1, name: 'demo-key' }] })
    getAvailable.mockResolvedValue([{ id: 1, name: 'default' }])
  })

  it('loads logs, stats, model stats, and snapshot on first render', async () => {
    const wrapper = mountUsageView()
    await flushPromises()

    expect(query).toHaveBeenCalled()
    expect(getStats).toHaveBeenCalled()
    expect(getDashboardModels).toHaveBeenCalled()
    expect(getDashboardSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      include_trend: true,
      include_model_stats: false,
      include_group_stats: true,
    }))
    expect(list).toHaveBeenCalledWith(1, 100)
    expect(getAvailable).toHaveBeenCalled()
    expect(wrapper.text()).toContain('default (1x)')
    expect(wrapper.findAll('[data-testid="token-details-trigger"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-testid="credit-details-trigger"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-testid="request-details-trigger"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="log-latency"]').text()).toContain('First token12ms')
    expect(wrapper.get('[data-testid="log-latency"]').text()).toContain('Total latency345ms')
  })

  it('shows auxiliary request metadata in the request details dialog', async () => {
    query.mockResolvedValue({
      items: [{ ...usageLog, reasoning_effort: 'high', inbound_endpoint: '/v1/responses' }],
      total: 1,
      pages: 1,
    })
    getById.mockResolvedValue({
      ...usageLog,
      reasoning_effort: 'high',
      inbound_endpoint: '/v1/responses',
      video_count: 1,
      video_resolution: '1080p',
      video_duration_seconds: 8,
      async_task: {
        kind: 'grok_video',
        task_id: 'video-task-1',
        status: 'submitted',
        status_url: '/v1/videos/video-task-1',
        result_urls: ['https://cdn.example.com/video.mp4'],
      },
    })

    const wrapper = mountUsageView()
    await flushPromises()

    await wrapper.get('[data-testid="request-details-trigger"]').trigger('click')
    await flushPromises()

    const dialog = document.body.querySelector('[data-testid="request-details-dialog"]')
    expect(dialog?.textContent).toContain('Request IDreq-user-export')
    expect(dialog?.textContent).toContain('Inbound Endpoint/v1/responses')
    expect(dialog?.textContent).toContain('Processing Node IDedge-cn-01')
    expect(dialog?.textContent).toContain('IP Address203.0.113.10')
    expect(dialog?.textContent).toContain('User-Agentopenai-node/4.71.1')
    expect(dialog?.textContent).toContain('Reasoning EffortHigh')
    expect(getById).toHaveBeenCalledWith(1)
    expect(dialog?.textContent).toContain('Video Parameters')
    expect(dialog?.textContent).toContain('Resolution1080p')
    expect(dialog?.textContent).toContain('Task IDvideo-task-1')
    expect(dialog?.querySelector('a[href="https://cdn.example.com/video.mp4"]')).not.toBeNull()
    expect(dialog?.textContent).not.toContain('Billing Context')

    wrapper.unmount()
  })

  it('shows the requested Credit breakdown details', async () => {
    const wrapper = mountUsageView()
    await flushPromises()

    await wrapper.get('[data-testid="credit-details-trigger"]').trigger('mouseenter')
    const tooltip = document.body.querySelector('.usage-credit-tooltip')

    expect(tooltip?.textContent).toContain('Input Cost2.0285 Credits')
    expect(tooltip?.textContent).toContain('Output Cost0.303 Credits')
    expect(tooltip?.textContent).toContain('Cache Creation Cost0.0001 Credits')
    expect(tooltip?.textContent).toContain('Cache Read Cost6.9568 Credits')
    expect(tooltip?.textContent).toContain('Input price500 Credits / 1M tokens')
    expect(tooltip?.textContent).toContain('Output price3,000 Credits / 1M tokens')
    expect(tooltip?.textContent).toContain('Service tierFast')
    expect(tooltip?.textContent).toContain('Rate1x')
    expect(tooltip?.querySelectorAll('dl > div')).toHaveLength(8)

    wrapper.unmount()
  })

  it('projects 7-day Credits from month-to-date actual usage', async () => {
    const wrapper = mountUsageView()
    await flushPromises()

    const now = new Date()
    const monthStart = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-01`
    const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
    const projectedCredits = (0.08 * 100 / now.getDate()) * 7
    const projectedText = new Intl.NumberFormat(undefined, {
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    }).format(projectedCredits)

    expect(getStats).toHaveBeenCalledWith(expect.objectContaining({
      start_date: monthStart,
      end_date: today,
    }))
    expect(wrapper.text()).toContain('usage.analytics.estimated7DayCredits')
    expect(wrapper.text()).toContain(`${projectedText} Credits`)

    const viewModel = wrapper.vm as any
    viewModel.applyDatePreset(30)
    await flushPromises()

    const projected30DayCredits = (0.08 * 100 / now.getDate()) * 30
    const projected30DayText = new Intl.NumberFormat(undefined, {
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    }).format(projected30DayCredits)
    expect(wrapper.text()).toContain('usage.analytics.estimated30DayCredits')
    expect(wrapper.text()).toContain(`${projected30DayText} Credits`)
  })

  it('exports csv with current filters and without admin-only fields', async () => {
    const wrapper = mountUsageView()
    await flushPromises()

    let exportedBlob: Blob | null = null
    let csvContent = ''
    const OriginalBlob = globalThis.Blob
    vi.stubGlobal('Blob', vi.fn((parts: BlobPart[], options?: BlobPropertyBag) => {
      csvContent = parts.map((part) => String(part)).join('')
      return new OriginalBlob(parts, options)
    }))
    const originalCreateObjectURL = window.URL.createObjectURL
    const originalRevokeObjectURL = window.URL.revokeObjectURL
    window.URL.createObjectURL = vi.fn((blob: Blob | MediaSource) => {
      exportedBlob = blob as Blob
      return 'blob:usage-export'
    }) as typeof window.URL.createObjectURL
    window.URL.revokeObjectURL = vi.fn(() => {}) as typeof window.URL.revokeObjectURL
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    await (wrapper.vm as any).exportToCSV()

    expect(exportedBlob).not.toBeNull()
    expect(query).toHaveBeenCalledWith(expect.objectContaining({
      page_size: 100,
      sort_by: 'created_at',
      sort_order: 'desc',
    }))
    expect(clickSpy).toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalled()
    expect(csvContent.startsWith('\uFEFF')).toBe(true)
    expect(csvContent.slice(1)).toBe([
      'Time,API Key Name,Model,Reasoning Effort,Inbound Endpoint,IP Address,Type,Billing Mode,Input Tokens,Output Tokens,Cache Read Tokens,Cache Creation Tokens,Rate Multiplier,Actual Credits,Standard Credits,First Token (ms),Duration (ms)',
      '2026-03-08T00:00:00Z,demo-key,gpt-5.4,"\'-",,203.0.113.10,Sync,Token,4057,101,278272,4,1,9.2883,9.2883,12,345',
    ].join('\n'))
    expect(csvContent).toContain('IP Address')
    expect(csvContent).toContain('203.0.113.10')
    expect(csvContent).toContain('Actual Credits')
    expect(csvContent).toContain('Standard Credits')
    expect(csvContent).not.toContain('Upstream Endpoint')
    expect(csvContent).not.toContain('account_cost')
    expect(csvContent).not.toContain('account_rate_multiplier')

    window.URL.createObjectURL = originalCreateObjectURL
    window.URL.revokeObjectURL = originalRevokeObjectURL
    vi.unstubAllGlobals()
    clickSpy.mockRestore()
  })

  it('exports historical image rows with image billing mode derived from image_count', async () => {
    query.mockResolvedValue({
      items: [
        {
          ...usageLog,
          request_id: 'req-user-export-legacy-image',
          actual_cost: 0.2,
          total_cost: 0.2,
          input_cost: 0,
          output_cost: 0,
          cache_creation_cost: 0,
          cache_read_cost: 0,
          input_tokens: 0,
          output_tokens: 0,
          cache_creation_tokens: 0,
          cache_read_tokens: 0,
          image_count: 1,
          model: 'gpt-image-2',
          billing_mode: null,
          ip_address: null,
        },
      ],
      total: 1,
      pages: 1,
    })

    const wrapper = mountUsageView()
    await flushPromises()

    let csvContent = ''
    const OriginalBlob = globalThis.Blob
    vi.stubGlobal('Blob', vi.fn((parts: BlobPart[], options?: BlobPropertyBag) => {
      csvContent = parts.map((part) => String(part)).join('')
      return new OriginalBlob(parts, options)
    }))
    const originalCreateObjectURL = window.URL.createObjectURL
    const originalRevokeObjectURL = window.URL.revokeObjectURL
    window.URL.createObjectURL = vi.fn(() => 'blob:usage-export') as typeof window.URL.createObjectURL
    window.URL.revokeObjectURL = vi.fn(() => {}) as typeof window.URL.revokeObjectURL
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    await (wrapper.vm as any).exportToCSV()

    expect(csvContent).toContain('Billing Mode')
    expect(csvContent).toContain('Image')
    expect(csvContent).not.toContain(',Token,0,0,0,0,')

    window.URL.createObjectURL = originalCreateObjectURL
    window.URL.revokeObjectURL = originalRevokeObjectURL
    vi.unstubAllGlobals()
    clickSpy.mockRestore()
  })
})
