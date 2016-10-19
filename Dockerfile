FROM ubuntu:14.04

ENV IMAGEMAGICK_VERSION 6.9.6-2
ENV IMAGEMAGICK_SHA256SUM 328fba877c15dece7ced2e36662230944d0fd7c9990cd994ae848b43e9d51414

ENV GOLANG_VERSION 1.5.2
ENV GOLANG_SHA256SUM b8041ec8a7c0da29dab0b110206794d016165d6d0806976d39b7a99d899aa015

ENV GOPATH=/go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH
ENV PKG_CONFIG_PATH=/usr/local/lib/pkgconfig/
ENV CGO_LDFLAGS="-Wl,-rpath=/usr/local/lib"

ENV DEBIAN_FRONTEND noninteractive
ENV TERM linux
ENV INITRD No

RUN \
    apt-get update -yqq && \
    apt-get install -yqq --no-install-recommends \
        make \
        gcc \
        g++ \
        git \
        curl \
        ca-certificates \
        libjpeg-turbo8-dev \
        libpng12-dev \
        libgif-dev \
        libwebp-dev \
        libfontconfig1-dev \
        fonts-ipafont-gothic && \
    apt-get clean && \
    rm -rf \
        /var/lib/apt/lists/* \
        /tmp/* \
        /var/tmp/* \
        /usr/share/man \
        /usr/share/doc \
        /usr/share/doc-base && \
    \
    cd /usr/local/src && \
    curl -fsSL https://www.imagemagick.org/download/ImageMagick-${IMAGEMAGICK_VERSION}.tar.gz > \
          ImageMagick-${IMAGEMAGICK_VERSION}.tar.gz && \
    tar xfz ImageMagick-${IMAGEMAGICK_VERSION}.tar.gz && \
    cd /usr/local/src/ImageMagick-${IMAGEMAGICK_VERSION} && \
    ./configure \
        '--prefix=/usr/local' \
        '--disable-openmp' \
        '--disable-opencl' \
        '--with-webp' \
        '--with-fontconfig' \
        '--disable-dependency-tracking' \
        '--enable-shared' \
        '--disable-static' \
        '--with-xml' \
        '--without-included-ltdl' \
        '--with-ltdl-include=/usr/include' \
        '--with-ltdl-lib=/usr/lib64' \
        '--without-perl' \
        'CFLAGS=-O3 -g -pipe -Wall -Wp,-D_FORTIFY_SOURCE=2 -grecord-gcc-switches -m64 -mtune=generic' \
        'LDFLAGS=-Wl,-z,relro' \
        'CXXFLAGS=-O3 -g -pipe -Wall -Wp,-D_FORTIFY_SOURCE=2 -grecord-gcc-switches -m64 -mtune=generic' && \
    make && \
    make install && \
    rm -rf /usr/local/src/* && \
    \
    cd /usr/local/src && \
    curl -fsSL https://golang.org/dl/go${GOLANG_VERSION}.linux-amd64.tar.gz -o golang.tar.gz && \
    echo "${GOLANG_SHA256SUM} golang.tar.gz" | sha256sum -c - && \
    tar -C /usr/local -xzf golang.tar.gz && \
    rm -rf /usr/local/src/* && \
    \
    go get gopkg.in/gographics/imagick.v2/imagick && \
    go get github.com/golang/glog && \
    go get github.com/naoina/toml

ADD thumberd /go/src/github.com/smartnews/yoya-thumber/thumberd
ADD thumbnail /go/src/github.com/smartnews/yoya-thumber/thumbnail

RUN \
    cd /go/src/github.com/smartnews/yoya-thumber/thumberd && \
    go install

ADD files/thumberd.toml /etc/thumberd.toml
ADD files/policy.xml /usr/local/etc/ImageMagick-6/

EXPOSE 8000

ENTRYPOINT ["thumberd"]
CMD ["-local", "0.0.0.0:8000", "-timeout", "30"]
