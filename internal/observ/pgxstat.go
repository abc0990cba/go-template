package observ

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// PoolStatsCollector turns pgxpool.Stat() into Prometheus metrics. It is a
// hand-written prometheus.Collector — the teachable example of the
// Describe/Collect interface — because pool saturation is invisible in the
// default go_* and process_* metrics: a service can look perfectly healthy
// while every request queues behind Acquire().
//
// The saturation signal to alert on is app_pgpool_waiters_total (acquires
// that found no idle connection and had to wait) together with the acquired
// share of app_pgpool_max_conns.
type PoolStatsCollector struct {
	stats func() *pgxpool.Stat

	acquired    *prometheus.Desc
	idle        *prometheus.Desc
	total       *prometheus.Desc
	max         *prometheus.Desc
	construct   *prometheus.Desc
	acquires    *prometheus.Desc
	acquireTime *prometheus.Desc
	waiters     *prometheus.Desc
	canceled    *prometheus.Desc
	newConns    *prometheus.Desc
}

// NewPoolStatsCollector observes the pgxpool guarded by the stats getter.
// Callers register it with Prometheus only when a Postgres pool exists.
func NewPoolStatsCollector(stats func() *pgxpool.Stat) *PoolStatsCollector {
	return &PoolStatsCollector{
		stats:       stats,
		acquired:    prometheus.NewDesc("app_pgpool_acquired_conns", "Connections currently checked out of the pool.", nil, nil),
		idle:        prometheus.NewDesc("app_pgpool_idle_conns", "Idle connections sitting in the pool.", nil, nil),
		total:       prometheus.NewDesc("app_pgpool_total_conns", "Connections currently in the pool (acquired + idle + constructing).", nil, nil),
		max:         prometheus.NewDesc("app_pgpool_max_conns", "Configured pool size limit.", nil, nil),
		construct:   prometheus.NewDesc("app_pgpool_constructing_conns", "Connections currently being established.", nil, nil),
		acquires:    prometheus.NewDesc("app_pgpool_acquires_total", "Cumulative successful Acquire calls.", nil, nil),
		acquireTime: prometheus.NewDesc("app_pgpool_acquire_duration_seconds_total", "Cumulative time spent in successful Acquire calls.", nil, nil),
		waiters:     prometheus.NewDesc("app_pgpool_waiters_total", "Acquires that found no idle connection and had to open one or wait.", nil, nil),
		canceled:    prometheus.NewDesc("app_pgpool_canceled_acquires_total", "Acquire calls canceled before completing (context deadline).", nil, nil),
		newConns:    prometheus.NewDesc("app_pgpool_new_conns_total", "Connections established since process start.", nil, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *PoolStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.acquired, c.idle, c.total, c.max, c.construct,
		c.acquires, c.acquireTime, c.waiters, c.canceled, c.newConns} {
		ch <- d
	}
}

// Collect implements prometheus.Collector. A nil snapshot (pool closed) is
// skipped rather than exporting zeros, which would fake recovery.
func (c *PoolStatsCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.stats()
	if s == nil {
		return
	}
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.construct, prometheus.GaugeValue, float64(s.ConstructingConns()))
	ch <- prometheus.MustNewConstMetric(c.acquires, prometheus.CounterValue, float64(s.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireTime, prometheus.CounterValue, s.AcquireDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(c.waiters, prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.canceled, prometheus.CounterValue, float64(s.CanceledAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.newConns, prometheus.CounterValue, float64(s.NewConnsCount()))
}
