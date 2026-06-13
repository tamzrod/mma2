package config

const defaultStateSealingExceptionCode uint8 = 0x06

const validStateSealingExceptionCodeList = "0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x08, 0x0A, 0x0B"

var validStateSealingExceptionCodes = map[uint8]struct{}{
	0x01: {},
	0x02: {},
	0x03: {},
	0x04: {},
	0x05: {},
	0x06: {},
	0x08: {},
	0x0A: {},
	0x0B: {},
}

func stateSealingEnabled(cfg *StateSealingConfig) bool {
	if cfg == nil {
		return false
	}
	if cfg.Enabled == nil {
		return true
	}
	return *cfg.Enabled
}

func stateSealingExceptionCode(cfg *StateSealingConfig) uint8 {
	if cfg == nil || cfg.Exception == nil {
		return defaultStateSealingExceptionCode
	}
	return *cfg.Exception
}

func isValidStateSealingExceptionCode(code uint8) bool {
	_, ok := validStateSealingExceptionCodes[code]
	return ok
}
