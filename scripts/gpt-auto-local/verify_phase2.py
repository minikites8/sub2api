"""GPT Auto Phase 2 end-to-end checks against the isolated local stack.

Reuses the Phase 1 harness (verify.py) for authentication, the synthetic
customer/account/proxy setup and HTTP helpers. Everything goes through the
supported public and admin APIs; SQL is only read for verification.
Results: artifacts/gpt-auto-phase2/phase2-results.json
"""
import concurrent.futures
import csv
import io
import json
import pathlib
import statistics
import sys
import time
import urllib.error
import urllib.request

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import verify as v  # noqa: E402

OUT = v.ROOT / "artifacts/gpt-auto-phase2"
OUT.mkdir(parents=True, exist_ok=True)
v.OUT = OUT
results = []
state = {}

ADMIN_EXPORT_COLUMNS_REQUIRED = [
    "export_schema", "timestamp", "request_id", "mode", "requested_model", "jev_model", "final_model", "served_model",
    "jev_effort", "final_effort", "complexity", "task_category", "model_confidence", "policy_version", "state_version",
    "classifier_schema", "classifier_config_hash", "routing_reason", "fallback_reason", "upstream_channel_id",
    "input_tokens", "cached_input_tokens", "output_tokens", "classifier_tokens", "classifier_cost_usd",
    "customer_charge_usd", "upstream_cost_usd", "gross_profit_usd", "gross_margin", "classifier_latency_ms",
    "upstream_ttft_ms", "total_latency_ms", "request_status", "error_category",
]
CUSTOMER_FORBIDDEN = ["upstream_cost", "gross_profit", "margin", "upstream_account", "channel", "classifier", "jev_",
                      "fallback_reason", "probabilities", "diagnostics", "user_id"]


def check(name, fn):
    start = time.monotonic()
    try:
        entry = {"name": name, "passed": True, "detail": fn()}
    except Exception as e:  # noqa: BLE001 - every failure is recorded verbatim
        import traceback
        entry = {"name": name, "passed": False, "error": str(e)[:2000], "traceback": traceback.format_exc()[-3000:]}
    entry["seconds"] = round(time.monotonic() - start, 3)
    results.append(entry)
    print(json.dumps({k: entry[k] for k in ("name", "passed", "seconds")} | ({"error": entry["error"][:500]} if not entry["passed"] else {}), ensure_ascii=False), flush=True)
    (OUT / "phase2-results.json").write_text(json.dumps(results, ensure_ascii=False, indent=2))
    return entry["passed"]


def raw_get(path, auth, base=v.BASE, timeout=120):
    req = urllib.request.Request(base + path, headers={"Authorization": "Bearer " + auth})
    try:
        response = urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.HTTPError as err:
        response = err
    with response:
        return response.code, dict(response.headers), response.read()


def mock_requests():
    return v.http("/control", base=v.MOCK)["data"]["requests"]


def gateway(endpoint, body, auth=None, stream=False, timeout=30, headers=None):
    raw = json.dumps(body).encode()
    hdrs = {"Content-Type": "application/json", "Authorization": "Bearer " + (auth or v.key)}
    hdrs.update(headers or {})
    req = urllib.request.Request(v.BASE + "/v1/" + endpoint, data=raw, headers=hdrs)
    start = time.perf_counter()
    try:
        response = urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.HTTPError as err:
        response = err
    with response:
        content = response.read().decode(errors="replace")
    data = None
    if not stream:
        try:
            data = json.loads(content)
        except ValueError:
            pass
    return {"status": response.code, "headers": dict(response.headers), "body": content, "data": data,
            "ms": (time.perf_counter() - start) * 1000}


def auto_chat(text="Help me debug this Go concurrency issue.", **kw):
    return {"model": "gpt-auto", "messages": [{"role": "user", "content": text}], **kw}


def auto_responses(text="Help me debug this Go concurrency issue.", **kw):
    return {"model": "gpt-auto", "input": [{"role": "user", "content": text}], **kw}


