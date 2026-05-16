package config

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/m-szalik/goutils"
)

type Config struct {
	ExperimentName       string
	Engine               string
	RunID                string
	Namespace            string
	BrokerHostPort       string
	ListenAddr           string
	TargetGSByFsNode     map[string]string
	DeliveryDeadline     time.Duration
	PendingPutsMaxSize   int
	BridgeClientIDPrefix string
}

func FromEnv() (*Config, error) {
	c := &Config{
		ExperimentName:       goutils.Env("EXPERIMENT_NAME", ""),
		Engine:               goutils.Env("ENGINE", ""),
		RunID:                goutils.Env("RUN_ID", ""),
		Namespace:            goutils.Env("NAMESPACE", ""),
		BrokerHostPort:       goutils.Env("MESSAGING_BROKER_HOST_PORT", "messaging:1883"),
		ListenAddr:           goutils.Env("LISTEN_ADDR", ":9090"),
		BridgeClientIDPrefix: goutils.Env("BRIDGE_CLIENT_ID_PREFIX", "metrics-bridge"),
		PendingPutsMaxSize:   100000,
	}
	if c.ExperimentName == "" {
		return nil, fmt.Errorf("EXPERIMENT_NAME is required")
	}
	if c.Engine == "" {
		return nil, fmt.Errorf("ENGINE is required")
	}
	if c.RunID == "" {
		return nil, fmt.Errorf("RUN_ID is required")
	}
	raw := goutils.Env("TARGET_GS_BY_FSNODE", "")
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.TargetGSByFsNode); err != nil {
			return nil, fmt.Errorf("invalid TARGET_GS_BY_FSNODE JSON: %w", err)
		}
	}
	d, err := time.ParseDuration(goutils.Env("DELIVERY_DEADLINE", "2h"))
	if err != nil {
		return nil, fmt.Errorf("invalid DELIVERY_DEADLINE: %w", err)
	}
	c.DeliveryDeadline = d
	return c, nil
}

func (c *Config) TargetGSFor(satellite string) string {
	if c.TargetGSByFsNode == nil {
		return ""
	}
	return c.TargetGSByFsNode[satellite]
}
