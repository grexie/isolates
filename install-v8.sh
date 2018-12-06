#!/bin/bash -e

V8DIR="$(cd $(dirname ${BASH_SOURCE[0]}) && pwd)"

WORKDIR=$(mktemp -d)
mkdir -p ${WORKDIR}
pushd ${WORKDIR} >/dev/null

trap "rm -Rf ${WORKDIR}" EXIT

shopt -s nocasematch

go env >libv8.env
. libv8.env
rm libv8.env

case $GOOS in
  darwin) curl -sSL https://rubygems.org/downloads/libv8-6.3.292.48.1-universal-darwin-18.gem | tar -xf -;;
  linux) case $GOARCH in
    arm) curl -sSL http://tim-behrsin-portfolio.s3.amazonaws.com/libv8-6.3.292.48.1-arm-linux.gem | tar -xf -;;
    *) curl -sSL https://rubygems.org/downloads/libv8-6.3.292.48.1-${GOARCH}-${GOOS}.gem | tar -xf -;;
  esac;;
  *) curl -sSL https://rubygems.org/downloads/libv8-6.3.292.48.1-${GOARCH}-${GOOS}.gem | tar -xf -;;
esac

tar -xzf data.tar.gz
rm -Rf ${V8DIR}/{include,libv8}
cp -r $(pwd)/vendor/v8/include ${V8DIR}/include
cp -r $(pwd)/vendor/v8/out/*.release ${V8DIR}/libv8

popd >/dev/null