def live_config(**changes):
    cfg = v.api("admin/gpt-auto/config")
    live = cfg["live"]
    live.update(changes)
    cfg["live"] = live
    cfg["classifier_api_key"] = ""
    saved = v.api("admin/gpt-auto/config", cfg, "PUT")
    for k, val in changes.items():
        assert saved["live"][k] == val or val == [] and saved["live"][k] is None, (k, saved["live"][k], val)
    time.sleep(0.1)
    return saved["live"]


def last_upstream(model_prefix="gpt"):
    for entry in reversed(mock_requests()):
        if entry["path"].endswith("/systemone"):
            continue
        if str(entry.get("model") or "").startswith(model_prefix):
            return entry
    raise AssertionError("no upstream request recorded")


def upstream_effort(entry):
    effort = entry.get("effort")
    return effort.get("effort") if isinstance(effort, dict) else effort


def usage_row(client_request_id):
    row = v.sql("SELECT model, COALESCE(requested_model,''), actual_cost::text, total_cost::text, rate_multiplier::text, "
                "COALESCE(reasoning_effort,''), input_tokens, cache_read_tokens, output_tokens FROM usage_logs "
                f"WHERE request_id = 'client:{client_request_id}'")
    return row.split("|") if row else None


def wait_usage(client_request_id, seconds=15):
    return v.wait(lambda: usage_row(client_request_id), seconds)


def decision_row(client_request_id, seconds=15):
    def find():
        out = v.sql("SELECT mode, final_model, final_effort, jev_model, routing_reason, fallback_reason, request_status, "
                    "COALESCE(http_status::text,''), affinity_outcome, retry_count, error_category FROM gpt_auto_shadow_decisions "
                    f"WHERE request_id = 'client:{client_request_id}' ORDER BY id DESC LIMIT 1")
        return out.split("|") if out else None
    return v.wait(find, seconds)


def header(response, name):
    # Go canonicalizes header names (X-Gpt-Auto-Model); compare case-insensitively.
    for key, value in response["headers"].items():
        if key.lower() == name.lower():
            return value
    return None


def rid(response):
    return header(response, "X-Client-Request-ID")


# ---------------------------------------------------------------------------


def setup():
    v.check("startup_assets_migrations", v.startup)
    v.check("authentication_and_runtime_versions", v.authenticate)
    v.check("supported_customer_setup", v.setup_customer)
    assert all(x["passed"] for x in v.results), [x for x in v.results if not x["passed"]]
    v.control(jev="ok", upstream="ok", unique_ids=True, jev_model="gpt-6-luna", jev_effort="low", cached=0)
    # Configure the classifier (shadow off) exactly as Phase 1 does.
    v.configure(False)
    user_token = v.api("auth/login", {"email": "customer@gpt-auto.local", "password": v.ENV["LOCAL_ADMIN_PASSWORD"]}, auth="")["access_token"]
    keys = v.api("keys?page_size=100", auth=user_token)
    keys = keys.get("items", keys) if isinstance(keys, dict) else keys
    key_a = next(k for k in keys if k["key"] == v.key)
    group_id = key_a["group_id"]
    # A second, unrelated customer for isolation checks.
    users = v.api("admin/users?page_size=100")
    users = users.get("items", users) if isinstance(users, dict) else users
    email_b = "customer-b@gpt-auto.local"
    if not any(u["email"] == email_b for u in users):
        v.api("admin/users", {"email": email_b, "password": v.ENV["LOCAL_ADMIN_PASSWORD"], "balance": 1000, "concurrency": 30, "allowed_groups": [group_id]}, "POST")
    token_b = v.api("auth/login", {"email": email_b, "password": v.ENV["LOCAL_ADMIN_PASSWORD"]}, auth="")["access_token"]
    key_b = v.api("keys", {"name": f"phase2-b-{int(time.time())}", "group_id": group_id}, "POST", auth=token_b)
    users = v.api("admin/users?page_size=100")
    users = users.get("items", users) if isinstance(users, dict) else users
    state.update(user_token=user_token, key_a_id=key_a["id"], group_id=group_id, token_b=token_b, key_b=key_b["key"],
                 key_b_id=key_b["id"], user_a_id=next(u["id"] for u in users if u["email"] == "customer@gpt-auto.local"),
                 user_b_id=next(u["id"] for u in users if u["email"] == email_b))
    return {k: state[k] for k in ("key_a_id", "key_b_id", "group_id")}


