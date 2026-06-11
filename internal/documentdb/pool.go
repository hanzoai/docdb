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

package documentdb

import (
	"log/slog"

	"github.com/AlekSi/lazyerrors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	metric "github.com/luxfi/metric"

	"github.com/hanzoai/docdb/internal/documentdb/cursor"
	"github.com/hanzoai/docdb/internal/util/logging"
	"github.com/hanzoai/docdb/internal/util/must"
	"github.com/hanzoai/docdb/internal/util/resource"
	"github.com/hanzoai/docdb/internal/util/state"
)

// Parts of Prometheus metric names.
const (
	namespace = "docdb"
	subsystem = "pool"
)

// Pool represent a pool of PostgreSQL connections.
type Pool struct {
	p      *pgxpool.Pool
	r      *cursor.Registry
	l      *slog.Logger
	tracer *tracer
	token  *resource.Token
}

// NewPool creates a new pool of PostgreSQL connections.
// No actual connections are established.
func NewPool(uri string, l *slog.Logger, sp *state.Provider) (*Pool, error) {
	must.NotBeZero(uri)
	must.NotBeZero(l)
	must.NotBeZero(sp)

	tl := logging.WithName(l, "pgx")
	t := newTracer(tl)

	p, err := newPgxPool(uri, tl, t, sp)
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	res := &Pool{
		p:      p,
		r:      cursor.NewRegistry(logging.WithName(l, "cursors")),
		l:      l,
		tracer: t,
		token:  resource.NewToken(),
	}
	resource.Track(res, res.token)

	return res, nil
}

// Close closes all connections in the pool.
func (p *Pool) Close() {
	p.r.Close(todoCtx)

	p.p.Close()

	resource.Untrack(p, p.token)
}

// Acquire acquires a connection from the pool.
//
// It is caller's responsibility to call [Conn.Release].
// Most callers should use [Pool.WithConn] instead.
func (p *Pool) Acquire() (*Conn, error) {
	conn, err := p.p.Acquire(todoCtx)
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	return newConn(conn), nil
}

// WithConn acquires a connection from the pool and calls the provided function with it.
// The connection is automatically released after the function returns.
func (p *Pool) WithConn(f func(*pgx.Conn) error) error {
	conn, err := p.Acquire()
	if err != nil {
		return lazyerrors.Error(err)
	}

	defer conn.Release()

	if err = f(conn.Conn()); err != nil {
		return lazyerrors.Error(err)
	}

	return nil
}

// Describe implements [metric.Collector].
func (p *Pool) Describe(ch chan<- *metric.Desc) {
	metric.DescribeByCollect(p, ch)
}

// Collect implements [metric.Collector].
func (p *Pool) Collect(ch chan<- metric.Metric) {
	p.r.Collect(ch)
	p.tracer.Collect(ch)

	stats := p.p.Stat()

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "acquires_total"),
			"The cumulative count of successful connection acquires from the pool.",
			nil, nil,
		),
		metric.CounterValue,
		float64(stats.AcquireCount()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "acquires_duration_seconds_total"),
			"The total duration of all successful connection acquires from the pool.",
			nil, nil,
		),
		metric.CounterValue,
		stats.AcquireDuration().Seconds(),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "acquired"),
			"The number of currently acquired connections in the pool.",
			nil, nil,
		),
		metric.GaugeValue,
		float64(stats.AcquiredConns()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "acquires_canceled_total"),
			"The cumulative count of connection acquires from the pool that were canceled.",
			nil, nil,
		),
		metric.CounterValue,
		float64(stats.CanceledAcquireCount()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "constructing"),
			"The number of connections with construction in progress in the pool.",
			nil, nil,
		),
		metric.GaugeValue,
		float64(stats.ConstructingConns()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "acquires_empty_total"),
			"The cumulative count of successful connection acquires from the pool "+
				"that waited for a resource to be released or constructed because the pool was empty.",
			nil, nil,
		),
		metric.CounterValue,
		float64(stats.EmptyAcquireCount()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "idle"),
			"The number of currently idle connections in the pool.",
			nil, nil,
		),
		metric.GaugeValue,
		float64(stats.IdleConns()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "max_size"),
			"The maximum size of the connection pool.",
			nil, nil,
		),
		metric.GaugeValue,
		float64(stats.MaxConns()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "size"),
			"Total number of connections currently in the pool. "+
				"Should be a sum of constructing, acquired, and idle.",
			nil, nil,
		),
		metric.GaugeValue,
		float64(stats.TotalConns()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "opened_total"),
			"The cumulative count of new connections opened.",
			nil, nil,
		),
		metric.CounterValue,
		float64(stats.NewConnsCount()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "destroyed_maxlifetime_total"),
			"The cumulative count of connections destroyed because they exceeded pool_max_conn_lifetime.",
			nil, nil,
		),
		metric.CounterValue,
		float64(stats.MaxLifetimeDestroyCount()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "destroyed_maxidle_total"),
			"The cumulative count of connections destroyed because they exceeded pool_max_conn_idle_time.",
			nil, nil,
		),
		metric.CounterValue,
		float64(stats.MaxIdleDestroyCount()),
	)

	ch <- metric.MustNewConstMetric(
		metric.NewDesc(
			metric.BuildFQName(namespace, subsystem, "acquires_empty_duration_seconds_total"),
			"The cumulative time waited for successful acquires from the pool "+
				"for a resource to be released or constructed because the pool was empty.",
			nil, nil,
		),
		metric.CounterValue,
		stats.EmptyAcquireWaitTime().Seconds(),
	)
}

// check interfaces
var (
	_ metric.Collector = (*Pool)(nil)
)
