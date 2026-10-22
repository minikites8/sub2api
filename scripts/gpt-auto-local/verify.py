"""Exercise the real, isolated stack using supported authentication/admin APIs."""
import concurrent.futures
import json
import os
import pathlib
import re
import statistics
import subprocess
import sys
import time
import traceback
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
os.chdir(ROOT)
ENV = dict(line.split("=", 1) for line in pathlib.Path(".env.gpt-auto-local").read_text().splitlines() if line and not line.startswith("#"))
OUT = ROOT / "artifacts/gpt-auto-local"
BASE = "http://127.0.0.1:" + ENV.get("LOCAL_HTTP_PORT", "18080")
DIRECT = "http://127.0.0.1:" + ENV.get("LOCAL_BACKEND_PORT", "18081")
MOCK = "http://127.0.0.1:" + ENV.get("LOCAL_MOCK_PORT", "18082")
DC = ["docker", "compose", "--env-file", ".env.gpt-auto-local", "-p", "gpt-auto-local", "-f", "compose.gpt-auto-local.yml"]
results = []
token = ""
key = ""
cfg = {}
account_id = 0
proxy_id = None
dependency_result = {}


def save(name, data):
    (OUT / name).write_text(json.dumps(data, ensure_ascii=False, indent=2))


def http(path, body=None, method=None, auth=None, base=BASE, timeout=20):
    raw = body if isinstance(body, bytes) else json.dumps(body, ensure_ascii=False).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if auth:
        headers["Authorization"] = "Bearer " + auth
    req = urllib.request.Request(base + path, data=raw, method=method, headers=headers)
    start = time.perf_counter()
    try:
        response = urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.HTTPError as err:
        response = err
    with response:
        content = response.read().decode(errors="replace")
        try:
            data = json.loads(content)
        except ValueError:
            data = None
        return {"status": response.code, "data": data, "body": content,
                "headers": dict(response.headers), "ms": (time.perf_counter() - start) * 1000}


def api(path, body=None, method=None, expected=200, auth=None):
    r = http("/api/v1/" + path, body, method, token if auth is None else auth)
    assert r["status"] == expected, (path, r["status"], r["body"][:1000])
    if expected == 200:
        assert r["data"]["code"] == 0, r["data"]
        return r["data"]["data"]
    return r


def dc(*args):
    return subprocess.check_output(DC + list(args), text=True, stderr=subprocess.STDOUT)


def sql(query):
    return dc("exec", "-T", "postgres", "psql", "-U", "gpt_auto_local", "-d", "gpt_auto_local", "-At", "-c", query).strip()


def check(name, fn):
    start = time.monotonic()
    try:
        detail = fn()
        entry = {"name": name, "passed": True, "detail": detail}
    except Exception as e:
        entry = {"name": name, "passed": False, "error": str(e), "traceback": traceback.format_exc()}
    entry["seconds"] = round(time.monotonic() - start, 3)
    results.append(entry)
    print(json.dumps(entry, ensure_ascii=False), flush=True)
    save("http-results.json", results)
    return entry["passed"]


def wait(fn, seconds=60):
    end = time.monotonic() + seconds
    last = None
    while time.monotonic() < end:
        try:
            last = fn()
            if last:
                return last
        except Exception as e:
            last = str(e)
        time.sleep(0.5)
    raise AssertionError("readiness deadline: " + str(last))


def control(**kwargs):
    r = http("/control", kwargs, base=MOCK)
    assert r["status"] == 200


def configure(enabled):
    global cfg
    cfg["shadow_enabled"] = enabled
    cfg["sample_rate"] = 100
    cfg["all_groups"] = True
    cfg["classifier_api_key"] = "synthetic-local-jev-key"
    cfg["classifier_base_url"] = "https://mock:8443"
    cfg["classifier_proxy_id"] = proxy_id
    cfg["classifier_timeout_ms"] = 500
    cfg["classifier_retry_count"] = 0
    cfg["classifier_daily_call_limit"] = 100000
    cfg["classifier_daily_cost_limit"] = 100
    cfg["classification_context_strategy"] = "task_aware"
    cfg = api("admin/gpt-auto/config", cfg, "PUT")
    assert cfg["shadow_enabled"] == enabled
    assert not cfg.get("classifier_api_key")
    time.sleep(0.15)


