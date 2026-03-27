package orchestrator

import (
	"fmt"
	"log/slog"

	dockercollector "up2date/collector/docker"
	"up2date/common/model"
	mqttpublisher "up2date/publisher/mqtt"
	dockerhubresolver "up2date/resolver/dockerhub"
)

func Build(cfg Config, logger *slog.Logger) (*Orchestrator, error) {
	collector, err := buildCollector(cfg.Job.Collector)
	if err != nil {
		return nil, fmt.Errorf("build collector for job %q: %w", cfg.Job.Name, err)
	}

	resolver, err := buildResolver(cfg.Job.Resolver)
	if err != nil {
		return nil, fmt.Errorf("build resolver for job %q: %w", cfg.Job.Name, err)
	}

	publisher, err := buildPublisher(cfg.Job.Publisher)
	if err != nil {
		return nil, fmt.Errorf("build publisher for job %q: %w", cfg.Job.Name, err)
	}

	jobs := []Job{
		{
			Name:      cfg.Job.Name,
			Interval:  cfg.Job.Interval,
			Collector: collector,
			Resolver:  resolver,
			Publishers: []Publisher{
				publisher,
			},
		},
	}

	node := model.Node{
		ID: cfg.Node.ID,
	}
	return New(node, jobs, logger), nil
}

func buildCollector(cfg CollectorConfig) (Collector, error) {
	switch cfg.Type {
	case "docker":
		return dockercollector.New(cfg.Docker), nil
	default:
		return nil, fmt.Errorf("unsupported collector type %q", cfg.Type)
	}
}

func buildResolver(cfg ResolverConfig) (Resolver, error) {
	switch cfg.Type {
	case "docker_hub":
		return dockerhubresolver.New(), nil
	default:
		return nil, fmt.Errorf("unsupported resolver type %q", cfg.Type)
	}
}

func buildPublisher(cfg PublisherConfig) (Publisher, error) {
	switch cfg.Type {
	case "mqtt":
		return mqttpublisher.New(cfg.MQTT)
	default:
		return nil, fmt.Errorf("unsupported publisher type %q", cfg.Type)
	}
}
