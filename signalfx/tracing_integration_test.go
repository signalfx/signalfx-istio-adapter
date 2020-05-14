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

package signalfx

import (
	"context"
	"net"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/signalfx/golib/trace"

	adapter_integration "istio.io/istio/mixer/pkg/adapter/test"
)

func TestReportTraces(t *testing.T) {
	fakeIngest := &fakeSfxIngest{
		Spans: make(chan *trace.Span, 3),
	}
	fakeIngest.Server = httptest.NewServer(fakeIngest)

	shutdown := make(chan error, 1)

	end := time.Unix(1000, 0)
	start := end.Add(-100 * time.Millisecond)

	adapter_integration.RunTest(
		t,
		nil,
		adapter_integration.Scenario{
			Setup: func() (ctx interface{}, err error) {
				pServer, err := NewAdapter("")
				if err != nil {
					return nil, err
				}
				go func() {
					pServer.Run(shutdown)
					_ = <-shutdown
				}()
				return pServer, nil
			},
			Teardown: func(ctx interface{}) {
				s := ctx.(Server)
				s.Close()
			},
			ParallelCalls: []adapter_integration.Call{
				{
					CallKind: adapter_integration.REPORT,
					Attrs: map[string]interface{}{
						"api.protocol":              "http",
						"context.protocol":          "http",
						"context.reporter.kind":     "outbound",
						"destination.ip":            []byte(net.ParseIP("10.0.0.2")),
						"destination.name":          "destsvc",
						"destination.service":       "destsvc",
						"destination.workload.name": "destsvcworkload",
						"request.headers": map[string]string{
							"x-b3-traceid": "463ac35c9f6413ad48485a3953bb6124",
							"x-b3-spanid":  "fedcba9876543210",
						},
						"request.host":         "example.istio.com",
						"request.method":       "POST",
						"request.path":         "/foo/bar",
						"request.size":         int64(128),
						"request.time":         start,
						"request.useragent":    "xxx",
						"response.code":        int64(200),
						"response.size":        int64(512),
						"response.time":        end,
						"source.ip":            []byte(net.ParseIP("10.0.0.1")),
						"source.labels":        map[string]string{"version": "v1"},
						"source.name":          "srcsvc",
						"source.service":       "srcsvc",
						"source.workload.name": "srcsvcworkload",
					},
				},
				{
					CallKind: adapter_integration.REPORT,
					Attrs: map[string]interface{}{
						"api.protocol":          "http",
						"context.protocol":      "http",
						"context.reporter.kind": "inbound",
						"destination.ip":        []byte(net.ParseIP("10.0.0.3")),
						"destination.service":   "destsvc",
						"request.headers": map[string]string{
							"x-b3-traceid":      "463ac35c9f6413ad48485a3953bb6124",
							"x-b3-spanid":       "b3a9b83bb2b3098f",
							"x-b3-parentspanid": "fedcba9876543210",
						},
						"request.host":      "example.istio.com",
						"request.method":    "POST",
						"request.path":      "/bar/baz",
						"request.size":      int64(128),
						"request.time":      start.Add(5 * time.Millisecond),
						"request.useragent": "xxx",
						"response.code":     int64(200),
						"response.size":     int64(512),
						"response.time":     end.Add(10 * time.Millisecond),
						"source.ip":         []byte(net.ParseIP("10.0.0.2")),
						"source.labels":     map[string]string{"version": "v1"},
						"source.service":    "srcsvc",
					},
				},
				{
					CallKind: adapter_integration.REPORT,
					Attrs: map[string]interface{}{
						"api.protocol":          "http",
						"context.protocol":      "http",
						"context.reporter.kind": "outbound",
						"destination.ip":        []byte(net.ParseIP("10.0.0.3")),
						"destination.service":   "destsvc",
						"request.headers": map[string]string{
							"x-b3-traceid":      "463ac35c9f6413ad48485a3953bb6124",
							"x-b3-spanid":       "abcdef0123456789",
							"x-b3-parentspanid": "fedcba9876543210",
						},
						"request.host":      "example.istio.com",
						"request.method":    "POST",
						"request.path":      "/bar/baz?q=whatever",
						"request.size":      int64(128),
						"request.time":      start.Add(6 * time.Millisecond),
						"request.useragent": "xxx",
						"response.code":     int64(500),
						"response.size":     int64(512),
						"response.time":     end.Add(11 * time.Millisecond),
						"source.ip":         []byte(net.ParseIP("10.0.0.2")),
						"source.service":    "srcsvc",
					},
				},
			},

			GetState: func(_ interface{}) (interface{}, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				var spans []*trace.Span
				for {
					select {
					case <-ctx.Done():
						cancel()
						return spans, nil
					case sp := <-fakeIngest.Spans:
						spans = append(spans, sp)
						sort.Slice(spans, func(i, j int) bool {
							return *spans[i].Timestamp < *spans[j].Timestamp
						})
						if len(spans) >= 3 {
							cancel()
							return spans, nil
						}
					}
				}
			},

			GetConfig: func(ctx interface{}) ([]string, error) {
				return getConfig(ctx.(Server), fakeIngest)
			},
			Want: `
            {
             "AdapterState": [
                  {
                   "annotations": null,
                   "debug": null,
                   "duration": 100000,
                   "id": "fedcba9876543210",
                   "kind": "CLIENT",
                   "localEndpoint": {
                    "ipv4": "10.0.0.1",
                    "ipv6": null,
                    "port": null,
                    "serviceName": "srcsvcworkload"
                   },
                   "name": "http.request",
                   "parentId": null,
                   "remoteEndpoint": {
                    "ipv4": "10.0.0.2",
                    "ipv6": null,
                    "port": null,
                    "serviceName": "destsvcworkload"
                   },
                   "shared": null,
                   "tags": {
                     "destination.ip": "10.0.0.2",
                     "destination.name": "destsvc",
                     "destination.namespace": "unknown",
                     "destination.workload.name": "destsvcworkload",
                     "http.status_code": "200",
                     "request.host": "example.istio.com",
                     "request.method": "POST",
                     "request.path": "/foo/bar",
                     "request.size": "128",
                     "request.useragent": "xxx",
                     "response.size": "512",
                     "source.ip": "10.0.0.1",
                     "source.name": "srcsvc",
                     "source.namespace": "unknown",
                     "source.version": "v1",
                     "source.workload.name": "srcsvcworkload"
                   },
                   "timestamp": 999900000,
                   "traceId": "463ac35c9f6413ad48485a3953bb6124"
                  },
                  {
                   "annotations": null,
                   "debug": null,
                   "duration": 105000,
                   "id": "b3a9b83bb2b3098f",
                   "kind": "SERVER",
                   "localEndpoint": {
                    "ipv4": "10.0.0.3",
                    "ipv6": null,
                    "port": null,
                    "serviceName": "unknown"
                   },
                   "name": "http.request",
                   "parentId": "fedcba9876543210",
                   "remoteEndpoint": {
                    "ipv4": "10.0.0.2",
                    "ipv6": null,
                    "port": null,
                    "serviceName": "unknown"
                   },
                   "shared": null,
                   "tags": {
                     "destination.ip": "10.0.0.3",
                     "destination.name": "unknown",
                     "destination.namespace": "unknown",
                     "destination.workload.name": "unknown",
                     "http.status_code": "200",
                     "request.host": "example.istio.com",
                     "request.method": "POST",
                     "request.path": "/bar/baz",
                     "request.size": "128",
                     "request.useragent": "xxx",
                     "response.size": "512",
                     "source.ip": "10.0.0.2",
                     "source.name": "unknown",
                     "source.namespace": "unknown",
                     "source.version": "v1",
                     "source.workload.name": "unknown"
                   },
                   "timestamp": 999905000,
                   "traceId": "463ac35c9f6413ad48485a3953bb6124"
                  },
                  {
                   "annotations": null,
                   "debug": null,
                   "duration": 105000,
                   "id": "abcdef0123456789",
                   "kind": "CLIENT",
                   "localEndpoint": {
                    "ipv4": "10.0.0.2",
                    "ipv6": null,
                    "port": null,
                    "serviceName": "unknown"
                   },
                   "name": "http.request",
                   "parentId": "fedcba9876543210",
                   "remoteEndpoint": {
                    "ipv4": "10.0.0.3",
                    "ipv6": null,
                    "port": null,
                    "serviceName": "unknown"
                   },
                   "shared": null,
                   "tags": {
                     "destination.ip": "10.0.0.3",
                     "destination.name": "unknown",
                     "destination.namespace": "unknown",
                     "destination.workload.name": "unknown",
                     "error": "server error",
                     "http.status_code": "500",
                     "request.host": "example.istio.com",
                     "request.method": "POST",
                     "request.path": "/bar/baz?q=whatever",
                     "request.size": "128",
                     "request.useragent": "xxx",
                     "response.size": "512",
                     "source.ip": "10.0.0.2",
                     "source.name": "unknown",
                     "source.namespace": "unknown",
                     "source.version": "unknown",
                     "source.workload.name": "unknown"
                   },
                   "timestamp": 999906000,
                   "traceId": "463ac35c9f6413ad48485a3953bb6124"
                  }
             ],
             "Returns": [
              {
               "Check": {
                "Status": {},
                "ValidDuration": 0,
                "ValidUseCount": 0
               },
               "Error": null,
               "Quota": null
              },
              {
               "Check": {
                "Status": {},
                "ValidDuration": 0,
                "ValidUseCount": 0
               },
               "Error": null,
               "Quota": null
              },
              {
               "Check": {
                "Status": {},
                "ValidDuration": 0,
                "ValidUseCount": 0
               },
               "Error": null,
               "Quota": null
              }
             ]
             }`,
		},
	)
}
