package mqtt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"up2date/common/model"
)

const (
	defaultPort        = 1883
	defaultTopicPrefix = "up2date"
	defaultClientID    = "up2date"
	defaultTimeout     = 10 * time.Second
)

var topicSegmentRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Config struct {
	Host           string `json:"host"`
	Port           int    `json:"port,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	TopicPrefix    string `json:"topic_prefix,omitempty"`
	ClientIDPrefix string `json:"client_id_prefix,omitempty"`
	ConnectTimeout string `json:"connect_timeout,omitempty"`
	Retain         *bool  `json:"retain,omitempty"`
}

type Publisher struct {
	host           string
	port           int
	username       string
	password       string
	topicPrefix    string
	clientIDPrefix string
	connectTimeout time.Duration
	retain         bool
}

type publishedField struct {
	name  string
	value string
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

	return &Publisher{
		host:           cfg.Host,
		port:           port,
		username:       cfg.Username,
		password:       cfg.Password,
		topicPrefix:    topicPrefix,
		clientIDPrefix: clientIDPrefix,
		connectTimeout: connectTimeout,
		retain:         retain,
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
