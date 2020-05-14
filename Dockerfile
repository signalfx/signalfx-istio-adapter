FROM golang:1.14.3 as builder

WORKDIR /app/

COPY go.mod .
COPY go.sum .
COPY ./Makefile ./Makefile
COPY ./cmd ./cmd
COPY ./signalfx ./signalfx

RUN make build

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/signalfx-istio-adapter", "-logtostderr"]
EXPOSE 8080
COPY --from=builder /app/bin/signalfx-istio-adapter /