def chat(text="Synthetic engineering fixture", **kw):
    return {"model": "gpt-6.1-sol", "messages": [{"role": "user", "content": text}], "reasoning_effort": "high", **kw}


def responses(text="Synthetic engineering fixture", **kw):
    return {"model": "gpt-6.1-sol", "input": [{"role": "user", "content": text}], "reasoning": {"effort": "high"}, **kw}


def infer(endpoint="chat/completions", body=None, base=BASE):
    r = http("/v1/" + endpoint, body or chat(), auth=key, base=base)
    assert r["status"] == 200, (r["status"], r["body"][:700])
    return r


def overview():
    return api("admin/gpt-auto/overview")


def decisions():
    return api("admin/gpt-auto/decisions?page_size=100")["items"]


def startup():
    for base in (BASE, DIRECT):
        assert http("/health", base=base)["status"] == 200
        html = http("/login", base=base)
        assert html["status"] == 200 and "<html" in html["body"]
        assets = re.findall(r'(?:src|href)="(/assets/[^\"]+)"', html["body"])
        assert assets, "embedded assets missing"
        for asset in assets:
            assert http(asset, base=base)["status"] == 200, asset
    applied = sql("SELECT filename FROM schema_migrations ORDER BY filename")
    assert "278_gpt_auto_shadow_decisions.sql" in applied
    assert "279_gpt_auto_classifier_versioning.sql" in applied
    assert dc("exec", "-T", "redis", "redis-cli", "ping").strip() == "PONG"
    (OUT / "migrations.txt").write_text(applied)
    return {"migration_count": len(applied.splitlines()), "proxy": BASE, "backend": DIRECT, "assets": assets}


def authenticate():
    global token, cfg
    for base in (BASE, DIRECT):
        r = http("/api/v1/admin/gpt-auto/meta", base=base)
        assert r["status"] == 401, r
        assert http("/api/v1/admin/gpt-auto/meta", auth="invalid", base=base)["status"] == 401
    data = api("auth/login", {"email": ENV["LOCAL_ADMIN_EMAIL"], "password": ENV["LOCAL_ADMIN_PASSWORD"]}, auth="")
    token = data["access_token"]
    cfg = api("admin/gpt-auto/config")
    meta = api("admin/gpt-auto/meta")
    assert meta["state_version"] == "state-v3"
    assert meta["classifier_schema"] == "jev-questions-v2"
    assert meta["diagnostics_version"] == "context-diagnostics-v1"
    assert meta["phase"] == "shadow" and meta["live_routing_active"] is False
    assert {m["model"] for m in meta["models"]} == {"gpt-5.6-luna", "gpt-5.6-sol", "gpt-6-luna", "gpt-6.1-sol"}
    save("runtime-meta.json", meta)
    return {k: meta[k] for k in ("state_version", "classifier_schema", "diagnostics_version", "phase", "live_routing_active")}


