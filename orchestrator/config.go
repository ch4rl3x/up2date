package orchestrator

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	dockercollector "up2date/collector/docker"
	ospackagecollector "up2date/collector/ospackage"
	mqttpublisher "up2date/publisher/mqtt"
)

const defaultInterval = time.Minute

type Config struct {
	Node NodeConfig
	Job  JobConfig
}

type NodeConfig struct {
	ID string
}

type JobConfig struct {
	Name      string
	Interval  time.Duration
	Collector CollectorConfig
	Resolver  ResolverConfig
	Publisher PublisherConfig
}

type CollectorConfig struct {
	Type    string
	Docker  dockercollector.Config
	Package ospackagecollector.Config
}

type ResolverConfig struct {
	Type string
}

type PublisherConfig struct {
	Type string
	MQTT mqttpublisher.Config
}

func Load() (Config, error) {
	nodeID := strings.TrimSpace(os.Getenv("UP2DATE_NODE_ID"))
	if nodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return Config{}, fmt.Errorf("derive node id from hostname: %w", err)
		}
		nodeID = hostname
	}

	collectorType := strings.TrimSpace(os.Getenv("UP2DATE_COLLECTOR_TYPE"))
	if collectorType == "" {
		return Config{}, fmt.Errorf("UP2DATE_COLLECTOR_TYPE is required")
	}
	resolverType := strings.TrimSpace(os.Getenv("UP2DATE_RESOLVER_TYPE"))
	if resolverType == "" {
		resolverType = defaultResolverTypeForCollector(collectorType)
	}
	if resolverType == "" {
		return Config{}, fmt.Errorf("UP2DATE_RESOLVER_TYPE is required when collector %q has no default resolver", collectorType)
	}

	publisherType := strings.TrimSpace(os.Getenv("UP2DATE_PUBLISHER_TYPE"))
	if publisherType == "" {
		return Config{}, fmt.Errorf("UP2DATE_PUBLISHER_TYPE is required")
	}

	interval, err := loadDuration("UP2DATE_INTERVAL", defaultInterval)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Node: NodeConfig{
			ID: nodeID,
		},
		Job: JobConfig{
			Name:      firstNonEmpty(strings.TrimSpace(os.Getenv("UP2DATE_JOB_NAME")), collectorType),
			Interval:  interval,
			Collector: CollectorConfig{Type: collectorType},
			Resolver:  ResolverConfig{Type: resolverType},
			Publisher: PublisherConfig{Type: publisherType},
		},
	}

	switch collectorType {
	case "docker":
		includeStopped, err := loadOptionalBool("UP2DATE_COLLECTOR_DOCKER_INCLUDE_STOPPED")
		if err != nil {
			return Config{}, err
		}
		excludeSelf, err := loadOptionalBool("UP2DATE_COLLECTOR_DOCKER_EXCLUDE_SELF")
		if err != nil {
			return Config{}, err
		}

		cfg.Job.Collector.Docker = dockercollector.Config{
			IncludeStopped: includeStopped,
			ExcludeSelf:    excludeSelf,
			ExcludeLabels:  loadOptionalCSV("UP2DATE_COLLECTOR_DOCKER_EXCLUDE_LABELS"),
		}
	case "package":
		cfg.Job.Collector.Package = ospackagecollector.Config{
			Manager: strings.TrimSpace(os.Getenv("UP2DATE_COLLECTOR_PACKAGE_MANAGER")),
			Names:   loadOptionalCSV("UP2DATE_COLLECTOR_PACKAGE_NAMES"),
		}
	default:
		return Config{}, fmt.Errorf("unsupported collector type %q", collectorType)
	}

	switch resolverType {
	case "brew_formula":
	case "docker_hub":
	case "none":
	default:
		return Config{}, fmt.Errorf("unsupported resolver type %q", resolverType)
	}

	switch publisherType {
	case "mqtt":
		port, err := loadOptionalInt("UP2DATE_PUBLISHER_MQTT_PORT")
		if err != nil {
			return Config{}, err
		}
		retain, err := loadOptionalBool("UP2DATE_PUBLISHER_MQTT_RETAIN")
		if err != nil {
			return Config{}, err
		}

		cfg.Job.Publisher.MQTT = mqttpublisher.Config{
			Host:           strings.TrimSpace(os.Getenv("UP2DATE_PUBLISHER_MQTT_HOST")),
			Port:           port,
			Username:       strings.TrimSpace(os.Getenv("UP2DATE_PUBLISHER_MQTT_USERNAME")),
			Password:       os.Getenv("UP2DATE_PUBLISHER_MQTT_PASSWORD"),
			TopicPrefix:    strings.TrimSpace(os.Getenv("UP2DATE_PUBLISHER_MQTT_TOPIC_PREFIX")),
			ClientIDPrefix: strings.TrimSpace(os.Getenv("UP2DATE_PUBLISHER_MQTT_CLIENT_ID_PREFIX")),
			ConnectTimeout: strings.TrimSpace(os.Getenv("UP2DATE_PUBLISHER_MQTT_CONNECT_TIMEOUT")),
			Retain:         retain,
		}
	default:
		return Config{}, fmt.Errorf("unsupported publisher type %q", publisherType)
	}

	return cfg, nil
}

func loadDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s has invalid duration %q: %w", name, raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return value, nil
}

func loadOptionalInt(name string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s has invalid integer %q: %w", name, raw, err)
	}
	return value, nil
}

func loadOptionalBool(name string) (*bool, error) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return nil, nil
	}

	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "1", "true", "yes", "on":
		result := true
		return &result, nil
	case "0", "false", "no", "off":
		result := false
		return &result, nil
	default:
		return nil, fmt.Errorf("%s has invalid boolean %q", name, raw)
	}
}

func loadOptionalCSV(name string) []string {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultResolverTypeForCollector(collectorType string) string {
	switch collectorType {
	case "docker":
		return "docker_hub"
	case "package":
		return "none"
	default:
		return ""
	}
}
