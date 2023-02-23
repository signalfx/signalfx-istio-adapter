module github.com/signalfx/signalfx-istio-adapter

go 1.14

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973 // indirect
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd // indirect
	github.com/d4l3k/messagediff v1.2.1 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/envoyproxy/go-control-plane v0.11.0 // indirect
	github.com/go-logfmt/logfmt v0.5.0 // indirect
	github.com/gogo/googleapis v1.1.0
	github.com/gogo/protobuf v1.1.1
	github.com/gogo/status v1.0.3 // indirect
	github.com/golang/glog v1.0.0
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/mux v1.6.2 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/hashicorp/go-multierror v1.0.0
	github.com/imdario/mergo v0.3.6 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/natefinch/lumberjack v0.0.0-20170531160350-a96e63847dc3 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/prometheus/client_golang v0.9.0 // indirect
	github.com/prometheus/common v0.0.0-20181015124227-bcb74de08d37 // indirect
	github.com/prometheus/procfs v0.0.0-20181005140218-185b4288413d // indirect
	github.com/prometheus/prom2json v0.0.0-20180620215746-7b8ed2aed129 // indirect
	github.com/signalfx/com_signalfx_metrics_protobuf v0.0.0-20190222193949-1fb69526e884
	github.com/signalfx/golib/v3 v3.0.1
	github.com/spf13/cobra v0.0.3 // indirect
	github.com/uber/jaeger-client-go v2.15.0+incompatible // indirect
	github.com/uber/jaeger-lib v1.5.0 // indirect
	go.uber.org/atomic v1.3.2 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.9.1 // indirect
	google.golang.org/grpc v1.52.0
	gopkg.in/d4l3k/messagediff.v1 v1.2.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	istio.io/api v0.0.0-20181018143345-246cae3eb30a
	istio.io/istio v0.0.0-00010101000000-000000000000
	k8s.io/api v0.0.0-20180904230853-4e7be11eab3f // indirect
	k8s.io/apimachinery v0.0.0-20180904193909-def12e63c512 // indirect
	k8s.io/client-go v0.0.0-20181010045704-56e7a63b5e38 // indirect
	k8s.io/kube-openapi v0.0.0-20230217203603-ff9a8e8fa21d // indirect
)

replace istio.io/istio => github.com/signalfx/istio v0.0.0-20181018184150-1f91a0f541eb
