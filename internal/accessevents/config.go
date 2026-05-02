// internal/accessevents/config.go
package accessevents

// AccessEventsConfig is the top-level configuration for the access event system.
// It maps to the access_events key in the YAML configuration file.
type AccessEventsConfig struct {
	Enabled        bool                     `yaml:"enabled"`
	Mode           string                   `yaml:"mode"`
	Window         int                      `yaml:"window"` // seconds
	KeyFields      []string                 `yaml:"key_fields"`
	IncludeCounter bool                     `yaml:"include_counter"`
	Limits         AccessEventsLimitsConfig `yaml:"limits"`
	Output         AccessEventsOutputConfig `yaml:"output"`
}

// AccessEventsLimitsConfig defines capacity bounds for the aggregation map.
type AccessEventsLimitsConfig struct {
	MaxKeys int `yaml:"max_keys"`
	TTL     int `yaml:"ttl"` // seconds
}

// AccessEventsOutputConfig defines the output transport configuration.
type AccessEventsOutputConfig struct {
	Type   string `yaml:"type"`
	Path   string `yaml:"path"`
	Listen string `yaml:"listen"` // TCP bind address for the HTTP server, e.g. ":8080"
}
