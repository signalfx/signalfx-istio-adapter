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
	"fmt"
	"time"

	"github.com/golang/glog"
	me "github.com/hashicorp/go-multierror"
	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/sfxclient"

	"github.com/signalfx/signalfx-istio-adapter/signalfx/config"
	"istio.io/api/mixer/adapter/model/v1beta1"
	mixer_v1beta1 "istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/pkg/adapter"
	"istio.io/istio/mixer/template/metric"
)

// How long time series continue reporting from the registry since their last
// update.
const registryExpiry = 5 * time.Minute

type metricHandler struct {
	ctx                context.Context
	cancel             context.CancelFunc
	scheduler          *sfxclient.Scheduler
	sink               *sfxclient.HTTPSink
	registry           *registry
	metricTypes        map[string]*metric.Type
	intervalSeconds    uint32
	metricConfigByName map[string]*config.Params_MetricConfig
}

func createMetricHandler(conf *config.Params) (*metricHandler, error) {
	confsByName := make(map[string]*config.Params_MetricConfig, len(conf.Metrics))
	for i := range conf.Metrics {
		confsByName[conf.Metrics[i].Name] = conf.Metrics[i]
	}

	var metricHandlerInst *metricHandler
	if conf.EnableMetrics && len(conf.Metrics) > 0 {
		metricHandlerInst = &metricHandler{
			sink:               createSignalFxClient(conf),
			intervalSeconds:    uint32(conf.DatapointInterval.Round(time.Second).Seconds()),
			metricConfigByName: confsByName,
		}

		if err := metricHandlerInst.InitMetrics(); err != nil {
			return nil, err
		}
	}
	return metricHandlerInst, nil
}

func (mh *metricHandler) InitMetrics() error {
	mh.ctx, mh.cancel = context.WithCancel(context.Background())

	mh.scheduler = sfxclient.NewScheduler()
	mh.scheduler.Sink = mh.sink
	mh.scheduler.ReportingDelay(time.Duration(mh.intervalSeconds) * time.Second)

	mh.registry = newRegistry(registryExpiry)
	mh.scheduler.AddCallback(mh.registry)

	go func() {
		err := mh.scheduler.Schedule(mh.ctx)
		if err != nil {
			if ec, ok := err.(*errors.ErrorChain); !ok || ec.Tail() != context.Canceled {
				glog.Errorf("Scheduler shutdown unexpectedly: %v", err)
			}
		}
	}()
	return nil
}

func (mh *metricHandler) Shutdown() {
	if mh.cancel != nil {
		mh.cancel()
	}
}

// metric.Handler#HandleMetric
func (mh *metricHandler) HandleMetric(ctx context.Context, req *metric.HandleMetricRequest) (*v1beta1.ReportResult, error) {
	if mh == nil {
		return nil, nil
	}
	var allErr *me.Error

	for i := range req.Instances {
		name := req.Instances[i].Name
		if conf := mh.metricConfigByName[name]; conf != nil {
			if err := mh.processMetric(conf, req.Instances[i]); err != nil {
				allErr = me.Append(allErr, err)
			}
		} else {
			allErr = me.Append(allErr, fmt.Errorf("received metric with no config: %s", name))
		}
	}

	return &mixer_v1beta1.ReportResult{}, allErr.ErrorOrNil()
}

func (mh *metricHandler) processMetric(conf *config.Params_MetricConfig, inst *metric.InstanceMsg) error {
	name := inst.Name
	dims := sfxDimsForInstance(inst)

	val, err := valueToFloat(decodeValue(inst.Value))
	if err != nil {
		return err
	}

	switch conf.Type {
	case config.COUNTER:
		cu := mh.registry.RegisterOrGetCumulative(name, dims)
		cu.Add(int64(val))
	case config.HISTOGRAM:
		rb := mh.registry.RegisterOrGetRollingBucket(name, dims, mh.intervalSeconds)
		rb.Add(val)
	}
	return nil
}

func valueToFloat(val interface{}) (float64, error) {
	if val == nil {
		return 0.0, errors.New("nil value received")
	}

	switch v := val.(type) {
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case bool:
		if v {
			return float64(1), nil
		}
		return float64(0), nil
	case time.Time:
		return float64(v.Unix()), nil
	case time.Duration:
		return float64(v), nil
	default:
		return 0.0, errors.New("unsupported value type")
	}
}

func sfxDimsForInstance(inst *metric.InstanceMsg) map[string]string {
	dims := map[string]string{}

	for key, val := range inst.Dimensions {
		dims[key] = adapter.Stringify(decodeValue(val))
	}

	if inst.MonitoredResourceType != "" {
		dims["monitored_resource_type"] = inst.MonitoredResourceType
	}

	for key, val := range inst.MonitoredResourceDimensions {
		dims["monitored_resource_"+key] = adapter.Stringify(decodeValue(val))
	}

	return dims
}
