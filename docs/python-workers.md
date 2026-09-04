# Isolated Python workers (planned)

Python-backed adapters will use Backpack-owned, versioned environments rather than global Python or pip state. A small versioned loopback worker protocol will cover health, load, infer, progress, errors, unload, and graceful shutdown. Qwen ASR and Kokoro will share this mechanism; implementation follows validation of the native bundle boundary and Whisper adapter.

This document records the intended boundary only. No Python worker capability is currently claimed.
