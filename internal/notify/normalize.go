// internal/notify/normalize.go
package notify

import "fmt"

// NormalizeRanges converts area-specific ranges into canonical NotifyRule entries.
// It performs only structural validation (range math + required fields).
//
// Validation (LOCKED for Stage 1):
// - count must be > 0
// - start+count-1 must not overflow 16-bit space
// - no memory layout bounds validation here (belongs to runtime write path)
func NormalizeRanges(port uint16, unitID uint16, area AreaType, ranges []RangeInput) ([]NotifyRule, error) {
	if len(ranges) == 0 {
		return nil, nil
	}

	out := make([]NotifyRule, 0, len(ranges))
	for i, rr := range ranges {
		if rr.Count == 0 {
			return nil, fmt.Errorf("%s.rules[%d]: count must be > 0", area.YAMLKey(), i)
		}

		// Compute end = start + count - 1 without overflow
		start := uint32(rr.Start)
		count := uint32(rr.Count)

		end := start + count - 1
		if end > 0xFFFF {
			return nil, fmt.Errorf(
				"%s.rules[%d]: start(%d)+count(%d) exceeds 16-bit address space",
				area.YAMLKey(),
				i,
				rr.Start,
				rr.Count,
			)
		}

		out = append(out, NotifyRule{
			Port:   port,
			UnitID: unitID,
			Area:   area,
			Start:  rr.Start,
			Count:  rr.Count,
			Name:   rr.Name,
		})
	}

	return out, nil
}

// RangeInput is a minimal input shape used by normalization.
// It intentionally mirrors config.NotifyRange but lives in notify to avoid config import cycles.
type RangeInput struct {
	Start uint16
	Count uint16
	Name  *string
}