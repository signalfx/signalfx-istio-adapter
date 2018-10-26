package signalfx

import (
	"context"
	"fmt"

	google_rpc "github.com/gogo/googleapis/google/rpc"
	"github.com/golang/glog"
	"github.com/signalfx/signalfx-istio-adapter/signalfx/config"
	mixer_v1beta1 "istio.io/api/mixer/adapter/model/v1beta1"
)

var _ mixer_v1beta1.InfrastructureBackendServer = &Adapter{}

// Validate is called by the mixer to ensure that the config is valid
func (s *Adapter) Validate(ctx context.Context, req *mixer_v1beta1.ValidateRequest) (*mixer_v1beta1.ValidateResponse, error) {
	conf, err := parseConfig(req.AdapterConfig)
	if err != nil {
		glog.Errorf("Error parsing adapter config: %s", err.Error())
		return nil, fmt.Errorf("could not parse adapter config: %s", err.Error())
	}

	if err := config.Validate(conf); err != nil {
		return nil, err
	}

	return &mixer_v1beta1.ValidateResponse{
		Status: &google_rpc.Status{
			Code: int32(google_rpc.OK),
		},
	}, nil
}

// CreateSession is called when the mixer wants a new session.
func (s *Adapter) CreateSession(ctx context.Context, req *mixer_v1beta1.CreateSessionRequest) (*mixer_v1beta1.CreateSessionResponse, error) {
	conf, err := parseConfig(req.AdapterConfig)
	if err != nil {
		glog.Errorf("Error parsing adapter config: %v", err)
		return nil, fmt.Errorf("could not parse adapter config: %v", err)
	}

	sessID, err := randomHex(16)
	if err != nil {
		return nil, err
	}

	metricHandlerInst, err := createMetricHandler(conf)
	if err != nil {
		return nil, err
	}

	traceSpanHandlerInst, err := createTracingHandler(conf)
	if err != nil {
		return nil, err
	}

	s.metricHandlers[sessionID(sessID)] = metricHandlerInst
	s.traceSpanHandlers[sessionID(sessID)] = traceSpanHandlerInst

	glog.Infof("Creating session %s", sessID)

	return &mixer_v1beta1.CreateSessionResponse{
		SessionId: sessID,
		Status: &google_rpc.Status{
			Code: int32(google_rpc.OK),
		},
	}, nil
}

// CloseSession is called by the mixer when a session is done and should be
// cleaned up
func (s *Adapter) CloseSession(ctx context.Context, req *mixer_v1beta1.CloseSessionRequest) (*mixer_v1beta1.CloseSessionResponse, error) {
	sessID := sessionID(req.GetSessionId())

	if h, ok := s.metricHandlers[sessID]; ok {
		h.Shutdown()
		delete(s.metricHandlers, sessID)
	}
	if h, ok := s.traceSpanHandlers[sessID]; ok {
		h.Shutdown()
		delete(s.traceSpanHandlers, sessID)
	}

	glog.Infof("Closed session %s", sessID)

	return &mixer_v1beta1.CloseSessionResponse{
		Status: &google_rpc.Status{
			Code: int32(google_rpc.OK),
		},
	}, nil
}
