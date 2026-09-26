# Basispoints native image inputs

Basispoints-enabled accounts automatically materialize inline Responses images through the native attachment endpoint. Deploy the updated backend and send ordinary `input_image` blocks containing `data:image/png;base64,...`. Existing HTTPS references and `file_id` references remain valid.

## Request flow

1. Validate the image references and decode inline images in the expanded Responses input, including message history and structured tool outputs. Preserve the original request bytes.
2. Upload each distinct image using `POST https://bps.openai.com/basispoints/api/attachments`, multipart fields `purpose=vision` and `file`. Reuse the selected account bearer, ChatGPT account ID, bound proxy and HTTP transport. Apply a 120-second upload timeout and disable redirects.
3. Read `openai_file_id`, `file_id` or `id`, replace the inline `image_url` with `file_id`, and retain the image detail level.
4. Forward the materialized request with `Copilot-Vision-Request: true`. Text-only requests retain their existing header behavior.

## Cache

- The key uses the local account ID, ChatGPT account ID and SHA-256 of decoded image bytes. Tokens and image bytes stay outside cached values.
- The process-local cache holds up to 512 references. Concurrent requests for the same account and image share one in-flight upload within that process.
- The existing Redis gateway cache persists attachment IDs with absolute 24-hour expiration and a 512-entry LRU bound per account scope. Service restarts reuse valid IDs while Redis retains the records. Redis outages fall back to the local cache.
- Expired entries trigger a fresh upload. Reference expiry controls cache reuse; upstream attachment retention follows Basispoints.

## Validation and failures

Accept PNG, JPEG, GIF and WebP with valid image headers and dimensions. Limits: 20 MiB per decoded image, 64 MiB of distinct decoded images per request, 100 megapixels per image, and 128 levels of input nesting. Validate all image references before beginning uploads.

Invalid images return HTTP 400; oversized batches return HTTP 413; attachment transport, status and response errors return HTTP 502. Error messages retain the upstream status code and redact response bodies, bearer tokens and image content. An upload failure prevents the Responses request from being forwarded.

The conversion activates inside the existing Basispoints route. It uses the native attachment API and the existing Redis infrastructure.

## Verification

Tests use synthetic PNGs, recorded HTTP requests and miniredis. They cover multipart fields, account proxy/authentication, streaming and non-streaming forwarding, history/tool images, stable replay metadata, source immutability, escaped JSON types, preserved HTTPS/file IDs, file-ID response variants, invalid and oversized inputs, error redaction, concurrent deduplication, account isolation, absolute expiry and process-restart cache reuse.

Reference implementation reviewed: Nonary/ghcp_proxy commit `dfb758b181e5caa6c52183ef957232140c384dcb`, `proxy.py` attachment materialization path. A live selected-account upload remains an operational deployment check.
