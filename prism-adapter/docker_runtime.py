"""Bootstrap the container's browser paths and gateway-managed bridge key."""

import argparse
import os
from pathlib import Path
import runpy
import time


def validate_key(key):
    if len(key) < 32 or any(character.isspace() or ord(character) < 32 for character in key):
        raise SystemExit("Prism bridge key must contain at least 32 characters without whitespace")
    return key


def wait_for_key(path, timeout=60):
    deadline = time.monotonic() + timeout
    while True:
        try:
            return validate_key(Path(path).read_text(encoding="utf-8").rstrip("\n"))
        except FileNotFoundError:
            if time.monotonic() >= deadline:
                raise SystemExit("Prism bridge key is missing; start the gateway with Prism enabled") from None
            time.sleep(0.2)
        except OSError:
            raise SystemExit("Prism bridge key is unreadable; check the shared data volume and UID 1000") from None


def browser_paths():
    from playwright.sync_api import sync_playwright

    chrome = os.environ.get("PRISM_ADAPTER_CHROME")
    if not chrome:
        with sync_playwright() as playwright:
            chrome = playwright.chromium.executable_path
    chrome = Path(chrome)
    if not chrome.is_file():
        raise SystemExit("Prism Chromium binary is missing from the adapter image")
    sandbox = os.environ.get("CHROME_DEVEL_SANDBOX")
    if sandbox:
        sandbox = Path(sandbox)
    else:
        sandbox = next((chrome.parent / name for name in ("chrome-sandbox", "chrome_sandbox")
                        if (chrome.parent / name).is_file()), None)
    if sandbox is None or not sandbox.is_file():
        raise SystemExit("Prism Chromium sandbox helper is missing from the adapter image")
    return chrome, sandbox


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--prepare-image", action="store_true")
    parser.add_argument("--check-browser", action="store_true")
    args = parser.parse_args()
    if not args.prepare_image and not args.check_browser and os.environ.get("PRISM_ADAPTER_ENABLED", "true").lower() in ("false", "f", "0"):
        from http.server import BaseHTTPRequestHandler, HTTPServer

        class DisabledHandler(BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(200 if self.path == "/health" else 503)
                self.end_headers()
                self.wfile.write(b'{"status":"disabled"}')

            def log_message(self, *arguments):
                pass

        print("Prism adapter disabled by deployment configuration", flush=True)
        HTTPServer(("127.0.0.1", int(os.environ.get("PRISM_ADAPTER_PORT", "8319"))), DisabledHandler).serve_forever()
        return
    chrome, sandbox = browser_paths()
    if args.prepare_image:
        os.chown(sandbox, 0, 0)
        sandbox.chmod(0o4755)
        return
    if os.geteuid() == 0:
        raise SystemExit("Prism adapter must run as a non-root user")
    os.environ["PRISM_ADAPTER_CHROME"] = str(chrome)
    os.environ["CHROME_DEVEL_SANDBOX"] = str(sandbox)
    if args.check_browser:
        from playwright.sync_api import sync_playwright

        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(executable_path=str(chrome), headless=True, chromium_sandbox=True)
            page = browser.new_page()
            page.goto("data:text/html,<title>Prism Docker smoke</title>")
            assert page.title() == "Prism Docker smoke"
            browser.close()
        print("Prism Chromium sandbox smoke passed", flush=True)
        return
    key = os.environ.get("PRISM_ADAPTER_API_KEY", "")
    os.environ["PRISM_ADAPTER_API_KEY"] = validate_key(key) if key else wait_for_key(
        os.environ.get("PRISM_ADAPTER_API_KEY_FILE", "/gateway-data/prism-adapter/bridge.key"))
    # Validate Chromium startup before reporting HTTP readiness.
    from playwright.sync_api import sync_playwright

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(executable_path=str(chrome), headless=True, chromium_sandbox=True)
        browser.close()
    print("Prism adapter ready to start on gateway loopback", flush=True)
    runpy.run_path(str(Path(__file__).with_name("server.py")), run_name="__main__")


if __name__ == "__main__":
    main()