def defaults_safe():
    meta = v.api("admin/gpt-auto/meta")
    cfg = v.api("admin/gpt-auto/config")
    assert meta["phase"] == "shadow" and meta["live_routing_active"] is False
    assert cfg["live"]["live_enabled"] is False and cfg["live"]["internal_only"] is True
    assert [p["model"] for p in cfg["model_profiles"]] == ["gpt-5.6-luna", "gpt-6-luna", "gpt-6.1-sol"]
    models = v.http("/v1/models", auth=v.key)
    assert models["status"] == 200 and "gpt-auto" not in models["body"]
    denied = gateway("chat/completions", auto_chat())
    assert denied["status"] == 404 and denied["data"]["error"]["code"] == "model_not_found", denied["body"]
    normal = gateway("chat/completions", {"model": "gpt-6.1-sol", "messages": [{"role": "user", "content": "hi"}]})
    assert normal["status"] == 200, normal["body"][:300]
    # gpt-5.6-sol is still an ordinary gateway model.
    sol = gateway("chat/completions", {"model": "gpt-5.6-sol", "messages": [{"role": "user", "content": "hi"}]})
    assert sol["status"] == 200, sol["body"][:300]
    return {"gpt_auto_status_when_disabled": denied["status"], "pool": meta["models"], "retired": meta["retired_models"]}


def enable_internal():
    live = live_config(live_enabled=True, internal_only=True, allowed_api_keys=[state["key_a_id"]], allowed_groups=[state["group_id"]],
                       classifier_timeout_ms=1500, live_request_budget=0, classifier_failure_policy="compatible_fallback")
    meta = v.api("admin/gpt-auto/meta")
    assert meta["phase"] == "live" and meta["live_routing_active"] is True
    a = v.http("/v1/models", auth=v.key)
    b = v.http("/v1/models", auth=state["key_b"])
    assert "gpt-auto" in a["body"], "admitted key lists gpt-auto"
    assert "gpt-auto" not in b["body"], "internal_only: group allow list is ignored"
    denied_b = gateway("chat/completions", auto_chat(), auth=state["key_b"])
    assert denied_b["status"] == 404, denied_b["body"]
    return {"live": live, "key_b_status": denied_b["status"]}


def routes_each_model():
    out = {}
    for model, effort in (("gpt-6.1-sol", "high"), ("gpt-5.6-luna", "low"), ("gpt-6-luna", "medium")):
        v.control(jev_model=model, jev_effort=effort)
        r = gateway("chat/completions", auto_chat(f"route me to {model}"))
        assert r["status"] == 200, r["body"][:500]
        upstream = last_upstream()
        assert upstream["model"] == model, upstream
        assert upstream_effort(upstream) == effort, upstream
        assert header(r, "X-GPT-Auto-Model") == model, r["headers"]
        assert header(r, "X-GPT-Auto-Reasoning-Effort") == effort
        row = decision_row(rid(r))
        assert row[0] == "live" and row[1] == model and row[4] == "classified" and row[6] == "succeeded", row
        out[model] = {"effort": upstream_effort(upstream), "latency_ms": round(r["ms"], 1)}
    return out


