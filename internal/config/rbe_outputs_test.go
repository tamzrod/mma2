package config

import "testing"

func TestRBEInfluxOnlyIsSupported(t *testing.T) {
	cfg := validRBETestConfig()
	cfg.RBE.TCP = nil
	cfg.RBE.Influx = &NotifyInfluxConfig{
		URL: "http://127.0.0.1:8086", Token: "test", Org: "mma", Bucket: "events",
	}
	rules, err := BuildRBERules(cfg)
	if err != nil { t.Fatal(err) }
	if len(rules) != 1 || rules[0].ID != 1 { t.Fatalf("unexpected rules: %+v", rules) }
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
