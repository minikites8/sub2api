"""Bounded, short-lived terminal responses for identical tool continuations."""
import json
import threading
import time
from collections import OrderedDict


class ResponseCache:
    def __init__(self, max_bytes=8 << 20, max_entries=128, ttl=900):
        self.max_bytes, self.max_entries, self.ttl = max_bytes, max_entries, ttl
        self.entries = OrderedDict()
        self.bytes = 0
        self.lock = threading.Lock()

    def remove(self, key):
        _, body = self.entries.pop(key)
        self.bytes -= len(body)

    def prune(self, now):
        for key, (expires, _) in list(self.entries.items()):
            if expires <= now:
                self.remove(key)

    def put(self, key, response):
        body = json.dumps(response, ensure_ascii=False, separators=(',', ':'), allow_nan=False).encode('utf-8')
        with self.lock:
            now = time.monotonic()
            self.prune(now)
            if key in self.entries:
                self.remove(key)
            if len(body) > self.max_bytes:
                return
            while self.entries and (self.bytes + len(body) > self.max_bytes or len(self.entries) >= self.max_entries):
                self.remove(next(iter(self.entries)))
            self.entries[key] = (now + self.ttl, body)
            self.bytes += len(body)

    def get(self, key):
        with self.lock:
            self.prune(time.monotonic())
            entry = self.entries.get(key)
            return json.loads(entry[1]) if entry else None
