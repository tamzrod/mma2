package config

import (
	"strings"
	"testing"
)

func validRBETestConfig() *Config {
	return &Config{
		RBE: &RBEOutputConfig{TCP: &RBETCPConfig{Listen: "127.0.0.1:9001"}},
		Ingress: []IngressGate{{ID: "lab", Listen: ":502", Memory: []MemoryDefinition{{
			UnitID: 1,
			InputRegs: Area{Start: 0, Count: 100},
			RBE: &RBERulesConfig{InputRegs: []RBERuleConfig{{ID: 1, Name: "Watched_IR_2_3", Start: 2, Count: 2}}},
		}}}},
	}
}

func TestBuildRBERulesValid(t *testing.T) {
	cfg := validRBETestConfig()
	rules, err := BuildRBERules(cfg)
	if err != nil { t.Fatal(err) }
	if len(rules) != 1 || rules[0].ID != 1 || rules[0].Memory.Port != 502 || rules[0].Memory.UnitID != 1 || rules[0].Count != 2 {
		t.Fatalf("unexpected rules: %+v", rules)
	}
}

func TestBuildRBERulesRejectsDuplicatesAndInvalidBounds(t *testing.T) {
	cases := []struct { name string; change func(*Config) }{
		{"zero ID", func(c *Config) { c.Ingress[0].Memory[0].RBE.InputRegs[0].ID = 0 }},
		{"over 255", func(c *Config) { c.Ingress[0].Memory[0].RBE.InputRegs[0].ID = 256 }},
		{"out of area", func(c *Config) { c.Ingress[0].Memory[0].RBE.InputRegs[0].Start = 99 }},
		{"zero count", func(c *Config) { c.Ingress[0].Memory[0].RBE.InputRegs[0].Count = 0 }},
		{"duplicate global ID", func(c *Config) {
			c.Ingress[0].Memory = append(c.Ingress[0].Memory, MemoryDefinition{UnitID: 2, InputRegs: Area{Count: 10}, RBE: &RBERulesConfig{InputRegs: []RBERuleConfig{{ID: 1, Name: "Other", Start: 0, Count: 1}}}})
		}},
		{"port collision", func(c *Config) { c.RBE.TCP.Listen = ":502" }},
		{"legacy mixed", func(c *Config) { c.Notify = &NotifyOutputConfig{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validRBETestConfig()
			tc.change(cfg)
			if _, err := BuildRBERules(cfg); err == nil { t.Fatal("invalid config accepted") }
		})
	}
}

// Rule names must stay on one line. Line breaks are rejected at startup.
func TestBuildRBERulesRejectsRuleNameWithLineBreak(t *testing.T) {
	for _, name := range []string{"Normal_Name", "with spaces", "a\nb", "a\rb"} {
		cfg := validRBETestConfig()
		cfg.Ingress[0].Memory[0].RBE.InputRegs[0].Name = name
		_, err := BuildRBERules(cfg)
		suspicious := strings.ContainsAny(name, "\r\n")
		if suspicious && err == nil {
			t.Fatalf("name %q accepted", name)
		}
		if !suspicious && err != nil {
			t.Fatalf("name %q rejected: %v", name, err)
		}
	}
}
