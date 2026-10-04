# syntax=docker/dockerfile:1.7
FROM python:3.12-slim-bookworm

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PLAYWRIGHT_BROWSERS_PATH=/opt/playwright \
    PRISM_ADAPTER_STATE_DIR=/var/lib/sub2api-prism \
    PRISM_ADAPTER_API_KEY_FILE=/gateway-data/prism-adapter/bridge.key \
    PRISM_ADAPTER_MODE=multiplex \
    PRISM_ADAPTER_MAX_INFLIGHT=4 \
    PRISM_ADAPTER_ACCOUNT_MAX_INFLIGHT=4 \
    PRISM_ADAPTER_MAX_QUEUED=8 \
    HOME=/var/lib/sub2api-prism

WORKDIR /opt/prism-adapter
COPY prism-adapter/requirements.txt ./
RUN python -m pip install --no-cache-dir --only-binary=:all: -r requirements.txt \
    && python -m playwright install --with-deps --no-shell chromium \
    && apt-get update \
    && apt-get install -y --no-install-recommends gosu tini \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 1000 sub2api \
    && useradd --uid 1000 --gid 1000 --home-dir /var/lib/sub2api-prism --no-create-home sub2api

COPY prism-adapter/*.py ./
COPY prism-adapter/docker-entrypoint.sh ./
RUN python docker_runtime.py --prepare-image \
    && chmod 755 docker-entrypoint.sh \
    && install -d -m 700 -o sub2api -g sub2api /var/lib/sub2api-prism

# The adapter binds only to loopback in the gateway's network namespace.
HEALTHCHECK --interval=10s --timeout=5s --start-period=90s --retries=3 \
    CMD python -c "import os, urllib.request; urllib.request.urlopen('http://127.0.0.1:' + os.environ.get('PRISM_ADAPTER_PORT', '8319') + '/health', timeout=3).close()"

ENTRYPOINT ["/usr/bin/tini", "--", "/opt/prism-adapter/docker-entrypoint.sh"]
CMD ["python", "docker_runtime.py"]
