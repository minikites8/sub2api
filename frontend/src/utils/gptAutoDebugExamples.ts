import type { GPTAutoEndpoint } from '@/api/admin/gptAuto'

// Synthetic requests for the GPT Auto state debugger. No customer data.

export type GPTAutoDebugExample = 'agentFailure' | 'followUp' | 'longChat' | 'previousResponse' | 'multimodal'

export const gptAutoDebugExamples: GPTAutoDebugExample[] = ['agentFailure', 'followUp', 'longChat', 'previousResponse', 'multimodal']

export function gptAutoDebugExample(name: GPTAutoDebugExample): { endpoint: GPTAutoEndpoint; request: Record<string, unknown> } {
  switch (name) {
    case 'followUp':
      return {
        endpoint: 'chat_completions',
        request: {
          model: 'gpt-5.6-sol',
          messages: [
            { role: 'system', content: 'You are a coding assistant with a run_tests tool.' },
            { role: 'user', content: 'Implement an LRU cache in Go with per-entry TTL and thread safety' },
            { role: 'assistant', content: '', tool_calls: [{ id: 't1', type: 'function', function: { name: 'run_tests', arguments: '{"pkg":"./cache"}' } }] },
            { role: 'tool', tool_call_id: 't1', content: '--- FAIL: TestExpiry\n    cache_test.go:31: entry not evicted after TTL\nFAIL' },
            { role: 'assistant', content: 'The eviction timer is never reset, so expiry tests still fail.' },
            { role: 'user', content: 'fix it' },
          ],
        },
      }
    case 'longChat': {
      const messages: { role: string; content: string }[] = [{ role: 'user', content: 'Help me plan a data migration from MySQL to PostgreSQL' }]
      for (let i = 0; i < 600; i++) {
        messages.push({ role: 'assistant', content: `Step ${i}: reviewed table group ${i}.` }, { role: 'user', content: `What about table group ${i + 1}?` })
      }
      messages.push({ role: 'user', content: 'Now write the rollback plan for the whole migration' })
      return { endpoint: 'chat_completions', request: { model: 'gpt-5.6-sol', messages } }
    }
    case 'previousResponse':
      return {
        endpoint: 'responses',
        request: {
          model: 'gpt-5.6-sol',
          previous_response_id: 'resp_example',
          input: [{ type: 'function_call_output', call_id: 'c1', output: 'Exit code: 1\nTraceback (most recent call last):\nValueError: invalid header' }],
        },
      }
    case 'multimodal':
      return {
        endpoint: 'chat_completions',
        request: {
          model: 'gpt-6-luna',
          messages: [{ role: 'user', content: [{ type: 'text', text: 'What is wrong with this chart?' }, { type: 'image_url', image_url: { url: 'https://example.invalid/chart.png' } }] }],
        },
      }
    default:
      return {
        endpoint: 'responses',
        request: {
          model: 'gpt-5.6-sol',
          instructions: 'You are a coding agent running in a sandbox with shell and apply_patch tools.',
          input: [
            { role: 'user', content: '<environment_context><cwd>/repo</cwd></environment_context>' },
            { role: 'user', content: 'Make the CSV header parser accept quoted commas' },
            { type: 'function_call', name: 'shell', call_id: 'c1', arguments: '{"command":["go","test","./..."]}' },
            { type: 'function_call_output', call_id: 'c1', output: 'Exit code: 1\n--- FAIL: TestParseHeader (0.00s)\n    parser_test.go:42: expected 3 fields, got 4\nFAIL' },
            { role: 'assistant', content: [{ type: 'output_text', text: 'The splitter ignores quotes; switching to encoding/csv.' }] },
            { type: 'function_call', name: 'apply_patch', call_id: 'c2', arguments: '{"patch":"*** Begin Patch"}' },
            { type: 'function_call_output', call_id: 'c2', output: 'Exit code: 0\nSuccess. Updated parser.go' },
          ],
        },
      }
  }
}
