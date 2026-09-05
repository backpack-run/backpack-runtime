from __future__ import annotations

import argparse
import json
import threading
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

PROTOCOL_VERSION = 1
ENGINE = "qwen-asr"
CAPABILITIES = ["transcription"]


class State:
    model = None
    error = None

    def load(self, directory: str) -> None:
        import torch
        from qwen_asr import Qwen3ASRModel

        self.model = Qwen3ASRModel.from_pretrained(
            str(Path(directory).resolve()), dtype=torch.float32, device_map="cpu",
            attn_implementation="eager", max_inference_batch_size=1, max_new_tokens=256,
        )

    def infer(self, payload: dict) -> dict:
        if self.model is None:
            raise RuntimeError("model is not loaded")
        audio = str(Path(payload["audio_path"]).resolve())
        results = self.model.transcribe(audio=audio, language=payload.get("language") or None)
        if not results:
            raise RuntimeError("runtime returned no transcription")
        return {"text": results[0].text.strip(), "language": results[0].language}

    def unload(self) -> None:
        self.model = None


def handler(state: State, server_ref: list):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            return

        def send_json(self, status: int, payload: dict) -> None:
            body = json.dumps(payload).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def payload(self) -> dict:
            length = int(self.headers.get("Content-Length", "0"))
            if length < 0 or length > 2 * 1024 * 1024:
                raise ValueError("invalid request size")
            return json.loads(self.rfile.read(length) or b"{}")

        def do_GET(self):
            if self.path == "/v1/health":
                self.send_json(200, {"protocol_version": PROTOCOL_VERSION, "engine": ENGINE,
                    "status": "ready", "capabilities": CAPABILITIES, "model_loaded": state.model is not None,
                    "error": state.error})
            elif self.path == "/v1/metadata":
                self.send_json(200, {"protocol_version": PROTOCOL_VERSION, "engine": ENGINE,
                    "capabilities": CAPABILITIES})
            else:
                self.send_json(404, {"error": "not found"})

        def do_POST(self):
            try:
                payload = self.payload()
                if self.path == "/v1/load":
                    state.load(payload["model_directory"])
                    self.send_json(200, {"loaded": True})
                elif self.path == "/v1/infer":
                    self.send_json(200, state.infer(payload))
                elif self.path == "/v1/unload":
                    state.unload()
                    self.send_json(200, {"loaded": False})
                elif self.path == "/v1/shutdown":
                    self.send_json(200, {"stopping": True})
                    threading.Thread(target=server_ref[0].shutdown, daemon=True).start()
                else:
                    self.send_json(404, {"error": "not found"})
            except Exception as exc:
                state.error = str(exc)
                self.send_json(HTTPStatus.BAD_REQUEST, {"error": str(exc)})
    return Handler


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", required=True, type=int)
    args = parser.parse_args()
    ref = [None]
    server = ThreadingHTTPServer((args.host, args.port), handler(State(), ref))
    ref[0] = server
    server.serve_forever()


if __name__ == "__main__":
    main()