def endpoints_streaming_tools():
    v.control(jev_model="gpt-6.1-sol", jev_effort="high")
    tool = [{"type": "function", "function": {"name": "local_tool", "parameters": {"type": "object", "properties": {"value": {"type": "integer"}}}}}]
    rtool = [{"type": "function", "name": "local_tool", "parameters": {"type": "object", "properties": {"value": {"type": "integer"}}}}]
    cases = {
        "chat": ("chat/completions", auto_chat(), False),
        "chat_stream": ("chat/completions", auto_chat(stream=True), True),
        "chat_tools": ("chat/completions", auto_chat(tools=tool), False),
        "chat_tools_stream": ("chat/completions", auto_chat(tools=tool, stream=True), True),
        "responses": ("responses", auto_responses(), False),
        "responses_stream": ("responses", auto_responses(stream=True), True),
        "responses_tools_stream": ("responses", auto_responses(tools=rtool, stream=True), True),
    }
    out = {}
    for name, (endpoint, body, stream) in cases.items():
        r = gateway(endpoint, body, stream=stream)
        assert r["status"] == 200, (name, r["body"][:500])
        if stream:
            assert "data:" in r["body"], name
            if endpoint == "responses":
                assert r["body"].count("event: response.completed") == 1, (name, "exactly one terminal event")
            else:
                assert r["body"].rstrip().endswith("data: [DONE]"), name
        if "tools" in name:
            assert "local_tool" in r["body"], name
        assert last_upstream()["model"] == "gpt-6.1-sol"
        out[name] = round(r["ms"], 1)
    # Tool result continuation (stateless chat): the follow-up turn routes normally.
    follow = auto_chat(tools=tool)
    follow["messages"] += [{"role": "assistant", "content": None, "tool_calls": [{"id": "call_local", "type": "function", "function": {"name": "local_tool", "arguments": "{\"value\":1}"}}]},
                           {"role": "tool", "tool_call_id": "call_local", "content": "{\"ok\":true}"}]
    r = gateway("chat/completions", follow)
    assert r["status"] == 200, r["body"][:500]
    out["chat_tool_result_followup"] = round(r["ms"], 1)
    return out


def effort_and_retired():
    v.control(jev_model="gpt-6.1-sol", jev_effort="none")
    r = gateway("chat/completions", auto_chat())
    assert r["status"] == 200
    up = last_upstream()
    assert up["model"] == "gpt-6.1-sol" and upstream_effort(up) == "low", up
    row = decision_row(rid(r))
    assert row[2] == "low" and row[3] == "gpt-6.1-sol", row
    v.control(jev_model="gpt-5.6-sol", jev_effort="high")
    r = gateway("chat/completions", auto_chat())
    assert r["status"] == 200
    up = last_upstream()
    assert up["model"] != "gpt-5.6-sol", "a retired Jev answer is never executed"
    v.control(jev_model="gpt-6.1-sol", jev_effort="high")
    # Client effort is recorded but Jev decides by default.
    r = gateway("chat/completions", auto_chat(reasoning_effort="xhigh"))
    assert upstream_effort(last_upstream()) == "high"
    return {"none_normalized_to": "low", "retired_answer_executed_as": up["model"]}


def billing():
    v.control(jev_model="gpt-6.1-sol", jev_effort="high", cached=40)
    direct = gateway("chat/completions", {"model": "gpt-6.1-sol", "reasoning_effort": "high", "messages": [{"role": "user", "content": "direct baseline"}]})
    routed = gateway("chat/completions", auto_chat("routed billing"))
    assert direct["status"] == 200 and routed["status"] == 200
    d = wait_usage(rid(direct))
    a = wait_usage(rid(routed))
    v.control(cached=0)
    assert a[0] == "gpt-6.1-sol" and a[1] == "gpt-auto", a
    assert d[1] == "gpt-6.1-sol", d
    assert a[6] == d[6] and a[7] == d[7] and a[8] == d[8], ("same tokens", a, d)
    assert int(a[7]) == 40, "cached input tokens recorded"
    assert abs(float(a[3]) - float(d[3])) < 1e-12, "list cost unchanged"
    ratio = float(a[2]) / float(d[2]) if float(d[2]) else None
    assert ratio is not None and abs(ratio - 1.5) < 1e-6, ("charge = direct x 1.5", a, d)
    # Balance moved by exactly the routed charge.
    before = float(v.sql(f"SELECT balance FROM users WHERE id = {state['user_a_id']}"))
    r = gateway("chat/completions", auto_chat("balance check"))
    row = wait_usage(rid(r))
    after = v.wait(lambda: (b := float(v.sql(f"SELECT balance FROM users WHERE id = {state['user_a_id']}"))) != before and b, 15)
    assert abs((before - after) - float(row[2])) < 1e-9, (before, after, row)
    return {"direct_actual_cost": d[2], "routed_actual_cost": a[2], "multiplier": ratio, "cached_tokens": a[7], "balance_delta": round(before - after, 10)}


