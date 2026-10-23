# CC Switch usage queries

`POST /api/usage` (also `GET`) accepts the site's API key in `Authorization: Bearer …`. No JSON request body or dashboard JWT is required. Responses are JSON with `Cache-Control: no-store`.

The query uses the key's bound group order and returns the first active, authorized group with remaining quota. It skips exhausted, missing or expired subscriptions, banned groups and wallet groups with no balance. Subscription quota is the smallest remaining configured daily/weekly/monthly allowance; expired usage windows are normalized in memory. Amounts are divided by the effective user/group multiplier, matching the API Key group's available-quota display. The query does not sum groups or replace this amount with the key's independent spending cap. A key that is expired or has exhausted its own cap reports invalidity.

For example, if the first subscription is exhausted and the second has a $10 daily limit, $4 used and a 2× multiplier:

```json
{"isValid":true,"balance":3,"remaining":3,"unit":"USD","planName":"Second subscription","group_id":2,"total":5,"used":2,"quota_window":"daily"}
```

`total` and `used` describe the limiting subscription window. Wallet groups return their available balance without a fabricated lifetime total. Unlimited groups use `unlimited: true` and `balance: -1`; the built-in import script displays “Unlimited” without a negative currency balance.

Invalid quota responses include `error`, `invalidMessage`, `isValid: false` and `balance: 0`. Invalid or disabled credentials still return HTTP 401, IP restrictions return 403, and repository outages return 503 with a top-level `error`. The query is read-only and does not invoke inference or update the key's last-used timestamp. Availability refers to group permissions and billing quota; this model-independent query does not probe upstream account capacity.

The following original CC Switch configuration works unchanged when `baseUrl` is the site root:

```javascript
({
  request: {
    url: "{{baseUrl}}/api/usage",
    method: "POST",
    headers: {
      "Authorization": "Bearer {{apiKey}}",
      "User-Agent": "cc-switch/1.0"
    }
  },
  extractor: function(response) {
    return {
      isValid: !response.error,
      remaining: response.balance,
      unit: "USD"
    };
  }
})
```

The API Key page's CC Switch import automatically includes an extended script that also shows the selected plan and limiting window totals, handles unlimited plans and strips provider suffixes (`/v1`, `/v1beta`, `/antigravity`) from imported base URLs. The existing `/v1/usage` endpoint retains its original behavior.

## Regression validation

Validated on macOS ARM64 on 2026-10-23 using an isolated copy of the committed baseline plus this change, preserving unrelated in-progress GPT Auto work:

- `GOMAXPROCS=2 go test -p 2 -tags=unit ./...`: 73 packages passed.
- Scoped `go test -race -tags=unit` for service, authentication middleware and routes: passed.
- Go lint: 0 issues.
- Frontend ESLint, Vue type checking, production build and the 85-file critical suite: passed (1,458 tests).

The HTTP regression tests exercise the registered Gin routes and actual API-key authentication with synthetic repository fixtures. They cover an empty POST, GET compatibility, fallback between subscriptions on different platforms, restored priority, exhausted or expired keys, IP restrictions, invalid credentials and repository failure. They also check the original extractor's required top-level `error` contract. These tests do not claim a production deployment or a live-database end-to-end run of the new endpoint.
