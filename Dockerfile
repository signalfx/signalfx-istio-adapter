FROM golang:1.11.1 as builder

WORKDIR /go/src/github.com/signalfx/signalfx-istio-adapter/

COPY ./vendor ./vendor
COPY ./Makefile ./Makefile
COPY ./cmd ./cmd
COPY ./signalfx ./signalfx

RUN make bin/signalfx-istio-adapter

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/signalfx-istio-adapter", "-logtostderr"]
EXPOSE 8080
COPY --from=builder /go/src/github.com/signalfx/signalfx-istio-adapter/bin/signalfx-istio-adapter /
