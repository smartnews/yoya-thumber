package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/naoina/toml"
	"github.com/smartnews/yoya-thumber/thumbnail"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"net"
)

var local = flag.String("local", "", "serve as webserver, example: 0.0.0.0:8000")
var timeout = flag.Int("timeout", 3, "timeout for upstream HTTP requests, in seconds")
var show_version = flag.Bool("version", false, "show version and exit")

var client http.Client

var version string

const maxDimension = 65000
const maxPixels = 10000000
const defaultScheme = "http"

var http_stats struct {
	received       int64
	inflight       int64
	ok             int64
	thumb_error    int64
	upstream_error int64
	arg_error      int64
	total_time_us  int64
}

func init() {
	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())
	loadToml()
	signalSetup()
}

type tomlConfig struct {
	Font struct {
		Name []string
	}
	Http struct {
		AvoidChunk bool
		UserAgent string
	}
	Image struct {
		BackgroundColor    string
		CompressionQuality int
		Gravity            int
		CropMode           int
	}
}

var config tomlConfig

func loadToml() {
	var f *os.File
	var err error
	f, err = os.Open("thumberd.toml")
	if err != nil {
		f, err = os.Open("/etc/thumberd.toml")
		if err != nil {
			glog.Error("No such file thumberd.toml or /etc/thumberd.toml")
			panic(err)
		}
	}
	defer f.Close()
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		glog.Error("read failed toml")
		panic(err)
	}
	if err := toml.Unmarshal(buf, &config); err != nil {
		glog.Error("toml Unmarshal failed ")
		panic(err)
	}
}

func errorServer(w http.ResponseWriter, r *http.Request) {
	glog.Error("404 Not Found")
	http.Error(w, "404 Not Found", http.StatusNotFound)
}

func statusServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "version %s\n", version)
	fmt.Fprintf(w, "received %d\n", atomic.LoadInt64(&http_stats.received))
	fmt.Fprintf(w, "inflight %d\n", atomic.LoadInt64(&http_stats.inflight))
	fmt.Fprintf(w, "ok %d\n", atomic.LoadInt64(&http_stats.ok))
	fmt.Fprintf(w, "thumb_error %d\n", atomic.LoadInt64(&http_stats.thumb_error))
	fmt.Fprintf(w, "upstream_error %d\n", atomic.LoadInt64(&http_stats.upstream_error))
	fmt.Fprintf(w, "arg_error %d\n", atomic.LoadInt64(&http_stats.arg_error))
	fmt.Fprintf(w, "total_time_us %d\n", atomic.LoadInt64(&http_stats.total_time_us))
}

// note: This function returns default scheme (http) if an error occured.
func getScheme(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return defaultScheme
	}
	if u.Scheme == "" {
		return defaultScheme
	}
	return u.Scheme
}

/*
 *  URL の正規化処理
 *  http: や http:/ 、又は protocal 自体の省略形を受理。(Akamai ACL 対策)
 */
func urlCanonical(url string, referer string) string {
	refererScheme := getScheme(referer)
	if strings.HasPrefix(url, "//") {
		return refererScheme + ":" + url;
	}

	words := strings.SplitN(url, ":", 2)
	if len(words) < 2 { // "example.org" => "http://example.org/"
		return "http://" + url
	}
	proto := words[0]
	switch proto {
	case "http": // "http:*example.org" => "http://example.org/"
	case "https": // "https:*example.org" => "https://example.org/"
	default: // "*example.org/*:*" => "*example.org/*:*"
		return "http://" + url
	}
	return proto + "://" + strings.TrimLeft(words[1], "/")
}

