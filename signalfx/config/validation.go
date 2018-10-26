package config

import (
	fmt "fmt"
	"net/url"
	"time"

	"istio.io/istio/mixer/pkg/adapter"
)

// Validate the parsed config
func Validate(conf *Params) error {
	var ce *adapter.ConfigErrors
	if conf.AccessToken == "" {
		ce = ce.Appendf("access_token", "You must specify the SignalFx access token")
	}

	if conf.EnableMetrics && len(conf.Metrics) == 0 {
		ce = ce.Appendf("metrics", "There must be at least one metric definition for this to be useful")
	}

	if _, err := url.Parse(conf.IngestUrl); conf.IngestUrl != "" && err != nil {
		ce = ce.Appendf("ingest_url", "Unable to parse url: "+err.Error())
	}

	if conf.DatapointInterval.Round(time.Second) < 1*time.Second {
		ce = ce.Appendf("datapoint_interval", "Interval must not be less than one second")
	}

	for i := range conf.Metrics {
		if conf.Metrics[i].Type == NONE {
			ce = ce.Appendf(fmt.Sprintf("metrics[%d].type", i), "type must be specified")
		}

		name := conf.Metrics[i].Name
		if len(name) == 0 {
			ce = ce.Appendf(fmt.Sprintf("metrics[%d].name", i), "name must not be blank")
			continue
		}
	}

	if conf.EnableTracing {
		if conf.Tracing == nil {
			ce = ce.Appendf("tracing", "must be specified if tracing is enabled")
		}

		if conf.Tracing.SampleProbability < 0 || conf.Tracing.SampleProbability > 1.0 {
			ce = ce.Appendf("tracing.sample_probability", "must be between 0.0 and 1.0 inclusive")
		}

		if conf.Tracing.BufferSize <= 0 {
			ce = ce.Appendf("tracing.buffer_size", "must be greater than 0")
		}

		if conf.Tracing.LocalEndpointNameTagKey == "" {
			ce = ce.Appendf("tracing.local_endpoint_name_tag_key", "must be specified")
		}

		if conf.Tracing.LocalEndpointIpTagKey == "" {
			ce = ce.Appendf("tracing.local_endpoint_ip_tag_key", "must be specified")
		}

		if conf.Tracing.RemoteEndpointNameTagKey == "" {
			ce = ce.Appendf("tracing.remote_endpoint_name_tag_key", "must be specified")
		}

		if conf.Tracing.RemoteEndpointIpTagKey == "" {
			ce = ce.Appendf("tracing.remote_endpoint_ip_tag_key", "must be specified")
		}
	}

	return ce
}
