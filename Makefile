TAG ?= latest

.PHONY: build
build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -o bin/signalfx-istio-adapter ./cmd

.PHONY: build-native
build-native:
	mkdir -p bin
	CGO_ENABLED=0 go build --no-cache -o bin/signalfx-istio-adapter ./cmd

.PHONY: image
image:
	docker build --pull -t quay.io/signalfx/istio-adapter:$(TAG) .
	if [[ "$(PUSH)" == "yes" ]]; then docker push quay.io/signalfx/istio-adapter:$(TAG); fi

.PHONY: image-dev
image-dev:
	docker build -t istio-adapter:dev .

.PHONY: resources
resources:
	cp ./signalfx/config/signalfx.yaml helm/signalfx-istio-adapter/templates/adapter.yaml
	./resources-from-helm

# requires git@github.com:signalfx/istio.git to be cloned to $GOPATH/src/istio.io/istio
.PHONY: generate
generate:
	./generate.sh
