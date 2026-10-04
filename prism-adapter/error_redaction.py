"""Bounded error text for administrator-requested upstream failure logging."""
import json
import re


SENSITIVE = r'(?:authorization|proxy-authorization|cookie|set-cookie|api[-_]?key|access[-_]?token|refresh[-_]?token|oauth[-_]?token|sandbox[-_]?token|session[-_]?token|token|password|secret|auth)'
JSON_SECRET = re.compile(r'("' + SENSITIVE + r'"\s*:\s*)("(?:\\.|[^"\\])*"|[^,}\s]+)', re.I)
HEADER_SECRET = re.compile(r'(\b(?:authorization|proxy-authorization|cookie|set-cookie)\s*:\s*)[^\r\n]+', re.I)
ASSIGNMENT_SECRET = re.compile(r'(\b' + SENSITIVE + r'\s*[=:]\s*)([^\s,;&"\']+)', re.I)
QUERY_SECRET = re.compile(r'([?&]' + SENSITIVE + r'=)[^&\s"\']+', re.I)
BEARER = re.compile(r'\b(?:Bearer|Basic)\s+[A-Za-z0-9._~+/=-]+', re.I)
JWT = re.compile(r'\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b')
API_KEY = re.compile(r'\bsk-[A-Za-z0-9_-]{8,}\b')
ERROR_FIELDS = ('message', 'name', 'code', 'type', 'cause', 'error', 'stderr')


def error_value(value, depth=0):
    if isinstance(value, str):
        return value
    if isinstance(value, dict) and depth < 3:
        return {key: error_value(value[key], depth + 1) for key in ERROR_FIELDS if key in value}
    return None


def redact_error(value, secrets=()):
    value = error_value(value)
    if value is None:
        return None
    text = value if isinstance(value, str) else json.dumps(value, ensure_ascii=False)
    for secret in secrets:
        if isinstance(secret, str) and secret:
            text = text.replace(secret, '[REDACTED]')
    text = JSON_SECRET.sub(lambda match: match[1] + '"[REDACTED]"', text)
    text = HEADER_SECRET.sub(lambda match: match[1] + '[REDACTED]', text)
    text = QUERY_SECRET.sub(lambda match: match[1] + '[REDACTED]', text)
    text = ASSIGNMENT_SECRET.sub(lambda match: match[1] + '[REDACTED]', text)
    text = BEARER.sub('Bearer [REDACTED]', text)
    text = JWT.sub('[REDACTED]', text)
    text = API_KEY.sub('[REDACTED]', text)
    return text if len(text) <= 4096 else text[:4079] + '\n[truncated]'
