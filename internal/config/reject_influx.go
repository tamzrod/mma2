package config

import "fmt"

func rejectRemovedInflux(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	if cfg.Notify != nil && cfg.Notify.Influx != nil {
		return fmt.Errorf("notify.influx is not supported; notify events stay in-process (stdout adapter)")
	}
	if cfg.RBE != nil && cfg.RBE.Influx != nil {
		return fmt.Errorf("rbe.influx is not supported; subscribe to rbe.tcp from an external process")
	}
	return nil
}
