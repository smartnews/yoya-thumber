# Yoya-thumber

Yoya-thumber is a dynamic image thumbnailing proxy.

## Features

- Support image scaling. Both of scaling up and scaling down. You can specify output width or height.
- Easy to use. Since it behave as a HTTP proxy, what you should do is only passing an origin image url to the yoya-thumber.
- Support multiple image formats including JPEG, GIF, PNG. If input image format and output image format are different, yoya-thumber automatically converts the image format.
- Works as a standalone HTTP server.
- Possible to superimpose text or another image on an image.
- Support image compression level adjustment. (Output image format must be JPEG or WEBP.)
- Stable. This software is already USED IN PRODUCTION environment in SmartNews.

## Dependencies

- Golang (Higher version is better. We tested on Golang 1.9.1, as of Oct 2017.)
- ImageMagick (We strongly recommend to use 6.9.9-15 or higher.)

## Install (CentOS 7, Amazon Linux)

-  ImageMagick
```
$ sudo yum install libjpeg-turbo-devel libpng-devel giflib-devel libwebp-devel fontconfig-devel
$ tar xfz ImageMagick-6.9.9-15.tar.gz
$ cd ImageMagick-6.9.9-15
$ ./configure --disable-openmp --disable-opencl --with-webp --with-fontconfig -without-perl
$ make
$ sudo make install
```

- GoImagick
```
$ go get gopkg.in/gographics/imagick.v2/imagick
$ go install gopkg.in/gographics/imagick.v2/imagick
```

- yoya-thumber
```
$ go get github.com/smartnews/yoya-thumber/thumberd
$ go install github.com/smartnews/yoya-thumber/thumberd
```

## Install (Mac)

Install ImageMagick via MacPort
```
sudo port install ImageMagick
```

Install Golang and pkg-config via HomeBrew
```
brew install Golang
brew install pkg-config
```

Install imagick and thumberd
```
// set this environment variable or go get will throw an exception.
export CGO_CFLAGS_ALLOW='-Xpreprocessor'

go get gopkg.in/gographics/imagick.v2/imagick
go install gopkg.in/gographics/imagick.v2/imagick

go get github.com/smartnews/yoya-thumber/thumberd
go install github.com/smartnews/yoya-thumber/thumberd
```

## Install (Ubuntu)

TBW

## Install (Docker)

```bash
docker build . -t yoya-thumber
docker run -p 8000:8000 -it yoya-thumber
```

## Usage

set GOPATH
```
# Edit ~/.bash_profile 

export GOPATH=/Users/$USER/go
export PATH=$GOPATH/bin:$PATH

# Reload profile : source ~/.bash_profile
```

Start thumberd
```
 $ thumberd -local localhost:8000
```

The HTTP server of thumberd (yoya-thumber) starts up. In this case, yoya-thumber will wait at the port 8000, but only access from localhost will be accepted. If you do not want to restrict the source host, you should do as following.

```
 $ thumberd -local 0.0.0.0:8000
```

Note that you must put the config file `thumberd.toml` in your current directory or in the `/etc` directory. You can find a sample of config file at: https://github.com/smartnews/yoya-thumber/blob/master/files/thumberd.toml

### URL example

- http://localhost:8000/?url=https%3A%2F%2Fwww.smartnews.com%2Fimg%2Fja%2Flogo-gray.png&w=300&fo=jpeg
- http://localhost:8000/fonts # fonts listing in json

###  Parameters:
- url: upstream image URL (required, should be url-encoded.)
- w:   thumbnail width (e.g. 300)
- h:   thumbnail height (e.g. 300)
- fo:  output format (supported type: jpeg, png, gif, webp)
- cm:  crop mode: 0:none, 1:crop, 2:margin
- cal: crop area limitation
- bg:  background color
- g:   crop or margin gravity
- q:   quality of output image
- u:   upscale enable
- a:   force aspect
- t:   text annotation
- tg:  text gravity
- ts:  text size
- tc:  text color
- tf:  text font name
- tm:  text margin
- io:  overlap image URL
- iog: overlap image gravity
- iox: overlap image x offset
- ioy: overlap image y offset
- iow: overlap image width
- ioh: overlap image height

### Notes

- The value of `url` parameter should be url-encoded.
- If the both of `w` and `h` parameters are specified, by default, stronger (stricter) one is used. In other words, the aspect ratio of the original image will be kept. You can change the behavior by `cm` option.
- At each request, you can specify the value of HTTP referer, by simply passing the `Referer` header in your HTTP request.
- Yoya-thumber's command line interface might be changed in the future. We'll do enough announcements before the change.

### Configurations

You can customize some behavior of yoya-thumber by editing the config file. Config file format is TOML. For example, you can set the user-agent. For more details, see `files/thumberd.toml`

## Why the name is yoya-thumber

*Yoya* comes from the name of core developer. He wrote this software under contract with [SmartNews, Inc](http://about.smartnews.com/en).

## Copyright

- Yoya-thumber is licensed under 3-clause BSD license. For more details, see LICENSE file.
- Yoya-thumber is a fork of [go-thumber](https://github.com/pixiv/go-thumber). The copyright of the original part is owned by pixiv Inc. (A part of thumberd.go is concerned, but it is difficult to separate it clearly.)
