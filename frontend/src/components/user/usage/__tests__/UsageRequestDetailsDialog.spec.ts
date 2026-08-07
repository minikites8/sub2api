import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import UsageRequestDetailsDialog from '../UsageRequestDetailsDialog.vue'

const labels: Record<string, string> = {
  'usage.requestDetails.title': 'Request details',
  'usage.requestDetails.request': 'Request',
  'usage.requestDetails.requestContext': 'Request context',
  'usage.requestDetails.requestId': 'Request ID',
  'usage.requestDetails.routing': 'Routing',
  'usage.requestDetails.client': 'Client',
  'usage.requestDetails.ipAddress': 'IP Address',
  'usage.requestDetails.nodeId': 'Processing Node ID',
  'usage.requestDetails.imageMetadata': 'Image metadata',
  'usage.requestDetails.mediaType': 'Media Type',
  'usage.requestDetails.asyncTask': 'Async Task',
  'usage.requestDetails.taskKind': 'Task Type',
  'usage.requestDetails.taskKindVideo': 'Video Generation',
  'usage.requestDetails.taskKindGrokVideo': 'Grok Video Generation',
  'usage.requestDetails.taskKindBatchImage': 'Batch Image Generation',
  'usage.requestDetails.taskId': 'Task ID',
  'usage.requestDetails.taskStatus': 'Task Status',
  'usage.requestDetails.statusUrl': 'Status URL',
  'usage.requestDetails.resultUrl': 'Result URL',
  'usage.requestDetails.expiresAt': 'Result Expires At',
  'usage.requestDetails.videoMetadata': 'Video Parameters',
  'usage.requestDetails.videoCount': 'Video Count',
  'usage.requestDetails.videoResolution': 'Resolution',
  'usage.requestDetails.videoDuration': 'Duration (seconds)',
  'usage.apiKeyFilter': 'API Key',
  'usage.inboundEndpoint': 'Inbound Endpoint',
  'usage.userAgent': 'User-Agent',
  'usage.reasoningEffort': 'Reasoning Effort',
  'usage.serviceTier': 'Service Tier',
  'usage.serviceTierStandard': 'Standard',
  'usage.type': 'Type',
  'usage.stream': 'Streaming',
  'usage.async': 'Async',
  'usage.imageCount': 'Image Count',
  'usage.imageBillingSize': 'Billing Size',
  'usage.imageInputSize': 'Input Size',
  'usage.imageOutputSize': 'Output Size',
  'usage.imageSizeSource': 'Size Source',
  'usage.imageSizeSourceOutput': 'Output',
  'admin.usage.group': 'Group',
  'usage.analytics.unknownGroup': 'Unknown group',
}

// createI18n 也得桩上：组件间接引到 @/utils/format，而它在模块加载时就建了 i18n 实例。
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => labels[key] ?? key }),
  createI18n: () => ({ global: { t: (key: string) => labels[key] ?? key } }),
}))

// BaseDialog 会 teleport 到 body，桩掉它可以直接在 wrapper 里断言内容。
const BaseDialogStub = {
  props: ['show', 'title', 'width'],
  template: '<div v-if="show"><slot /></div>',
}

const baseLog = {
  id: 1,
  created_at: '2026-07-24T08:30:00Z',
  model: 'claude-sonnet-4',
  stream: true,
  rate_multiplier: 1,
  image_count: 0,
  video_count: 0,
  video_resolution: null,
  video_duration_seconds: null,
  request_id: '',
  node_id: null,
  ip_address: null,
  user_agent: null,
  inbound_endpoint: null,
  reasoning_effort: null,
  service_tier: null,
  media_type: null,
  image_size: null,
  image_input_size: null,
  image_output_size: null,
  image_size_source: null,
}

const mountDialog = (log: Record<string, unknown>) => mount(UsageRequestDetailsDialog, {
  props: { log: log as any },
  global: { stubs: { BaseDialog: BaseDialogStub, Icon: true } },
})

