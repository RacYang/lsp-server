package engine

import "time"

// TimeoutConfig 定义各等待态的服务端托管时长。
type TimeoutConfig struct {
	OpeningDefault  time.Duration
	OpeningByAction map[string]time.Duration
	ClaimWindow     time.Duration
	TsumoWindow     time.Duration
	Discard         time.Duration
	SurrenderAction time.Duration
}

// DefaultTimeoutConfig 返回 Phase 5 定时器默认值。
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		OpeningDefault:  15 * time.Second,
		ClaimWindow:     5 * time.Second,
		TsumoWindow:     15 * time.Second,
		Discard:         15 * time.Second,
		SurrenderAction: time.Second,
	}
}

// WithDefaults 用默认值填充未设定的字段，返回补全后的副本。
func (cfg TimeoutConfig) WithDefaults() TimeoutConfig {
	def := DefaultTimeoutConfig()
	hasOpeningActionOverrides := cfg.OpeningByAction != nil
	if cfg.OpeningDefault <= 0 {
		cfg.OpeningDefault = def.OpeningDefault
	}
	openingByAction := make(map[string]time.Duration)
	if !hasOpeningActionOverrides && cfg.OpeningDefault == def.OpeningDefault {
		for action, dur := range def.OpeningByAction {
			openingByAction[action] = dur
		}
	} else {
		for action, dur := range cfg.OpeningByAction {
			if action != "" && dur > 0 {
				openingByAction[action] = dur
			}
		}
	}
	cfg.OpeningByAction = openingByAction
	if cfg.ClaimWindow <= 0 {
		cfg.ClaimWindow = def.ClaimWindow
	}
	if cfg.TsumoWindow <= 0 {
		cfg.TsumoWindow = def.TsumoWindow
	}
	if cfg.Discard <= 0 {
		cfg.Discard = def.Discard
	}
	if cfg.SurrenderAction <= 0 {
		cfg.SurrenderAction = def.SurrenderAction
	}
	return cfg
}