def setup_customer():
    global key, account_id, proxy_id
    # Supported public admin APIs; no auth bypass or direct account SQL writes.
    groups = api("admin/groups?page_size=100")
    groups = groups.get("items", groups) if isinstance(groups, dict) else groups
    group = next((x for x in groups if x["name"] == "gpt-auto-local-synthetic"), None)
    if group is None:
        group = api("admin/groups", {"name": "gpt-auto-local-synthetic", "platform": "openai", "rate_multiplier": 1}, "POST")
    accounts = api("admin/accounts?page_size=100")
    accounts = accounts.get("items", accounts) if isinstance(accounts, dict) else accounts
    account = next((x for x in accounts if x["name"] == "gpt-auto-local-mock"), None)
    if account is None:
        account = api("admin/accounts", {"name": "gpt-auto-local-mock", "platform": "openai", "type": "apikey",
            "credentials": {"api_key": "synthetic-local-upstream-key", "base_url": "http://mock:8080/v1", "openai_responses_mode": "force_responses", "openai_capabilities": ["chat_completions"]},
            "extra": {"openai_passthrough": True}, "concurrency": 30, "priority": 1, "group_ids": [group["id"]]}, "POST")
    account_id = account["id"]
    proxies = api("admin/proxies?page_size=100")
    proxies = proxies.get("items", proxies) if isinstance(proxies, dict) else proxies
    proxy = next((x for x in proxies if x["name"] == "gpt-auto-local-classifier-proxy"), None)
    if proxy is None:
        proxy = api("admin/proxies", {"name": "gpt-auto-local-classifier-proxy", "protocol": "http", "host": "mock", "port": 8080}, "POST")
    proxy_id = proxy["id"]
    users = api("admin/users?page_size=100")
    users = users.get("items", users) if isinstance(users, dict) else users
    email = "customer@gpt-auto.local"
    if not any(x["email"] == email for x in users):
        api("admin/users", {"email": email, "password": ENV["LOCAL_ADMIN_PASSWORD"], "balance": 10000, "concurrency": 30, "allowed_groups": [group["id"]]}, "POST")
    user_token = api("auth/login", {"email": email, "password": ENV["LOCAL_ADMIN_PASSWORD"]}, auth="")["access_token"]
    assert http("/api/v1/admin/gpt-auto/meta", auth=user_token)["status"] == 403
    api_key = api("keys", {"name": "gpt-auto-local-synthetic", "group_id": group["id"]}, "POST", auth=user_token)
    key = api_key["key"]
    control(jev="ok", upstream="ok")
    reset_account()
    configure(False)
    return {"group_id": group["id"], "regular_user_admin_status": 403}


def reset_account():
    api(f"admin/accounts/{account_id}/clear-error", {}, "POST")
    api(f"admin/accounts/{account_id}/clear-rate-limit", {}, "POST")
    api(f"admin/accounts/{account_id}/temp-unschedulable", method="DELETE")
    time.sleep(0.25)
    # Existing in-memory model cooldown is independent of the account's stored
    # error flags. Wait for its natural expiry, then a successful call resets it.
    if key:
        wait(lambda: http("/v1/chat/completions", chat(), auth=key)["status"] == 200, 30)


def admin_endpoints():
    global cfg
    configure(True)
    classifier = api("admin/gpt-auto/classifier")
    save("classifier.json", classifier)
    draft = dict(classifier["instructions"])
    draft["model_selection"] += " Use synthetic local fixtures only for this validation."
    preview = api("admin/gpt-auto/classifier/preview", draft)
    assert preview["hash"] != classifier["hash"]
    updated = api("admin/gpt-auto/classifier", draft, "PUT")
    assert updated["hash"] == preview["hash"]
    restored = api("admin/gpt-auto/classifier", classifier["instructions"], "PUT")
    assert restored["instructions"]["model_selection"] == classifier["instructions"]["model_selection"]
    original_profiles = json.loads(json.dumps(cfg["model_profiles"]))
    cfg["model_profiles"][0]["routing_weight"] += 1
    cfg["model_profiles"][0]["billing_multiplier"] = 1.25
    cfg = api("admin/gpt-auto/config", cfg, "PUT")
    assert cfg["model_profiles"][0]["billing_multiplier"] == 1.25
    cfg["model_profiles"] = original_profiles
    cfg = api("admin/gpt-auto/config", cfg, "PUT")
    api("admin/gpt-auto/overview")
    api("admin/gpt-auto/decisions")
    test = api("admin/gpt-auto/test", {"prompt": "Write a Go function to sort an integer list", "endpoint": "chat_completions"})
    assert test["classification"]["model"] == "gpt-6-luna", test
    assert test["classifier_config_hash"]
    cfg["sample_rate"] = 101
    api("admin/gpt-auto/config", cfg, "PUT", expected=400)
    cfg["sample_rate"] = 100
    return {"classifier_config_hash": test["classifier_config_hash"], "lab_model": test["classification"]["model"], "profile_count": len(cfg["model_profiles"])}


