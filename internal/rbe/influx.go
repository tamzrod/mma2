package rbe

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MultiSink fans out to independent, nonblocking sinks. The TCP publisher and
// Influx writer do not share a network queue or network I/O goroutine.
type MultiSink struct { Sinks []Sink }

func (m *MultiSink) Publish(id uint8) {
	if m == nil { return }
	for _, s := range m.Sinks {
		if s != nil { s.Publish(id) }
	}
}

// InfluxSink writes RBE event metadata only. It never serializes register
// values. Publish only enqueues; the HTTP call runs in a separate goroutine.
type InfluxSink struct {
	endpoint string
	token string
	measurement string
	rules map[uint8]Rule
	queue chan uint8
	client *http.Client
}

func NewInfluxSink(baseURL, org, bucket, token, measurement string, rules []Rule) (*InfluxSink, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("rbe influx: invalid URL %q", baseURL)
	}
	if strings.TrimSpace(org) == "" || strings.TrimSpace(bucket) == "" || strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("rbe influx: org, bucket and token are required")
	}
	if measurement == "" { measurement = "mma_rbe" }
	if strings.ContainsAny(measurement, " ,\r\n") {
		return nil, fmt.Errorf("rbe influx: invalid measurement name")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v2/write"
	q := u.Query()
	q.Set("org", org)
	q.Set("bucket", bucket)
	q.Set("precision", "ns")
	u.RawQuery = q.Encode()
	s := &InfluxSink{
		endpoint: u.String(), token: token, measurement: measurement,
		rules: make(map[uint8]Rule, len(rules)), queue: make(chan uint8, 1024),
		client: &http.Client{Timeout: 2 * time.Second},
	}
	for _, r := range rules { s.rules[r.ID] = r }
	go s.loop()
	return s, nil
}

func (s *InfluxSink) Publish(id uint8) {
	if s == nil || id == 0 { return }
	select {
	case s.queue <- id:
	default:
		// Observability may drop events; the PPC TCP path must not wait.
	}
}

func (s *InfluxSink) loop() {
	for id := range s.queue {
		r, ok := s.rules[id]
		if !ok { continue }
		ts := time.Now().UnixNano()
		lp := fmt.Sprintf("%s,rule_id=%d,name=%s,port=%d,unit=%d,area=%s start=%di,count=%di %d",
			s.measurement, r.ID, escapeTag(r.Name), r.Memory.Port, r.Memory.UnitID,
			r.Area.String(), r.Start, r.Count, ts)
		req, err := http.NewRequest(http.MethodPost, s.endpoint, bytes.NewBufferString(lp))
		if err != nil { continue }
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Authorization", "Token "+s.token)
		resp, err := s.client.Do(req)
		if err != nil { continue }
		_ = resp.Body.Close()
	}
}

func escapeTag(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, " ", "\\ ")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "=", "\\=")
	return s
}

// RuleIDString retains decimal formatting for diagnostics without inflating
// the one-byte TCP wire format.
func RuleIDString(id uint8) string { return strconv.FormatUint(uint64(id), 10) }