describe('UsageRequestDetailsDialog', () => {
  // 之前每个字段无论有没有值都占一格，一个普通请求要显示十几个 "--"。
  it('omits fields the log has no value for', () => {
    const wrapper = mountDialog(baseLog)
    const text = wrapper.text()

    expect(text).not.toContain('Request ID')
    expect(text).not.toContain('Processing Node ID')
    expect(text).not.toContain('IP Address')
    expect(text).not.toContain('User-Agent')
    expect(text).not.toContain('Reasoning Effort')
    expect(text).not.toContain('Inbound Endpoint')
    // 整段都没有取值时连标题也不出现。
    expect(text).not.toContain('Client')
    expect(wrapper.findAll('dl > div')).toHaveLength(3)
  })

  it('keeps the fields that do have values', () => {
    const wrapper = mountDialog({
      ...baseLog,
      request_id: 'req-abc',
      node_id: 'edge-cn-01',
      ip_address: '203.0.113.10',
      user_agent: 'openai-node/4.71.1',
      inbound_endpoint: '/v1/responses',
      api_key: { name: 'default key' },
    })
    const text = wrapper.text()

    expect(text).toContain('Request IDreq-abc')
    expect(text).toContain('Inbound Endpoint/v1/responses')
    expect(text).toContain('Processing Node IDedge-cn-01')
    expect(text).toContain('IP Address203.0.113.10')
    expect(text).toContain('User-Agentopenai-node/4.71.1')
    expect(text).toContain('Client')
  })

  // UsageLog 里根本没有状态字段，弹窗头部原先写死了一个 "200 OK"，
  // 任何请求都照样显示——那是编出来的，不是这条记录的数据。
  it('does not claim a status the log never carried', () => {
    expect(mountDialog(baseLog).text()).not.toContain('200 OK')
  })

  // "传输"只会说流式/同步/WebSocket，和"类型"讲的是同一件事。
  it('does not repeat the request type as a separate transport row', () => {
    const text = mountDialog(baseLog).text()
    expect(text).toContain('TypeStreaming')
    expect(text).not.toContain('Transport')
  })

  it('shows image metadata only for image requests', () => {
    expect(mountDialog(baseLog).text()).not.toContain('Image metadata')

    const withImages = mountDialog({
      ...baseLog,
      image_count: 2,
      media_type: 'image/png',
      image_size: '1024x1024',
      image_size_source: 'output',
    })
    expect(withImages.text()).toContain('Image metadata')
    expect(withImages.text()).toContain('Image Count2')
    expect(withImages.text()).toContain('Size SourceOutput')
    // 图片段里没取到的尺寸同样不占位。
    expect(withImages.text()).not.toContain('Input Size')
  })

  it('shows async task result links and video metadata', () => {
    const wrapper = mountDialog({
      ...baseLog,
      request_type: 'async',
      video_count: 1,
      video_resolution: '1080p',
      video_duration_seconds: 8,
      async_task: {
        kind: 'video',
        task_id: 'video-task-1',
        status: 'completed',
        status_url: '/v1/videos/video-task-1',
        result_urls: ['https://cdn.example.com/video.mp4', 'javascript:alert(1)'],
      },
    })

    const text = wrapper.text()
    expect(text).toContain('TypeAsync')
    expect(text).toContain('Video Parameters')
    expect(text).toContain('Resolution1080p')
    expect(text).toContain('Task IDvideo-task-1')
    expect(wrapper.find('a[href="/v1/videos/video-task-1"]').exists()).toBe(true)
    expect(wrapper.find('a[href="https://cdn.example.com/video.mp4"]').exists()).toBe(true)
    expect(wrapper.find('a[href^="javascript:"]').exists()).toBe(false)
  })

  it('renders nothing when no log is selected', () => {
    const wrapper = mount(UsageRequestDetailsDialog, {
      props: { log: null },
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: true } },
    })
    expect(wrapper.find('[data-testid="request-details-dialog"]').exists()).toBe(false)
  })
})