def affinity():
    v.control(jev_model="gpt-6.1-sol", jev_effort="high")
    first = gateway("responses", auto_responses("start an agent task"))
    assert first["status"] == 200, first["body"][:300]
    response_id = first["data"]["id"]
    v.control(jev_model="gpt-5.6-luna", jev_effort="low")
    calls_before = sum(1 for e in mock_requests() if e["path"].endswith("/systemone"))
    nxt = gateway("responses", {"model": "gpt-auto", "previous_response_id": response_id,
                                "input": [{"type": "function_call_output", "call_id": "call_local", "output": "{\"ok\":true}"}]})
    assert nxt["status"] == 200, nxt["body"][:500]
    up = last_upstream()
    assert up["model"] == "gpt-6.1-sol" and up["previous_response_id"] == response_id, up
    calls_after = sum(1 for e in mock_requests() if e["path"].endswith("/systemone"))
    assert calls_after == calls_before, "affinity hit makes no Jev call"
    row = decision_row(rid(nxt))
    assert row[4] == "session_affinity" and row[8] == "hit", row
    # Session header affinity for chat agents.
    v.control(jev_model="gpt-6-luna", jev_effort="medium")
    s1 = gateway("chat/completions", auto_chat(), headers={"session-id": "phase2-agent-session"})
    v.control(jev_model="gpt-6.1-sol", jev_effort="high")
    s2 = gateway("chat/completions", auto_chat(), headers={"session-id": "phase2-agent-session"})
    assert s1["status"] == s2["status"] == 200
    assert last_upstream()["model"] == "gpt-6-luna", "agent session keeps its model"
    # nginx drops underscore headers by default: the same session via
    # "session_id" never reaches the gateway behind the proxy.
    keys = v.sql("SELECT count(*) FROM gpt_auto_shadow_decisions WHERE affinity_outcome = 'hit'")
    return {"response_id": response_id, "affinity_hits": keys}


def failures():
    out = {}
    v.control(jev_model="gpt-6.1-sol", jev_effort="high")
    live_config(classifier_timeout_ms=500, fallback_model="gpt-6-luna", fallback_effort="medium")
    for mode in ("delay", "429", "401", "malformed", "incomplete"):
        v.control(jev=mode, delay=2.0)
        r = gateway("chat/completions", auto_chat(f"failure {mode}"))
        assert r["status"] == 200, (mode, r["status"], r["body"][:300])
        assert last_upstream()["model"] == "gpt-6-luna", mode
        row = decision_row(rid(r))
        assert row[4] == "fallback" and row[5], (mode, row)
        out[mode] = {"status": r["status"], "fallback_reason": row[5], "ms": round(r["ms"], 1)}
    v.control(jev="ok")
    # Reject policy: structured 503, never an unexplained 502.
    live_config(classifier_failure_policy="reject_request")
    v.control(jev="500")
    time.sleep(31)  # let the live circuit (opened above) reach half-open
    r = gateway("chat/completions", auto_chat("reject policy"))
    assert r["status"] == 503 and r["data"]["error"]["code"] == "gpt_auto_routing_unavailable", r["body"][:300]
    out["reject_policy"] = {"status": r["status"], "code": r["data"]["error"]["code"]}
    v.control(jev="ok")
    live_config(classifier_failure_policy="compatible_fallback", classifier_timeout_ms=1500)
    time.sleep(31)
    # Upstream failures are reported as upstream errors with the routed model logged.
    v.control(upstream="500")
    r = gateway("chat/completions", auto_chat("upstream 500"))
    v.control(upstream="ok")
    v.reset_account()
    row = decision_row(rid(r))
    assert r["status"] == 502 and "upstream" in r["body"].lower(), r["body"][:300]
    assert row[6] == "failed", row
    out["upstream_500"] = {"status": r["status"], "body": r["data"], "decision_status": row[6]}
    # No compatible model: every profile disabled.
    cfg = v.api("admin/gpt-auto/config")
    original = json.loads(json.dumps(cfg["model_profiles"]))
    for p in cfg["model_profiles"]:
        p["enabled"] = False
    cfg["classifier_api_key"] = ""
    v.api("admin/gpt-auto/config", cfg, "PUT")
    r = gateway("chat/completions", auto_chat("nothing eligible"))
    cfg["model_profiles"] = original
    v.api("admin/gpt-auto/config", cfg, "PUT")
    assert r["status"] == 400 and r["data"]["error"]["code"] == "gpt_auto_no_compatible_model", r["body"][:300]
    out["no_compatible_model"] = r["data"]["error"]
    return out


