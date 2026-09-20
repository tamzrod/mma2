// internal/transport/rawingest/decoder_response_test.go
package rawingest

import (
	"errors"
	"io"
	"testing"
)

func TestDecodeOneTruncatedFrames(t *testing.T) {
	validHeader := []byte{'R', 'I', Version1, 3, 0, 1, 0, 0, 0, 1}
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{name: "clean EOF", data: nil, want: io.EOF},
		{name: "partial header", data: validHeader[:5], want: ErrInvalidLength},
		{name: "missing payload", data: validHeader, want: ErrInvalidLength},
		{name: "partial payload", data: append(append([]byte(nil), validHeader...), 0x12), want: ErrInvalidLength},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeOne(&oneShotReader{data: tt.data}, 502)
			if !errors.Is(err, tt.want) {
				t.Fatalf("DecodeOne error = %v, want %v", err, tt.want)
			}
		})
	}
}

// oneShotReader returns the supplied bytes and then EOF.
type oneShotReader struct {
	data []byte
}

func (r *oneShotReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
