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

package clientconn

import (
	"time"

	metric "github.com/luxfi/metric"
)

// Parts of Prometheus metric names.
const (
	// TODO https://github.com/hanzoai/docdb/issues/3420
	namespace = "docdb"
	subsystem = "client"
)

// listenerMetrics represents listener metrics.
type listenerMetrics struct {
	accepts   *metric.CounterVec
	durations *metric.HistogramVec
}

// NewListenerMetrics creates new listener metrics.
func NewListenerMetrics() *listenerMetrics {
	lm := &listenerMetrics{
		accepts: metric.NewCounterVec(
			metric.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "accepts_total",
				Help:      "Total number of accepted client connections.",
			},
			[]string{"error"},
		),
		durations: metric.NewHistogramVec(
			metric.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "duration_seconds",
				Help:      "Client connection lifetime in seconds.",
				Buckets: []float64{
					1,
					5,
					10,
					30,
					(1 * time.Minute).Seconds(),
					(5 * time.Minute).Seconds(),
					(10 * time.Minute).Seconds(),
					(30 * time.Minute).Seconds(),
				},
			},
			[]string{"error"},
		),
	}

	lm.accepts.WithLabelValues("0")
	lm.durations.WithLabelValues("0")

	return lm
}

// Describe implements [metric.Collector].
func (lm *listenerMetrics) Describe(ch chan<- *metric.Desc) {
	lm.accepts.Describe(ch)
	lm.durations.Describe(ch)
}

// Collect implements [metric.Collector].
func (lm *listenerMetrics) Collect(ch chan<- metric.Metric) {
	lm.accepts.Collect(ch)
	lm.durations.Collect(ch)
}

// check interfaces
var (
	_ metric.Collector = (*listenerMetrics)(nil)
)
