package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"up2date/common/model"
)

type Collector interface {
	Collect(ctx context.Context, node model.Node, jobName string) (model.Snapshot, error)
}

type Resolver interface {
	Resolve(ctx context.Context, snapshot model.Snapshot) ([]model.CheckResult, error)
}

type Publisher interface {
	Publish(ctx context.Context, checks []model.CheckResult) error
}

type Job struct {
	Name     string
	Interval time.Duration
	Collector Collector
	Resolver Resolver
	Publishers  []Publisher
}

type Orchestrator struct {
	node   model.Node
	jobs   []Job
	logger *slog.Logger
}

func New(node model.Node, jobs []Job, logger *slog.Logger) *Orchestrator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	return &Orchestrator{
		node:   node,
		jobs:   jobs,
		logger: logger,
	}
}

func (r *Orchestrator) Run(ctx context.Context, once bool) error {
	if once {
		for _, job := range r.jobs {
			if err := r.runJob(ctx, job); err != nil {
				return err
			}
		}
		return nil
	}

	var waitGroup sync.WaitGroup
	for _, job := range r.jobs {
		job := job
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			r.loop(ctx, job)
		}()
	}

	<-ctx.Done()
	waitGroup.Wait()
	return nil
}

func (r *Orchestrator) loop(ctx context.Context, job Job) {
	if err := r.runJob(ctx, job); err != nil {
		r.logger.Error("job run failed", "job", job.Name, "error", err)
	}

	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.runJob(ctx, job); err != nil {
				r.logger.Error("job run failed", "job", job.Name, "error", err)
			}
		}
	}
}

func (r *Orchestrator) runJob(ctx context.Context, job Job) error {
	snapshot, err := job.Collector.Collect(ctx, r.node, job.Name)
	if err != nil {
		return fmt.Errorf("collect snapshot for job %q: %w", job.Name, err)
	}

	checks, err := job.Resolver.Resolve(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("resolve snapshot for job %q: %w", job.Name, err)
	}

	for _, publisher := range job.Publishers {
		if err := publisher.Publish(ctx, checks); err != nil {
			return fmt.Errorf("publish job %q: %w", job.Name, err)
		}
	}

	r.logger.Info(
		"job completed",
		"job", job.Name,
		"observations", len(snapshot.Observations),
		"checks", len(checks),
	)
	return nil
}
