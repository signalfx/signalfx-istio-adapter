TAG ?= latest

bin/signalfx-istio-adapter:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -o bin/signalfx-istio-adapter ./cmd

.PHONY: image
image:
	docker build --pull -t quay.io/signalfx/istio-adapter:$(TAG) .
	if [[ "$(PUSH)" == "yes" ]]; then docker push quay.io/signalfx/istio-adapter:$(TAG); fi

.PHONY: resources
resources:
	./resources-from-helm
