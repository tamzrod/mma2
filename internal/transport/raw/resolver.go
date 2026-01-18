// internal/transport/raw/resolver.go
package raw

import "MMA2.0/internal/runtime"

// RuntimeResolver resolves memories from the MMA runtime.
type RuntimeResolver struct {
	rt *runtime.Runtime
}

func NewRuntimeResolver(rt *runtime.Runtime) *RuntimeResolver {
	return &RuntimeResolver{rt: rt}
}

// ResolveMemoryByID implements raw.MemoryResolver.
func (r *RuntimeResolver) ResolveMemoryByID(id uint16) (RawWritableMemory, bool) {
	for _, port := range r.rt.Ports {
		unit := port.Units[uint8(id)]
		if unit != nil {
			return &memoryAdapter{mem: unit.Memory}, true
		}
	}
	return nil, false
}