def client_cancellation():
    v.control(upstream="delay", delay=3.0)
    try:
        gateway("chat/completions", auto_chat("cancel me", stream=True), stream=True, timeout=1)
        raise AssertionError("expected client timeout")
    except (TimeoutError, OSError):
        pass
    finally:
        time.sleep(3.5)
        v.control(upstream="ok")
    row = v.sql("SELECT request_status, error_category FROM gpt_auto_shadow_decisions WHERE mode = 'live' ORDER BY id DESC LIMIT 1")
    v.reset_account()
    assert row.startswith("client_cancelled|"), row
    return {"last_live_decision": row}


def insufficient_balance():
    live_config(allowed_api_keys=[state["key_a_id"], state["key_b_id"]])
    balance = float(v.sql(f"SELECT balance FROM users WHERE id = {state['user_b_id']}"))
    if balance > 0:
        v.api(f"admin/users/{state['user_b_id']}/balance", {"balance": balance, "operation": "subtract", "notes": "phase2 insufficient balance check"}, "POST")
    time.sleep(1)
    r = gateway("chat/completions", auto_chat("no balance"), auth=state["key_b"])
    v.api(f"admin/users/{state['user_b_id']}/balance", {"balance": 1000, "operation": "add", "notes": "phase2 restore"}, "POST")
    assert r["status"] in (402, 403, 429), r["body"][:300]
    assert r["status"] != 502
    return {"status": r["status"], "body": r["data"]}


def concurrency():
    v.control(jev_model="gpt-6-luna", jev_effort="low")
    with concurrent.futures.ThreadPoolExecutor(max_workers=30) as pool:
        statuses = list(pool.map(lambda i: gateway("chat/completions", auto_chat(f"concurrent {i}"))["status"], range(60)))
    assert statuses.count(200) == 60, statuses
    return {"requests": 60, "ok": statuses.count(200)}


def long_agent():
    v.control(jev_model="gpt-6.1-sol", jev_effort="high")
    items = [{"role": "user", "content": "Refactor the billing module step by step."}]
    for i in range(1000):
        items.append({"type": "function_call", "call_id": f"c{i}", "name": "local_tool", "arguments": "{\"value\":1}"})
        items.append({"type": "function_call_output", "call_id": f"c{i}", "output": "ok"})
    r = gateway("responses", {"model": "gpt-auto", "input": items}, timeout=60)
    assert r["status"] == 200, r["body"][:300]
    return {"items": len(items), "ms": round(r["ms"], 1), "model": last_upstream()["model"]}


