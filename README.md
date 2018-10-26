# SignalFx Istio Mixer Adapter

[![CircleCI](https://circleci.com/gh/signalfx/signalfx-istio-adapter.svg?style=shield)](https://circleci.com/gh/signalfx/signalfx-istio-adapter)

This repo contains the Istio mixer adapater that converts and sends metrics and
trace spans to SignalFx.  This was originally written for a Kubernetes Istio
deployment but should theoretically work outside of K8s with some small tweaks.

## Installation in Kubernetes

To deploy this adapter, simply apply the resource in the `resources/` dir of
this repo to your Kubernetes cluster like so:

`cat resources/* | sed -e 's/{MY_ACCESS_TOKEN}/MY_ORG_ACCESS_TOKEN/' | kubectl apply -f -`

Replacing `MY_ORG_ACCESS_TOKEN` with your organization's SignalFx access
token.  You can also just replace it in the [./resources/handler.yaml](./resources/handler.yaml)
as well and apply that whole directory.

The [config docs](./signalfx/config/signalfx.config.pb.html) explain the
configuration options and how to specify custom tracespan instances that will
influence the tags that get put on the emitted spans to SignalFx.

## TODO

 - Enable session support when implemented in Istio mixer
 - Add Helm chart to deploy this
