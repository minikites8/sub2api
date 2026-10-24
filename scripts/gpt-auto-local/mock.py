"""Synthetic upstream + Jev. No credentials or prompt contents are logged."""
import json
import select
import socket
import ssl
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

lock = threading.Lock()
# Phase 2 knobs are opt-in so Phase 1 checks see byte-identical responses:
# jev_model/jev_effort choose Jev's answer, unique_ids makes response ids
# distinct (needed for previous_response_id continuations), cached sets
# cached prompt tokens in upstream usage.
control = {"jev": "ok", "upstream": "ok", "delay": 2.0, "jev_model": "gpt-6-luna", "jev_effort": "low",
           "unique_ids": False, "cached": 0, "input_tokens": None, "output_tokens": None, "stream_delay": 0}
requests = []
counter = [0]


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

    def reply(self, status, data):
        raw = json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        try:
            self.wfile.write(raw)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def do_GET(self):
        if self.path == "/health":
            return self.reply(200, {"ok": True})
        if self.path == "/control":
            with lock:
                return self.reply(200, {"control": dict(control), "requests": list(requests)})
        return self.reply(404, {})

    def do_CONNECT(self):
        # Supported classifier proxy mode keeps the production DNS-rebinding
        # guard intact. This isolated proxy can reach exactly the local TLS mock.
        if self.path != 'mock:8443':
            return self.reply(403, {"error": "local fixture proxy target denied"})
        with socket.create_connection(('127.0.0.1', 8443), timeout=5) as upstream:
            self.send_response(200, 'Connection established')
            self.end_headers()
            self.close_connection = True
            peers = (self.connection, upstream)
            while True:
                ready, _, _ = select.select(peers, [], [], 10)
                if not ready:
                    return
                for peer in ready:
                    data = peer.recv(65536)
                    if not data:
                        return
                    (upstream if peer is self.connection else self.connection).sendall(data)

    def do_POST(self):
        raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        try:
            data = json.loads(raw)
        except ValueError:
            return self.reply(400, {"error": {"message": "invalid JSON", "type": "invalid_request_error"}})
        if self.path == "/control":
            with lock:
                for k in control:
                    if k in data:
                        control[k] = data[k]
                if data.get("reset"):
                    requests.clear()
            return self.reply(200, {"ok": True})
        with lock:
            mode = dict(control)
            requests.append({"path": self.path, "model": data.get("model"),
                             "effort": data.get("reasoning_effort", data.get("reasoning")),
                             "stream": data.get("stream", False), "bytes": len(raw),
                             "tools": len(data.get("tools", [])),
                             "previous_response_id": data.get("previous_response_id")})
            counter[0] += 1
            serial = counter[0]
            del requests[:-2000]
        if self.path.endswith("/systemone"):
            if mode["jev"] == "delay":
                time.sleep(mode["delay"])
            if mode["jev"] in ("401", "403", "429", "500"):
                return self.reply(int(mode["jev"]), {"error": "synthetic Jev error"})
            if mode["jev"] == "malformed":
                self.send_response(200)
                self.send_header("Content-Length", "8")
                self.end_headers()
                self.wfile.write(b"not-json")
                return
            if mode["jev"] == "incomplete":
                return self.reply(200, {"model": "jev-local", "answers": {}})
            return self.reply(200, {"model": "jev-local", "usage": {"input_tokens": 200, "output_tokens": 20},
                                   "answers": {
                                       "target_model": {"type": "choice", "choice": mode["jev_model"], "confidence": 0.9, "probabilities": {mode["jev_model"]: 0.9}},
                                       "reasoning_effort": {"type": "choice", "choice": mode["jev_effort"], "confidence": 0.9},
                                       "task_complexity": {"type": "score", "score": 2, "confidence": 0.9},
                                       "task_category": {"type": "choice", "choice": "coding_debugging", "confidence": 0.9}}})
        if mode["upstream"] == "delay":
            time.sleep(mode["delay"])
        if mode["upstream"] in ("401", "429", "500", "502"):
            return self.reply(int(mode["upstream"]), {"error": {"message": "synthetic upstream failure", "type": "server_error", "code": "synthetic_upstream_error"}})
        model = data.get("model", "gpt-6.1-sol")
        is_chat = self.path.endswith("/chat/completions")
        tool = bool(data.get("tools")) and data.get("tool_choice") != "none"
        hosted_search = tool and any(t.get("type") in ("web_search", "web_search_preview") for t in data.get("tools", []))
        message = {"role": "assistant", "content": "synthetic local reply"}
        if tool:
            message.update(content=None, tool_calls=[{"id": "call_local", "type": "function", "function": {"name": "local_tool", "arguments": '{"value":1}'}}])
        usage = {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
        response_id = f"resp_local_{serial}" if mode["unique_ids"] else "resp_local"
        if mode["cached"]:
            usage = {"prompt_tokens": 100, "completion_tokens": 5, "total_tokens": 105, "prompt_tokens_details": {"cached_tokens": mode["cached"]}}
        if is_chat:
            result = {"id": "chatcmpl-local", "object": "chat.completion", "created": 1, "model": model,
                      "choices": [{"index": 0, "message": message, "finish_reason": "tool_calls" if tool else "stop"}], "usage": usage}
        else:
            output = [{"id": "msg_local", "type": "message", "status": "completed", "role": "assistant", "content": [{"type": "output_text", "text": "synthetic local reply", "annotations": []}]}]
            if tool:
                output = [{"id": "fc_local", "type": "function_call", "call_id": "call_local", "name": "local_tool", "arguments": '{"value":1}', "status": "completed"}]
            if hosted_search:
                output = [{"id": "ws_local", "type": "web_search_call", "status": "completed", "action": {"type": "search", "query": "local fixture"}}]
            result = {"id": response_id, "object": "response", "created_at": 1, "status": "completed", "model": model,
                      "output": output, "usage": {"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}
            if mode["cached"]:
                result["usage"] = {"input_tokens": 100, "output_tokens": 5, "total_tokens": 105, "input_tokens_details": {"cached_tokens": mode["cached"]}}
        if mode["input_tokens"] is not None:
            total, out = int(mode["input_tokens"]), int(mode["output_tokens"] or 0)
            result["usage"] = ({"prompt_tokens": total, "completion_tokens": out, "total_tokens": total+out, "prompt_tokens_details": {"cached_tokens": mode["cached"]}, "completion_tokens_details": {"reasoning_tokens": min(out, 100)}} if is_chat else {"input_tokens": total, "output_tokens": out, "total_tokens": total+out, "input_tokens_details": {"cached_tokens": mode["cached"]}, "output_tokens_details": {"reasoning_tokens": min(out, 100)}})
        if not data.get("stream"):
            return self.reply(200, result)
        events = []
        if is_chat:
            for delta, finish in [({"role": "assistant"}, None), ({k: v for k, v in message.items() if k != "role"}, None), ({}, "tool_calls" if tool else "stop")]:
                events.append((None, {"id": "chatcmpl-local", "object": "chat.completion.chunk", "created": 1, "model": model, "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}))
            events.append((None, {"id":"chatcmpl-local", "object":"chat.completion.chunk", "choices":[], "usage":result["usage"]}))
            events.append((None, "[DONE]"))
        else:
            events = [("response.created", {"type": "response.created", "sequence_number": 0, "response": {**result, "status": "in_progress", "output": []}}),
                      ("response.output_item.added", {"type": "response.output_item.added", "sequence_number": 1, "output_index": 0, "item": result["output"][0]}),
                      ("response.output_text.delta", {"type": "response.output_text.delta", "sequence_number": 2, "item_id": "msg_local", "output_index": 0, "content_index": 0, "delta": "synthetic local reply"}),
                      ("response.output_item.done", {"type": "response.output_item.done", "sequence_number": 3, "output_index": 0, "item": result["output"][0]}),
                      ("response.completed", {"type": "response.completed", "sequence_number": 4, "response": result})]
            if tool and not hosted_search:
                events = [("response.created", {"type": "response.created", "sequence_number": 0, "response": {**result, "status": "in_progress", "output": []}}),
                          ("response.output_item.added", {"type": "response.output_item.added", "sequence_number": 1, "output_index": 0, "item": {**result["output"][0], "arguments": "", "status": "in_progress"}}),
                          ("response.function_call_arguments.delta", {"type": "response.function_call_arguments.delta", "sequence_number": 2, "item_id": "fc_local", "output_index": 0, "delta": '{"value":1}'}),
                          ("response.function_call_arguments.done", {"type": "response.function_call_arguments.done", "sequence_number": 3, "item_id": "fc_local", "output_index": 0, "arguments": '{"value":1}'}),
                          ("response.output_item.done", {"type": "response.output_item.done", "sequence_number": 4, "output_index": 0, "item": result["output"][0]}),
                          ("response.completed", {"type": "response.completed", "sequence_number": 5, "response": result})]
        if mode["upstream"] == "partial" and not is_chat:
            events[-1] = ("response.failed", {"type":"response.failed", "sequence_number":4, "response": {**result, "status":"failed", "error":{"code":"server_error", "message":"synthetic partial failure"}}})
        raw = "".join((f"event: {event}\n" if event else "") + "data: " + (data if isinstance(data, str) else json.dumps(data)) + "\n\n" for event, data in events).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        if mode["stream_delay"]:
            split = raw.find(b"\n\n") + 2
            self.wfile.write(raw[:split]); self.wfile.flush()
            time.sleep(float(mode["stream_delay"]))
            raw = raw[split:]
        try:
            self.wfile.write(raw)
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


threading.Thread(target=ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever, daemon=True).start()
tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
tls.load_cert_chain('/tls/mock.crt', '/tls/mock.key')
https = ThreadingHTTPServer(("0.0.0.0", 8443), Handler)
https.socket = tls.wrap_socket(https.socket, server_side=True)
https.serve_forever()
