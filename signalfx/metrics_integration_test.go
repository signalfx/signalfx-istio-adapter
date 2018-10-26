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
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	sfxproto "github.com/signalfx/com_signalfx_metrics_protobuf"
	"github.com/signalfx/golib/trace"

	adapter_integration "istio.io/istio/mixer/pkg/adapter/test"
)

func getConfig(s Server, fakeIngest *fakeSfxIngest) ([]string, error) {
	operatorCfgBytes, err := ioutil.ReadFile("../resources/handler.yaml")
	if err != nil {
		return nil, err
	}
	operatorCfg := string(operatorCfgBytes)

	// Ripped out of the istio adapter integration test code since they don't
	// load tracespan templates by default
	_, filename, _, _ := runtime.Caller(0)
	additionalCrs := []string{
		"../resources/adapter.yaml",
		"../resources/tracespan-template.yaml",
		"../resources/metric-instances.yaml",
		"../resources/tracespan.yaml",
		"../resources/rules-metrics.yaml",
		"../resources/rules-tracing.yaml",
	}

	var data []string
	for _, fileRelativePath := range additionalCrs {
		if f, err := filepath.Abs(path.Join(path.Dir(filename), fileRelativePath)); err != nil {
			return nil, fmt.Errorf("cannot load attributes.yaml: %v", err)
		} else if f, err := ioutil.ReadFile(f); err != nil {
			return nil, fmt.Errorf("cannot load attributes.yaml: %v", err)
		} else {
			data = append(data, string(f))
		}
	}

	return append(data, []string{
		strings.Replace(strings.Replace(operatorCfg, "https://ingest.signalfx.com", fakeIngest.URL, 1), "signalfx-adapter:8080", s.Addr(), 1),
	}...), nil
}

type fakeSfxIngest struct {
	*httptest.Server
	DPs   chan *sfxproto.DataPoint
	Spans chan *trace.Span
}

func (f *fakeSfxIngest) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var body io.ReadCloser
	if req.Header.Get("Content-Encoding") == "gzip" {
		body, _ = gzip.NewReader(req.Body)
	} else {
		body = req.Body
	}
	contents, _ := ioutil.ReadAll(body)
	defer body.Close()

	rw.WriteHeader(http.StatusOK)
	if strings.Contains(req.URL.Path, "datapoint") {
		if n, err := io.WriteString(rw, "\"OK\""); err != nil || n != 4 {
			panic("could not write response back to test client")
		}

		dpUpload := &sfxproto.DataPointUploadMessage{}
		err := proto.Unmarshal(contents, dpUpload)
		if err == nil {
			for i := range dpUpload.Datapoints {
				f.DPs <- dpUpload.Datapoints[i]
			}
		}
	} else {
		var spans []*trace.Span
		err := json.Unmarshal(contents, &spans)
		if err != nil {
			panic("Unable to deserialize span request: " + err.Error())
		}

		for i := range spans {
			f.Spans <- spans[i]
		}

		if _, err := io.WriteString(rw, fmt.Sprintf(`{"valid": %d}`, len(spans))); err != nil {
			panic("could not write response back to test client")
		}
	}
}

