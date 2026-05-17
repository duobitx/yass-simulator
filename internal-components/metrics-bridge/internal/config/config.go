package config

import (
	"encoding/json"
	"fmt"
	"strings"
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
	LokiURL              string
	LokiTenant           string
	ExporterBin          string
	ExportDir            string
	ExportGrace          time.Duration
	ExportLookback       time.Duration
	// K8sEventsSkipKinds lists Loki-event kinds that should NOT be mirrored
	// as Kubernetes Events on the Experiment CR. Comma-separated env
	// K8S_EVENTS_SKIP_KINDS; empty (default) mirrors every kind.
	K8sEventsSkipKinds []string
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
		LokiURL:              goutils.Env("LOKI_URL", "http://loki.yass-system.svc.cluster.local:3100"),
		LokiTenant:           goutils.Env("LOKI_TENANT", ""),
		ExporterBin:          goutils.Env("EXPORTER_BIN", "/events-exporter"),
		ExportDir:            goutils.Env("EXPORT_DIR", "/var/yass-observability/exports"),
	}
	graceStr := goutils.Env("EXPORT_GRACE", "10s")
	if g, err := time.ParseDuration(graceStr); err == nil {
		c.ExportGrace = g
	} else {
		c.ExportGrace = 10 * time.Second
	}
	lookbackStr := goutils.Env("EXPORT_LOOKBACK", "24h")
	if l, err := time.ParseDuration(lookbackStr); err == nil {
		c.ExportLookback = l
	} else {
		c.ExportLookback = 24 * time.Hour
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
	if raw := goutils.Env("K8S_EVENTS_SKIP_KINDS", ""); raw != "" {
		for _, k := range strings.Split(raw, ",") {
			if k = strings.TrimSpace(k); k != "" {
				c.K8sEventsSkipKinds = append(c.K8sEventsSkipKinds, k)
			}
		}
	}
	return c, nil
}

func (c *Config) TargetGSFor(satellite string) string {
	if c.TargetGSByFsNode == nil {
		return ""
	}
	return c.TargetGSByFsNode[satellite]
}
