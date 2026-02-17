// internal/notify/influx_adapter.go
package notify

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// InfluxAdapter emits notify events using manual HTTP Line Protocol.
// Compatible with InfluxDB 2 and 3 (via /api/v2/write).
//
// HARD CONSTRAINTS:
// - Emit MUST NOT block write path
// - MUST NEVER panic
// - Adapter failure must NOT affect write success
// - No retry logic
// - No batching optimizations
// - Timestamp = evt.Timestamp
// - Drop silently if queue full
type InfluxAdapter struct {
	endpoint    string
	token       string
	measurement string

	client *http.Client
	queue  chan Event
}

// NewInfluxAdapter constructs the adapter using existing config schema.
func NewInfluxAdapter(
	url string,
	org string,
	bucket string,
	token string,
	measurement string,
) *InfluxAdapter {

	m := strings.TrimSpace(measurement)
	if m == "" {
		m = "mma_write_events"
	}

	// Build endpoint internally from existing config fields
	base := strings.TrimRight(url, "/")
	endpoint := fmt.Sprintf(
		"%s/api/v2/write?org=%s&bucket=%s&precision=ns",
		base,
		org,
		bucket,
	)

	a := &InfluxAdapter{
		endpoint:    endpoint,
		token:       token,
		measurement: m,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
		queue: make(chan Event, 1024),
	}

	go a.writerLoop()
	return a
}

// Emit is non-blocking and panic-safe.
func (a *InfluxAdapter) Emit(evt Event) {
	if a == nil {
		return
	}

	defer func() { _ = recover() }()

	select {
	case a.queue <- evt:
	default:
		// Drop silently if queue full
	}
}

func (a *InfluxAdapter) writerLoop() {
	defer func() { _ = recover() }()

	for evt := range a.queue {
		lp := a.buildLineProtocol(evt)
		if lp == "" {
			continue
		}

		req, err := http.NewRequest("POST", a.endpoint, bytes.NewBufferString(lp))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "text/plain")

		if strings.TrimSpace(a.token) != "" {
			req.Header.Set("Authorization", "Token "+a.token)
		}

		resp, err := a.client.Do(req)
		if err != nil {
			continue
		}

		resp.Body.Close()
	}
}

func (a *InfluxAdapter) buildLineProtocol(evt Event) string {
	defer func() { _ = recover() }()

	if evt.Timestamp.IsZero() {
		return ""
	}

	tags := []string{
		"port=" + strconv.FormatUint(uint64(evt.Port), 10),
		"unit=" + strconv.FormatUint(uint64(evt.UnitID), 10),
		"area=" + evt.Area.YAMLKey(),
		"source=" + evt.Source.String(),
	}

	if ip := strings.TrimSpace(evt.SourceIP); ip != "" {
		tags = append(tags, "ip="+escapeTag(ip))
	}

	if evt.Name != nil {
		n := strings.TrimSpace(*evt.Name)
		if n != "" {
			tags = append(tags, "name="+escapeTag(n))
		}
	}

	fields := fmt.Sprintf(
		"start=%di,count=%di",
		int64(evt.Start),
		int64(evt.Count),
	)

	return fmt.Sprintf(
		"%s,%s %s %d",
		a.measurement,
		strings.Join(tags, ","),
		fields,
		evt.Timestamp.UnixNano(),
	)
}

func escapeTag(s string) string {
	s = strings.ReplaceAll(s, " ", "\\ ")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "=", "\\=")
	return s
}