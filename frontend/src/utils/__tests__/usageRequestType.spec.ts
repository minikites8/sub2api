import { describe, expect, it } from 'vitest'

import { requestTypeToLegacyStream, resolveUsageRequestType } from '../usageRequestType'

describe('usageRequestType', () => {
  it('recognizes async usage records', () => {
    expect(resolveUsageRequestType({ request_type: 'async', stream: true })).toBe('async')
  })

  it('keeps async filters independent from the legacy stream flag', () => {
    expect(requestTypeToLegacyStream('async')).toBeNull()
  })
})