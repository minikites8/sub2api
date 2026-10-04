"""Bound model-visible tool output while retaining complete result identities."""
import copy
import json


MIN_RESULT_PREVIEW_BYTES = 1024


def output_text(output):
    return output if isinstance(output, str) else '\n'.join(part['text'] for part in output)


def preview(text, budget):
    raw = text.encode('utf-8')
    if len(raw) <= budget:
        return text
    marker = (f'\n[PRISM_CONTEXT_OMISSION: original output {len(raw)} UTF-8 bytes; '
              'middle omitted. Request targeted output from the client tool for further details.]\n')
    available = max(0, budget - len(marker.encode('utf-8')))
    head = available * 2 // 3
    tail = available - head
    return (raw[:head].decode('utf-8', errors='ignore') + marker
            + (raw[-tail:].decode('utf-8', errors='ignore') if tail else ''))


def compact_results(payload, positions, parse_prompt, max_bytes):
    """Shrink the oldest tool output first; validated source objects stay intact.

    Instructions, messages, call arguments, identities and recent small results
    retain their full contents. The returned count describes visible omissions.
    """
    body = copy.deepcopy(payload)

    def render():
        # All original messages were already validated by the caller. Keep the
        # envelope limit separate from this model-visible history target.
        return parse_prompt(body, max_bytes=1 << 20)[0]

    prompt = render()
    before = len(prompt.encode('utf-8'))
    count = 0
    for position in positions:
        if len(prompt.encode('utf-8')) <= max_bytes:
            break
        item = body['input'][position]
        result = json.loads(item['content'][len('CLIENT_TOOL_RESULT '):])
        text = output_text(result['output'])
        size = len(text.encode('utf-8'))
        if size <= MIN_RESULT_PREVIEW_BYTES:
            continue
        # Use only as much compaction as needed, including JSON escape costs.
        target = max(MIN_RESULT_PREVIEW_BYTES,
                     size - (len(prompt.encode('utf-8')) - max_bytes) - 256)
        original = prompt
        while True:
            visible = dict(result, output=preview(text, target),
                           output_truncated=True, original_output_bytes=size)
            item['content'] = 'CLIENT_TOOL_RESULT ' + json.dumps(visible, ensure_ascii=False)
            prompt = render()
            if len(prompt.encode('utf-8')) <= max_bytes or target == MIN_RESULT_PREVIEW_BYTES:
                break
            target = max(MIN_RESULT_PREVIEW_BYTES, target // 2)
        if len(prompt.encode('utf-8')) < len(original.encode('utf-8')):
            count += 1
        else:
            item['content'] = payload['input'][position]['content']
            prompt = original
    return prompt, count, before