func myClientImageGet(imageUrl string, referer string, userAgent string) (*http.Response, error) {
	imageUrl = urlCanonical(imageUrl, referer)
	var srcReader *http.Response
	var err error
	var u *url.URL

	u, err = url.Parse(imageUrl)

	// these codes are referencing net/http/transport.go useProxy method.
	if err != nil {
		return nil, err
	}
	if u.Host == "localhost" {
		return nil, errors.New("localhost is prohibited.")
	}
	if ip := net.ParseIP(u.Host); ip != nil {
		if ip.IsLoopback() {
			return nil, errors.New("loopback address is prohibited.")
		}
	}

	req, err := http.NewRequest("GET", imageUrl, nil)
	req.Header.Add("Referer", referer)
	if err != nil {
		glog.Error("Failed to create NewRequest.")
		return nil, err
	}

	if userAgent != "" {
		req.Header.Add("User-Agent", userAgent)
	}

	srcReader, err = client.Do(req)
	if err != nil {
		glog.Warning("imageUrl not find " + imageUrl)
		return nil, err
	}
	// 200 以外はエラーにする (302 とかはどうしよう？)
	if srcReader.StatusCode != http.StatusOK {
		srcReader.Body.Close()
		return nil, errors.New("upstream status:" + srcReader.Status)
	}
	return srcReader, nil
}

func isHexColor(color string) bool {
	n := len(color)
	if n != 3 && n != 4 && n != 6 && n != 8 {
		return false
	}
	for i := 0; i < n; i++ {
		c := color[i]
		if (c < '0' || '9' < c) &&
			(c < 'A' || 'F' < c) &&
			(c < 'a' || 'f' < c) {
			return false
		}
	}
	return true
}

/*
 *  色HEX表現の正規化処理
 *  # が頭に付いてないのにHEX表現に見える場合 # をつける
 */
func colorHexCanonical(color string) string {
	if len(color) == 0 {
		return color
	}
	if color[0] != '#' && isHexColor(color) {
		return "#" + color
	}
	return color
}

