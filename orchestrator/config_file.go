package orchestrator

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	dockercollector "up2date/collector/docker"
	ospackagecollector "up2date/collector/ospackage"
	mqttpublisher "up2date/publisher/mqtt"
)

type configLine struct {
	number int
	indent int
	text   string
}

func LoadFromFile(path string) (Config, error) {
	document, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %q: %w", path, err)
	}

	root, err := parseConfigDocument(document)
	if err != nil {
		return Config{}, fmt.Errorf("parse config file %q: %w", path, err)
	}

	return buildConfigFromDocument(root)
}

func buildConfigFromDocument(root map[string]any) (Config, error) {
	if _, ok := root["resolver"]; ok {
		return Config{}, fmt.Errorf("resolver is not configurable in file configs; it is derived automatically from the collector")
	}
	if err := validateKeys(root, "config", "collector", "interval", "job_name", "node_id", "publisher"); err != nil {
		return Config{}, err
	}

	nodeID, ok, err := optionalStringValue(root, "node_id", "config.node_id")
	if err != nil {
		return Config{}, err
	}
	if !ok || nodeID == "" {
		hostname, hostnameErr := os.Hostname()
		if hostnameErr != nil {
			return Config{}, fmt.Errorf("derive node id from hostname: %w", hostnameErr)
		}
		nodeID = hostname
	}

	interval := defaultInterval
	if rawInterval, ok, err := optionalStringValue(root, "interval", "config.interval"); err != nil {
		return Config{}, err
	} else if ok {
		interval, err = parseConfigDuration(rawInterval, "config.interval")
		if err != nil {
			return Config{}, err
		}
	}

	collectorMap, err := requiredMapValue(root, "collector", "config.collector")
	if err != nil {
		return Config{}, err
	}
	if err := validateKeys(collectorMap, "config.collector", "docker", "package", "type"); err != nil {
		return Config{}, err
	}

	collectorType, err := requiredStringValue(collectorMap, "type", "config.collector.type")
	if err != nil {
		return Config{}, err
	}
	jobName, _, err := optionalStringValue(root, "job_name", "config.job_name")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Node: NodeConfig{
			ID: nodeID,
		},
		Job: JobConfig{
			Name:      firstNonEmpty(jobName, collectorType),
			Interval:  interval,
			Collector: CollectorConfig{Type: collectorType},
		},
	}

	switch collectorType {
	case "docker":
		dockerMap, ok, err := optionalMapValue(collectorMap, "docker", "config.collector.docker")
		if err != nil {
			return Config{}, err
		}
		if ok {
			if err := validateKeys(dockerMap, "config.collector.docker", "exclude_labels", "exclude_self", "include_stopped"); err != nil {
				return Config{}, err
			}
		}

		includeStopped, err := optionalBoolPointerValue(dockerMap, "include_stopped", "config.collector.docker.include_stopped")
		if err != nil {
			return Config{}, err
		}
		excludeSelf, err := optionalBoolPointerValue(dockerMap, "exclude_self", "config.collector.docker.exclude_self")
		if err != nil {
			return Config{}, err
		}
		excludeLabels, _, err := optionalStringSliceValue(dockerMap, "exclude_labels", "config.collector.docker.exclude_labels")
		if err != nil {
			return Config{}, err
		}

		cfg.Job.Collector.Docker = dockercollector.Config{
			IncludeStopped: includeStopped,
			ExcludeSelf:    excludeSelf,
			ExcludeLabels:  excludeLabels,
		}

	case "package":
		packageMap, ok, err := optionalMapValue(collectorMap, "package", "config.collector.package")
		if err != nil {
			return Config{}, err
		}
		if ok {
			if err := validateKeys(packageMap, "config.collector.package", "manager", "names"); err != nil {
				return Config{}, err
			}
		}

		manager, _, err := optionalStringValue(packageMap, "manager", "config.collector.package.manager")
		if err != nil {
			return Config{}, err
		}
		names, _, err := optionalStringSliceValue(packageMap, "names", "config.collector.package.names")
		if err != nil {
			return Config{}, err
		}

		cfg.Job.Collector.Package = ospackagecollector.Config{
			Manager: manager,
			Names:   names,
		}

	default:
		return Config{}, fmt.Errorf("unsupported collector type %q", collectorType)
	}

	cfg.Job.Resolver.Type = defaultResolverTypeForCollector(cfg.Job.Collector)
	if cfg.Job.Resolver.Type == "" {
		return Config{}, fmt.Errorf("could not derive resolver type for collector %q", collectorType)
	}

	publisherMap, err := requiredMapValue(root, "publisher", "config.publisher")
	if err != nil {
		return Config{}, err
	}
	if err := validateKeys(publisherMap, "config.publisher", "mqtt", "type"); err != nil {
		return Config{}, err
	}

	publisherType, _, err := optionalStringValue(publisherMap, "type", "config.publisher.type")
	if err != nil {
		return Config{}, err
	}
	publisherType = firstNonEmpty(publisherType, "mqtt")
	cfg.Job.Publisher.Type = publisherType

	switch publisherType {
	case "mqtt":
		mqttMap, ok, err := optionalMapValue(publisherMap, "mqtt", "config.publisher.mqtt")
		if err != nil {
			return Config{}, err
		}
		if !ok {
			return Config{}, fmt.Errorf("config.publisher.mqtt is required when publisher.type is %q", publisherType)
		}
		if err := validateKeys(mqttMap, "config.publisher.mqtt", "client_id_prefix", "connect_timeout", "host", "password", "port", "retain", "topic_prefix", "username"); err != nil {
			return Config{}, err
		}

		host, _, err := optionalStringValue(mqttMap, "host", "config.publisher.mqtt.host")
		if err != nil {
			return Config{}, err
		}
		username, _, err := optionalStringValue(mqttMap, "username", "config.publisher.mqtt.username")
		if err != nil {
			return Config{}, err
		}
		password, _, err := optionalStringValue(mqttMap, "password", "config.publisher.mqtt.password")
		if err != nil {
			return Config{}, err
		}
		topicPrefix, _, err := optionalStringValue(mqttMap, "topic_prefix", "config.publisher.mqtt.topic_prefix")
		if err != nil {
			return Config{}, err
		}
		clientIDPrefix, _, err := optionalStringValue(mqttMap, "client_id_prefix", "config.publisher.mqtt.client_id_prefix")
		if err != nil {
			return Config{}, err
		}
		connectTimeout, _, err := optionalStringValue(mqttMap, "connect_timeout", "config.publisher.mqtt.connect_timeout")
		if err != nil {
			return Config{}, err
		}
		port, err := optionalIntValue(mqttMap, "port", "config.publisher.mqtt.port")
		if err != nil {
			return Config{}, err
		}
		retain, err := optionalBoolPointerValue(mqttMap, "retain", "config.publisher.mqtt.retain")
		if err != nil {
			return Config{}, err
		}

		cfg.Job.Publisher.MQTT = mqttpublisher.Config{
			Host:           host,
			Port:           port,
			Username:       username,
			Password:       password,
			TopicPrefix:    topicPrefix,
			ClientIDPrefix: clientIDPrefix,
			ConnectTimeout: connectTimeout,
			Retain:         retain,
		}

	default:
		return Config{}, fmt.Errorf("unsupported publisher type %q", publisherType)
	}

	return cfg, nil
}

