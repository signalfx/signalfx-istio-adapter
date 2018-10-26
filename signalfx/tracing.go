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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/golang/glog"
	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/pointer"
	"github.com/signalfx/golib/sfxclient"
	"github.com/signalfx/golib/trace"
	"github.com/signalfx/signalfx-istio-adapter/signalfx/config"
	octrace "go.opencensus.io/trace"

	mixer_v1beta1 "istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/pkg/adapter"
	"istio.io/istio/mixer/template/tracespan"
)

// How long to wait for a response from ingest when sending spans.  New spans will
// buffer during the round trip so we shouldn't wait too long.
const spanSendTimeout = 8 * time.Second

type traceSpanHandler struct {
	ctx      context.Context
	cancel   context.CancelFunc
	sink     *sfxclient.HTTPSink
	spanChan chan *tracespan.InstanceMsg
	sampler  octrace.Sampler
	conf     *config.Params_TracingConfig
}

func createTracingHandler(conf *config.Params) (*traceSpanHandler, error) {
	var traceSpanHandlerInst *traceSpanHandler
	if conf.EnableTracing {
		traceSpanHandlerInst = &traceSpanHandler{
			sink: createSignalFxClient(conf),
			conf: conf.Tracing,
		}

		if err := traceSpanHandlerInst.InitTracing(int(conf.Tracing.BufferSize), conf.Tracing.SampleProbability); err != nil {
			return nil, err
		}
	}
	return traceSpanHandlerInst, nil
}

func (th *traceSpanHandler) InitTracing(bufferLen int, sampleProbability float64) error {
	th.ctx, th.cancel = context.WithCancel(context.Background())

	th.spanChan = make(chan *tracespan.InstanceMsg, bufferLen)

	// Use the OpenCensus span sampler even though we don't use its wire
	// format.
	th.sampler = octrace.ProbabilitySampler(sampleProbability)

	go th.sendTraces()
	return nil
}

func (th *traceSpanHandler) Shutdown() {
	if th.cancel != nil {
		th.cancel()
	}
}

// Pull spans out of spanChan as they come in and send them to SignalFx.
func (th *traceSpanHandler) sendTraces() {
	for {
		select {
		case <-th.ctx.Done():
			return
		case inst := <-th.spanChan:
			allInsts := th.drainChannel(inst)

			spans := make([]*trace.Span, 0, len(allInsts))
			for i := range allInsts {
				span := th.convertInstance(allInsts[i])
				if span.ID == "" {
					continue
				}
				spans = append(spans, span)
			}

			ctx, cancel := context.WithTimeout(th.ctx, spanSendTimeout)
			err := th.sink.AddSpans(ctx, spans)
			cancel()
			if err != nil {
				glog.Errorf("Could not send spans: %s", err.Error())
			}
		}
	}
}

// Pull spans out of the spanChan until it is empty.  This helps cut down on
// the number of requests to ingest by batching things when traces come in
// fast.
func (th *traceSpanHandler) drainChannel(initial *tracespan.InstanceMsg) (out []*tracespan.InstanceMsg) {
	out = []*tracespan.InstanceMsg{initial}
	for {
		select {
		case inst := <-th.spanChan:
			out = append(out, inst)
		default:
			return out
		}
	}
}

func (th *traceSpanHandler) HandleTraceSpan(ctx context.Context, req *tracespan.HandleTraceSpanRequest) (*mixer_v1beta1.ReportResult, error) {
	if th == nil {
		return nil, nil
	}

	for i := range req.Instances {
		span := req.Instances[i]
		if !th.shouldSend(span) {
			continue
		}

		select {
		case th.spanChan <- span:
			continue
		default:
			// Just abandon any remaining spans in `values` at this point to
			// help relieve pressure on the buffer
			return nil, errors.New("dropping span because trace buffer is full -- you should increase the capacity of it")
		}
	}
	return &mixer_v1beta1.ReportResult{}, nil
}

func (th *traceSpanHandler) shouldSend(span *tracespan.InstanceMsg) bool {
	parentContext, ok := adapter.ExtractParentContext(span.TraceId, span.ParentSpanId)
	if !ok {
		return false
	}
	spanContext, ok := adapter.ExtractSpanContext(span.SpanId, parentContext)
	if !ok {
		return false
	}

	params := octrace.SamplingParameters{
		ParentContext:   parentContext,
		TraceID:         spanContext.TraceID,
		SpanID:          spanContext.SpanID,
		Name:            span.SpanName,
		HasRemoteParent: true,
	}
	return th.sampler(params).Sample
}

var (
	clientKind = "CLIENT"
	serverKind = "SERVER"
)

// Converts an istio span to a SignalFx span, which is currently equivalent to
// a Zipkin V2 span.
func (th *traceSpanHandler) convertInstance(istioSpan *tracespan.InstanceMsg) *trace.Span {
	startTime, err := types.TimestampFromProto(istioSpan.StartTime.GetValue())
	if err != nil {
		glog.Error("Couldn't convert span start time: ", err)
		return nil
	}
	endTime, err := types.TimestampFromProto(istioSpan.EndTime.GetValue())

	kind := &serverKind
	if istioSpan.ClientSpan {
		kind = &clientKind
	}

	tags := map[string]string{}

	// Special handling for the span name since the Istio attribute that is the
	// most suited for the span name is request.path which can contain query
	// params which could cause high cardinality of the span name.
	spanName := istioSpan.SpanName
	if strings.Contains(spanName, "?") {
		idx := strings.LastIndex(spanName, "?")
		qs := spanName[idx+1:]
		if vals, err := url.ParseQuery(qs); err == nil {
			for k, v := range vals {
				tags[k] = strings.Join(v, "; ")
			}
		}

		spanName = spanName[:idx]
	}

	for k, v := range istioSpan.SpanTags {
		tags[k] = adapter.Stringify(decodeValue(v))
	}

	if istioSpan.HttpStatusCode != 0 {
		tags["http.status_code"] = strconv.FormatInt(istioSpan.HttpStatusCode, 10)
		if istioSpan.HttpStatusCode >= 500 {
			tags["error"] = "server error"
		}
	}

	span := &trace.Span{
		ID:             istioSpan.SpanId,
		Name:           &spanName,
		TraceID:        istioSpan.TraceId,
		Kind:           kind,
		Timestamp:      pointer.Float64(float64(startTime.UnixNano()) / 1000),
		Duration:       pointer.Float64(float64(endTime.UnixNano()-startTime.UnixNano()) / 1000),
		Tags:           tags,
		LocalEndpoint:  &trace.Endpoint{},
		RemoteEndpoint: &trace.Endpoint{},
	}

	if n, ok := decodeValue(istioSpan.SpanTags[th.conf.LocalEndpointNameTagKey]).(string); ok && n != "" {
		span.LocalEndpoint.ServiceName = &n
	}

	if ip, ok := decodeValue(istioSpan.SpanTags[th.conf.LocalEndpointIpTagKey]).(net.IP); ok {
		ips := ip.String()
		span.LocalEndpoint.Ipv4 = &ips
	}

	if n, ok := decodeValue(istioSpan.SpanTags[th.conf.RemoteEndpointNameTagKey]).(string); ok && n != "" {
		span.RemoteEndpoint.ServiceName = &n
	}

	if ip, ok := decodeValue(istioSpan.SpanTags[th.conf.RemoteEndpointIpTagKey]).(net.IP); ok {
		ips := ip.String()
		span.RemoteEndpoint.Ipv4 = &ips
	}

	if istioSpan.ParentSpanId != "" {
		span.ParentID = &istioSpan.ParentSpanId
	}

	return span
}
