// internal/config/loader_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// minimalYAML is the smallest valid config accepted by Load + Validate.
// It uses a single listener with one memory unit so that the YAML parser
// and the config struct are both exercised.
const minimalYAML = `listeners:
  - id: test
    listen: ":5020"
    memory:
      - unit_id: 1
        holding_registers:
          start: 0
          count: 10
`

func writeTemp(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return filepath.Clean(f.Name())
}

func TestLoad_UnixLF(t *testing.T) {
	path := writeTemp(t, []byte(minimalYAML))
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Ingress) != 1 {
		t.Fatalf("expected 1 ingress gate, got %d", len(cfg.Ingress))
	}
}

// TestLoad_CRLF verifies that a config file with Windows-style CRLF line
// endings is accepted.  This is the primary scenario that causes the
// "control characters are not allowed" error when Docker containers are
// recreated from a Windows host.
func TestLoad_CRLF(t *testing.T) {
	crlf := []byte{}
	for _, line := range []string{
		"listeners:\r\n",
		"  - id: test\r\n",
		"    listen: \":5020\"\r\n",
		"    memory:\r\n",
		"      - unit_id: 1\r\n",
		"        holding_registers:\r\n",
		"          start: 0\r\n",
		"          count: 10\r\n",
	} {
		crlf = append(crlf, []byte(line)...)
	}

	path := writeTemp(t, crlf)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error with CRLF input: %v", err)
	}
	if len(cfg.Ingress) != 1 {
		t.Fatalf("expected 1 ingress gate, got %d", len(cfg.Ingress))
	}
}

// TestLoad_UTF8BOM verifies that a config file with a UTF-8 BOM prefix is
// accepted.  Some Windows editors (Notepad, some VS Code setups) prepend this
// three-byte sequence to every saved file.
func TestLoad_UTF8BOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	content := append(bom, []byte(minimalYAML)...)

	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error with UTF-8 BOM input: %v", err)
	}
	if len(cfg.Ingress) != 1 {
		t.Fatalf("expected 1 ingress gate, got %d", len(cfg.Ingress))
	}
}

// TestLoad_BOMandCRLF covers the worst-case combination: BOM + CRLF.
func TestLoad_BOMandCRLF(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	crlf := []byte(
		"listeners:\r\n" +
			"  - id: test\r\n" +
			"    listen: \":5020\"\r\n" +
			"    memory:\r\n" +
			"      - unit_id: 1\r\n" +
			"        holding_registers:\r\n" +
			"          start: 0\r\n" +
			"          count: 10\r\n",
	)
	content := append(bom, crlf...)

	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error with BOM+CRLF input: %v", err)
	}
	if len(cfg.Ingress) != 1 {
		t.Fatalf("expected 1 ingress gate, got %d", len(cfg.Ingress))
	}
}

// TestNormaliseYAMLBytes_IdempotentOnLF confirms that plain LF content is
// unchanged by normalisation.
func TestNormaliseYAMLBytes_IdempotentOnLF(t *testing.T) {
	input := []byte("key: value\nother: thing\n")
	got := normaliseYAMLBytes(input)
	if string(got) != string(input) {
		t.Fatalf("normalise changed LF-only input: got %q", got)
	}
}

// TestLoad_NullByte verifies that a config file with embedded NUL bytes is
// accepted after normalisation strips the disallowed control characters.
func TestLoad_NullByte(t *testing.T) {
	// Insert a NUL byte (0x00) into an otherwise valid YAML document.
	content := []byte("listeners:\x00\n  - id: test\n    listen: \":5020\"\n    memory:\n      - unit_id: 1\n        holding_registers:\n          start: 0\n          count: 10\n")

	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error with NUL byte in input: %v", err)
	}
	if len(cfg.Ingress) != 1 {
		t.Fatalf("expected 1 ingress gate, got %d", len(cfg.Ingress))
	}
}

// TestLoad_MiscControlChars verifies that form-feed, vertical-tab, and DEL
// bytes are stripped so that the YAML parser does not reject the file.
func TestLoad_MiscControlChars(t *testing.T) {
	// Inject form-feed (0x0C), vertical-tab (0x0B), and DEL (0x7F).
	content := []byte("\x0C\x0B\x7Flisteners:\n  - id: test\n    listen: \":5020\"\n    memory:\n      - unit_id: 1\n        holding_registers:\n          start: 0\n          count: 10\n")

	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error with misc control chars in input: %v", err)
	}
	if len(cfg.Ingress) != 1 {
		t.Fatalf("expected 1 ingress gate, got %d", len(cfg.Ingress))
	}
}

// TestNormaliseYAMLBytes_PreservesTabAndLF confirms that TAB and LF are kept
// intact (they are valid YAML whitespace and must not be removed).
func TestNormaliseYAMLBytes_PreservesTabAndLF(t *testing.T) {
	input := []byte("key:\tvalue\nother: thing\n")
	got := normaliseYAMLBytes(input)
	if string(got) != string(input) {
		t.Fatalf("normalise altered TAB/LF content: got %q", got)
	}
}

// TestIsDisallowedControl spot-checks the byte classifier.
func TestIsDisallowedControl(t *testing.T) {
	allowed := []byte{0x09 /* TAB */, 0x0A /* LF */, 0x20 /* SP */, 0x41 /* A */, 0xFF}
	for _, b := range allowed {
		if isDisallowedControl(b) {
			t.Errorf("byte 0x%02X should be allowed but was classified as disallowed", b)
		}
	}

	disallowed := []byte{0x00, 0x01, 0x08, 0x0B, 0x0C, 0x0E, 0x1F, 0x7F}
	for _, b := range disallowed {
		if !isDisallowedControl(b) {
			t.Errorf("byte 0x%02X should be disallowed but was classified as allowed", b)
		}
	}
}