func TestReportMetrics(t *testing.T) {
	fakeIngest := &fakeSfxIngest{
		DPs:   make(chan *sfxproto.DataPoint),
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
						"api.protocol":          "http",
						"context.protocol":      "http",
						"context.reporter.kind": "outbound",
						"destination.ip":        []byte(net.ParseIP("10.0.0.2")),
						"destination.name":      "dest1",
						"destination.service":   "dest1",
						"request.headers": map[string]string{
							"x-b3-traceid": "463ac35c9f6413ad48485a3953bb6124",
							"x-b3-spanid":  "a2fb4a1d1a96d312",
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
						"source.service":       "src1",
						"source.workload.name": "src1",
					},
				},
				{
					CallKind: adapter_integration.REPORT,
					Attrs: map[string]interface{}{
						"api.protocol":          "http",
						"context.protocol":      "http",
						"context.reporter.kind": "outbound",
						"destination.ip":        []byte(net.ParseIP("10.0.0.2")),
						"destination.name":      "dest2",
						"destination.service":   "dest2",
						"request.headers": map[string]string{
							"x-b3-traceid": "463ac35c9f6413ad48485a3953bb6124",
							"x-b3-spanid":  "a2fb4a1d1a96d312",
						},
						"request.host":         "example.istio.com",
						"request.method":       "POST",
						"request.path":         "/baz",
						"request.size":         int64(200),
						"request.time":         start,
						"request.useragent":    "xxx",
						"response.code":        int64(200),
						"response.size":        int64(600),
						"response.time":        end,
						"source.ip":            []byte(net.ParseIP("10.0.0.1")),
						"source.labels":        map[string]string{"version": "v1"},
						"source.service":       "src2",
						"source.workload.name": "src2",
					},
				},
			},

			GetState: func(_ interface{}) (interface{}, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				var dps []*sfxproto.DataPoint
				for {
					select {
					case <-ctx.Done():
						cancel()
						return dps, nil
					case dp := <-fakeIngest.DPs:
						// Remove timestamp since it is difficult to match
						// against
						dp.Timestamp = nil
						// Dimensions are a slice of random order so make them
						// predictable
						sort.Slice(dp.Dimensions, func(i, j int) bool {
							return *dp.Dimensions[i].Key <= *dp.Dimensions[j].Key
						})

						dps = append(dps, dp)

						sort.Slice(dps, func(i, j int) bool {
							if *dps[i].Metric != *dps[j].Metric {
								return *dps[i].Metric <= *dps[j].Metric
							}

							return *dps[i].Dimensions[18].Value <= *dps[j].Dimensions[18].Value
						})
						if len(dps) >= 8 {
							cancel()
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
           "dimensions": [
            {
             "key": "connection_mtls",
             "value": "false"
            },
            {
             "key": "destination_app",
             "value": "unknown"
            },
            {
             "key": "destination_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_principal",
             "value": "unknown"
            },
            {
             "key": "destination_service",
             "value": "unknown"
            },
            {
             "key": "destination_service_name",
             "value": "unknown"
            },
            {
             "key": "destination_service_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_version",
             "value": "unknown"
            },
            {
             "key": "destination_workload",
             "value": "unknown"
            },
            {
             "key": "destination_workload_namespace",
             "value": "unknown"
            },
            {
             "key": "monitored_resource_type",
             "value": "UNSPECIFIED"
            },
            {
             "key": "reporter",
             "value": "client"
            },
            {
             "key": "request_protocol",
             "value": "http"
            },
            {
             "key": "response_code",
             "value": "200"
            },
            {
             "key": "source_app",
             "value": "unknown"
            },
            {
             "key": "source_namespace",
             "value": "unknown"
            },
            {
             "key": "source_principal",
             "value": "unknown"
            },
            {
             "key": "source_version",
             "value": "v1"
            },
            {
             "key": "source_workload",
             "value": "src1"
            },
            {
             "key": "source_workload_namespace",
             "value": "unknown"
            }
           ],
           "metric": "requestcount.instance.istio-system",
           "metricType": 3,
           "value": {
            "intValue": 1
           }
          },
          {
           "dimensions": [
            {
             "key": "connection_mtls",
             "value": "false"
            },
            {
             "key": "destination_app",
             "value": "unknown"
            },
            {
             "key": "destination_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_principal",
             "value": "unknown"
            },
            {
             "key": "destination_service",
             "value": "unknown"
            },
            {
             "key": "destination_service_name",
             "value": "unknown"
            },
            {
             "key": "destination_service_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_version",
             "value": "unknown"
            },
            {
             "key": "destination_workload",
             "value": "unknown"
            },
            {
             "key": "destination_workload_namespace",
             "value": "unknown"
            },
            {
             "key": "monitored_resource_type",
             "value": "UNSPECIFIED"
            },
            {
             "key": "reporter",
             "value": "client"
            },
            {
             "key": "request_protocol",
             "value": "http"
            },
            {
             "key": "response_code",
             "value": "200"
            },
            {
             "key": "source_app",
             "value": "unknown"
            },
            {
             "key": "source_namespace",
             "value": "unknown"
            },
            {
             "key": "source_principal",
             "value": "unknown"
            },
            {
             "key": "source_version",
             "value": "v1"
            },
            {
             "key": "source_workload",
             "value": "src2"
            },
            {
             "key": "source_workload_namespace",
             "value": "unknown"
            }
           ],
           "metric": "requestcount.instance.istio-system",
           "metricType": 3,
           "value": {
            "intValue": 1
           }
          },
          {
           "dimensions": [
            {
             "key": "connection_mtls",
             "value": "false"
            },
            {
             "key": "destination_app",
             "value": "unknown"
            },
            {
             "key": "destination_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_principal",
             "value": "unknown"
            },
            {
             "key": "destination_service",
             "value": "unknown"
            },
            {
             "key": "destination_service_name",
             "value": "unknown"
            },
            {
             "key": "destination_service_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_version",
             "value": "unknown"
            },
            {
             "key": "destination_workload",
             "value": "unknown"
            },
            {
             "key": "destination_workload_namespace",
             "value": "unknown"
            },
            {
             "key": "monitored_resource_type",
             "value": "UNSPECIFIED"
            },
            {
             "key": "reporter",
             "value": "client"
            },
            {
             "key": "request_protocol",
             "value": "http"
            },
            {
             "key": "response_code",
             "value": "200"
            },
            {
             "key": "source_app",
             "value": "unknown"
            },
            {
             "key": "source_namespace",
             "value": "unknown"
            },
            {
             "key": "source_principal",
             "value": "unknown"
            },
            {
             "key": "source_version",
             "value": "v1"
            },
            {
             "key": "source_workload",
             "value": "src1"
            },
            {
             "key": "source_workload_namespace",
             "value": "unknown"
            }
           ],
           "metric": "requestduration.instance.istio-system",
           "metricType": 3,
           "value": {
            "intValue": 0
           }
          },
          {
           "dimensions": [
            {
             "key": "connection_mtls",
             "value": "false"
            },
            {
             "key": "destination_app",
             "value": "unknown"
            },
            {
             "key": "destination_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_principal",
             "value": "unknown"
            },
            {
             "key": "destination_service",
             "value": "unknown"
            },
            {
             "key": "destination_service_name",
             "value": "unknown"
            },
            {
             "key": "destination_service_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_version",
             "value": "unknown"
            },
            {
             "key": "destination_workload",
             "value": "unknown"
            },
            {
             "key": "destination_workload_namespace",
             "value": "unknown"
            },
            {
             "key": "monitored_resource_type",
             "value": "UNSPECIFIED"
            },
            {
             "key": "reporter",
             "value": "client"
            },
            {
             "key": "request_protocol",
             "value": "http"
            },
            {
             "key": "response_code",
             "value": "200"
            },
            {
             "key": "source_app",
             "value": "unknown"
            },
            {
             "key": "source_namespace",
             "value": "unknown"
            },
            {
             "key": "source_principal",
             "value": "unknown"
            },
            {
             "key": "source_version",
             "value": "v1"
            },
            {
             "key": "source_workload",
             "value": "src2"
            },
            {
             "key": "source_workload_namespace",
             "value": "unknown"
            }
           ],
           "metric": "requestduration.instance.istio-system",
           "metricType": 3,
           "value": {
            "intValue": 0
           }
          },
          {
           "dimensions": [
            {
             "key": "connection_mtls",
             "value": "false"
            },
            {
             "key": "destination_app",
             "value": "unknown"
            },
            {
             "key": "destination_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_principal",
             "value": "unknown"
            },
            {
             "key": "destination_service",
             "value": "unknown"
            },
            {
             "key": "destination_service_name",
             "value": "unknown"
            },
            {
             "key": "destination_service_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_version",
             "value": "unknown"
            },
            {
             "key": "destination_workload",
             "value": "unknown"
            },
            {
             "key": "destination_workload_namespace",
             "value": "unknown"
            },
            {
             "key": "monitored_resource_type",
             "value": "UNSPECIFIED"
            },
            {
             "key": "reporter",
             "value": "client"
            },
            {
             "key": "request_protocol",
             "value": "http"
            },
            {
             "key": "response_code",
             "value": "200"
            },
            {
             "key": "source_app",
             "value": "unknown"
            },
            {
             "key": "source_namespace",
             "value": "unknown"
            },
            {
             "key": "source_principal",
             "value": "unknown"
            },
            {
             "key": "source_version",
             "value": "v1"
            },
            {
             "key": "source_workload",
             "value": "src1"
            },
            {
             "key": "source_workload_namespace",
             "value": "unknown"
            }
           ],
           "metric": "requestsize.instance.istio-system",
           "metricType": 3,
           "value": {
            "intValue": 128
           }
          },
          {
           "dimensions": [
            {
             "key": "connection_mtls",
             "value": "false"
            },
            {
             "key": "destination_app",
             "value": "unknown"
            },
            {
             "key": "destination_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_principal",
             "value": "unknown"
            },
            {
             "key": "destination_service",
             "value": "unknown"
            },
            {
             "key": "destination_service_name",
             "value": "unknown"
            },
            {
             "key": "destination_service_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_version",
             "value": "unknown"
            },
            {
             "key": "destination_workload",
             "value": "unknown"
            },
            {
             "key": "destination_workload_namespace",
             "value": "unknown"
            },
            {
             "key": "monitored_resource_type",
             "value": "UNSPECIFIED"
            },
            {
             "key": "reporter",
             "value": "client"
            },
            {
             "key": "request_protocol",
             "value": "http"
            },
            {
             "key": "response_code",
             "value": "200"
            },
            {
             "key": "source_app",
             "value": "unknown"
            },
            {
             "key": "source_namespace",
             "value": "unknown"
            },
            {
             "key": "source_principal",
             "value": "unknown"
            },
            {
             "key": "source_version",
             "value": "v1"
            },
            {
             "key": "source_workload",
             "value": "src2"
            },
            {
             "key": "source_workload_namespace",
             "value": "unknown"
            }
           ],
           "metric": "requestsize.instance.istio-system",
           "metricType": 3,
           "value": {
            "intValue": 200
           }
          },
          {
           "dimensions": [
            {
             "key": "connection_mtls",
             "value": "false"
            },
            {
             "key": "destination_app",
             "value": "unknown"
            },
            {
             "key": "destination_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_principal",
             "value": "unknown"
            },
            {
             "key": "destination_service",
             "value": "unknown"
            },
            {
             "key": "destination_service_name",
             "value": "unknown"
            },
            {
             "key": "destination_service_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_version",
             "value": "unknown"
            },
            {
             "key": "destination_workload",
             "value": "unknown"
            },
            {
             "key": "destination_workload_namespace",
             "value": "unknown"
            },
            {
             "key": "monitored_resource_type",
             "value": "UNSPECIFIED"
            },
            {
             "key": "reporter",
             "value": "client"
            },
            {
             "key": "request_protocol",
             "value": "http"
            },
            {
             "key": "response_code",
             "value": "200"
            },
            {
             "key": "source_app",
             "value": "unknown"
            },
            {
             "key": "source_namespace",
             "value": "unknown"
            },
            {
             "key": "source_principal",
             "value": "unknown"
            },
            {
             "key": "source_version",
             "value": "v1"
            },
            {
             "key": "source_workload",
             "value": "src1"
            },
            {
             "key": "source_workload_namespace",
             "value": "unknown"
            }
           ],
           "metric": "responsesize.instance.istio-system",
           "metricType": 3,
           "value": {
            "intValue": 512
           }
          },
          {
           "dimensions": [
            {
             "key": "connection_mtls",
             "value": "false"
            },
            {
             "key": "destination_app",
             "value": "unknown"
            },
            {
             "key": "destination_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_principal",
             "value": "unknown"
            },
            {
             "key": "destination_service",
             "value": "unknown"
            },
            {
             "key": "destination_service_name",
             "value": "unknown"
            },
            {
             "key": "destination_service_namespace",
             "value": "unknown"
            },
            {
             "key": "destination_version",
             "value": "unknown"
            },
            {
             "key": "destination_workload",
             "value": "unknown"
            },
            {
             "key": "destination_workload_namespace",
             "value": "unknown"
            },
            {
             "key": "monitored_resource_type",
             "value": "UNSPECIFIED"
            },
            {
             "key": "reporter",
             "value": "client"
            },
            {
             "key": "request_protocol",
             "value": "http"
            },
            {
             "key": "response_code",
             "value": "200"
            },
            {
             "key": "source_app",
             "value": "unknown"
            },
            {
             "key": "source_namespace",
             "value": "unknown"
            },
            {
             "key": "source_principal",
             "value": "unknown"
            },
            {
             "key": "source_version",
             "value": "v1"
            },
            {
             "key": "source_workload",
             "value": "src2"
            },
            {
             "key": "source_workload_namespace",
             "value": "unknown"
            }
           ],
           "metric": "responsesize.instance.istio-system",
           "metricType": 3,
           "value": {
            "intValue": 600
           }
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
              }
             ]
             }`,
		},
	)
}
