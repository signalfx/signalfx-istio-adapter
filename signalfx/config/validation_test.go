// Copyright 2018 Istio Authors
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

package config

import (
	"testing"
	"time"

	"istio.io/api/policy/v1beta1"
	"istio.io/istio/mixer/pkg/adapter"
	"istio.io/istio/mixer/template/metric"
)

var (
	metrics = map[string]*metric.Type{
		"bytes_sent": {
			Value: v1beta1.INT64,
		},
		"error_count": {
			Value: v1beta1.INT64,
		},
	}

	badMetrics = map[string]*metric.Type{
		"service_status": {
			Value: v1beta1.STRING,
		},
		"error_count": {
			Value: v1beta1.INT64,
		},
	}
)

func TestValidation(t *testing.T) {
	tests := []struct {
		name          string
		metricTypes   map[string]*metric.Type
		conf          Params
		expectedError *adapter.ConfigError
	}{
		{
			"Valid Config",
			metrics,
			Params{
				AccessToken:       "abcd",
				DatapointInterval: 5 * time.Second,
				Tracing: &Params_TracingConfig{
					BufferSize:               1000,
					LocalEndpointNameTagKey:  "a",
					LocalEndpointIpTagKey:    "b",
					RemoteEndpointNameTagKey: "a",
					RemoteEndpointIpTagKey:   "b",
				},
				Metrics: []*Params_MetricConfig{
					{
						Name: "bytes_sent",
						Type: COUNTER,
					},
					{
						Name: "error_count",
						Type: COUNTER,
					},
				},
			}, nil},
		{"No access token", metrics, Params{}, &adapter.ConfigError{Field: "access_token", Underlying: nil}},
		{
			"Malformed ingest URI",
			metrics,
			Params{
				IngestUrl: "a;//asdf$%^:abb",
			},
			&adapter.ConfigError{Field: "ingest_url", Underlying: nil}},
		{"No metrics", metrics, Params{EnableMetrics: true}, &adapter.ConfigError{Field: "metrics", Underlying: nil}},
		{
			"Unknown SignalFx type",
			metrics,
			Params{
				AccessToken: "abcd",
				Metrics: []*Params_MetricConfig{
					{
						Name: "bytes_sent",
						Type: NONE,
					},
				},
			}, &adapter.ConfigError{Field: "metrics[0].type", Underlying: nil}},
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			err := Validate(&v.conf)

			if v.expectedError == nil {
				if err != (*adapter.ConfigErrors)(nil) {
					t.Fatalf("Validate() should not have produced this error: %v", err)
				}
				return
			}

			errFound := false
			for _, ce := range err.(*adapter.ConfigErrors).Multi.WrappedErrors() {
				if ce.(adapter.ConfigError).Field == v.expectedError.Field {
					if v.expectedError.Underlying == nil ||
						ce.(adapter.ConfigError).Underlying.Error() == v.expectedError.Underlying.Error() {
						errFound = true
					}
				}
			}

			if !errFound {
				t.Fatalf("Validate() did not produce expected error %v\nbut did produce: %v", v.expectedError, err)
			}
		})
	}
}
