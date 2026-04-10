// internal/config/loader.go
package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"mma2/internal/version"
)

// utf8BOM is the three-byte UTF-8 byte order mark that some Windows editors
// prepend to text files.  The YAML spec does not require it and
// gopkg.in/yaml.v3 treats it as an invalid control-character sequence.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Load reads a YAML configuration file from disk
// and unmarshals it into a Config struct.
//
// It normalises the raw bytes before parsing so that config files produced
// on Windows (CRLF line endings, optional UTF-8 BOM) are accepted without
// error.  This is the most common cause of the "control characters are not
// allowed" error that appears when Docker containers are recreated from a
// Windows host or when the config is written via a shell heredoc/redirect.
//
// This function performs no validation beyond YAML parsing.
// Structural validation is handled separately.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	data = normaliseYAMLBytes(data)

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		if strings.Contains(err.Error(), "control characters are not allowed") {
			return nil, fmt.Errorf(
				"parse config yaml [mma2 v%s]: the config file contains a disallowed control character.\n"+
					"  Common causes: Windows CRLF line endings, UTF-8 BOM, null bytes, or\n"+
					"  other non-printable characters injected by editors or shell redirects.\n"+
					"  Fix: save the file with Unix (LF) line endings and no BOM, or let mma2\n"+
					"  normalise it automatically by ensuring no stray bytes remain.\n"+
					"  Original error: %w",
				version.Version, err,
			)
		}
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	return &cfg, nil
}

// normaliseYAMLBytes strips an optional UTF-8 BOM, converts Windows-style
// CRLF line endings to Unix LF, and removes any remaining bare control
// characters that gopkg.in/yaml.v3 rejects with "control characters are not
// allowed".
//
// Characters removed (beyond CRLF/CR):
//
//	0x00–0x08  NUL through BS
//	0x0B       vertical tab
//	0x0C       form feed
//	0x0E–0x1F  SO through US (all other C0 control chars except TAB/LF/CR)
//	0x7F       DEL
//
// TAB (0x09) and LF (0x0A) are preserved as valid YAML whitespace.
func normaliseYAMLBytes(data []byte) []byte {
	// Strip UTF-8 BOM if present.
	data = bytes.TrimPrefix(data, utf8BOM)

	// Replace CRLF with LF.  Do this before the lone-CR pass so that a
	// well-formed \r\n pair is collapsed to a single \n rather than two.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	// Replace any remaining bare CR (old Mac line endings) with LF.
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))

	// Strip any other control characters that YAML v3 does not allow.
	// Build the result in a single pass to keep allocations minimal.
	// data[:0:len(data)] creates a zero-length slice backed by the same
	// underlying array as data, so appending avoids an extra allocation
	// when the input contains no disallowed bytes.
	out := data[:0:len(data)]
	for _, b := range data {
		if isDisallowedControl(b) {
			continue
		}
		out = append(out, b)
	}

	return out
}

// isDisallowedControl reports whether b is a control character that
// gopkg.in/yaml.v3 rejects.  TAB (0x09) and LF (0x0A) are valid YAML
// whitespace and are therefore not considered disallowed.
func isDisallowedControl(b byte) bool {
	switch {
	case b <= 0x08: // NUL – BS
		return true
	case b == 0x0B, b == 0x0C: // VT, FF
		return true
	case b >= 0x0E && b <= 0x1F: // SO – US
		return true
	case b == 0x7F: // DEL
		return true
	}
	return false
}
