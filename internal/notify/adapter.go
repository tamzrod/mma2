// internal/notify/adapter.go
package notify

import "log"

// Adapter defines the output boundary for notify events.
// It must NEVER block the write path.
type Adapter interface {
	Emit(Event)
}

// StdoutAdapter prints notify events to log output.
type StdoutAdapter struct{}

func NewStdoutAdapter() *StdoutAdapter {
	return &StdoutAdapter{}
}

func (a *StdoutAdapter) Emit(evt Event) {
	name := ""
	if evt.Name != nil {
		name = *evt.Name
	}

	log.Printf(
		"[NOTIFY] port=%d unit=%d area=%s start=%d count=%d name=%s source=%s ip=%s ts=%s",
		evt.Port,
		evt.UnitID,
		evt.Area.YAMLKey(),
		evt.Start,
		evt.Count,
		name,
		evt.Source.String(),
		evt.SourceIP,
		evt.Timestamp.Format("2006-01-02 15:04:05.000"),
	)
}