def exports():
    end = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() + 60))
    start = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() - 3 * 3600))
    q = f"start_time={start}&end_time={end}"
    status, headers, body = raw_get(f"/api/v1/admin/gpt-auto/decisions/export?format=csv&{q}", v.token)
    assert status == 200 and headers["Content-Type"].startswith("text/csv"), (status, body[:300])
    text = body.decode("utf-8-sig")
    rows = list(csv.DictReader(io.StringIO(text)))
    assert rows, "admin CSV has rows"
    missing = [c for c in ADMIN_EXPORT_COLUMNS_REQUIRED if c not in rows[0]]
    assert not missing, missing
    live = [r for r in rows if r["mode"] == "live"]
    shadow = [r for r in rows if r["mode"] == "shadow"]
    assert live, "live rows exported"
    billed = [r for r in live if r["billing_status"] == "final"]
    assert billed and all(r["customer_charge_usd"] for r in billed)
    assert all("." in r["customer_charge_usd"] and len(r["customer_charge_usd"].split(".")[1]) == 10 for r in billed)
    assert all(r["timestamp"].endswith("Z") for r in rows)
    (OUT / "admin-export.csv").write_bytes(body)

    status, headers, body = raw_get(f"/api/v1/admin/gpt-auto/decisions/export?format=jsonl&mode=live&final_model=gpt-6.1-sol&{q}", v.token)
    assert status == 200 and "ndjson" in headers["Content-Type"]
    lines = [json.loads(line) for line in body.decode().splitlines() if line.strip()]
    assert lines and all(x["mode"] == "live" and x["final_model"] == "gpt-6.1-sol" for x in lines), "server-side filters"
    assert any(isinstance(x["model_probabilities"], dict) for x in lines), "nested probabilities in JSONL"
    (OUT / "admin-export-live.jsonl").write_bytes(body)
    count = v.api(f"admin/gpt-auto/decisions/export/count?mode=live&final_model=gpt-6.1-sol&{q}")
    assert count["rows"] == len(lines), (count, len(lines))

    # Customer A: own rows only, safe schema.
    status, _, body = raw_get(f"/api/v1/gpt-auto/routing-logs/export?format=csv&{q}&user_id={state['user_b_id']}", state["user_token"])
    assert status == 200, body[:300]
    crow = list(csv.DictReader(io.StringIO(body.decode("utf-8-sig"))))
    header = ",".join(crow[0].keys()) if crow else ""
    assert crow and all(not any(f in col for f in CUSTOMER_FORBIDDEN) for col in crow[0].keys()), header
    a_ids = set(v.sql(f"SELECT request_id FROM gpt_auto_shadow_decisions WHERE mode='live' AND user_id = {state['user_a_id']}").split())
    assert all(r["request_id"] in a_ids for r in crow), "customer export contains only own rows"
    (OUT / "customer-a-export.csv").write_bytes(body)
    # Customer B: never sees A's rows, whatever it passes.
    status, _, body = raw_get(f"/api/v1/gpt-auto/routing-logs/export?format=jsonl&{q}&user_id={state['user_a_id']}&mode=shadow", state["token_b"])
    b_lines = [json.loads(line) for line in body.decode().splitlines() if line.strip()]
    assert status == 200 and not any(x["request_id"] in a_ids for x in b_lines), "no cross-customer export"
    # Authorization boundaries.
    assert raw_get(f"/api/v1/admin/gpt-auto/decisions/export?{q}", state["user_token"])[0] == 403
    assert raw_get(f"/api/v1/gpt-auto/routing-logs/export?{q}", "invalid")[0] == 401
    audits = v.sql("SELECT scope, count(*) FROM gpt_auto_export_audit GROUP BY scope ORDER BY scope")
    assert "admin" in audits and "customer" in audits, audits
    # Customer export switch.
    live_config(customer_export_enabled=False)
    disabled = raw_get(f"/api/v1/gpt-auto/routing-logs/export?{q}", state["user_token"])[0]
    live_config(customer_export_enabled=True)
    assert disabled == 403
    return {"admin_rows": len(rows), "live": len(live), "shadow": len(shadow), "filtered_jsonl": len(lines),
            "customer_a_rows": len(crow), "customer_b_rows": len(b_lines), "audit": audits}