def debug_fixtures():
    base_task = {"role": "user", "content": "Design a bounded Go queue with retries and fencing tokens"}
    agent = [base_task, {"role": "assistant", "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "local_tool", "arguments": "{}"}}]},
             {"role": "tool", "tool_call_id": "call_1", "content": "Error: process failed with exit code 1"}, {"role": "user", "content": "continue"}]
    fixtures = {
        "simple": ("chat_completions", chat("Hello synthetic fixture")),
        "coding": ("chat_completions", chat("Write a Python list sorting function")),
        "complex": ("chat_completions", chat(base_task["content"])),
        "continue": ("chat_completions", chat(messages=[base_task, {"role": "assistant", "content": "I will implement it"}, {"role": "user", "content": "continue"}])),
        "chinese": ("chat_completions", chat(messages=[base_task, {"role": "assistant", "content": "开始实现"}, {"role": "user", "content": "继续"}])),
        "agent_failure": ("chat_completions", chat(messages=agent)),
        "600_messages": ("chat_completions", chat(messages=[{"role": "user" if n % 2 == 0 else "assistant", "content": f"Synthetic turn {n}"} for n in range(600)])),
        "1202_messages": ("chat_completions", chat(messages=[{"role": "user" if n % 2 == 0 else "assistant", "content": f"Synthetic turn {n}"} for n in range(1202)])),
        "long_agent": ("chat_completions", chat(messages=agent[:-1] + agent[1:3] * 650 + [agent[-1]])),
        "missing_history": ("responses", responses("continue", previous_response_id="resp_missing_synthetic")),
        "multimodal": ("chat_completions", chat(messages=[{"role": "user", "content": [{"type": "text", "text": "Describe this synthetic image"}, {"type": "image_url", "image_url": {"url": "data:image/png;base64,c3ludGhldGlj"}}]}])),
        "redaction": ("chat_completions", chat("Synthetic contact private@example.test and Authorization: Bearer sk-test-secret-abcdefghijklmnopqrstuvwxyz")),
        "truncation": ("chat_completions", chat("Synthetic long task " * 10000)),
    }
    summary = {}
    for name, (endpoint, request) in fixtures.items():
        result = api("admin/gpt-auto/debug/state", {"endpoint": endpoint, "request": request})
        ctx = result["context"]
        diag = ctx["diagnostics"]
        assert ctx["state_version"] == "state-v3"
        assert ctx["strategy"] == "task_aware"
        assert ctx["used_chars"] <= ctx["budget_chars"]
        assert diag["diagnostics_version"] == "context-diagnostics-v1"
        assert len(json.dumps(diag).encode()) < 4096
        counts = diag["items"]
        assert counts["eligible"] == counts["represented"] + counts["budget_dropped"] + counts["not_selected"]
        assert ctx["items_total"] == sum(counts[x] for x in ("eligible", "privacy", "unsupported", "external_references", "compacted", "empty"))
        state = result["jev_request"]["state"]
        if name in ("continue", "chinese"):
            assert diag["follow_up"]["resolution"] == "linked_request", diag
            assert base_task["content"] in state
        if name == "agent_failure":
            assert diag["agent"]["tool_failures_detected"] >= 1, diag
        if name in ("600_messages", "1202_messages"):
            assert ctx["items_total"] == int(name.split("_")[0])
        if name == "missing_history":
            assert "history_by_reference" in ctx["limitations"]
            assert counts["conversation_length_known"] is False
        if name == "multimodal":
            assert counts["with_images"] == 1
            assert "c3ludGhldGlj" not in state
        if name == "redaction":
            assert "private@example.test" not in state and "sk-test-secret" not in state
        if name == "truncation":
            assert diag["context_truncated"]
        summary[name] = {"context": ctx, "state_chars": result["state_chars"], "classifier_config_hash": result["classifier_config_hash"]}
    api("admin/gpt-auto/debug/state", {"request": chat("x" * (4 * 1024 * 1024))}, expected=400)
    api("admin/gpt-auto/debug/state", {"endpoint": "unsupported", "request": {}}, expected=400)
    save("debug-fixtures.json", summary)
    return {"fixtures": len(summary), "oversized_debug_status": 400}


def gateway_compat():
    outputs = {}
    for enabled in (False, True):
        configure(enabled)
        control(jev="ok", upstream="ok", reset=True)
        values = {}
        for base in (BASE, DIRECT):
            for endpoint, make in (("chat/completions", chat), ("responses", responses)):
                for stream in (False, True):
                    r = infer(endpoint, make(stream=stream), base)
                    if stream:
                        assert "text/event-stream" in r["headers"].get("Content-Type", "")
                        assert "data:" in r["body"]
                    else:
                        assert r["data"]["model"] == "gpt-6.1-sol", r["body"]
                    values[f"{base}/{endpoint}/{stream}"] = {"status": r["status"], "body": r["body"]}
        for endpoint, make in (("chat/completions", chat), ("responses", responses)):
            tool = {"type": "function", "function": {"name": "local_tool", "parameters": {"type": "object", "properties": {}}}} if endpoint == "chat/completions" else {"type": "function", "name": "local_tool", "parameters": {"type": "object", "properties": {}}}
            for stream in (False, True):
                r = infer(endpoint, make(tools=[tool], stream=stream))
                assert "call_local" in r["body"]
                values[endpoint + "/tool/" + str(stream)] = {"status": r["status"], "body": r["body"]}
        for raw in (b"not-json", b'{}', b'{"model":"gpt-6.1-sol","stream":"invalid"}'):
            r = http("/v1/chat/completions", raw, auth=key)
            assert r["status"] == 400, (r["status"], r["body"])
        assert http("/v1/chat/completions", chat())["status"] == 401
        infer(body=chat("Synthetic large body " * 60000))
        with concurrent.futures.ThreadPoolExecutor(max_workers=10) as pool:
            concurrent_ms = list(pool.map(lambda _: infer()["ms"], range(30)))
        upstream = http("/control", base=MOCK)["data"]["requests"]
        inference = [x for x in upstream if not x["path"].endswith("systemone")]
        assert inference and all(x["model"] == "gpt-6.1-sol" for x in inference)
        # The mock observes the actual body after every gateway compatibility transform.
        assert all((x["effort"].get("effort") if isinstance(x["effort"], dict) else x["effort"]) == "high" for x in inference), inference[:5]
        outputs[str(enabled)] = values
        save("gateway-upstream-" + str(enabled) + ".json", inference)
    save("gateway-responses.json", outputs)
    # Compare payloads and ordered SSE events, accounting for the gateway's
    # per-request IDs/timestamps in Chat Completions compatibility conversion.
    def canonical_value(value):
        if isinstance(value, dict):
            return {k: canonical_value(v) for k, v in value.items() if k not in ("created", "created_at")}
        if isinstance(value, list):
            return [canonical_value(x) for x in value]
        return value
    def canonical_response(value):
        raw = value["body"]
        if raw.startswith("{"):
            body = canonical_value(json.loads(raw))
        else:
            body = []
            for line in raw.splitlines():
                if line.startswith("data: ") and line[6:] != "[DONE]":
                    body.append(("data", canonical_value(json.loads(line[6:]))))
                elif line:
                    body.append(line)
        return {"status": value["status"], "body": body}
    left = {k: canonical_response(v) for k, v in outputs["False"].items()}
    right = {k: canonical_response(v) for k, v in outputs["True"].items()}
    assert left == right, "shadow changed customer response or SSE sequence; see gateway-responses.json"
    return {"exact_response_pairs": len(outputs["True"]), "concurrent_requests_per_mode": 30, "last_concurrent_median_ms": statistics.median(concurrent_ms)}


def classification_failures():
    configure(True)
    detail = {}
    for mode in ("delay", "401", "403", "malformed", "incomplete"):
        wait(lambda: overview()["runtime"]["queue_length"] == 0 and overview()["runtime"]["active_calls"] == 0)
        control(jev=mode, upstream="ok", delay=2)
        before = int(sql("SELECT coalesce(max(id),0) FROM gpt_auto_shadow_decisions"))
        r = infer()
        assert r["ms"] < 1500, (mode, r["ms"])
        row = wait(lambda: sql(f"SELECT evaluation_status || ':' || error_class FROM gpt_auto_shadow_decisions WHERE id > {before} ORDER BY id DESC LIMIT 1"), 15)
        expected = {"delay": "timeout", "401": "auth", "403": "auth", "malformed": "invalid_answer", "incomplete": "invalid_answer"}[mode]
        assert row == "failed:" + expected, (mode, row)
        detail[mode] = {"customer_status": 200, "customer_ms": r["ms"], "decision": row}
        time.sleep(0.15)
    control(jev="ok")
    assert overview()["runtime"]["circuit_state"] == "open"
    skipped = overview()["runtime"]["counters"]["skipped_circuit_open"]
    assert infer()["status"] == 200
    assert overview()["runtime"]["counters"]["skipped_circuit_open"] > skipped
    wait(lambda: overview()["runtime"]["circuit_state"] != "open", 35)
    infer()  # Half-open probe succeeds and restores the classifier circuit.
    wait(lambda: overview()["runtime"]["circuit_state"] == "closed")
    detail["circuit"] = {"opened_after_failures": True, "customer_status_while_open": 200, "recovered": True}
    save("jev-failures.json", detail)
    return detail


def upstream_errors():
    values = {}
    configure(False)
    try:
        for mode in ("500", "502", "delay"):
            values[mode] = []
            for base in (BASE, DIRECT):
                control(upstream="ok")
                reset_account()
                control(upstream=mode, delay=4)
                r = http("/v1/chat/completions", chat(), auth=key, base=base, timeout=30)
                assert r["status"] in (500, 502, 503, 504), r
                values[mode].append({"status": r["status"], "body": r["body"], "ms": r["ms"], "content_type": r["headers"].get("Content-Type")})
            assert values[mode][0]["status"] == values[mode][1]["status"]
    finally:
        control(upstream="ok")
        reset_account()
        configure(True)
    save("upstream-errors.json", values)
    return values


def dependency_failures():
    global dependency_result
    values = {}
    for enabled in (True, False):
        configure(enabled)
        infer()  # Warm the existing gateway auth, billing and account caches.
        wait(lambda: overview()["runtime"]["queue_length"] == 0 and overview()["runtime"]["active_calls"] == 0)
        budget_before = overview()["runtime"]["counters"]["skipped_budget_unavailable"]
        dc("stop", "redis")
        try:
            r = http("/v1/chat/completions", chat(), auth=key, timeout=30)
            values["redis_down_shadow_" + str(enabled)] = {"health": http("/health")["status"], "status": r["status"], "body": r["body"][:500], "ms": r["ms"]}
        finally:
            dc("start", "redis")
            wait(lambda: dc("exec", "-T", "redis", "redis-cli", "ping").strip() == "PONG")
        wait(lambda: http("/health")["status"] == 200)
        wait(lambda: http("/v1/chat/completions", chat(), auth=key)["status"] == 200)
        budget_after = overview()["runtime"]["counters"]["skipped_budget_unavailable"]
        values["redis_down_shadow_" + str(enabled)]["budget_skip_delta"] = budget_after - budget_before
        save("dependency-failures.json", values)
    assert values["redis_down_shadow_True"]["status"] == values["redis_down_shadow_False"]["status"], values
    assert values["redis_down_shadow_True"]["budget_skip_delta"] >= 1, values
    values["redis_recovered"] = True
    configure(True)
    dc("restart", "postgres")
    wait(lambda: "accepting connections" in dc("exec", "-T", "postgres", "pg_isready", "-U", "gpt_auto_local"))
    wait(lambda: api("admin/gpt-auto/config"))
    infer()
    values["postgres_recovered"] = True
    dc("stop", "backend")
    try:
        r = http("/health")
        values["backend_stopped"] = {"proxy_status": r["status"], "proxy_body": r["body"]}
        save("dependency-failures.json", values)
        assert r["status"] in (502, 504) and "nginx" in r["body"], values
        try:
            http("/health", base=DIRECT, timeout=3)
            raise AssertionError("stopped backend accepted direct request")
        except (urllib.error.URLError, ConnectionError):
            values["backend_stopped"]["direct"] = "connection refused"
    finally:
        dc("start", "backend")
        wait(lambda: http("/health", base=DIRECT)["status"] == 200, 120)
        wait(lambda: http("/health")["status"] == 200)
    meta = api("admin/gpt-auto/meta")
    assert meta["state_version"] == "state-v3" and not meta["live_routing_active"], meta
    infer()
    values["backend_recovered"] = True
    dependency_result = values
    save("dependency-failures.json", values)
    return values


def redis_continuity():
    assert dependency_result and dependency_result["redis_down_shadow_True"]["status"] == 200, {
        "requirement": "customer inference continues during whole Redis outage",
        "observed": dependency_result,
        "finding": "Existing gateway concurrency admission also requires Redis; do not silently fail open to pass this check.",
    }


def persistence():
    configure(True)
    control(jev="ok", upstream="ok")
    before = int(sql("SELECT coalesce(max(id),0) FROM gpt_auto_shadow_decisions"))
    infer(body=chat("UNIQUE_SYNTHETIC_PRIVACY_MARKER contact unique-synthetic@example.test"))
    wait(lambda: int(sql("SELECT coalesce(max(id),0) FROM gpt_auto_shadow_decisions")) > before)
    rows = decisions()
    row = api("admin/gpt-auto/decisions/" + str(rows[0]["id"]))
    ctx = row["features"]["context"]
    assert ctx["state_version"] == "state-v3"
    assert ctx["diagnostics"]["diagnostics_version"] == "context-diagnostics-v1"
    assert row["classifier_config_hash"]
    raw = sql("SELECT row_to_json(d)::text FROM gpt_auto_shadow_decisions d ORDER BY id DESC LIMIT 1")
    assert "UNIQUE_SYNTHETIC_PRIVACY_MARKER" not in raw and "unique-synthetic@example.test" not in raw
    save("stored-decision.json", row)
    save("overview.json", overview())
    return {"id": row["id"], "state_version": ctx["state_version"], "classifier_config_hash": row["classifier_config_hash"], "raw_prompt_persisted": False}


def performance():
    detail = {}
    for enabled in (False, True):
        configure(enabled)
        samples = [infer()["ms"] for _ in range(40)]
        samples.sort()
        detail[str(enabled)] = {"n": len(samples), "median_ms": statistics.median(samples), "p95_ms": samples[int(len(samples)*0.95)-1], "max_ms": max(samples)}
    wait(lambda: overview()["runtime"]["queue_length"] == 0)
    detail["runtime_after_drain"] = overview()["runtime"]
    (OUT / "docker-stats.txt").write_text(dc("stats", "--no-stream"))
    save("latency.json", detail)
    return detail


def billing_compatibility():
    # Compare the persisted usage for an identical billable non-streaming call.
    # Wait for older async usage writes before selecting the next usage ID.
    time.sleep(2)
    detail = {}
    for enabled in (False, True):
        configure(enabled)
        before = int(sql("SELECT coalesce(max(id),0) FROM usage_logs"))
        infer()
        def newest():
            raw = sql(f"SELECT row_to_json(t) FROM (SELECT account_id,model,requested_model,upstream_model,reasoning_effort,input_tokens,output_tokens,total_cost,actual_cost,rate_multiplier FROM usage_logs WHERE id>{before} ORDER BY id DESC LIMIT 1) t")
            return json.loads(raw) if raw else None
        detail[str(enabled)] = wait(newest, 20)
    assert detail["False"] == detail["True"], detail
    assert detail["True"]["actual_cost"] > 0, "fixture did not exercise billing"
    save("billing-comparison.json", detail)
    return detail


def oversized_requests():
    configure(True)
    control(jev="ok", upstream="ok")
    before = overview()["runtime"]["counters"]["skipped_too_large"]
    r = infer(body=chat("s" * (17 * 1024 * 1024)))
    after = overview()["runtime"]["counters"]["skipped_too_large"]
    assert after == before + 1, (before, after)
    rejection = {}
    for base in (BASE, DIRECT):
        rejected = http("/v1/chat/completions", chat("s" * (33 * 1024 * 1024)), auth=key, base=base)
        assert rejected["status"] == 413, rejected["status"]
        rejection[base] = {"status": rejected["status"], "body": rejected["body"][:500]}
    return {"17MiB_customer_status": r["status"], "shadow_scan_skipped": True, "over_32MiB": rejection}


def large_agent_session():
    items = [{"role": "user", "content": "Synthetic local task: migrate a bounded ledger service"}]
    for i in range(5000):
        items.extend([{"type": "function_call", "name": "shell", "call_id": f"c{i}", "arguments": '{"command":["go","test","./..."]}'},
                      {"type": "function_call_output", "call_id": f"c{i}", "output": "Exit code: 0\n" + "ok  pkg 0.1s\n" * 150}])
    body = json.dumps(responses(input=items), separators=(",", ":")).encode()
    result = {"bytes": len(body), "items": len(items), "measure": "end-to-end HTTP; use Go benchmark for extraction alone"}
    for enabled in (False, True):
        configure(enabled)
        samples = [infer("responses", body)["ms"] for _ in range(3)]
        result[str(enabled)] = {"n": 3, "median_ms": statistics.median(samples), "samples_ms": samples}
    wait(lambda: overview()["runtime"]["queue_length"] == 0 and overview()["runtime"]["active_calls"] == 0)
    raw = sql("SELECT features->'context' FROM gpt_auto_shadow_decisions WHERE endpoint='responses' ORDER BY id DESC LIMIT 1")
    context = json.loads(raw)
    assert context["items_total"] == len(items), context
    assert context["diagnostics"]["diagnostics_version"] == "context-diagnostics-v1"
    result["stored_context"] = context
    save("large-agent-http.json", result)
    return result


def main():
    full = len(sys.argv) > 1 and sys.argv[1] == "full"
    if not check("startup_assets_migrations", startup):
        return 1
    if not check("authentication_and_runtime_versions", authenticate):
        return 1
    if not check("supported_customer_setup", setup_customer):
        return 1
    check("admin_configuration_and_test_lab", admin_endpoints)
    check("context_extraction_and_diagnostics", debug_fixtures)
    if full:
        check("shadow_gateway_compatibility", gateway_compat)
        check("jev_failure_isolation", classification_failures)
        check("external_upstream_errors_and_timeout", upstream_errors)
        check("dependency_restart_and_proxy_502", dependency_failures)
        check("redis_outage_customer_continuity", redis_continuity)
        check("decision_versions_and_privacy", persistence)
        check("persisted_billing_and_account_compatibility", billing_compatibility)
        check("oversized_gateway_requests", oversized_requests)
        check("large_sampled_agent_session", large_agent_session)
        check("latency_and_queue_drain", performance)
    save("http-results.json", results)
    summary = {"scenarios": len(results), "passed": sum(x["passed"] for x in results),
               "failed": [x["name"] for x in results if not x["passed"]],
               "application_url": BASE, "project": "gpt-auto-local"}
    save("http-summary.json", summary)
    print(json.dumps(summary), flush=True)
    return 0 if all(x["passed"] for x in results) else 1


if __name__ == "__main__":
    sys.exit(main())
