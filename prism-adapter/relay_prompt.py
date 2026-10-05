"""Validated client history and lossless, bounded Prism transport.

Inspired by prism-bridge a12aaed's conversation continuation and split turns.
The adapter keeps its own client tool protocol, validation and usage contract.
"""
import hashlib
import json
import os
import time


TURN_BYTES = 86000
MAX_PARTS = 8
PART_GAP_SECONDS = 8
CONVERSATION_TTL_SECONDS = 900


def transport_bytes(text):
    return len(json.dumps(text, ensure_ascii=False).encode('utf-8')) - 2


def split_prompt(text, error, *, budget=TURN_BYTES, max_parts=MAX_PARTS):
    """Preserve every character, including JSON escapes and Unicode boundaries."""
    if transport_bytes(text) <= budget:
        return [text]
    pieces, remaining = [], text
    while remaining:
        if transport_bytes(remaining) <= budget - 1024:
            pieces.append(remaining)
            break
        low, high = 0, len(remaining)
        while low < high:
            middle = (low + high + 1) // 2
            if transport_bytes(remaining[:middle]) <= budget - 1024:
                low = middle
            else:
                high = middle - 1
        if low == 0:
            raise error(413, 'request_too_large', 'Prism transport budget cannot fit a text character', not_submitted=True)
        # Prefer a nearby line boundary while keeping at least half the capacity.
        boundary = remaining.rfind('\n', low // 2, low)
        end = boundary + 1 if boundary >= 0 else low
        pieces.append(remaining[:end])
        remaining = remaining[end:]
        if len(pieces) > max_parts:
            raise error(413, 'request_too_large',
                        f'Prism request exceeds the {max_parts}-part transport budget', not_submitted=True)
    if len(pieces) > max_parts:
        raise error(413, 'request_too_large', f'Prism request exceeds the {max_parts}-part transport budget', not_submitted=True)
    return pieces


def part_prompt(piece, index, count):
    if count == 1:
        return piece
    if index == count:
        directive = ('All request parts have arrived. Read the parts in order as one continuous '
                     'request and produce its requested response now.')
    else:
        directive = ('Buffer this request part for the next part. Reply with exactly ACK. '
                     'Defer all client and sandbox operations until the final part arrives.')
    return f'[PRISM_TRANSPORT_PART {index}/{count}]\n{directive}\n<request_part>\n{piece}\n</request_part>\n{directive}'


def history_keys(items):
    if isinstance(items, str):
        items = [{'role': 'user', 'content': items}]
    keys = []
    for item in items:
        kind = item.get('type', 'message')
        if kind == 'additional_tools':
            continue
        if kind == 'reasoning':
            summary = item.get('summary', [])
            if not summary:
                continue
            value = [kind, summary]
        elif kind == 'message':
            content = item['content']
            text = content if isinstance(content, str) else '\n'.join(part['text'] for part in content)
            value = [kind, item.get('role', 'user'), text]
        elif kind in ('function_call', 'custom_tool_call'):
            argument = json.loads(item['arguments']) if kind == 'function_call' else item['input']
            value = [kind, item['call_id'], item.get('namespace', ''), item['name'], argument]
        else:
            value = [kind, item['call_id'], item['output']]
        raw = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode('utf-8')
        keys.append(hashlib.sha256(raw).digest())
    return tuple(keys)


class RelayPrompt(str):
    """A request-local plan; browser objects remain on the asyncio owner thread."""
    def __new__(cls, prompt, payload, api, bridge=None):
        instance = super().__new__(cls, prompt)
        instance.keys = history_keys(payload['input'])
        options = {key: payload.get(key) for key in
                   ('model', 'instructions', 'tools', 'additional_tools', 'tool_choice', 'parallel_tool_calls')}
        # In-history additional declarations also affect the complete catalog.
        if bridge is not None:
            options['catalog'] = bridge.catalog
        instance.signature = hashlib.sha256(json.dumps(options, sort_keys=True, ensure_ascii=False,
                                                       separators=(',', ':')).encode('utf-8')).digest()
        instance.payload, instance.bridge, instance.api = payload, bridge, api
        instance.record = None
        instance.context = None
        instance.mode = 'full'
        instance.parts = 0
        instance.sent_bytes = 0
        return instance

    def delta(self, start, calls=()):
        if self.bridge is None:
            items = self.payload['input']
            if isinstance(items, str):
                items = [{'role': 'user', 'content': items}]
            body = dict(self.payload, instructions='', input=items[start:])
            return self.api.parse_prompt(body, max_bytes=1 << 20)[0]
        bridge = self.bridge
        body = dict(bridge.history_payload, instructions='', input=bridge.history_payload['input'][start:])
        history = self.api.parse_prompt(body, max_bytes=2 << 20)[0]
        if calls:
            history = ('Client call identities assigned to your previous response:\n'
                       + json.dumps(calls, ensure_ascii=False) + '\n\n' + history)
        return bridge.build_prompt(bridge.prompt_catalog, history=history)

    def commit(self, output):
        """Expose a head for reuse only after terminal client output is validated."""
        if self.record is not None:
            self.record['keys'] = self.keys + history_keys(output)
            self.record['calls'] = [{key: item[key] for key in ('type', 'call_id', 'name', 'namespace') if key in item}
                                    for item in output if item.get('type') in ('function_call', 'custom_tool_call')]
            self.record['committed'] = True


def continuation_context(start, data, snapshot):
    """Use server-confirmed conversation, response and Codex listen identities."""
    payload = (data.get('response') or {}).get('payload') or {}
    if not isinstance(payload, dict):
        return None
    template = getattr(start, 'template', None)
    if template is None and getattr(start, 'start_request', None) is not None:
        template = start.start_request.post_data_json
    if not isinstance(template, dict) or not isinstance(template.get('metadata'), dict):
        return None
    cid, rid = payload.get('conversationId'), payload.get('id')
    if not cid:
        cid = template.get('conversationId')
    if (not isinstance(cid, str) or not cid or not isinstance(rid, str) or not rid
            or len(cid) > 1024 or len(rid) > 1024
            or not isinstance(snapshot, dict) or snapshot.get('conversation_id') != cid
            or snapshot.get('project_id') != start.project
            or not isinstance(snapshot.get('codex_session_id'), str) or not snapshot['codex_session_id']):
        return None
    metadata = dict(template['metadata'])
    for key in ('sandbox_token', 'sandbox_url'):
        if isinstance(snapshot.get(key), str) and snapshot[key]:
            metadata[key] = snapshot[key]
    return {'metadata': metadata, 'conversation_id': cid,
            'response_id': rid, 'snapshot': dict(snapshot)}


def relay_settings():
    budget = int(os.environ.get('PRISM_ADAPTER_TURN_BYTES', TURN_BYTES))
    count = int(os.environ.get('PRISM_ADAPTER_MAX_TURN_PARTS', MAX_PARTS))
    gap = float(os.environ.get('PRISM_ADAPTER_PART_GAP_SECONDS', PART_GAP_SECONDS))
    if not 4096 <= budget <= TURN_BYTES or not 1 <= count <= 16 or not 0 <= gap <= 60:
        raise ValueError('invalid Prism relay transport settings')
    return budget, count, gap


def record_matches(record, prompt, model, effort):
    return bool(record and record.get('committed') and record['signature'] == prompt.signature
                and record['model'] == model and record['effort'] == effort
                and time.monotonic() - record['at'] < CONVERSATION_TTL_SECONDS
                and len(prompt.keys) > len(record['keys'])
                and prompt.keys[:len(record['keys'])] == record['keys'])
