package orchestrator

import (
	"fmt"
	"log/slog"

	dockercollector "up2date/collector/docker"
	ospackagecollector "up2date/collector/ospackage"
	"up2date/common/model"
	mqttpublisher "up2date/publisher/mqtt"
	brewformularesolver "up2date/resolver/brewformula"
	dockerresolver "up2date/resolver/docker"
	noneresolver "up2date/resolver/none"
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
	case "package":
		return ospackagecollector.New(cfg.Package)
	default:
		return nil, fmt.Errorf("unsupported collector type %q", cfg.Type)
	}
}

func buildResolver(cfg ResolverConfig) (Resolver, error) {
	switch cfg.Type {
	case "brew_formula":
		return brewformularesolver.New(), nil
	case "docker", "docker_hub":
		return dockerresolver.New(), nil
	case "none":
		return noneresolver.New(), nil
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
