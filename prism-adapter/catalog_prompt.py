"""Lossless shared catalog fields and bounded, inspectable tool indexes."""
import json
from collections import Counter


MAX_PRISM_PROMPT_BYTES = 112 * 1024
MAX_CATALOG_INSPECTIONS = 4


def encoded(value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'))


def shared_catalog(catalog):
    values = [(key, encoded(tool[key])) for tool in catalog
              for key in ('description', 'parameters', 'format') if key in tool]
    counts = Counter(values)
    repeated = {value for value, count in counts.items() if count > 1 and len(value[1]) >= 64}
    shared, lookup, entries = {}, {}, []
    for tool in catalog:
        entry = dict(tool)
        for key in ('description', 'parameters', 'format'):
            if key not in tool:
                continue
            identity = (key, encoded(tool[key]))
            if identity in repeated:
                reference = lookup.setdefault(identity, 's' + str(len(lookup)))
                shared[reference] = tool[key]
                entry.pop(key)
                entry['shared_' + key] = reference
        entries.append(entry)
    return encoded({'shared': shared, 'tools': entries})


def tool_index(catalog, preview_chars):
    return [{'name': tool['name'], 'type': tool['type'],
             **({'description_preview': tool['description'][:preview_chars]}
                if tool.get('description') and preview_chars else {})}
            for tool in catalog]
