from __future__ import annotations

import argparse
import base64
import io
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

PROTOCOL_VERSION = 1
ENGINE = "kokoro"
CAPABILITIES = ["speech"]


class State:
    pipelines = None
    voices = None
    error = None

    def load(self, directory: str) -> None:
        import torch
        from kokoro import KModel, KPipeline

        root = Path(directory).resolve()
        model = KModel(repo_id="backpack-run/Kokoro-82M-Backpack-TTS",
                       config=str(root / "config.json"), model=str(root / "kokoro-v1_0.pth")).to("cpu").eval()
        self.pipelines = {"a": KPipeline(lang_code="a", repo_id=None, model=model),
                          "b": KPipeline(lang_code="b", repo_id=None, model=model)}
        self.voices = {
            name: torch.load(root / "voices" / f"{name}.pt", map_location="cpu", weights_only=True)
            for name in ("af_heart", "am_michael", "bf_emma", "bm_george")
        }

    def infer(self, payload: dict) -> dict:
        import numpy as np
        import soundfile as sf

        if self.pipelines is None:
            raise RuntimeError("model is not loaded")
        text = str(payload.get("input", "")).strip()[:12000]
        voice = str(payload.get("voice") or "af_heart")
        speed = float(payload.get("speed") or 1.0)
        if not text or voice not in self.voices or not 0.5 <= speed <= 2.0:
            raise ValueError("invalid synthesis request")
        pipeline = self.pipelines["b" if voice.startswith("b") else "a"]
        chunks = [audio.numpy() for _, _, audio in pipeline(text, voice=self.voices[voice], speed=speed)]
        if not chunks:
            raise RuntimeError("Kokoro produced no audio")
        output = io.BytesIO()
        sf.write(output, np.concatenate(chunks), 24000, format="WAV", subtype="PCM_16")
        return {"audio_base64": base64.b64encode(output.getvalue()).decode("ascii"),
                "format": "wav", "sample_rate": 24000}

    def unload(self) -> None:
        self.pipelines = None
        self.voices = None


def handler(state: State, server_ref: list):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args): return
        def send_json(self, status, payload):
            body = json.dumps(payload).encode("utf-8")
            self.send_response(status); self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)
        def payload(self):
            length = int(self.headers.get("Content-Length", "0"))
            if length < 0 or length > 256 * 1024: raise ValueError("invalid request size")
            return json.loads(self.rfile.read(length) or b"{}")
        def do_GET(self):
            if self.path == "/v1/health":
                self.send_json(200, {"protocol_version": PROTOCOL_VERSION, "engine": ENGINE, "status": "ready",
                    "capabilities": CAPABILITIES, "model_loaded": state.pipelines is not None, "error": state.error})
            elif self.path == "/v1/metadata":
                self.send_json(200, {"protocol_version": PROTOCOL_VERSION, "engine": ENGINE, "capabilities": CAPABILITIES,
                    "voices": ["af_heart", "am_michael", "bf_emma", "bm_george"], "formats": ["wav"]})
            else: self.send_json(404, {"error": "not found"})
        def do_POST(self):
            try:
                payload = self.payload()
                if self.path == "/v1/load": state.load(payload["model_directory"]); self.send_json(200, {"loaded": True})
                elif self.path == "/v1/infer": self.send_json(200, state.infer(payload))
                elif self.path == "/v1/unload": state.unload(); self.send_json(200, {"loaded": False})
                elif self.path == "/v1/shutdown":
                    self.send_json(200, {"stopping": True}); threading.Thread(target=server_ref[0].shutdown, daemon=True).start()
                else: self.send_json(404, {"error": "not found"})
            except Exception as exc:
                state.error = str(exc); self.send_json(400, {"error": str(exc)})
    return Handler


def main():
    parser = argparse.ArgumentParser(); parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", required=True, type=int); args = parser.parse_args()
    ref = [None]; server = ThreadingHTTPServer((args.host, args.port), handler(State(), ref)); ref[0] = server
    server.serve_forever()


if __name__ == "__main__": main()