func parseConfigDocument(document []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(document)
	if len(trimmed) == 0 {
		return map[string]any{}, nil
	}

	if trimmed[0] == '{' {
		var root map[string]any
		if err := json.Unmarshal(trimmed, &root); err != nil {
			return nil, fmt.Errorf("decode JSON config: %w", err)
		}
		if root == nil {
			return map[string]any{}, nil
		}
		return root, nil
	}

	return parseSimpleYAMLDocument(trimmed)
}

func parseSimpleYAMLDocument(document []byte) (map[string]any, error) {
	lines, err := tokenizeConfigLines(document)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return map[string]any{}, nil
	}

	root, next, err := parseConfigBlock(lines, 0, lines[0].indent)
	if err != nil {
		return nil, err
	}
	if next != len(lines) {
		return nil, fmt.Errorf("line %d: unexpected trailing content", lines[next].number)
	}

	rootMap, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top-level config must be a mapping")
	}
	return rootMap, nil
}

func tokenizeConfigLines(document []byte) ([]configLine, error) {
	scanner := bufio.NewScanner(bytes.NewReader(document))
	lines := make([]configLine, 0)

	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		raw := scanner.Text()
		if strings.ContainsRune(raw, '\t') {
			return nil, fmt.Errorf("line %d: tabs are not supported in config files", lineNumber)
		}

		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		lines = append(lines, configLine{
			number: lineNumber,
			indent: indent,
			text:   strings.TrimSpace(raw),
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config document: %w", err)
	}
	return lines, nil
}

func parseConfigBlock(lines []configLine, start, indent int) (any, int, error) {
	if start >= len(lines) {
		return nil, start, nil
	}
	if lines[start].indent != indent {
		return nil, start, fmt.Errorf("line %d: expected indentation %d, got %d", lines[start].number, indent, lines[start].indent)
	}

	if strings.HasPrefix(lines[start].text, "- ") {
		return parseConfigList(lines, start, indent)
	}
	return parseConfigMap(lines, start, indent)
}

func parseConfigMap(lines []configLine, start, indent int) (map[string]any, int, error) {
	values := make(map[string]any)

	index := start
	for index < len(lines) {
		line := lines[index]
		if line.indent < indent {
			break
		}
		if line.indent > indent {
			return nil, index, fmt.Errorf("line %d: unexpected indentation", line.number)
		}
		if strings.HasPrefix(line.text, "- ") {
			return nil, index, fmt.Errorf("line %d: expected a mapping entry, found a list item", line.number)
		}

		key, rawValue, ok := strings.Cut(line.text, ":")
		if !ok {
			return nil, index, fmt.Errorf("line %d: expected key:value syntax", line.number)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, index, fmt.Errorf("line %d: empty key is not allowed", line.number)
		}
		if _, exists := values[key]; exists {
			return nil, index, fmt.Errorf("line %d: duplicate key %q", line.number, key)
		}

		rawValue = strings.TrimSpace(rawValue)
		if rawValue != "" {
			values[key] = parseScalarValue(rawValue)
			index++
			continue
		}

		index++
		if index >= len(lines) || lines[index].indent <= indent {
			return nil, index, fmt.Errorf("line %d: key %q requires an indented value block", line.number, key)
		}

		child, next, err := parseConfigBlock(lines, index, lines[index].indent)
		if err != nil {
			return nil, next, err
		}
		values[key] = child
		index = next
	}

	return values, index, nil
}

func parseConfigList(lines []configLine, start, indent int) ([]any, int, error) {
	values := make([]any, 0)

	index := start
	for index < len(lines) {
		line := lines[index]
		if line.indent < indent {
			break
		}
		if line.indent > indent {
			return nil, index, fmt.Errorf("line %d: unexpected indentation", line.number)
		}
		if !strings.HasPrefix(line.text, "- ") {
			return nil, index, fmt.Errorf("line %d: expected a list item", line.number)
		}

		rawValue := strings.TrimSpace(strings.TrimPrefix(line.text, "- "))
		if rawValue != "" {
			values = append(values, parseScalarValue(rawValue))
			index++
			continue
		}

		index++
		if index >= len(lines) || lines[index].indent <= indent {
			return nil, index, fmt.Errorf("line %d: list item requires an indented value block", line.number)
		}

		child, next, err := parseConfigBlock(lines, index, lines[index].indent)
		if err != nil {
			return nil, next, err
		}
		values = append(values, child)
		index = next
	}

	return values, index, nil
}

func parseScalarValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 {
		if (raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'') {
			return raw[1 : len(raw)-1]
		}
	}
	return raw
}

