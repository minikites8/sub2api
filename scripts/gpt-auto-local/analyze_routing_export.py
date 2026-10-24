"""Offline analysis of a GPT Auto admin routing-log export (gpt-auto-export-v1 or v2; v2 appends pricing columns).

Usage:
    pip install pandas
    python analyze_routing_export.py gpt-auto-routing-admin-....csv
    python analyze_routing_export.py gpt-auto-routing-admin-....jsonl

Reads CSV (UTF-8 with BOM) or JSONL. Money columns are fixed-point strings
with 10 decimals and parse as floats; empty/null means "unknown", never zero.

Caveats printed with the results:
  * Shadow rows are hypothetical recommendations: the request ran on the
    customer's own model. est_* columns are estimates, and customer_charge_usd
    on a shadow row is the charge of the model that actually ran, not of the
    recommendation.
  * Agreement between shadow recommendations and live choices is not a quality
    measure; it only shows the classifier is consistent across modes.
"""
import json
import sys

import pandas as pd


def load(path: str) -> pd.DataFrame:
    if path.endswith(".jsonl"):
        frame = pd.read_json(path, lines=True, dtype=False)
    else:
        frame = pd.read_csv(path, encoding="utf-8-sig", keep_default_na=True, dtype=str)
    numeric = [
        "complexity", "model_confidence", "input_tokens", "cached_input_tokens", "output_tokens",
        "classifier_tokens", "classifier_cost_usd", "customer_charge_usd", "upstream_cost_usd",
        "gross_profit_usd", "gross_margin", "fully_loaded_profit_usd", "classifier_latency_ms",
        "gateway_latency_ms", "upstream_ttft_ms", "total_latency_ms", "retry_count",
    ]
    for column in numeric:
        if column in frame:
            frame[column] = pd.to_numeric(frame[column], errors="coerce")
    frame["timestamp"] = pd.to_datetime(frame["timestamp"], utc=True)
    schemas = set(frame["export_schema"].dropna())
    if schemas - {"gpt-auto-export-v1", "gpt-auto-export-v2"}:
        print(f"warning: unexpected export schema {schemas}", file=sys.stderr)
    return frame


def section(title: str, value) -> None:
    print(f"\n== {title}")
    print(value.to_string() if hasattr(value, "to_string") else value)


def main(path: str) -> None:
    df = load(path)
    live = df[df["mode"] == "live"]
    shadow = df[df["mode"] == "shadow"]
    executed = live[live["request_status"] == "succeeded"]

    section("Rows by mode", df["mode"].value_counts())
    section("Live: executed model distribution", executed["final_model"].value_counts(normalize=True).round(4))
    section("Live: reasoning effort distribution", executed["final_effort"].value_counts(normalize=True).round(4))
    classified = df[df["classifier_latency_ms"].notna()]
    section("Average classifier latency (ms) by mode", classified.groupby("mode")["classifier_latency_ms"].mean().round(1))
    section("Live P95 total request latency (ms)", round(live["total_latency_ms"].quantile(0.95), 1))
    section("Live P95 routing (gateway) latency (ms)", round(live["gateway_latency_ms"].quantile(0.95), 1))
    fallback = (live["routing_reason"] == "fallback") | (live["retry_count"].fillna(0) > 0)
    section("Live fallback/reroute rate", round(float(fallback.mean()) if len(live) else 0.0, 4))

    billed = executed[executed["billing_status"] == "final"]
    section("Live: average customer charge per executed model (USD, billed rows only)",
            billed.groupby("final_model")["customer_charge_usd"].mean().round(10))
    by_model = billed.groupby("final_model")[["customer_charge_usd", "upstream_cost_usd", "gross_profit_usd", "classifier_cost_usd"]].sum()
    by_model["model_gross_margin"] = (by_model["gross_profit_usd"] / by_model["customer_charge_usd"]).round(6)
    by_model["fully_loaded_margin"] = ((by_model["gross_profit_usd"] - by_model["classifier_cost_usd"]) / by_model["customer_charge_usd"]).round(6)
    section("Live: realised gross margin by model (negative = loss)", by_model)

    comparison = pd.DataFrame({
        "shadow_recommended": shadow[shadow["evaluation_status"] == "success"]["final_model"].value_counts(normalize=True),
        "live_executed": executed["final_model"].value_counts(normalize=True),
    }).fillna(0).round(4)
    section("Shadow recommendation vs live executed distribution (hypothetical vs actual)", comparison)

    drift = df[df["evaluation_status"] == "success"].groupby(["policy_version", "jev_model"]).size().unstack(fill_value=0)
    drift = drift.div(drift.sum(axis=1), axis=0).round(4)
    section("Classification drift: Jev model share per policy version", drift)
    if "classifier_config_hash" in df:
        per_hash = df[df["evaluation_status"] == "success"].groupby("classifier_config_hash")["jev_model"].value_counts(normalize=True).round(4)
        section("Jev model share per classifier configuration", per_hash)

    print("\nNotes: shadow est_* economics are hypothetical; live figures are billed usage. "
          "Recommendation agreement does not measure answer quality.")
    if path.endswith(".jsonl") and "model_probabilities" in df:
        sample = df["model_probabilities"].dropna().head(1).tolist()
        print("JSONL keeps nested probabilities, e.g.:", json.dumps(sample[0]) if sample else "none")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
