#!/usr/bin/env python3
"""A stub of the ArtefactFlow endpoints this action calls.

It exists so the action can be exercised end to end — the container, the input
plumbing, and $GITHUB_OUTPUT — without an ArtefactFlow instance and without any
secret, which is what lets the smoke test run on pull requests from forks too.

It is deliberately strict about the things the action could get wrong silently:
the token endpoint checks the credentials, and the read endpoints reject a
request that does not present the minted token. A wrong bearer token fails here
rather than passing unnoticed.

    python3 testdata/stub-api.py [port]
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

CLIENT_ID = "507f191e810c19729de860ed"
CLIENT_SECRET = "afsa_stub-client-secret"
MINTED_TOKEN = "stub-minted-jwt"

ARTEFACT_ID = "0123456789abcdef01234567"
ARTEFACT_NAME = "acme-payment-service"

SUGGESTIONS = {
    "suggested_version": "1.2.4",
    "increments": {"major": "2.0.0", "minor": "1.3.0", "patch": "1.2.4"},
    "latest": "1.2.3",
    "pre_releases": {
        "alpha": "1.2.4-alpha.1",
        "beta": "1.2.4-beta.1",
        "rc": "1.2.4-rc.1",
    },
}


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        # One line per request, on stderr, without the noisy default prefix.
        sys.stderr.write("stub-api: " + fmt % args + "\n")

    def _send(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _error(self, status, code, message):
        self._send(status, {"code": code, "message": message, "logid": "stub-log-id"})

    def _authorised(self):
        # Only the token this stub minted is accepted. The action has no other way to
        # authenticate, so anything else means the credentials flow is broken.
        header = self.headers.get("Authorization", "")
        if header == f"Bearer {MINTED_TOKEN}":
            return True
        self._error(401, "AUTH00002", f"Unexpected Authorization header: {header!r}")
        return False

    def do_POST(self):
        path = self.path.split("?", 1)[0]
        if path != "/v1/auth/token":
            self._error(404, "GEN00404", "No such endpoint")
            return

        length = int(self.headers.get("Content-Length") or 0)
        try:
            body = json.loads(self.rfile.read(length) or b"{}")
        except json.JSONDecodeError:
            self._error(400, "GEN00001", "Malformed JSON body")
            return

        if body.get("client_id") != CLIENT_ID or body.get("client_secret") != CLIENT_SECRET:
            # The real API does not distinguish an unknown id from a wrong secret.
            self._error(401, "AUTH00001", "Invalid credentials")
            return

        self._send(200, {"access_token": MINTED_TOKEN, "expire_in": 300, "token_type": "Bearer"})

    def do_GET(self):
        path, _, query = self.path.partition("?")

        if path == "/v1/artefacts/resolve":
            if not self._authorised():
                return
            name = ""
            for pair in query.split("&"):
                key, _, value = pair.partition("=")
                if key == "name":
                    name = value
            if name != ARTEFACT_NAME:
                self._error(404, "ARTEFACT00509", "Artefact not found")
                return
            self._send(200, {"id": ARTEFACT_ID, "name": ARTEFACT_NAME})
            return

        if path == f"/v1/artefacts/{ARTEFACT_ID}/suggested-versions":
            if not self._authorised():
                return
            self._send(200, SUGGESTIONS)
            return

        self._error(404, "GEN00404", "No such endpoint")


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8080
    HTTPServer(("0.0.0.0", port), Handler).serve_forever()
