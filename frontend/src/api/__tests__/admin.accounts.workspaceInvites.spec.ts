import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { inviteOpenAIWorkspaceMembers } from '@/api/admin/accounts'

describe('admin OpenAI workspace invitations API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { account_invites: [], errored_emails: [] } })
  })

  it('omits seat_type when the workspace default seat is used', async () => {
    await inviteOpenAIWorkspaceMembers(7, 'workspace-id', ['member@example.com'])

    expect(post).toHaveBeenCalledWith('/admin/openai/accounts/7/workspace-invites', {
      workspace_account_id: 'workspace-id',
      email_addresses: ['member@example.com'],
      role: 'standard-user',
      resend_emails: true
    })
    expect(post.mock.calls[0][1]).not.toHaveProperty('seat_type')
  })

  it('includes a non-empty seat_type selected by an owner', async () => {
    await inviteOpenAIWorkspaceMembers(7, 'workspace-id', ['member@example.com'], 'prolite')

    expect(post.mock.calls[0][1]).toMatchObject({ seat_type: 'prolite' })
  })
})
