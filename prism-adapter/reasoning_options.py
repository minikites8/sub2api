"""Resolve client reasoning preferences to the account-visible Prism tiers."""
from model_selection import EFFORTS


ALIASES = {
    'max': 'xhigh',
    'ultra': 'xhigh',
    'x-high': 'xhigh',
    'extra-high': 'xhigh',
    'extra_high': 'xhigh',
    'minimal': 'low',
    'none': 'low',
}
SUMMARIES = (None, 'none', 'auto', 'concise', 'detailed')


def resolve_reasoning(payload, error):
    reasoning = payload.get('reasoning')
    if reasoning is None:
        reasoning = {}
    if not isinstance(reasoning, dict):
        raise error(422, 'unsupported_reasoning', 'reasoning must be an object')
    requested = reasoning.get('effort')
    if requested is None:
        effort = 'medium'
    elif isinstance(requested, str):
        token = requested.strip().lower() or 'medium'
        effort = ALIASES.get(token, token)
    else:
        effort = None
    if effort not in EFFORTS:
        raise error(422, 'unsupported_reasoning',
                    'Unsupported Prism reasoning effort; choose low, medium, high, xhigh, max, ultra, minimal or none')
    if reasoning.get('summary') not in SUMMARIES:
        raise error(422, 'unsupported_reasoning_summary',
                    'Unsupported reasoning.summary; choose auto, concise, detailed or none')
    return requested, effort
