package config

import "testing"

func TestRBEInfluxOnlyIsRejected(t *testing.T) {
	cfg := validRBETestConfig()
	cfg.RBE.TCP = nil
	cfg.RBE.Influx = &yamlRejectedInflux{}
	if _, err := BuildRBERules(cfg); err == nil {
		t.Fatal("influx-only RBE unexpectedly accepted")
	}
}

func TestRBERequiresTCP(t *testing.T) {
	cfg := validRBETestConfig()
	cfg.RBE.TCP = nil
	if _, err := BuildRBERules(cfg); err == nil {
		t.Fatal("RBE without tcp unexpectedly accepted")
	}
}

func TestRBEDisallowsLegacyNotifyOnOtherUnit(t *testing.T) {
	cfg := validRBETestConfig()
	cfg.Ingress[0].Memory = append(cfg.Ingress[0].Memory, MemoryDefinition{
		UnitID: 2, HoldingRegs: Area{Count: 8}, Notify: &NotifyConfig{},
	})
	if _, err := BuildRBERules(cfg); err == nil {
		t.Fatal("legacy notify unexpectedly accepted with global RBE")
	}
}

func TestLeftoverNotifyInfluxIsRejected(t *testing.T) {
	cfg := validRBETestConfig()
	cfg.RBE = nil
	cfg.Notify = &NotifyOutputConfig{Influx: &yamlRejectedInflux{}}
	if _, err := BuildRBERules(cfg); err == nil {
		t.Fatal("notify.influx unexpectedly accepted")
	}
}

func TestLeftoverRBEInfluxIsRejected(t *testing.T) {
	cfg := validRBETestConfig()
	cfg.RBE.Influx = &yamlRejectedInflux{}
	if _, err := BuildRBERules(cfg); err == nil {
		t.Fatal("rbe.influx unexpectedly accepted")
	}
}
