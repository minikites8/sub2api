"""Shared budgets for flattened client catalogs and their validation worker."""

# Codex catalogs include tools from multiple namespaces and connectors.
MAX_TOOLS = 512
MAX_HISTORY_CALLS = 64
MAX_VALIDATION_COMMANDS = MAX_TOOLS + MAX_HISTORY_CALLS
MAX_TOOL_PAYLOAD_BYTES = 1 << 20
