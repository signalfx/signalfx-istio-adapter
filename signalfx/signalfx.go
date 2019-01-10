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

// nolint: lll
//go:generate bash -c "cd $GOPATH/src/istio.io/istio/mixer/adapter && test -d ./signalfx && rm -rf signalfx-old && mv signalfx signalfx-old"
//go:generate cp -r $PWD/signalfx $GOPATH/src/istio.io/istio/mixer/adapter/signalfx
//go:generate $GOPATH/src/istio.io/istio/bin/mixer_codegen.sh -a mixer/adapter/signalfx/config/config.proto -x "-s=false -n signalfx -t metric -t tracespan"
//go:generate bash -c "cp $GOPATH/src/istio.io/istio/mixer/adapter/signalfx/config/* $PWD/signalfx/config/"

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	google_protobuf2 "github.com/gogo/protobuf/types"
	"github.com/golang/glog"
	"github.com/signalfx/golib/sfxclient"
	"github.com/signalfx/signalfx-istio-adapter/signalfx/config"
	"google.golang.org/grpc"
	mixer_v1beta1 "istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/template/metric"
	"istio.io/istio/mixer/template/tracespan"
)

type sessionID string

// Server is basic server interface
type Server interface {
	Addr() string
	Close() error
	Run(shutdown chan error)
}

// Adapter is the SignalFx GRCP adapter server
type Adapter struct {
	sync.Mutex
	listener          net.Listener
	server            *grpc.Server
	metricHandlers    map[sessionID]*metricHandler
	traceSpanHandlers map[sessionID]*traceSpanHandler
	cancel            context.CancelFunc
}

// ensure types implement the requisite interfaces
var _ metric.HandleMetricServiceServer = &Adapter{}
var _ tracespan.HandleTraceSpanServiceServer = &Adapter{}

func parseConfig(rawConfig *google_protobuf2.Any) (*config.Params, error) {
	cfg := &config.Params{
		EnableMetrics:     true,
		EnableTracing:     true,
		DatapointInterval: 10 * time.Second,
		Tracing: &config.Params_TracingConfig{
			BufferSize:        1000,
			SampleProbability: 1.0,
		},
	}
	err := cfg.Unmarshal(rawConfig.Value)
	return cfg, err
}

func createSignalFxClient(conf *config.Params) *sfxclient.HTTPSink {
	sink := sfxclient.NewHTTPSink()
	sink.AuthToken = conf.AccessToken
	if sink.AuthToken == "" {
		sink.AuthToken = os.Getenv(config.AccessTokenEnvvar)
	}

	if conf.IngestUrl != "" {
		sink.DatapointEndpoint = fmt.Sprintf("%s/v2/datapoint", conf.IngestUrl)
		sink.TraceEndpoint = fmt.Sprintf("%s/v1/trace", conf.IngestUrl)
	}

	return sink
}

// Session ID is the hash of the config until proper session based adapters
// are supported
func hashOfConfig(confValue []byte) sessionID {
	hash := md5.Sum(confValue)
	return sessionID(hex.EncodeToString(hash[:]))
}

// HandleMetric accepts metric values from Istio and delegates to the proper
// session handler if applicable
func (s *Adapter) HandleMetric(ctx context.Context, req *metric.HandleMetricRequest) (*mixer_v1beta1.ReportResult, error) {
	if req.AdapterConfig == nil {
		return nil, fmt.Errorf("no adapter config received")
	}

	// TODO: switch this to use real session id from the config once
	// implemented
	sessID := hashOfConfig(req.AdapterConfig.Value)

	s.Lock()
	if s.metricHandlers[sessID] == nil {
		conf, err := parseConfig(req.AdapterConfig)
		if err != nil {
			glog.Errorf("Error parsing adapter config: %v", err)
			s.Unlock()
			return nil, fmt.Errorf("could not parse adapter config: %v", err)
		}

		glog.Infof("Initializing metric session %s", sessID)
		s.metricHandlers[sessID], err = createMetricHandler(conf)
	}
	handler := s.metricHandlers[sessID]
	s.Unlock()

	return handler.HandleMetric(ctx, req)
}

// HandleTraceSpan accepts trace spans from the Istio telemetry server and
// forwards them to SignalFx.  This method delegates to the proper session
// handler.
func (s *Adapter) HandleTraceSpan(ctx context.Context, req *tracespan.HandleTraceSpanRequest) (*mixer_v1beta1.ReportResult, error) {
	if req.AdapterConfig == nil {
		return nil, fmt.Errorf("no adapter config received")
	}

	// TODO: switch this to use real session id from the config once
	// implemented
	sessID := hashOfConfig(req.AdapterConfig.Value)

	s.Lock()
	if s.traceSpanHandlers[sessID] == nil {
		conf, err := parseConfig(req.AdapterConfig)
		if err != nil {
			glog.Errorf("Error parsing adapter config: %v", err)
			s.Unlock()
			return nil, fmt.Errorf("could not parse adapter config: %v", err)
		}

		glog.Infof("Initializing traceSpan session %s", sessID)
		s.traceSpanHandlers[sessID], err = createTracingHandler(conf)
	}
	handler := s.traceSpanHandlers[sessID]
	s.Unlock()

	return handler.HandleTraceSpan(ctx, req)
}

// Addr returns the listening address of the server
func (s *Adapter) Addr() string {
	return s.listener.Addr().String()
}

// Run starts the server run
func (s *Adapter) Run(shutdown chan error) {
	shutdown <- s.server.Serve(s.listener)
}

// Close gracefully shuts down the server; used for testing
func (s *Adapter) Close() error {
	if s.server != nil {
		s.server.GracefulStop()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	return nil
}

// NewAdapter creates a new IBP adapter that listens at provided port.
func NewAdapter(port string) (Server, error) {
	if port == "" {
		port = "8080"
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", port))
	if err != nil {
		return nil, fmt.Errorf("unable to listen on socket: %v", err)
	}
	s := &Adapter{
		listener:          listener,
		metricHandlers:    map[sessionID]*metricHandler{},
		traceSpanHandlers: map[sessionID]*traceSpanHandler{},
	}
	glog.Infof("listening on \"%v\"\n", s.Addr())
	s.server = grpc.NewServer()

	metric.RegisterHandleMetricServiceServer(s.server, s)
	tracespan.RegisterHandleTraceSpanServiceServer(s.server, s)
	//mixer_v1beta1.RegisterInfrastructureBackendServer(s.server, s)

	return s, nil
}
