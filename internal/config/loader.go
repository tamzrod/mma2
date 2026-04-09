// internal/config/loader.go
package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
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
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	return &cfg, nil
}

// normaliseYAMLBytes strips an optional UTF-8 BOM and converts Windows-style
// CRLF line endings to Unix LF so that gopkg.in/yaml.v3 does not reject the
// input with "control characters are not allowed".
func normaliseYAMLBytes(data []byte) []byte {
	// Strip UTF-8 BOM if present.
	data = bytes.TrimPrefix(data, utf8BOM)

	// Replace CRLF with LF.  Do this before the lone-CR pass so that a
	// well-formed \r\n pair is collapsed to a single \n rather than two.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	// Replace any remaining bare CR (old Mac line endings) with LF.
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))

	return data
}
