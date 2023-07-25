#!/bin/bash -e

VERSION=$1
VERSION=${VERSION:-11.4.183.17}
SCHEME=$2
SCHEME=${SCHEME:-release}

DIR=/usr/local/include/v8
rm -Rf ${DIR}
mkdir -p ${DIR}
URL=https://github.com/grexie/v8-builder/releases/download/${VERSION}/v8-headers-${VERSION}.zip
cd ${DIR}
curl -sSL ${URL} -o v8-headers.zip
test -f v8-headers.zip && unzip v8-headers.zip 2>/dev/null >/dev/null && echo "Installed v8-headers-${VERSION}" || true
rm -f v8-headers.zip

for PLATFORM in macos linux ios android windows; do
  for ARCH in arm64 x64 x86 arm; do
    DIR=/usr/local/lib/v8/${ARCH}/${PLATFORM}
    rm -Rf ${DIR}
    mkdir -p ${DIR}
    URL=https://github.com/grexie/v8-builder/releases/download/${VERSION}/v8-${PLATFORM}-${ARCH}-${SCHEME}-${VERSION}.zip
    cd ${DIR}
    curl -sSL ${URL} -o v8.zip
    test -f v8.zip && unzip -j v8.zip 2>/dev/null >/dev/null && echo "Installed v8-${VERSION} for ${PLATFORM} ${ARCH} (${SCHEME})" || true
    rm -f v8.zip
  done
done