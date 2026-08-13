package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"up2date/common/model"
)

const (
	defaultPort              = 1883
	defaultTopicPrefix       = "up2date"
	defaultClientID          = "up2date"
	defaultTimeout           = 10 * time.Second
	defaultHADiscoveryPrefix = "homeassistant"
)

var topicSegmentRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type HomeAssistantConfig struct {
	Enabled         bool   `json:"enabled,omitempty"`
	DiscoveryPrefix string `json:"discovery_prefix,omitempty"`
}

type Config struct {
	Host           string               `json:"host"`
	Port           int                  `json:"port,omitempty"`
	Username       string               `json:"username,omitempty"`
	Password       string               `json:"password,omitempty"`
	TopicPrefix    string               `json:"topic_prefix,omitempty"`
	ClientIDPrefix string               `json:"client_id_prefix,omitempty"`
	ConnectTimeout string               `json:"connect_timeout,omitempty"`
	Retain         *bool                `json:"retain,omitempty"`
	HomeAssistant  *HomeAssistantConfig `json:"homeassistant,omitempty"`
}

type Publisher struct {
	host              string
	port              int
	username          string
	password          string
	topicPrefix       string
	clientIDPrefix    string
	connectTimeout    time.Duration
	retain            bool
	haEnabled         bool
	haDiscoveryPrefix string
}

type publishedField struct {
	name  string
	value string
}

type haDevice struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
}

type haUpdateConfig struct {
	Name               string   `json:"name"`
	Title              string   `json:"title,omitempty"`
	UniqueID           string   `json:"unique_id"`
	ObjectID           string   `json:"object_id"`
	StateTopic         string   `json:"state_topic"`
	LatestVersionTopic string   `json:"latest_version_topic"`
	ReleaseURLTopic    string   `json:"release_url_topic,omitempty"`
	EntityCategory     string   `json:"entity_category,omitempty"`
	Device             haDevice `json:"device"`
}

type haSensorConfig struct {
	Name           string   `json:"name"`
	UniqueID       string   `json:"unique_id"`
	ObjectID       string   `json:"object_id"`
	StateTopic     string   `json:"state_topic"`
	Icon           string   `json:"icon,omitempty"`
	EntityCategory string   `json:"entity_category,omitempty"`
	Device         haDevice `json:"device"`
}

func New(cfg Config) (*Publisher, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("mqtt publisher requires host")
	}
	if cfg.Password != "" && cfg.Username == "" {
		return nil, fmt.Errorf("mqtt publisher requires username when password is set")
	}

	port := cfg.Port
	if port == 0 {
		port = defaultPort
	}

	topicPrefix := cfg.TopicPrefix
	if topicPrefix == "" {
		topicPrefix = defaultTopicPrefix
	}

	clientIDPrefix := cfg.ClientIDPrefix
	if clientIDPrefix == "" {
		clientIDPrefix = defaultClientID
	}

	connectTimeout := defaultTimeout
	if cfg.ConnectTimeout != "" {
		parsed, err := time.ParseDuration(cfg.ConnectTimeout)
		if err != nil {
			return nil, fmt.Errorf("parse mqtt connect_timeout: %w", err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("mqtt connect_timeout must be positive")
		}
		connectTimeout = parsed
	}

	retain := true
	if cfg.Retain != nil {
		retain = *cfg.Retain
	}

	haEnabled := false
	haDiscoveryPrefix := defaultHADiscoveryPrefix
	if cfg.HomeAssistant != nil {
		haEnabled = cfg.HomeAssistant.Enabled
		if cfg.HomeAssistant.DiscoveryPrefix != "" {
			haDiscoveryPrefix = cfg.HomeAssistant.DiscoveryPrefix
		}
	}

	return &Publisher{
		host:              cfg.Host,
		port:              port,
		username:          cfg.Username,
		password:          cfg.Password,
		topicPrefix:       topicPrefix,
		clientIDPrefix:    clientIDPrefix,
		connectTimeout:    connectTimeout,
		retain:            retain,
		haEnabled:         haEnabled,
		haDiscoveryPrefix: haDiscoveryPrefix,
	}, nil
}

