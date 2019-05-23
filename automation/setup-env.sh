#!/bin/bash -xe
echo "Setup Go paths"
cd ..
export GOROOT=/usr/local/go
export GOPATH=$(pwd)/go
export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
mkdir -p $GOPATH

echo "Install Go 1.10"
export GIMME_GO_VERSION=1.11.5
mkdir -p /gimme
curl -sL https://raw.githubusercontent.com/travis-ci/gimme/master/gimme | HOME=/gimme bash >> /etc/profile.d/gimme.sh
source /etc/profile.d/gimme.sh

echo "Install operator repository to the right place"
mkdir -p $GOPATH/src/kubevirt.io
mkdir -p $GOPATH/pkg
ln -s $(pwd)/node-maintenance-operator $GOPATH/src/kubevirt.io/
cd $GOPATH/src/kubevirt.io/node-maintenance-operator
