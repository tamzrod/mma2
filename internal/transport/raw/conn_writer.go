// internal/rawingest/conn_writer.go
// PURPOSE: Write single-byte Raw Ingest responses.
// ALLOWED: io writes only
// FORBIDDEN: logging, retries, semantics

package raw

import "io"

func writeOK(w io.Writer) error {
	_, err := w.Write([]byte{ResponseOK})
	return err
}

func writeRejected(w io.Writer) error {
	_, err := w.Write([]byte{ResponseRejected})
	return err
}
