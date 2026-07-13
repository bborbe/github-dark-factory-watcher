// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import "github.com/prometheus/client_golang/prometheus"

//counterfeiter:generate -o ../mocks/metrics.go --fake-name Metrics . Metrics

// Metrics exposes counters for observable watcher behaviour.
type Metrics interface {
	// IncPollCycle increments the poll cycle counter with the given result label.
	// result: "success", "rate_limited", "github_error"
	IncPollCycle(result string)
	// IncPublished increments the published counter with the given status label.
	// status: "create", "skipped", "error"
	IncPublished(status string)
	// IncReposScanned increments the scanned-PR counter (sanity check on the scope filter).
	IncReposScanned()
	// IncFilterSkipped increments the filter-skipped counter with the given reason.
	// reason: "out_of_scope", "details_error", "empty_sha", "not_draft", "not_open",
	// "no_dark_factory_yaml", "no_spec_in_diff", "prompts_in_flight", "evaluate_error"
	IncFilterSkipped(reason string)
}

const metricsNamespace = "github_dark_factory_watcher"

// NewMetrics returns a Metrics implementation backed by Prometheus counters
// registered against the given registerer. Passing nil resolves to
// prometheus.DefaultRegisterer (production); tests pass prometheus.NewRegistry()
// so repeated construction in the same process does not panic with a duplicate
// registration.
func NewMetrics(registerer prometheus.Registerer) Metrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	m := &prometheusMetrics{
		pollCyclesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "poll_cycles_total",
			Help:      "Total number of GitHub poll cycles by result.",
		}, []string{"result"}),
		publishedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "published_total",
			Help:      "Total number of emitted tasks by status.",
		}, []string{"status"}),
		reposScannedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "repos_scanned_total",
			Help:      "Total number of PRs scanned across poll cycles.",
		}),
		filterSkippedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "filter_skipped_total",
			Help:      "Total number of PRs skipped by reason.",
		}, []string{"reason"}),
	}
	registerer.MustRegister(
		m.pollCyclesTotal,
		m.publishedTotal,
		m.reposScannedTotal,
		m.filterSkippedTotal,
	)
	for _, result := range []string{"success", "rate_limited", "github_error"} {
		m.pollCyclesTotal.WithLabelValues(result).Add(0)
	}
	for _, status := range []string{"create", "skipped", "error"} {
		m.publishedTotal.WithLabelValues(status).Add(0)
	}
	for _, reason := range []string{
		"out_of_scope", "details_error", "empty_sha", "not_draft", "not_open",
		"no_dark_factory_yaml", "no_spec_in_diff", "prompts_in_flight", "evaluate_error",
	} {
		m.filterSkippedTotal.WithLabelValues(reason).Add(0)
	}
	return m
}

type prometheusMetrics struct {
	pollCyclesTotal    *prometheus.CounterVec
	publishedTotal     *prometheus.CounterVec
	reposScannedTotal  prometheus.Counter
	filterSkippedTotal *prometheus.CounterVec
}

func (m *prometheusMetrics) IncPollCycle(result string) {
	m.pollCyclesTotal.WithLabelValues(result).Inc()
}

func (m *prometheusMetrics) IncPublished(status string) {
	m.publishedTotal.WithLabelValues(status).Inc()
}

func (m *prometheusMetrics) IncReposScanned() {
	m.reposScannedTotal.Inc()
}

func (m *prometheusMetrics) IncFilterSkipped(reason string) {
	m.filterSkippedTotal.WithLabelValues(reason).Inc()
}
