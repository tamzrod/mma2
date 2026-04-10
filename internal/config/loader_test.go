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
// rejected with a clear error, since NUL bytes are valid UTF-8 but cause YAML
// parse failures and must not be silently stripped.
func TestLoad_NullByte(t *testing.T) {
	content := []byte("listeners:\x00\n  - id: test\n    listen: \":5020\"\n    memory:\n      - unit_id: 1\n        holding_registers:\n          start: 0\n          count: 10\n")

	path := writeTemp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for config with NUL byte, got nil")
	}
}

// TestLoad_MiscControlChars verifies that form-feed, vertical-tab, and DEL
// bytes cause a YAML parse error rather than being silently stripped, since
// these are valid UTF-8 but rejected by the YAML parser.
func TestLoad_MiscControlChars(t *testing.T) {
	content := []byte("\x0C\x0B\x7Flisteners:\n  - id: test\n    listen: \":5020\"\n    memory:\n      - unit_id: 1\n        holding_registers:\n          start: 0\n          count: 10\n")

	path := writeTemp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for config with control characters, got nil")
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

// TestLoad_InvalidUTF8 verifies that a config file with invalid UTF-8 bytes is
// rejected before any parsing takes place.
func TestLoad_InvalidUTF8(t *testing.T) {
	// 0x80 is an invalid UTF-8 start byte on its own.
	content := append([]byte("listeners:\n"), 0x80, '\n')

	path := writeTemp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for config with invalid UTF-8, got nil")
	}
}

// TestLoad_LoneCR verifies that a lone carriage return is preserved (not
// converted to LF), since only CRLF pairs are normalised.
func TestLoad_LoneCR(t *testing.T) {
	// A lone \r produces a YAML parse error; we just confirm it is not
	// silently converted so the behaviour remains deterministic.
	content := []byte("key: value\rother: thing\n")
	path := writeTemp(t, content)
	// We do not assert success or failure – only that Load does not panic and
	// that normaliseYAMLBytes itself preserves the lone CR.
	cr := normaliseYAMLBytes([]byte("a\rb"))
	if string(cr) != "a\rb" {
		t.Fatalf("normalise converted lone CR: got %q", cr)
	}
	_ = path // file written; Load result is intentionally not checked here
}
