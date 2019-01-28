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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gogo/protobuf/types"
	"github.com/golang/glog"
	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/pointer"
	"github.com/signalfx/golib/sfxclient"
	"github.com/signalfx/golib/trace"
	"github.com/signalfx/signalfx-istio-adapter/signalfx/config"

	mixer_v1beta1 "istio.io/api/mixer/adapter/model/v1beta1"
	istio_policy_v1beta1 "istio.io/api/policy/v1beta1"
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
	conf     *config.Params_TracingConfig
}

func createTracingHandler(conf *config.Params) (*traceSpanHandler, error) {
	var traceSpanHandlerInst *traceSpanHandler
	if conf.EnableTracing {
		traceSpanHandlerInst = &traceSpanHandler{
			sink: createSignalFxClient(conf),
			conf: conf.Tracing,
		}

		if err := traceSpanHandlerInst.InitTracing(int(conf.Tracing.BufferSize)); err != nil {
			return nil, err
		}
	}
	return traceSpanHandlerInst, nil
}

func (th *traceSpanHandler) InitTracing(bufferLen int) error {
	th.ctx, th.cancel = context.WithCancel(context.Background())

	th.spanChan = make(chan *tracespan.InstanceMsg, bufferLen)

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

		if os.Getenv("SIGNALFX_TRACE_DUMP") == "true" {
			spew.Dump(span, "\n")
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

var (
	clientKind = "CLIENT"
	serverKind = "SERVER"
)

// Converts an istio span to a SignalFx span, which is currently equivalent to
// a Zipkin V2 span.
func (th *traceSpanHandler) convertInstance(istioSpan *tracespan.InstanceMsg) *trace.Span {
	outSpanID := istioSpan.SpanId
	outParentSpanID := istioSpan.ParentSpanId

	// Fix up span ids so that client/server spans don't share the same span
	// ID, if the span has this flag marked.
	if istioSpan.RewriteClientSpanId {
		if istioSpan.ClientSpan {
			outSpanID = deriveNewSpanID(istioSpan.SpanId)
		} else {
			// We need to make the parent of this server span the client span
			// that was rewritten.
			outParentSpanID = deriveNewSpanID(istioSpan.SpanId)
		}
	}

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
		ID:             outSpanID,
		Name:           &spanName,
		TraceID:        istioSpan.TraceId,
		Kind:           kind,
		Timestamp:      pointer.Float64(float64(startTime.UnixNano()) / 1000),
		Duration:       pointer.Float64(float64(endTime.UnixNano()-startTime.UnixNano()) / 1000),
		Tags:           tags,
		LocalEndpoint:  &trace.Endpoint{},
		RemoteEndpoint: &trace.Endpoint{},
	}

	needsEndpointSwap := !istioSpan.ClientSpan && th.conf.SwapLocalRemoteEndpoints

	if needsEndpointSwap {
		th.populateLocalEndpoint(istioSpan.SpanTags, span.RemoteEndpoint)
		th.populateRemoteEndpoint(istioSpan.SpanTags, span.LocalEndpoint)
	} else {
		th.populateLocalEndpoint(istioSpan.SpanTags, span.LocalEndpoint)
		th.populateRemoteEndpoint(istioSpan.SpanTags, span.RemoteEndpoint)
	}

	if outParentSpanID != "" {
		span.ParentID = &outParentSpanID
	}

	return span
}

func (th *traceSpanHandler) populateLocalEndpoint(spanTags map[string]*istio_policy_v1beta1.Value, localEndpoint *trace.Endpoint) {
	if n, ok := decodeValue(spanTags[th.conf.LocalEndpointNameTagKey]).(string); ok && n != "" {
		localEndpoint.ServiceName = &n
	}

	if ip, ok := decodeValue(spanTags[th.conf.LocalEndpointIpTagKey]).(net.IP); ok {
		ips := ip.String()
		localEndpoint.Ipv4 = &ips
	}
}

func (th *traceSpanHandler) populateRemoteEndpoint(spanTags map[string]*istio_policy_v1beta1.Value, remoteEndpoint *trace.Endpoint) {
	if n, ok := decodeValue(spanTags[th.conf.RemoteEndpointNameTagKey]).(string); ok && n != "" {
		remoteEndpoint.ServiceName = &n
	}

	if ip, ok := decodeValue(spanTags[th.conf.RemoteEndpointIpTagKey]).(net.IP); ok {
		ips := ip.String()
		remoteEndpoint.Ipv4 = &ips
	}
}

var hexMapper = map[rune]rune{
	'0': '1',
	'1': '2',
	'2': '3',
	'3': '4',
	'4': '5',
	'5': '6',
	'6': '7',
	'7': '8',
	'8': '9',
	'9': 'a',
	'a': 'b',
	'b': 'c',
	'c': 'd',
	'd': 'e',
	'e': 'f',
	'f': '0',
	'A': 'b',
	'B': 'c',
	'C': 'd',
	'D': 'e',
	'E': 'f',
	'F': '0',
}

// This applies a simple Caeser cypher that shifts the hex characters by one
// forward.  If spanID contains non-hex chars, the output is undefined.
func deriveNewSpanID(spanID string) string {
	var newSpanIDBuilder strings.Builder
	for _, r := range spanID {
		newSpanIDBuilder.WriteRune(hexMapper[r])
	}
	return newSpanIDBuilder.String()
}
