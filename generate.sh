#!/usr/bin/env bash

set -e

WORKDIR=$PWD
MIXER_REPO=$GOPATH/src/istio.io/istio/mixer
ISTIO=$GOPATH/src/istio.io/istio

rm -rf $MIXER_REPO/adapter/signalfx
cp -r signalfx $MIXER_REPO/adapter/

cd $ISTIO
./bin/mixer_codegen.sh -a mixer/adapter/signalfx/config/config.proto -x "-s=false -n signalfx -t metric -t tracespan"

cd $WORKDIR
cp -r $MIXER_REPO/adapter/signalfx/config/* signalfx/config/