func validateKeys(values map[string]any, context string, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}

	unexpected := make([]string, 0)
	for key := range values {
		if _, ok := allowedSet[key]; !ok {
			unexpected = append(unexpected, key)
		}
	}

	if len(unexpected) == 0 {
		return nil
	}

	sort.Strings(unexpected)
	return fmt.Errorf("%s has unsupported key(s): %s", context, strings.Join(unexpected, ", "))
}

func requiredMapValue(values map[string]any, key, context string) (map[string]any, error) {
	result, ok, err := optionalMapValue(values, key, context)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s is required", context)
	}
	return result, nil
}

func optionalMapValue(values map[string]any, key, context string) (map[string]any, bool, error) {
	if values == nil {
		return nil, false, nil
	}

	raw, ok := values[key]
	if !ok {
		return nil, false, nil
	}

	result, ok := raw.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("%s must be a mapping", context)
	}
	return result, true, nil
}

func requiredStringValue(values map[string]any, key, context string) (string, error) {
	value, ok, err := optionalStringValue(values, key, context)
	if err != nil {
		return "", err
	}
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", context)
	}
	return value, nil
}

func optionalStringValue(values map[string]any, key, context string) (string, bool, error) {
	if values == nil {
		return "", false, nil
	}

	raw, ok := values[key]
	if !ok {
		return "", false, nil
	}

	value, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be a string", context)
	}
	return strings.TrimSpace(value), true, nil
}

