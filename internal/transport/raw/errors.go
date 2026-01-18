// internal/transport/raw/errors.go
package raw

import "errors"

// ErrRejected is returned when a raw ingest frame
// is syntactically valid but rejected by policy or bounds.
var ErrRejected = errors.New("raw ingest rejected")
