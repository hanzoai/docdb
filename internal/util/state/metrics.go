// Copyright 2021 Hanzo AI Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package state

import (
	"strconv"

	metric "github.com/luxfi/metric"

	"github.com/hanzoai/docdb/build/version"
)

// metricsCollector exposes provider's state as Prometheus metric.
type metricsCollector struct {
	p       *Provider
	addUUID bool
}

// newMetricsCollector creates a new metricsCollector.
//
// If addUUID is true, then the "uuid" label is added.
func newMetricsCollector(p *Provider, addUUID bool) *metricsCollector {
	return &metricsCollector{
		p:       p,
		addUUID: addUUID,
	}
}

// Describe implements [metric.Collector].
func (mc *metricsCollector) Describe(ch chan<- *metric.Desc) {
	metric.DescribeByCollect(mc, ch)
}

// Collect implements [metric.Collector].
// It exposes a single metric with various labels.
func (mc *metricsCollector) Collect(ch chan<- metric.Metric) {
	info := version.Get()
	constLabels := metric.Labels{
		"version": info.Version,
		"commit":  info.Commit,
		"branch":  info.Branch,
		"dirty":   strconv.FormatBool(info.Dirty),
		"package": info.Package,
		"dev":     strconv.FormatBool(info.DevBuild),
	}

	labels := []string{
		"postgresql",
		"documentdb",
		"telemetry",
		"update_available",
	}
	if mc.addUUID {
		labels = append(labels, "uuid")
	}

	s := mc.p.Get()

	values := []string{
		s.PostgreSQLVersion,
		s.DocumentDBVersion,
		s.TelemetryString(),
		strconv.FormatBool(s.UpdateAvailable),
	}
	if mc.addUUID {
		values = append(values, s.UUID)
	}

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			"docdb_up",
			"DocDB instance state.",
			labels,
			constLabels,
		),
		metric.GaugeValue,
		1,
		values...,
	)
}

// check interfaces
var (
	_ metric.Collector = (*metricsCollector)(nil)
)
