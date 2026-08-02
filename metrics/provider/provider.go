package provider

import "github.com/prometheus/client_golang/prometheus"

type Empty struct{}

func (e *Empty) GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{}
}
