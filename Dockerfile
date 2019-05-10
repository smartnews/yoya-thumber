FROM ubuntu:16.04

ENV IMAGEMAGICK_VERSION  6.9.9-15

ENV GOLANG_VERSION 1.9.1

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
        fonts-ipafont-gothic \
        xz-utils && \
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
    curl -fsSL https://github.com/ImageMagick/ImageMagick6/archive/${IMAGEMAGICK_VERSION}.tar.gz > \
          ImageMagick-${IMAGEMAGICK_VERSION}.tar.gz && \
    tar xf ImageMagick-${IMAGEMAGICK_VERSION}.tar.gz && \
    cd /usr/local/src/ImageMagick6-${IMAGEMAGICK_VERSION} && \
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
    tar -C /usr/local -xzf golang.tar.gz && \
    rm -rf /usr/local/src/* && \
    \
    go get gopkg.in/gographics/imagick.v2/imagick && \
    go get github.com/golang/glog && \
    go get github.com/naoina/toml && \
    go get golang.org/x/net/http2

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
