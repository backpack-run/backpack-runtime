package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const maxCodexAppBodyBytes = int64(8 << 20)

// decodeCodexAppRequest handles the compressed request transport used by the
// desktop app. Decoding stays at this integration boundary so normal API
// clients and RuntimeAdapters continue to exchange ordinary JSON.
func decodeCodexAppRequest(request *http.Request) error {
	encoding := strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Encoding")))
	if encoding == "" || encoding == "identity" {
		return nil
	}
	if encoding != "zstd" {
		return fmt.Errorf("unsupported Codex App content encoding %q", encoding)
	}

	compressed, err := io.ReadAll(io.LimitReader(request.Body, maxCodexAppBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read compressed Codex App request: %w", err)
	}
	if int64(len(compressed)) > maxCodexAppBodyBytes {
		return fmt.Errorf("compressed Codex App request exceeds %d bytes", maxCodexAppBodyBytes)
	}
	decoder, err := zstd.NewReader(bytes.NewReader(compressed), zstd.WithDecoderMaxMemory(uint64(maxCodexAppBodyBytes)))
	if err != nil {
		return fmt.Errorf("decode Codex App zstd request: %w", err)
	}
	decompressed, err := io.ReadAll(io.LimitReader(decoder, maxCodexAppBodyBytes+1))
	decoder.Close()
	if err != nil {
		return fmt.Errorf("decompress Codex App request: %w", err)
	}
	if int64(len(decompressed)) > maxCodexAppBodyBytes {
		return fmt.Errorf("decompressed Codex App request exceeds %d bytes", maxCodexAppBodyBytes)
	}

	request.Body = io.NopCloser(bytes.NewReader(decompressed))
	request.ContentLength = int64(len(decompressed))
	request.Header.Del("Content-Encoding")
	request.Header.Set("Content-Type", "application/json")
	return nil
}
