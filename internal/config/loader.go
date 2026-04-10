// internal/config/loader.go
package config

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"unicode/utf8"

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
	log.Printf("[mma2 v%s] loading config: %s", version.Version, path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	log.Printf("[mma2 v%s] config file byte length: %d", version.Version, len(data))
	if len(data) >= 32 {
		log.Printf("[mma2 v%s] first 32 bytes: %s", version.Version, hex.EncodeToString(data[:32]))
		log.Printf("[mma2 v%s] last  32 bytes: %s", version.Version, hex.EncodeToString(data[len(data)-32:]))
	} else {
		log.Printf("[mma2 v%s] all bytes (< 32): %s", version.Version, hex.EncodeToString(data))
	}

	validBefore := utf8.Valid(data)
	log.Printf("[mma2 v%s] UTF-8 valid before normalisation: %v", version.Version, validBefore)

	if !validBefore {
		return nil, fmt.Errorf("parse config yaml [mma2 v%s]: config is not valid UTF-8", version.Version)
	}

	data = normaliseYAMLBytes(data)

	validAfter := utf8.Valid(data)
	log.Printf("[mma2 v%s] UTF-8 valid after normalisation: %v", version.Version, validAfter)

	if !validAfter {
		return nil, fmt.Errorf("parse config yaml [mma2 v%s]: config became invalid UTF-8 after normalization", version.Version)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	return &cfg, nil
}

// normaliseYAMLBytes strips an optional UTF-8 BOM and converts Windows-style
// CRLF line endings to Unix LF.  No other byte transformations are performed so
// that the original byte integrity of valid UTF-8 content is preserved.
func normaliseYAMLBytes(data []byte) []byte {
	// Strip UTF-8 BOM if present.
	data = bytes.TrimPrefix(data, utf8BOM)

	// Replace CRLF with LF.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	return data
}
