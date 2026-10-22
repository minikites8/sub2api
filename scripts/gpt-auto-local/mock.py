"""Synthetic upstream + Jev. No credentials or prompt contents are logged."""
import json
import select
import socket
import ssl
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

lock = threading.Lock()
control = {"jev": "ok", "upstream": "ok", "delay": 2.0}
requests = []


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
                             "tools": len(data.get("tools", []))})
            del requests[:-2000]
        if self.path.endswith("/systemone"):
            if mode["jev"] == "delay":
                time.sleep(mode["delay"])
            if mode["jev"] in ("401", "403", "500"):
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
                                       "target_model": {"type": "choice", "choice": "gpt-6-luna", "confidence": 0.9, "probabilities": {"gpt-6-luna": 0.9, "gpt-5.6-luna": 0.1}},
                                       "reasoning_effort": {"type": "choice", "choice": "low", "confidence": 0.9},
                                       "task_complexity": {"type": "score", "score": 2, "confidence": 0.9},
                                       "task_category": {"type": "choice", "choice": "coding_debugging", "confidence": 0.9}}})
        if mode["upstream"] == "delay":
            time.sleep(mode["delay"])
        if mode["upstream"] in ("401", "429", "500", "502"):
            return self.reply(int(mode["upstream"]), {"error": {"message": "synthetic upstream failure", "type": "server_error", "code": "synthetic_upstream_error"}})
        model = data.get("model", "gpt-6.1-sol")
        is_chat = self.path.endswith("/chat/completions")
        tool = bool(data.get("tools")) and data.get("tool_choice") != "none"
        message = {"role": "assistant", "content": "synthetic local reply"}
        if tool:
            message.update(content=None, tool_calls=[{"id": "call_local", "type": "function", "function": {"name": "local_tool", "arguments": '{"value":1}'}}])
        usage = {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
        if is_chat:
            result = {"id": "chatcmpl-local", "object": "chat.completion", "created": 1, "model": model,
                      "choices": [{"index": 0, "message": message, "finish_reason": "tool_calls" if tool else "stop"}], "usage": usage}
        else:
            output = [{"id": "msg_local", "type": "message", "status": "completed", "role": "assistant", "content": [{"type": "output_text", "text": "synthetic local reply", "annotations": []}]}]
            if tool:
                output = [{"id": "fc_local", "type": "function_call", "call_id": "call_local", "name": "local_tool", "arguments": '{"value":1}', "status": "completed"}]
            result = {"id": "resp_local", "object": "response", "created_at": 1, "status": "completed", "model": model,
                      "output": output, "usage": {"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}
        if not data.get("stream"):
            return self.reply(200, result)
        events = []
        if is_chat:
            for delta, finish in [({"role": "assistant"}, None), ({k: v for k, v in message.items() if k != "role"}, None), ({}, "tool_calls" if tool else "stop")]:
                events.append((None, {"id": "chatcmpl-local", "object": "chat.completion.chunk", "created": 1, "model": model, "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}))
            events.append((None, "[DONE]"))
        else:
            events = [("response.created", {"type": "response.created", "sequence_number": 0, "response": {**result, "status": "in_progress", "output": []}}),
                      ("response.output_item.added", {"type": "response.output_item.added", "sequence_number": 1, "output_index": 0, "item": result["output"][0]}),
                      ("response.output_text.delta", {"type": "response.output_text.delta", "sequence_number": 2, "item_id": "msg_local", "output_index": 0, "content_index": 0, "delta": "synthetic local reply"}),
                      ("response.output_item.done", {"type": "response.output_item.done", "sequence_number": 3, "output_index": 0, "item": result["output"][0]}),
                      ("response.completed", {"type": "response.completed", "sequence_number": 4, "response": result})]
            if tool:
                events = [("response.created", {"type": "response.created", "sequence_number": 0, "response": {**result, "status": "in_progress", "output": []}}),
                          ("response.output_item.added", {"type": "response.output_item.added", "sequence_number": 1, "output_index": 0, "item": {**result["output"][0], "arguments": "", "status": "in_progress"}}),
                          ("response.function_call_arguments.delta", {"type": "response.function_call_arguments.delta", "sequence_number": 2, "item_id": "fc_local", "output_index": 0, "delta": '{"value":1}'}),
                          ("response.function_call_arguments.done", {"type": "response.function_call_arguments.done", "sequence_number": 3, "item_id": "fc_local", "output_index": 0, "arguments": '{"value":1}'}),
                          ("response.output_item.done", {"type": "response.output_item.done", "sequence_number": 4, "output_index": 0, "item": result["output"][0]}),
                          ("response.completed", {"type": "response.completed", "sequence_number": 5, "response": result})]
        raw = "".join((f"event: {event}\n" if event else "") + "data: " + (data if isinstance(data, str) else json.dumps(data)) + "\n\n" for event, data in events).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)
        self.wfile.flush()


threading.Thread(target=ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever, daemon=True).start()
tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
tls.load_cert_chain('/tls/mock.crt', '/tls/mock.key')
https = ThreadingHTTPServer(("0.0.0.0", 8443), Handler)
https.socket = tls.wrap_socket(https.socket, server_side=True)
https.serve_forever()