def dependency_restarts():
    out = {}
    v.dc("restart", "postgres")
    v.wait(lambda: gateway("chat/completions", auto_chat("after pg restart"))["status"] == 200, 90)
    out["postgres_restart"] = "recovered"
    v.dc("stop", "redis")
    try:
        r = gateway("chat/completions", auto_chat("redis down"), timeout=30)
        out["redis_down"] = {"status": r["status"], "ms": round(r["ms"]), "body": r["body"][:200]}
        assert r["status"] != 502, "never an unexplained 502"
    finally:
        v.dc("start", "redis")
    v.wait(lambda: gateway("chat/completions", auto_chat("after redis restart"))["status"] == 200, 90)
    out["redis_restart"] = "recovered"
    return out


def latency_overhead():
    v.control(jev="ok", jev_model="gpt-6.1-sol", jev_effort="high", unique_ids=True)
    def sample(body):
        times = []
        for _ in range(25):
            r = gateway("chat/completions", body)
            assert r["status"] == 200
            times.append(r["ms"])
        times.sort()
        return {"median": round(statistics.median(times), 2), "p95": round(times[int(len(times) * 0.95) - 1], 2)}
    direct = sample({"model": "gpt-6.1-sol", "reasoning_effort": "high", "messages": [{"role": "user", "content": "latency"}]})
    routed = sample(auto_chat("latency"))
    classifier = v.sql("SELECT round(avg(classifier_latency_ms)), percentile_cont(0.95) WITHIN GROUP (ORDER BY classifier_latency_ms) "
                       "FROM gpt_auto_shadow_decisions WHERE mode='live' AND routing_reason='classified' AND created_at > now() - interval '5 minutes'")
    gateway_ms = v.sql("SELECT round(avg(gateway_latency_ms)) FROM gpt_auto_shadow_decisions WHERE mode='live' AND created_at > now() - interval '5 minutes'")
    return {"direct_ms": direct, "gpt_auto_ms": routed, "median_overhead_ms": round(routed["median"] - direct["median"], 2),
            "classifier_avg_p95_ms": classifier, "routing_avg_ms": gateway_ms,
            "note": "local mock Jev over loopback TLS; production Jev latency will dominate"}


def disable_restores():
    live_config(live_enabled=False)
    r = gateway("chat/completions", auto_chat("after disable"))
    assert r["status"] == 404 and r["data"]["error"]["code"] == "model_not_found", r["body"][:300]
    normal = gateway("chat/completions", {"model": "gpt-6-luna", "messages": [{"role": "user", "content": "hi"}]})
    assert normal["status"] == 200
    assert "gpt-auto" not in v.http("/v1/models", auth=v.key)["body"]
    meta = v.api("admin/gpt-auto/meta")
    assert meta["phase"] == "shadow"
    return {"gpt_auto_after_disable": r["status"], "ordinary": normal["status"]}


def main():
    if not check("setup", setup):
        return 1
    check("defaults_disabled_and_three_model_pool", defaults_safe)
    if not check("enable_internal_live_routing", enable_internal):
        return 1
    check("jev_selects_each_pool_model_and_effort", routes_each_model)
    check("chat_responses_sse_and_tools", endpoints_streaming_tools)
    check("effort_normalization_and_retired_answer", effort_and_retired)
    check("dynamic_billing_on_executed_model", billing)
    check("session_affinity_and_stateful_continuation", affinity)
    check("failure_injection_and_policies", failures)
    check("client_cancellation", client_cancellation)
    check("insufficient_balance", insufficient_balance)
    check("concurrent_requests", concurrency)
    check("long_agent_session", long_agent)
    check("csv_jsonl_export_and_isolation", exports)
    check("latency_overhead", latency_overhead)
    check("dependency_restarts", dependency_restarts)
    check("disable_restores_safe_behaviour", disable_restores)
    summary = {"scenarios": len(results), "passed": sum(x["passed"] for x in results),
               "failed": [x["name"] for x in results if not x["passed"]]}
    (OUT / "phase2-summary.json").write_text(json.dumps(summary, indent=2))
    print(json.dumps(summary), flush=True)
    return 0 if not summary["failed"] else 1


if __name__ == "__main__":
    sys.exit(main())
