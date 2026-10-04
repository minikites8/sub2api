"""Bound model-visible history while retaining validated source identities."""
import copy
import json


MIN_RESULT_PREVIEW_BYTES = 1024
MIN_OLD_RESULT_PREVIEW_BYTES = 144


def output_text(output):
    return output if isinstance(output, str) else '\n'.join(part['text'] for part in output)


def preview(text, budget, label='output'):
    raw = text.encode('utf-8')
    if len(raw) <= budget:
        return text
    marker = (f'\n[PRISM_CONTEXT_OMISSION: original {label} {len(raw)} UTF-8 bytes; '
              'middle omitted; use client tools for details.]\n')
    available = max(0, budget - len(marker.encode('utf-8')))
    head = available * 2 // 3
    tail = available - head
    return (raw[:head].decode('utf-8', errors='ignore') + marker
            + (raw[-tail:].decode('utf-8', errors='ignore') if tail else ''))


def compact_results(payload, positions, parse_prompt, max_bytes, *, call_positions=(), assistant_positions=()):
    """Shrink the oldest tool output first; validated source objects stay intact.

    Instructions and user messages retain their full contents. Only positions
    identified by the validated translator can be interpreted as tool records.
    """
    body = copy.deepcopy(payload)

    def render():
        # All original messages were already validated by the caller. Keep the
        # envelope limit separate from this model-visible history target.
        return parse_prompt(body, max_bytes=1 << 20)[0]

    prompt = render()
    before = len(prompt.encode('utf-8'))
    compacted = set()
    history_compacted = set()

    def shrink(position, text, minimum, replace, label='output'):
        nonlocal prompt
        size = len(text.encode('utf-8'))
        if size <= minimum or len(prompt.encode('utf-8')) <= max_bytes:
            return False
        item = body['input'][position]
        original_item = copy.deepcopy(item)
        original_prompt = prompt
        target = max(minimum, size - (len(prompt.encode('utf-8')) - max_bytes) - 256)
        while True:
            replace(item, preview(text, target, label), size)
            prompt = render()
            if len(prompt.encode('utf-8')) <= max_bytes or target == minimum:
                break
            target = max(minimum, target // 2)
        if len(prompt.encode('utf-8')) < len(original_prompt.encode('utf-8')):
            return True
        body['input'][position] = original_item
        prompt = original_prompt
        return False

    def result_preview(position, minimum):
        item = payload['input'][position]
        result = json.loads(item['content'][len('CLIENT_TOOL_RESULT '):])
        def replace(item, text, size):
            visible = (dict(result, output=text, output_truncated=True, original_output_bytes=size)
                       if minimum == MIN_RESULT_PREVIEW_BYTES else
                       {'call_id': result['call_id'], 'output': text})
            item['content'] = 'CLIENT_TOOL_RESULT ' + json.dumps(visible, ensure_ascii=False)
        if shrink(position, output_text(result['output']), minimum, replace):
            compacted.add(position)

    for position in positions:
        if len(prompt.encode('utf-8')) <= max_bytes:
            break
        result_preview(position, MIN_RESULT_PREVIEW_BYTES)

    # Long assistant answers, reasoning summaries and completed call inputs can
    # dominate a later user turn even after every result has been clipped.
    for position in sorted(set(assistant_positions) | set(call_positions)):
        if len(prompt.encode('utf-8')) <= max_bytes:
            break
        content = payload['input'][position]['content']
        if position in call_positions:
            call = json.loads(content[len('CLIENT_TOOL_CALL '):])
            field = 'arguments' if call['type'] == 'function_call' else 'input'
            def replace(item, text, size):
                visible = dict(call, **{field: text, 'input_truncated': True, 'original_input_bytes': size})
                item['content'] = 'CLIENT_TOOL_CALL ' + json.dumps(visible, ensure_ascii=False)
            text, label = call[field], 'completed call input'
        else:
            text, label = output_text(content), 'assistant history'
            def replace(item, text, _size):
                item['content'] = text
        if shrink(position, text, MIN_RESULT_PREVIEW_BYTES, replace, label):
            history_compacted.add(position)

    # Many short results also accumulate. Keep the newest result's larger
    # preview and reduce older records only when the first stages still overflow.
    for position in positions[:-1]:
        if len(prompt.encode('utf-8')) <= max_bytes:
            break
        result_preview(position, MIN_OLD_RESULT_PREVIEW_BYTES)
    return prompt, len(compacted), before, len(history_compacted)