func optionalStringSliceValue(values map[string]any, key, context string) ([]string, bool, error) {
	if values == nil {
		return nil, false, nil
	}

	raw, ok := values[key]
	if !ok {
		return nil, false, nil
	}

	switch typed := raw.(type) {
	case string:
		parts := strings.Split(typed, ",")
		items := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				items = append(items, part)
			}
		}
		return items, true, nil

	case []any:
		items := make([]string, 0, len(typed))
		for index, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, false, fmt.Errorf("%s[%d] must be a string", context, index)
			}
			value = strings.TrimSpace(value)
			if value != "" {
				items = append(items, value)
			}
		}
		return items, true, nil

	default:
		return nil, false, fmt.Errorf("%s must be a string or a list of strings", context)
	}
}

func optionalBoolPointerValue(values map[string]any, key, context string) (*bool, error) {
	if values == nil {
		return nil, nil
	}

	raw, ok := values[key]
	if !ok {
		return nil, nil
	}

	value, err := parseBooleanValue(raw, context)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseBooleanValue(raw any, context string) (bool, error) {
	switch typed := raw.(type) {
	case bool:
		return typed, nil
	case string:
		switch strings.TrimSpace(strings.ToLower(typed)) {
		case "1", "true", "yes", "on":
			return true, nil
		case "0", "false", "no", "off":
			return false, nil
		default:
			return false, fmt.Errorf("%s has invalid boolean %q", context, typed)
		}
	default:
		return false, fmt.Errorf("%s must be a boolean", context)
	}
}

func optionalIntValue(values map[string]any, key, context string) (int, error) {
	if values == nil {
		return 0, nil
	}

	raw, ok := values[key]
	if !ok {
		return 0, nil
	}

	switch typed := raw.(type) {
	case float64:
		if typed != float64(int(typed)) {
			return 0, fmt.Errorf("%s must be an integer", context)
		}
		return int(typed), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, fmt.Errorf("%s has invalid integer %q: %w", context, typed, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", context)
	}
}

func parseConfigDuration(raw, context string) (time.Duration, error) {
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s has invalid duration %q: %w", context, raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", context)
	}
	return value, nil
}
