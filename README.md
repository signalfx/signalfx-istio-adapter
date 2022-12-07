>ℹ️&nbsp;&nbsp;SignalFx was acquired by Splunk in October 2019. See [Splunk SignalFx](https://www.splunk.com/en_us/investor-relations/acquisitions/signalfx.html) for more information.

# SignalFx Istio Mixer Adapter

[![CircleCI](https://circleci.com/gh/signalfx/signalfx-istio-adapter.svg?style=shield)](https://circleci.com/gh/signalfx/signalfx-istio-adapter)

:warning: **SignalFx Istio Mixer Adapter is deprecated. For details, see the
[Deprecation Notice](./DEPRECATION-NOTICE.md)** :warning:

This repo contains the Istio mixer adapater that converts and sends metrics and
trace spans to SignalFx.  This was originally written for a Kubernetes Istio
deployment but should theoretically work outside of K8s with some small tweaks.

## Installation in Kubernetes

To deploy this adapter, you can either use the resource files from the
`resources/` directory directly, or you can use the included Helm chart.

### Configuring the ingest endpoint for your realm
A realm is a self-contained deployment of SignalFx in which your organization is hosted.
Different realms have different API endpoints.
For example, the endpoint for sending data in the us1 realm is ingest.us1.signalfx.com,
and ingest.eu0.signalfx.com for the eu0 realm. If you try to send data to the incorrect realm,
your access token will be denied.

**By default, this plugin sends to the us0 realm**

To determine what realm you are in, check your profile page in the SignalFx web application.

To change the ingest endpoint to your realm, you can override the `ingestUrl` config option:

```
ingestUrl: https://ingest.YOUR_SIGNALFX_REALM.signalfx.com
```

### Non-Helm
If you aren't using Helm, simply apply the resource in the `resources/` dir of
this repo to your Kubernetes cluster like so:

`cat resources/* | sed -e 's/MY_ACCESS_TOKEN/MY_ORG_ACCESS_TOKEN/' | kubectl apply -f -`

Replacing `MY_ORG_ACCESS_TOKEN` with your organization's SignalFx access
token.  You can also just replace it in the [./resources/handler.yaml](./resources/handler.yaml)
as well and apply that whole directory.

The [config docs](./signalfx/config/signalfx.config.pb.html) explain the
configuration options and how to specify custom tracespan instances that will
influence the tags that get put on the emitted spans to SignalFx.

### Helm

To install the chart with Helm, run something like the following command:

`$ helm install --name signalfx-adapter --set fullnameOverride=signalfx-adapter --namespace istio-system --set-string accessToken=MY_ORG_ACCESS_TOKEN ./helm/signalfx-istio-adapter/`

or, if using your own values config file:

`$ helm install --name signalfx-adapter -f my_config_values.yaml`

This assumes you only want to deploy one release of the adapter (the normal
case), and so it overrides the name to avoid the Helm auto-generated names.
You will need to replace `MY_ORG_ACCESS_TOKEN` with the appropriate value, or
you can add a Secret resource, in which case you must set the values
`accessTokenSecret.name` and `accessTokenSecret.value` when deploying the
chart.

#### Adding dimensions to your metrics
You can add any global dimensions you want to the metrics produced by the adapter by
using the `globalDimensions` config. For example, if you are using the [SignalFx
Smart Agent](https://docs.signalfx.com/en/latest/integrations/agent/kubernetes-setup.html) 
to monitor a Kubernetes cluster, you can associate the istio metrics
with the Smart Agent Kubernetes metrics by adding the `kubernetes_cluster` dimension:

```
globalDimensions:
  kubernetes_cluster: '"istio-cluster"'
```

String should be wrapped in double and single quotes. This is neccesary to support dynamic
values, such as:

```
globalDimensions:
  source_app: source.labels["app"] | "unknown"
```


## Releasing a new version (until CircleCI job is added)

To release a new version:

1. Tag a release on the Github repo of the form `vX.X.X` that makes sense given the prior release.
2. Update the `image` field in the pod spec of `resources/deployment.yaml` to reflect the new version
3. Run `PUSH=yes TAG=X.X.X make image` with the new tag version
4. Commit the update to the Deployment spec to Github (this commit should be *after* the tag created in step 1).

## TODO

 - Add CircleCI deployment job when a new tag it made against master
 - Enable session support when implemented in Istio mixer
 - Add Helm chart to deploy this
