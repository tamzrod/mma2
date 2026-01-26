// internal/memorycore/memory.go
package memorycore

import (
	"encoding/binary"
	"sync"
)

type MemoryLayouts struct {
	Coils          *AreaLayout
	DiscreteInputs *AreaLayout
	HoldingRegs    *AreaLayout
	InputRegs      *AreaLayout
}

type Memory struct {
	mu sync.RWMutex

	coilsLayout          *AreaLayout
	discreteInputsLayout *AreaLayout
	holdingRegsLayout    *AreaLayout
	inputRegsLayout      *AreaLayout

	coilsBits          []byte
	discreteInputsBits []byte

	holdingRegs []uint16
	inputRegs   []uint16

	// ---- State Sealing metadata (no behavior here) ----
	stateSealing *StateSealingDef
}

func NewMemory(layouts MemoryLayouts) (*Memory, error) {
	m := &Memory{}

	if layouts.Coils != nil {
		if err := layouts.Coils.Validate(); err != nil {
			return nil, err
		}
		m.coilsLayout = layouts.Coils
		m.coilsBits = make([]byte, bytesForBits(layouts.Coils.Size))
	}

	if layouts.DiscreteInputs != nil {
		if err := layouts.DiscreteInputs.Validate(); err != nil {
			return nil, err
		}
		m.discreteInputsLayout = layouts.DiscreteInputs
		m.discreteInputsBits = make([]byte, bytesForBits(layouts.DiscreteInputs.Size))
	}

	if layouts.HoldingRegs != nil {
		if err := layouts.HoldingRegs.Validate(); err != nil {
			return nil, err
		}
		m.holdingRegsLayout = layouts.HoldingRegs
		m.holdingRegs = make([]uint16, layouts.HoldingRegs.Size)
	}

	if layouts.InputRegs != nil {
		if err := layouts.InputRegs.Validate(); err != nil {
			return nil, err
		}
		m.inputRegsLayout = layouts.InputRegs
		m.inputRegs = make([]uint16, layouts.InputRegs.Size)
	}

	return m, nil
}

func (m *Memory) ReadBits(area Area, address uint16, count uint16, dst []byte) error {
	if m == nil {
		return ErrNilMemory
	}
	if count == 0 {
		return ErrCountZero
	}

	var layout *AreaLayout
	var backing []byte

	switch area {
	case AreaCoils:
		layout = m.coilsLayout
		backing = m.coilsBits
	case AreaDiscreteInputs:
		layout = m.discreteInputsLayout
		backing = m.discreteInputsBits
	default:
		return ErrInvalidArea
	}

	if layout == nil {
		return ErrAreaNotDefined
	}
	if !layout.Contains(address, count) {
		return ErrOutOfBounds
	}

	want := bytesForBits(count)
	if len(dst) < want {
		return ErrDstTooSmall
	}

	off := layout.Offset(address)

	m.mu.RLock()
	copyBits(dst[:want], backing, off, count)
	m.mu.RUnlock()

	return nil
}

func (m *Memory) WriteBits(area Area, address uint16, count uint16, src []byte) error {
	if m == nil {
		return ErrNilMemory
	}
	if count == 0 {
		return ErrCountZero
	}

	var layout *AreaLayout
	var backing []byte

	switch area {
	case AreaCoils:
		layout = m.coilsLayout
		backing = m.coilsBits
	case AreaDiscreteInputs:
		layout = m.discreteInputsLayout
		backing = m.discreteInputsBits
	default:
		return ErrInvalidArea
	}

	if layout == nil {
		return ErrAreaNotDefined
	}
	if !layout.Contains(address, count) {
		return ErrOutOfBounds
	}

	want := bytesForBits(count)
	if len(src) < want {
		return ErrSrcTooSmall
	}

	off := layout.Offset(address)

	m.mu.Lock()
	writeBits(backing, off, count, src[:want])
	m.mu.Unlock()

	return nil
}

func (m *Memory) ReadRegs(area Area, address uint16, count uint16, dst []byte) error {
	if m == nil {
		return ErrNilMemory
	}
	if count == 0 {
		return ErrCountZero
	}

	want := int(count) * 2
	if len(dst) < want {
		return ErrDstTooSmall
	}

	var layout *AreaLayout
	var backing []uint16

	switch area {
	case AreaHoldingRegs:
		layout = m.holdingRegsLayout
		backing = m.holdingRegs
	case AreaInputRegs:
		layout = m.inputRegsLayout
		backing = m.inputRegs
	default:
		return ErrInvalidArea
	}

	if layout == nil {
		return ErrAreaNotDefined
	}
	if !layout.Contains(address, count) {
		return ErrOutOfBounds
	}

	off := layout.Offset(address)

	m.mu.RLock()
	for i := uint16(0); i < count; i++ {
		v := backing[int(off+i)]
		binary.BigEndian.PutUint16(dst[int(i)*2:int(i)*2+2], v)
	}
	m.mu.RUnlock()

	return nil
}

func (m *Memory) WriteRegs(area Area, address uint16, count uint16, src []byte) error {
	if m == nil {
		return ErrNilMemory
	}
	if count == 0 {
		return ErrCountZero
	}

	want := int(count) * 2
	if len(src) < want {
		return ErrSrcTooSmall
	}

	var layout *AreaLayout
	var backing []uint16

	switch area {
	case AreaHoldingRegs:
		layout = m.holdingRegsLayout
		backing = m.holdingRegs
	case AreaInputRegs:
		layout = m.inputRegsLayout
		backing = m.inputRegs
	default:
		return ErrInvalidArea
	}

	if layout == nil {
		return ErrAreaNotDefined
	}
	if !layout.Contains(address, count) {
		return ErrOutOfBounds
	}

	off := layout.Offset(address)

	m.mu.Lock()
	for i := uint16(0); i < count; i++ {
		v := binary.BigEndian.Uint16(src[int(i)*2 : int(i)*2+2])
		backing[int(off+i)] = v
	}
	m.mu.Unlock()

	return nil
}
