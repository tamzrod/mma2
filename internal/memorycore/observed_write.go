package memorycore

import "encoding/binary"

// BitProbe selects one ordinary memory bit to sample immediately before the
// write, under the SAME memory lock. This is a neutral atomic memory primitive:
// it knows nothing about state sealing, RBE, rules, or output adapters.
type BitProbe struct {
	Area    Area
	Address uint16
}

// WriteObservation reports raw pre-write state for the committed write.
// Previous is packed LSB-first for bits or big-endian uint16 for registers.
// ProbeBefore is meaningful only when a probe was supplied.
type WriteObservation struct {
	Previous    []byte
	ProbeBefore bool
}

func (m *Memory) readProbeLocked(probe *BitProbe) (bool, error) {
	if probe == nil {
		return false, nil
	}
	var layout *AreaLayout
	var backing []byte
	switch probe.Area {
	case AreaCoils:
		layout, backing = m.coilsLayout, m.coilsBits
	case AreaDiscreteInputs:
		layout, backing = m.discreteInputsLayout, m.discreteInputsBits
	default:
		return false, ErrInvalidArea
	}
	if layout == nil {
		return false, ErrAreaNotDefined
	}
	if !layout.Contains(probe.Address, 1) {
		return false, ErrOutOfBounds
	}
	off := layout.Offset(probe.Address)
	return backing[int(off)/8]&(1<<(off%8)) != 0, nil
}

// WriteBitsObserved commits a write and returns the exact state it overwrote.
// The optional probe is sampled under the same lock, before the write.
// Unused high bits of the final source byte are never written or observed.
func (m *Memory) WriteBitsObserved(area Area, address, count uint16, src []byte, probe *BitProbe) (WriteObservation, error) {
	var result WriteObservation
	if m == nil {
		return result, ErrNilMemory
	}
	if count == 0 {
		return result, ErrCountZero
	}
	var layout *AreaLayout
	var backing []byte
	switch area {
	case AreaCoils:
		layout, backing = m.coilsLayout, m.coilsBits
	case AreaDiscreteInputs:
		layout, backing = m.discreteInputsLayout, m.discreteInputsBits
	default:
		return result, ErrInvalidArea
	}
	if layout == nil {
		return result, ErrAreaNotDefined
	}
	if !layout.Contains(address, count) {
		return result, ErrOutOfBounds
	}
	want := bytesForBits(count)
	if len(src) != want {
		return result, ErrSrcInvalidLength
	}
	result.Previous = make([]byte, want)
	off := layout.Offset(address)
	m.mu.Lock()
	defer m.mu.Unlock()
	before, err := m.readProbeLocked(probe)
	if err != nil {
		return WriteObservation{}, err
	}
	result.ProbeBefore = before
	copyBits(result.Previous, backing, off, count)
	writeBits(backing, off, count, src)
	return result, nil
}

// WriteRegsObserved is the register equivalent of WriteBitsObserved. It
// validates all inputs before mutating and performs probe, copy, and commit
// atomically under one lock.
func (m *Memory) WriteRegsObserved(area Area, address, count uint16, src []byte, probe *BitProbe) (WriteObservation, error) {
	var result WriteObservation
	if m == nil {
		return result, ErrNilMemory
	}
	if count == 0 {
		return result, ErrCountZero
	}
	var layout *AreaLayout
	var backing []uint16
	switch area {
	case AreaHoldingRegs:
		layout, backing = m.holdingRegsLayout, m.holdingRegs
	case AreaInputRegs:
		layout, backing = m.inputRegsLayout, m.inputRegs
	default:
		return result, ErrInvalidArea
	}
	if layout == nil {
		return result, ErrAreaNotDefined
	}
	if !layout.Contains(address, count) {
		return result, ErrOutOfBounds
	}
	want := int(count) * 2
	if len(src) != want {
		return result, ErrSrcInvalidLength
	}
	result.Previous = make([]byte, want)
	off := layout.Offset(address)
	m.mu.Lock()
	defer m.mu.Unlock()
	before, err := m.readProbeLocked(probe)
	if err != nil {
		return WriteObservation{}, err
	}
	result.ProbeBefore = before
	for i := 0; i < int(count); i++ {
		index := int(off) + i
		binary.BigEndian.PutUint16(result.Previous[i*2:i*2+2], backing[index])
		backing[index] = binary.BigEndian.Uint16(src[i*2 : i*2+2])
	}
	return result, nil
}