func thumbServer(w http.ResponseWriter, r *http.Request, sem chan int) {

	startTime := time.Now()
	defer func() {
		//経過時間
		elapsed := int64(time.Now().Sub(startTime) / 1000)
		atomic.AddInt64(&http_stats.total_time_us, elapsed)
	}()

	atomic.AddInt64(&http_stats.received, 1)
	//現在処理している数
	atomic.AddInt64(&http_stats.inflight, 1)
	//終わったら-1
	defer atomic.AddInt64(&http_stats.inflight, -1)

	// path := r.URL.RequestURI()
	// 参考) net/url/url.go => parse
	path := r.RequestURI

	// Defaults
	var params = thumbnail.ThumbnailParameters{
		Width:  0,
		Height: 0,

		Upscale:     false, // false: 元より大きい場合はリサイズしない
		ForceAspect: false, // false:アスペクト比は変更しない
		//jpeg quality
		Quality:      config.Image.CompressionQuality,
		Gravity:      config.Image.Gravity,
		ImageOverlap: nil, //合成画像のストリーム
		//上書きする画像の横幅
		ImageOverlapWidthRatio: 0,
		//上書きする画像の縦幅
		ImageOverlapHeightRatio: 0,
		//上書きする画像のグラビティ
		ImageOverlapGravity: 0,
		//アノテーションのグラビティ
		TextGravity:  9, //右下表示
		TextFontSize: 10.0,
		//アノテーションのマージン
		TextMargin: 0,
		//余白をつけるかクロップするか
		CropMode: config.Image.CropMode,
		//余白の色指定
		Background: config.Image.BackgroundColor,
		//フォント
		TextFont: config.Font.Name,
		//アノテーションの色
		TextColor: "",
		//アノテーションの文字列
		Text: "",
		// HTTP Chunk を禁ずる
		HttpAvoidChunk: config.Http.AvoidChunk,
		// 出力フォーマット
		FormatOutput: "",
		// クロップ面積制限(0 == 制限なし)
		CropAreaLimitation: 0,
	}

	if path[0] != '/' {
		glog.Error("Path should start with /", http.StatusBadRequest)
		http.Error(w, "Path should start with /", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}

	// "/url=foo.jpg,io=baa.jpg?w=100&h=100"
	// => ["url=foo.jpg" "io=baa.jpg" "w=100" "h=100"]
	path_param := strings.SplitN(path[1:], "?", 2)
	urlParams := strings.Split(path_param[0], ",")
	if len(path_param) > 1 {
		urlParams2 := strings.Split(path_param[1], "&")
		urlParams = append(urlParams, urlParams2...)
	}
	fmt.Println(urlParams)
	for _, arg := range urlParams {
		if arg == "" {
			continue
		}
		tup := strings.SplitN(arg, "=", 2)
		if len(tup) != 2 {
			glog.Error("Arguments must have the form name=value", http.StatusBadRequest)
			http.Error(w, "Arguments must have the form name=value", http.StatusBadRequest)
			atomic.AddInt64(&http_stats.arg_error, 1)
			return
		}
		switch tup[0] {
		case "w", "h", "q", "u", "a", "g", "ow", "oh", "og", "tg", "tm", "cm", "igt", "iog":
			val, err := strconv.Atoi(tup[1])
			if err != nil {
				glog.Error("Invalid integer value for "+tup[0], http.StatusBadRequest)
				http.Error(w, "Invalid integer value for "+tup[0], http.StatusBadRequest)
				atomic.AddInt64(&http_stats.arg_error, 1)
				return
			}
			switch tup[0] {
			case "w":
				params.Width = val
			case "h":
				params.Height = val
			case "q":
				params.Quality = val
			case "u":
				params.Upscale = val != 0
			case "a":
				params.ForceAspect = val != 0
			case "g":
				params.Gravity = val
			case "tg":
				params.TextGravity = val
			case "tm":
				params.TextMargin = val
			case "cm":
				params.CropMode = val
			case "iog":
				params.ImageOverlapGravity = val
			}
		case "p", "iow", "ioh", "iox", "ioy", "cal":
			val, err := strconv.ParseFloat(tup[1], 64)
			if err != nil {
				glog.Error("Invalid integer value for "+tup[0], http.StatusBadRequest)
				http.Error(w, "Invalid float value for "+tup[0], http.StatusBadRequest)
				atomic.AddInt64(&http_stats.arg_error, 1)
				return
			}
			if val > 1 {
				atomic.AddInt64(&http_stats.arg_error, 1)
				glog.Error("can't use than 1 for "+tup[0], http.StatusBadRequest)
				http.Error(w, "can't use than 1 for "+tup[0], http.StatusBadRequest)
				return
			}
			switch tup[0] {
			case "iow":
				params.ImageOverlapWidthRatio = val
			case "ioh":
				params.ImageOverlapHeightRatio = val
			case "iox":
				params.ImageOverlapXRatio = val
			case "ioy":
				params.ImageOverlapYRatio = val
			case "cal":
				params.CropAreaLimitation = val
			}
		case "ts":
			val, err := strconv.ParseFloat(tup[1], 64)
			if err != nil {
				glog.Error("Invalid integer value for "+tup[0], http.StatusBadRequest)
				http.Error(w, "Invalid float value for "+tup[0], http.StatusBadRequest)
				atomic.AddInt64(&http_stats.arg_error, 1)
				return
			}
			switch tup[0] {
			case "ts":
				params.TextFontSize = val
			}

		case "t":
			val := tup[1]
			params.Text, _ = url.QueryUnescape(val)
		case "url":
			val := tup[1]
			params.ImageUrl, _ = url.QueryUnescape(val)
		case "io":
			val, _ := url.QueryUnescape(tup[1])
			OverlapsrcReader, err := myClientImageGet(val, r.Referer(), config.Http.UserAgent)
			if err != nil {
				glog.Error("Upstream Overlap Image failed : "+err.Error(), http.StatusBadGateway)
				http.Error(w, "Upstream Overlap Image failed : "+err.Error(), http.StatusBadGateway)
				atomic.AddInt64(&http_stats.upstream_error, 1)
				return
			}

			defer OverlapsrcReader.Body.Close()
			params.ImageOverlap = OverlapsrcReader.Body
		case "bg":
			val := tup[1]
			params.Background = val
		case "tf":
			val := tup[1]
			textFonts, _ := url.QueryUnescape(val)
			params.TextFont = strings.Split(textFonts, ",")
		case "tc":
			val := tup[1]
			params.TextColor = val
		case "fo": // Format for Output
			val := tup[1]
			params.FormatOutput = val
		}
	}

	params.Background = colorHexCanonical(params.Background)
	params.TextColor = colorHexCanonical(params.TextColor)

	if params.Width > maxDimension {
		glog.Error("Width (w) invalid", http.StatusBadRequest)
		http.Error(w, "Width (w) invalid", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}

	if params.Height > maxDimension {
		glog.Error("Height (h) invalid", http.StatusBadRequest)
		http.Error(w, "Height (h) invalid", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}

	if params.Width*params.Height > maxPixels {
		glog.Error("Image dimensions are insane", http.StatusBadRequest)
		http.Error(w, "Image dimensions are insane", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}

	if params.Quality > 100 || params.Quality < 0 {
		glog.Error("Quality must be between 0 and 100", http.StatusBadRequest)
		http.Error(w, "Quality must be between 0 and 100", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}

	srcReader, err := myClientImageGet(params.ImageUrl, r.Referer(), config.Http.UserAgent)
	if err != nil {
		message := "Upstream failed\tpath:" + path + "\treferer:" + r.Referer() + "\terror:" + err.Error()
		glog.Error(message, http.StatusBadGateway)
		http.Error(w, message, http.StatusBadGateway)
		atomic.AddInt64(&http_stats.upstream_error, 1)
		return
	}
	defer srcReader.Body.Close()

	content_type := ""
	switch params.FormatOutput {
	case "":
		content_type = srcReader.Header.Get("Content-Type")
		content_type, _ = url.QueryUnescape(content_type)
	case "jpg", "jpeg":
		content_type = "image/jpeg"
	case "webp":
		content_type = "image/webp"
	case "png":
		content_type = "image/png"
	case "gif":
		content_type = "image/gif"
	}

	fmt.Printf("%#v\n", params)

	w.Header().Set("Content-Type", content_type) //
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))

	// sem is the semaphore to restrict concurrent ImageMagick workers to the number of CPU core
	sem <- 1
	err = thumbnail.MakeThumbnailMagick(srcReader.Body, w, params)
	<-sem

	atomic.AddInt64(&http_stats.ok, 1)
}

func fontsServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	thumbnail.FontsMagick(w)
}

type Handler struct {
	sem chan int
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Path
	//  fmt.Fprint(w, uri)
	switch uri {
	case "/server-status":
		statusServer(w, r)
	case "/fonts":
		fontsServer(w, r)
	case "/favicon.ico":
		errorServer(w, r)
	default:
		thumbServer(w, r, h.sem)
	}
}

func signalSetup() {
	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan,
		syscall.SIGHUP,
	)

	exit_chan := make(chan int)

	go func() {
		for {
			s := <-signal_chan
			switch s {
			case syscall.SIGHUP:
				loadToml()
			default:
				exit_chan <- 1
			}
		}
	}()
}

func main() {

	flag.Parse()
	if *show_version {
		fmt.Printf("thumberd %s\n", version)
		return
	}

	client.Timeout = time.Duration(*timeout) * time.Second

	var err error
	if *local != "" { // Run as a local web server
		handler := new(Handler)
		handler.sem = make(chan int, runtime.NumCPU())
		err = http.ListenAndServe(*local, handler)
	} else { // Run as FCGI via standard I/O
		err = fcgi.Serve(nil, nil)
	}
	if err != nil {
		log.Fatal(err)
	}
}
