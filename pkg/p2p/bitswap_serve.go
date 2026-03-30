package p2p

import (
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

// ServeContent registers a stream handler on the host for the content exchange
// protocol. When a remote peer requests content by CID, the handler looks up
// the content using the provided ContentServer and sends it back.
func (ce *ContentExchange) ServeContent(cas ContentServer) {
	ce.mu.Lock()
	ce.server = cas
	ce.mu.Unlock()

	ce.host.host.SetStreamHandler(contentProtocol, func(stream network.Stream) {
		defer stream.Close()
		ce.handleContentStream(stream)
	})
}

// handleContentStream processes a single incoming content request stream.
func (ce *ContentExchange) handleContentStream(stream network.Stream) {
	if err := stream.SetDeadline(time.Now().Add(streamDeadline)); err != nil {
		slog.Warn("content exchange: failed to set stream deadline", "error", err)
		return
	}

	cidBytes, err := readCIDFromStream(stream)
	if err != nil {
		slog.Debug("content exchange: failed to read CID from stream", "error", err)
		return
	}

	cidHex := hex.EncodeToString(cidBytes)

	ce.mu.RLock()
	srv := ce.server
	ce.mu.RUnlock()

	if srv == nil {
		slog.Warn("content exchange: no content server configured")
		ce.writeEmptyResponse(stream)
		return
	}

	source, found := srv(cidHex)
	if !found || source == nil || source.Reader == nil {
		ce.writeEmptyResponse(stream)
		return
	}
	defer source.Reader.Close()

	if err := writeContentResponse(stream, source.Reader, source.Size); err != nil {
		slog.Warn("content exchange: failed to write response", "cid", cidHex, "error", err)
	}
}

// readCIDFromStream reads the 4-byte length prefix and CID bytes from a stream.
func readCIDFromStream(stream network.Stream) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(stream, header[:]); err != nil {
		return nil, err
	}
	cidLen := uint32(header[0])<<24 | uint32(header[1])<<16 |
		uint32(header[2])<<8 | uint32(header[3])

	if cidLen == 0 || cidLen > 1024 {
		return nil, fmt.Errorf("invalid CID length: %d", cidLen)
	}

	cidBytes := make([]byte, cidLen)
	if _, err := io.ReadFull(stream, cidBytes); err != nil {
		return nil, err
	}

	return cidBytes, nil
}

// writeContentResponse sends a length-prefixed content response to a stream.
func writeContentResponse(w io.Writer, r io.Reader, size int64) error {
	if size <= 0 {
		return fmt.Errorf("invalid content size: %d", size)
	}
	if size > int64(maxContentSize) {
		return fmt.Errorf("content too large to serve: %d > %d", size, maxContentSize)
	}

	dataLen := uint32(size)
	respHeader := []byte{
		byte(dataLen >> 24),
		byte(dataLen >> 16),
		byte(dataLen >> 8),
		byte(dataLen),
	}
	if _, err := w.Write(respHeader); err != nil {
		return fmt.Errorf("writing response length: %w", err)
	}
	written, err := io.CopyN(w, r, size)
	if err != nil {
		return fmt.Errorf("writing response data: %w", err)
	}
	if written != size {
		return fmt.Errorf("short content write: wrote %d bytes, want %d", written, size)
	}
	return nil
}

// writeEmptyResponse sends a zero-length response to indicate "not found".
func (ce *ContentExchange) writeEmptyResponse(stream network.Stream) {
	zeros := []byte{0, 0, 0, 0}
	_, _ = stream.Write(zeros)
}