func (o *Publisher) Publish(ctx context.Context, checks []model.CheckResult) error {
	if len(checks) == 0 {
		return nil
	}

	clientID := fmt.Sprintf("%s-%d", o.clientIDPrefix, time.Now().UnixNano())
	client, err := Dial(ctx, o.host, o.port, clientID, o.username, o.password, o.connectTimeout)
	if err != nil {
		return err
	}
	defer client.Close()

	for _, check := range checks {
		for _, field := range publishedFields(check) {
			if err := o.publishField(ctx, client, check, field.name, field.value); err != nil {
				return err
			}
		}

		if o.haEnabled {
			if err := o.publishHomeAssistantDiscovery(ctx, client, check); err != nil {
				return err
			}
		}
	}

	return nil
}

func publishedFields(check model.CheckResult) []publishedField {
	return []publishedField{
		{name: "artifact_name", value: check.ArtifactName},
		{name: "current_version", value: check.CurrentVersion},
		{name: "latest_version", value: check.LatestVersion},
		{name: "latest_version_url", value: check.LatestVersionURL},
		{name: "observed_at", value: check.ObservedAt},
		{name: "check_status", value: check.CheckStatus},
	}
}

func (o *Publisher) publishHomeAssistantDiscovery(ctx context.Context, client *Client, check model.CheckResult) error {
	sanitizedNodeID := sanitizeTopicSegment(check.NodeID)
	sanitizedServiceName := sanitizeTopicSegment(check.ServiceName)

	device := haDevice{
		Identifiers:  []string{fmt.Sprintf("up2date_%s", sanitizedNodeID)},
		Name:         check.NodeID,
		Manufacturer: "up2date",
		Model:        "up2date Agent",
	}

	title := check.ArtifactName
	if title == "" {
		title = check.ServiceName
	}

	// 1. Update entity config
	updateConfig := haUpdateConfig{
		Name:               fmt.Sprintf("%s Update", title),
		Title:              title,
		UniqueID:           fmt.Sprintf("up2date_%s_%s_update", sanitizedNodeID, sanitizedServiceName),
		ObjectID:           fmt.Sprintf("%s_%s_update", sanitizedNodeID, sanitizedServiceName),
		StateTopic:         o.fieldTopic(check.NodeID, check.ServiceName, "current_version"),
		LatestVersionTopic: o.fieldTopic(check.NodeID, check.ServiceName, "latest_version"),
		ReleaseURLTopic:    o.fieldTopic(check.NodeID, check.ServiceName, "latest_version_url"),
		Device:             device,
	}

	updatePayload, err := json.Marshal(updateConfig)
	if err != nil {
		return fmt.Errorf("marshal home assistant update config: %w", err)
	}

	updateTopic := fmt.Sprintf("%s/update/%s_%s/config", o.haDiscoveryPrefix, sanitizedNodeID, sanitizedServiceName)
	if err := client.Publish(ctx, updateTopic, updatePayload, true); err != nil {
		return fmt.Errorf("publish home assistant update discovery: %w", err)
	}

	// 2. Check status sensor config
	sensorConfig := haSensorConfig{
		Name:           fmt.Sprintf("%s Check Status", title),
		UniqueID:       fmt.Sprintf("up2date_%s_%s_check_status", sanitizedNodeID, sanitizedServiceName),
		ObjectID:       fmt.Sprintf("%s_%s_check_status", sanitizedNodeID, sanitizedServiceName),
		StateTopic:     o.fieldTopic(check.NodeID, check.ServiceName, "check_status"),
		Icon:           "mdi:shield-search",
		EntityCategory: "diagnostic",
		Device:         device,
	}

	sensorPayload, err := json.Marshal(sensorConfig)
	if err != nil {
		return fmt.Errorf("marshal home assistant sensor config: %w", err)
	}

	sensorTopic := fmt.Sprintf("%s/sensor/%s_%s_check_status/config", o.haDiscoveryPrefix, sanitizedNodeID, sanitizedServiceName)
	if err := client.Publish(ctx, sensorTopic, sensorPayload, true); err != nil {
		return fmt.Errorf("publish home assistant sensor discovery: %w", err)
	}

	// 3. Current version sensor config
	currentVersionConfig := haSensorConfig{
		Name:           fmt.Sprintf("%s Current Version", title),
		UniqueID:       fmt.Sprintf("up2date_%s_%s_current_version", sanitizedNodeID, sanitizedServiceName),
		ObjectID:       fmt.Sprintf("%s_%s_current_version", sanitizedNodeID, sanitizedServiceName),
		StateTopic:     o.fieldTopic(check.NodeID, check.ServiceName, "current_version"),
		Icon:           "mdi:tag-outline",
		EntityCategory: "diagnostic",
		Device:         device,
	}

	currentVersionPayload, err := json.Marshal(currentVersionConfig)
	if err != nil {
		return fmt.Errorf("marshal home assistant current version sensor config: %w", err)
	}

	currentVersionTopic := fmt.Sprintf("%s/sensor/%s_%s_current_version/config", o.haDiscoveryPrefix, sanitizedNodeID, sanitizedServiceName)
	if err := client.Publish(ctx, currentVersionTopic, currentVersionPayload, true); err != nil {
		return fmt.Errorf("publish home assistant current version sensor discovery: %w", err)
	}

	// 4. Latest version sensor config
	latestVersionConfig := haSensorConfig{
		Name:           fmt.Sprintf("%s Latest Version", title),
		UniqueID:       fmt.Sprintf("up2date_%s_%s_latest_version", sanitizedNodeID, sanitizedServiceName),
		ObjectID:       fmt.Sprintf("%s_%s_latest_version", sanitizedNodeID, sanitizedServiceName),
		StateTopic:     o.fieldTopic(check.NodeID, check.ServiceName, "latest_version"),
		Icon:           "mdi:tag-arrow-up-outline",
		EntityCategory: "diagnostic",
		Device:         device,
	}

	latestVersionPayload, err := json.Marshal(latestVersionConfig)
	if err != nil {
		return fmt.Errorf("marshal home assistant latest version sensor config: %w", err)
	}

	latestVersionTopic := fmt.Sprintf("%s/sensor/%s_%s_latest_version/config", o.haDiscoveryPrefix, sanitizedNodeID, sanitizedServiceName)
	if err := client.Publish(ctx, latestVersionTopic, latestVersionPayload, true); err != nil {
		return fmt.Errorf("publish home assistant latest version sensor discovery: %w", err)
	}

	return nil
}

func (o *Publisher) clearRetained(ctx context.Context, client *Client, topic string) error {
	if err := client.Publish(ctx, topic, nil, true); err != nil {
		return fmt.Errorf("clear retained topic %q: %w", topic, err)
	}
	return nil
}

func (o *Publisher) publishField(ctx context.Context, client *Client, check model.CheckResult, fieldName, value string) error {
	topic := o.fieldTopic(check.NodeID, check.ServiceName, fieldName)
	if strings.TrimSpace(value) == "" {
		return o.clearRetained(ctx, client, topic)
	}

	if err := client.Publish(ctx, topic, []byte(value), o.retain); err != nil {
		return fmt.Errorf("publish field %q: %w", fieldName, err)
	}
	return nil
}

func (o *Publisher) fieldTopic(nodeID, serviceName, fieldName string) string {
	return fmt.Sprintf(
		"%s/%s/%s/%s",
		o.topicPrefix,
		nodeID,
		sanitizeTopicSegment(serviceName),
		sanitizeTopicSegment(fieldName),
	)
}

func sanitizeTopicSegment(value string) string {
	sanitized := topicSegmentRe.ReplaceAllString(strings.TrimSpace(value), "_")
	if sanitized == "" {
		return "unknown"
	}
	return sanitized
}

