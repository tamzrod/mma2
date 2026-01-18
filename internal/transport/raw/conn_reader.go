// internal/rawingest/conn_reader.go
// PURPOSE: Read one complete Raw Ingest V1 frame from a stream (length-framed).
// ALLOWED: io reads, framing, size guards
// FORBIDDEN: memory access, logging, retries, semantics

package raw

import (
	"bufio"
	"io"
)

// frame is a validated, length-framed unit ready for resolve+apply.
type frame struct {
	Header  v1Header
	Payload []byte
}

// readFrame reads exactly one frame:
//   - reads fixed header (12 bytes)
//   - parses header
//   - computes payload length
//   - reads payload exactly
//
// It rejects any mismatch or ambiguity.
// maxPacketBytes is a hard cap for header+payload.
func readFrame(r *bufio.Reader, maxPacketBytes int) (frame, error) {
	if maxPacketBytes <= RawHeaderV1Size {
		return frame{}, ErrRejected
	}

	hdr := make([]byte, RawHeaderV1Size)
	if err := readExact(r, hdr); err != nil {
		return frame{}, ErrRejected
	}

	h, err := parseV1Header(hdr)
	if err != nil {
		return frame{}, ErrRejected
	}

	pLen, err := payloadSize(h.Area, h.Count)
	if err != nil {
		return frame{}, ErrRejected
	}

	total := RawHeaderV1Size + pLen
	if total > maxPacketBytes {
		return frame{}, ErrRejected
	}

	payload := make([]byte, pLen)
	if err := readExact(r, payload); err != nil {
		return frame{}, ErrRejected
	}

	return frame{
		Header:  h,
		Payload: payload,
	}, nil
}

// readExact reads exactly len(buf) bytes or fails.
func readExact(r *bufio.Reader, buf []byte) error {
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}
	return nil
}
