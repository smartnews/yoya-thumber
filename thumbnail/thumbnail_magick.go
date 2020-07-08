// return the thumbnailed version.
package thumbnail

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http" // XXX
	"strings"

	"github.com/golang/glog"
	"gopkg.in/gographics/imagick.v2/imagick"
)

// ThumbnailParameters configures the thumbnailing process
type ThumbnailParameters struct {
	Width       int  // Target width
	Height      int  // Target height
	Upscale     bool // Whether to upscale images that are smaller than the target
	ForceAspect bool // アスペクト比を変更可能か
	Quality     int  // JPEG quality (0-99)
	//yoya thumberd 拡張追加

	ImageUrl                string
	Text                    string
	TextColor               string
	Gravity                 int
	ImageOverlap            io.Reader
	ImageOverlapWidthRatio  float64
	ImageOverlapHeightRatio float64
	ImageOverlapGravity     int
	ImageOverlapXRatio      float64
	ImageOverlapYRatio      float64
	TextFontSize            float64
	TextGravity             int
	TextMargin              int
	CropMode                int
	Background              string
	TextFont                []string
	HttpAvoidChunk          bool
	FormatOutput            string
	CropAreaLimitation      float64
	MaxPixels               uint
}

func round(f float64) uint {
	return uint(math.Floor(f + .5))
}

func roundInt(f float64) int {
	return int(math.Floor(f + .5))
}

/*
 * Cropモードの時の Gravity に応じたオフセット(X,Y)位置を計算する
 */
func getCropGeometry(srcAspect, destAspect, srcHeight, cropHeight, srcWidth, cropWidth float64, gravity int) (cropX, cropY uint) {
	if srcAspect < destAspect { // 元の画像より横長の場合
		switch getVertical(gravity) {
		case 0.0:
			cropX = 0
			cropY = 0
		case 0.5:
			cropX = 0
			cropY = round((srcHeight - cropHeight) / 2.0)
		case 1.0:
			cropX = 0
			cropY = round(srcHeight - cropHeight)
		}

	} else { // 元の画像より縦長の場合
		switch getHorizontal(gravity) {
		case 0.0:
			//座標横軸原点からcropWidth分クロップ
			cropX = 0
			cropY = 0
		case 0.5:
			cropX = round((srcWidth - cropWidth) / 2.0)
			cropY = 0
		case 1.0:
			//原点+resizeWidthからsrcWidth最後までクロップ
			cropX = round(srcWidth - cropWidth)
			cropY = 0
		}
	}
	return

}

/*
 * Margin モードや画像上書き処理での Gravity に応じた X ratio を返す
 */
func getHorizontal(gravity int) float64 {
	switch gravity {
	case 1, 4, 7: // NorthWest, West, SouthWest
		return 0.0
	case 2, 5, 8: //North, Center, South
		return 0.5
	case 3, 6, 9: //NorthEast, East, SouthEast
		return 1.0
	}
	return 0
}

/*
 * Margin モードや画像上書き処理での Gravity に応じた Y ratio を返す
 */
func getVertical(gravity int) float64 {
	switch gravity {
	case 1, 2, 3: // NorthWest, North, NorthEast
		return 0.0
	case 4, 5, 6: // West, Center, East
		return 0.5
	case 7, 8, 9: //SouthWest, South, SouthEast
		return 1.0
	}
	return 0
}

/*
 * Gravity のパラメータ値に対応する ImageMagick の定数を返す
 */
func getGravityValue(gravity int) imagick.GravityType {
	switch gravity {
	case 1:
		return imagick.GRAVITY_NORTH_WEST
	case 2:
		return imagick.GRAVITY_NORTH
	case 3:
		return imagick.GRAVITY_NORTH_EAST
	case 4:
		return imagick.GRAVITY_WEST
	case 5:
		return imagick.GRAVITY_CENTER
	case 6:
		return imagick.GRAVITY_EAST
	case 7:
		return imagick.GRAVITY_SOUTH_WEST
	case 8:
		return imagick.GRAVITY_SOUTH
	case 9:
		return imagick.GRAVITY_SOUTH_EAST
	}
	return imagick.GRAVITY_SOUTH_EAST
}

func isFormatTransparent(format string) bool {
	format = strings.ToLower(format)
	switch format {
	case "png":
		return true
	case "webp":
		return true
	case "gif":
		return true
	}
	return false
}

func isOutputTransparent(inputFormat, outputFormat string) bool {
	if isFormatTransparent(outputFormat) {
		return true
	} else if outputFormat == "" {
		if isFormatTransparent(inputFormat) {
			return true
		}
	}
	return false
}

const (
	FORMAT_JPEG  = iota
	FORMAT_GIF   = iota
	FORMAT_PNG   = iota
	FORMAT_WEBP  = iota
	FORMAT_BMP   = iota
	FORMAT_HEIC  = iota
	FORMAT_OTHER = iota
)

func isJPEG(bytes []byte) bool {
	return bytes[0] == 0xFF && bytes[1] == 0xD8
}

func isGIF(bytes []byte) bool {
	// 0x47 = G, 0x49 = I, 0x46 = F, 0x38 = 8
	return bytes[0] == 0x47 && bytes[1] == 0x49 && bytes[2] == 0x46 && bytes[3] == 0x38
}

func isPNG(bytes []byte) bool {
	// 0x50 = P, 0x4E = N, 0x47 = G
	return bytes[0] == 0x89 && bytes[1] == 0x50 && bytes[2] == 0x4E && bytes[3] == 0x47
}

func isWEBP(bytes []byte) bool {
	// "RIFF" = {0x52, 0x49, 0x46, 0x46}
	if bytes[0] != 0x52 || bytes[1] != 0x49 || bytes[2] != 0x46 || bytes[3] != 0x46 {
		return false
	}
	// 0x57 = W, 0x45 = E, 0x42 = B, 0x50 = P
	if bytes[8] != 0x57 || bytes[9] != 0x45 || bytes[10] != 0x42 || bytes[11] != 0x50 {
		return false
	}
	return true
}

func isBMP(bytes []byte) bool {
	return bytes[0] == 0x42 && bytes[1] == 0x4D
}

func isHEIC(bytes []byte) bool {
	// too big ftyp box.
	if bytes[0] != 0 || bytes[1] != 0 || bytes[2] != 0 {
		return false
	}
	// 0x66 = f, 0x74 = t, 0x79 = y, 0x70 = p
	if bytes[4] != 0x66 || bytes[5] != 0x74 || bytes[6] != 0x79 || bytes[7] != 0x70 {
		return false
	}
	// "heic" = {0x68, 0x65, 0x69, 0x63}
	if bytes[8] == 0x68 && bytes[9] == 0x65 && bytes[10] == 0x69 && bytes[11] == 0x63 {
		return true
	}
	// "heix" = {0x68, 0x65, 0x69, 0x78}
	if bytes[8] == 0x68 && bytes[9] == 0x65 && bytes[10] == 0x69 && bytes[11] == 0x78 {
		return true
	}
	// "mif1" = {0x6d, 0x69, 0x66, 0x31}
	if bytes[8] == 0x6d && bytes[9] == 0x69 && bytes[10] == 0x66 && bytes[11] == 0x31 {
		return true
	}
	return false
}

func detectImageFormat(bytes []byte) int {
	if len(bytes) < 12 {
		return FORMAT_OTHER
	}

	if isJPEG(bytes) {
		return FORMAT_JPEG
	} else if isGIF(bytes) {
		return FORMAT_GIF
	} else if isPNG(bytes) {
		return FORMAT_PNG
	} else if isWEBP(bytes) {
		return FORMAT_WEBP
	} else if isBMP(bytes) {
		return FORMAT_BMP
	} else if isHEIC(bytes) {
		return FORMAT_HEIC
	}
	return FORMAT_OTHER
}

// This function comes from yoya san's gist. For more details, see: https://gist.github.com/yoya/2ae952716dbf70bc749181781eda27a8
func extractGIF1stFrame(bytes []byte) (int, error) {
	size := len(bytes)
	if size < 13 {
		return size, errors.New("too short header")
	}
	flags := bytes[10]
	globalColorTableFlag := (flags & 0x80) >> 7
	sizeOfGlobalColorTable := (flags & 0x07)
	var offset = 13
	if globalColorTableFlag != 0 {
		colorTableSize := int(math.Pow(2, float64(sizeOfGlobalColorTable+1)))
		offset += 3 * colorTableSize
		if size < offset {
			return size, errors.New("too short global colorTable")
		}
	}
	for {
		if size < (offset + 1) {
			return size, errors.New("missing separator")
		}
		separator := bytes[offset]
		offset++
		switch separator {
		case 0x3B: // Trailer
		case 0x21: // Extention
			if size < (offset + 2) {
				return size, errors.New("missing extention block header")
			}
			extensionBlockLabel := bytes[offset]
			extensionDataSize := bytes[offset+1]
			offset += 2 + int(extensionDataSize)
			if size < offset {
				return size, errors.New("too short extension block")
			}
			if extensionBlockLabel == 0xff { // Application Extension
				for {
					if size < (offset + 1) {
						return size, errors.New("missing extension subblock size field")
					}
					subBlockSize := bytes[offset]
					offset++
					if subBlockSize == 0 {
						break
					}
					offset += int(subBlockSize)
					if size < offset {
						return size, errors.New("to short extension subblock")
					}
				}
			} else {
				offset++ // extensionBlock Trailer
			}
		case 0x2C: // Image
			if size < (offset + 9) {
				return size, errors.New("too short image header")
			}
			flags := bytes[offset+8]
			localColorTableFlag := (flags & 0x80) >> 7
			sizeOfLocalColorTable := (flags & 0x07)
			offset += 9
			if localColorTableFlag != 0 {
				colorTableSize := int(math.Pow(2, float64(sizeOfLocalColorTable+1)))
				offset += 3 * colorTableSize
				if size < offset {
					return size, errors.New("too short local colorTable")
				}
			}
			offset++ // LZWMinimumCodeSize
			for {
				if size < (offset + 1) {
					return size, errors.New("missing image subblock size field")
				}
				subBlockSize := bytes[offset]
				offset++
				if subBlockSize == 0 {
					break
				}
				offset += int(subBlockSize)
				if size < offset {
					return size, errors.New("too short image subblock")
				}
			}
			if size < (offset + 1) {
				return size, errors.New("missing separator for trailer overwrite")
			}
			bytes[offset] = 0x3B // trailer overwrite
		default:
			// nothing to do
		}
		if separator == 0x3B {
			break
		}
	}
	return offset, nil
}

func init() {
	imagick.Initialize()
}

/*
 * サムネール処理
 */
func MakeThumbnailMagick(src io.Reader, dst http.ResponseWriter, params ThumbnailParameters) error {

	// var err error
	var mw *imagick.MagickWand

	mw = imagick.NewMagickWand()
	defer mw.Destroy()
	mw.SetResourceLimit(imagick.RESOURCE_THREAD, 1)

	buf := make([]byte, 20)
	_, err := io.ReadFull(src, buf)
	if err != nil {
		return err
	}

	// FORMAT_OTHER means this file format is not supported.
	// For security purposes, we are restricting our input image format.
	if detectImageFormat(buf) == FORMAT_OTHER {
		msg := "input image format is not supported"
		glog.Error(msg)
		log.Println(msg)
		return errors.New(msg)
	}

	//画像入力
	bytes, err := ioutil.ReadAll(src)
	if err != nil {
		glog.Error("Upstream read failed" + err.Error())
		log.Println("Upstream read failed" + err.Error())
		return err
	}

	bytes = append(buf, bytes...)

	err = mw.PingImageBlob(bytes)
	if err != nil {
		glog.Error("Upstream PingImageBlob failed" + err.Error())
		log.Println("Upstream PingImageBlob failed" + err.Error())
		return err
	}

	mw.SetFirstIterator()
	srcWidth := float64(mw.GetImageWidth())
	srcHeight := float64(mw.GetImageHeight())

	if mw.GetImageFormat() == "GIF" {
		_, err := extractGIF1stFrame(bytes)
		if err != nil {
			glog.Error("extractGIF1stFrame failed" + err.Error())
			log.Println("extractGIF1stFrame failed" + err.Error())
		}
	}

	pixelNum := mw.GetImageWidth() * mw.GetImageHeight()
	if uint(pixelNum) > params.MaxPixels {
		glog.Error("origin image size too big, exceed max pixel num")
		log.Println("origin image size too big, exceed max pixel num")
		return errors.New("origin image size too big, exceed max pixel num")
	}

	var cropX uint = 0
	var cropY uint = 0
	var cropWidth float64 = 0
	var cropHeight float64 = 0
	// 変換後のファイルの縦横サイズ
	var destWidth float64 = float64(params.Width)
	var destHeight float64 = float64(params.Height)

	//地画像のサイズの補完
	if params.Width != 0 && params.Height == 0 {
		destHeight = (float64(params.Width) / srcWidth) * srcHeight
	} else if params.Height != 0 && params.Width == 0 {
		destWidth = (float64(params.Height) / srcHeight) * srcWidth
	} else if params.Width == 0 && params.Height == 0 {
		destWidth = srcWidth
		destHeight = srcHeight
	}

	// 変換前と後のアスペクト比。(横サイズが分子)
	var srcAspect float64 = float64(srcWidth) / float64(srcHeight)
	var destAspect float64 = float64(destWidth) / float64(destHeight)

	// 変換後サイズのうち画像が実際に表示される領域(マージンを抜かす)
	var mappedX uint = 0
	var mappedY uint = 0
	var mappedWidth float64 = float64(destWidth)
	var mappedHeight float64 = float64(destHeight)
	var virtualMappedWidth float64 = float64(destWidth)
	var virtualMappedHeight float64 = float64(destHeight)

	/*
	 * リサイズの計算。(クロップ方式、マージン方式)
	 */
	if params.CropMode == 0 {
		// 横と縦、両方の辺が元より大きい場合は、リサイズしない
		if !params.Upscale && srcWidth < destWidth &&
			srcHeight < destHeight {
			destWidth = srcWidth
			destHeight = srcHeight
		}
		// リサイズのみ。クロップもマージンも無し
		if destWidth != 0 && destHeight != 0 && params.ForceAspect {
			// destWidth, destsHeight をそのまま
		} else {
			if srcAspect < destAspect {
				// 縦に合わせる
				destWidth = destHeight * srcAspect
			} else {
				// 横に合わせる
				destHeight = destWidth / srcAspect
			}
			mappedWidth = destWidth
			mappedHeight = destHeight
		}
		virtualMappedWidth = destWidth
		virtualMappedHeight = destHeight
	} else if params.CropMode == 1 {
		// クロップする

		// 横と縦、両方の辺が元より大きい場合は、リサイズしない
		var largerThanSrc = !params.Upscale && srcWidth <= destWidth && srcHeight <= destHeight
		if largerThanSrc {
			destWidth = srcWidth
			destHeight = srcHeight
			cropWidth = srcWidth
			cropHeight = srcHeight
		} else {
			if srcAspect < destAspect { // 元の画像より横長の場合
				//縦を基準にして横のサイズを出す
				cropWidth = srcWidth
				cropHeight = (srcWidth / destAspect)
				// 画像の面積がクロップ面積制限以下のサイズになった場合は下限値に補正する
				if !largerThanSrc && srcAspect/destAspect < params.CropAreaLimitation {
					var oldDestHeight = destHeight
					cropHeight = (srcWidth / srcAspect * params.CropAreaLimitation)
					destHeight = (destWidth / srcAspect * params.CropAreaLimitation)
					glog.Warningf("Required height is lesser than crop area limitation (%f, %f) -> (%f, %f)", destWidth, oldDestHeight, destWidth, destHeight)
					log.Printf("Required height is lesser than crop area limitation (%f, %f) -> (%f, %f)", destWidth, oldDestHeight, destWidth, destHeight)
				}
			} else {
				//縦を基準にして横のサイズを出す
				cropWidth = (srcHeight * destAspect)
				cropHeight = srcHeight
				// 画像の面積がクロップ面積制限以下のサイズになった場合は下限値に補正する
				if !largerThanSrc && destAspect/srcAspect < params.CropAreaLimitation {
					var oldDestWidth = destWidth
					cropWidth = (srcHeight * srcAspect * params.CropAreaLimitation)
					destWidth = (destHeight * srcAspect * params.CropAreaLimitation)
					glog.Warningf("Required width is lesser than crop area limitation (%f, %f) -> (%f, %f)", oldDestWidth, destHeight, destWidth, destHeight)
					log.Printf("Required width is lesser than crop area limitation (%f, %f) -> (%f, %f)", oldDestWidth, destHeight, destWidth, destHeight)
				}
			}

			if params.Gravity == 0 {
				glog.Error("cropmode=1 & gravity = 0 is invalid condition")
				log.Println("cropmode=1 & gravity = 0 is invalid condition")
				return nil
			} else {
				cropX, cropY = getCropGeometry(srcAspect, destAspect, srcHeight, cropHeight, srcWidth, cropWidth, params.Gravity)
			}
		}
		virtualMappedWidth = srcWidth
		virtualMappedHeight = destWidth
	} else if params.CropMode == 2 {
		// 余白をつける (マージン方式)
		// 横と縦、両方の辺が元より大きい場合は、リサイズしない
		if !params.Upscale && srcWidth < destWidth &&
			srcHeight < destHeight {
			mappedWidth = srcWidth
			mappedHeight = srcHeight
		} else {
			// 元画像のアスペクト比に合わせた縦横サイズを計算する
			// (アスペクト比の小さい辺に合わせてリサイズする)
			if srcAspect < destAspect {
				// 縦に余白をつける。
				mappedWidth = destHeight * srcAspect
				mappedHeight = destHeight
			} else {
				// 横に余白をつける
				mappedWidth = destWidth
				mappedHeight = destWidth / srcAspect
			}
		}

		mappedX = round((destWidth - mappedWidth) * getHorizontal(params.Gravity))
		mappedY = round((destHeight - mappedHeight) * getVertical(params.Gravity))
		virtualMappedWidth = mappedWidth
		virtualMappedHeight = mappedHeight
	} else {
		glog.Error("Invalie CropMode:%d", params.CropMode)
		log.Printf("Invalie CropMode:%d", params.CropMode)
		return nil
	}

	// JPEG scaling decode hinting
	if virtualMappedWidth < srcWidth/2 && virtualMappedHeight < srcHeight/2 {
		jpegSize := fmt.Sprintf("%dx%d", roundInt(virtualMappedWidth*2), roundInt(virtualMappedHeight*2))
		mw.SetOption("jpeg:size", jpegSize)
	}

	// Decode Image
	mw = imagick.NewMagickWand()
	defer mw.Destroy()
	mw.SetResourceLimit(imagick.RESOURCE_THREAD, 1)

	err = mw.ReadImageBlob(bytes)
	if err != nil {
		glog.Error("Upstream ReadImageBlob failed: " + err.Error())
		log.Println("Upstream ReadImageBlob failed: " + err.Error())
		return err
	}

	mw.SetFirstIterator()

	for mw.GetNumberImages() > 1 {
		mw.NextImage()
		mw.RemoveImage()
		mw.SetFirstIterator()
	}

	if !isOutputTransparent(mw.GetImageFormat(), params.FormatOutput) &&
		len(params.Background) == 9 && params.Background[0] == '#' {
		params.Background = params.Background[0:7]
	}

	/*
	 * 画像のリサイズ処理。(クロップ方式、マージン方式)
	 */
	if params.CropMode == 0 {
		// リサイズのみ。クロップもマージンも無し
		err = mw.ResizeImage(round(destWidth), round(destHeight), imagick.FILTER_UNDEFINED, 1)
		if err != nil {
			glog.Error("Upstream ResizeImage failed: " + err.Error())
			log.Println("Upstream ResizeImage failed: " + err.Error())
			return err
		}
	} else if params.CropMode == 1 {
		// クロップとリサイズを同時に行う
		// fmt.Printf("TransformImage:  cropX:%d cropY:%d cropWidth:%f, cropHeight:%f destWidth:%f, destHeight:%f\n", cropX, cropY, cropWidth, cropHeight, destWidth, destHeight)
		geoSrc := fmt.Sprintf("%dx%d+%d+%d", round(cropWidth), round(cropHeight), cropX, cropY)
		geoDest := fmt.Sprintf("%dx%d!", round(destWidth), round(destHeight))
		//		fmt.Println("geo_src, geo_dest: ", geo_src, geo_dest)
		mw2 := mw.TransformImage(geoSrc, geoDest)
		defer mw2.Destroy()
		err := mw2.GetLastError()
		if err != nil {
			glog.Error("TransformImage failed")
			log.Println("TransformImage failed")
			return err
		}
		mw = mw2
		mw.ResetImagePage("") // +repage
	} else if params.CropMode == 2 {
		// 余白をつける (マージン方式)
		pw := imagick.NewPixelWand()
		defer pw.Destroy()

		pw.SetColor(params.Background)
		mw.SetImageBackgroundColor(pw) // 余白の色

		err = mw.ResizeImage(round(mappedWidth), round(mappedHeight), imagick.FILTER_UNDEFINED, 1)
		if err != nil {
			glog.Error("Upstream ResizeImage failed: " + err.Error())
			log.Println("Upstream ResizeImage failed: " + err.Error())
			return err
		}
	}
	/*
	 * 上書き画像の処理
	 */
	if params.ImageOverlap != nil {

		mwc := imagick.NewMagickWand()
		defer mwc.Destroy()

		mwc.SetResourceLimit(imagick.RESOURCE_THREAD, 1)

		// 上書き画像を読み込み
		bytes, err := ioutil.ReadAll(params.ImageOverlap)
		if err != nil {
			glog.Error("Upstream ImageOverlap image read failed")
			return err
		}

		err = mwc.PingImageBlob(bytes)
		if err != nil {
			glog.Error("Upstream ImageOverlap read pingimage image read failed")
			return err
		}

		var srcOverlapWidth float64 = float64(mwc.GetImageWidth())
		var srcOverlapHeight float64 = float64(mwc.GetImageHeight())

		//合成画像のサイズを出す
		var imageOverlapWidth, imageOverlapHeight uint
		var xScaleFactor, yScaleFactor float64
		if params.ImageOverlapHeightRatio == 0 && params.ImageOverlapWidthRatio == 0 {
			// 指定がなければ地画像と同じスケール変化
			xScaleFactor = mappedWidth / srcWidth
			yScaleFactor = xScaleFactor
		} else if params.ImageOverlapWidthRatio != 0 && params.ImageOverlapHeightRatio == 0 {
			//片方だけ指定されてたら同アスペクトで変化
			xScaleFactor = params.ImageOverlapWidthRatio * mappedWidth / srcOverlapWidth
			yScaleFactor = xScaleFactor
		} else if params.ImageOverlapHeightRatio != 0 && params.ImageOverlapWidthRatio == 0 {
			yScaleFactor = params.ImageOverlapHeightRatio * mappedHeight / srcOverlapHeight
			xScaleFactor = yScaleFactor
		} else {
			// この場合アスペクト比を維持しない事に注意
			xScaleFactor = params.ImageOverlapWidthRatio * mappedWidth / srcOverlapWidth
			yScaleFactor = params.ImageOverlapHeightRatio * mappedHeight / srcOverlapHeight
		}

		imageOverlapWidth = round(xScaleFactor * srcOverlapWidth)
		imageOverlapHeight = round(yScaleFactor * srcOverlapHeight)

		// 上書きする位置を計算する
		var iox, ioy int
		var xRatio float64 = params.ImageOverlapXRatio
		var yRatio float64 = params.ImageOverlapYRatio
		if params.ImageOverlapGravity != 0 { // gravityを指定されている場合
			xRatio = getHorizontal(params.ImageOverlapGravity)
			yRatio = getVertical(params.ImageOverlapGravity)
		}

		iox = roundInt(xRatio * (mappedWidth - float64(imageOverlapWidth)))
		ioy = roundInt(yRatio * (mappedHeight - float64(imageOverlapHeight)))
		if (float64(imageOverlapWidth) < srcOverlapWidth/2) &&
			(float64(imageOverlapHeight) < srcOverlapHeight/2) {
			jpeg_size := fmt.Sprintf("%dx%d", uint(imageOverlapWidth*2), uint(imageOverlapHeight*2))
			mwc.SetOption("jpeg:size", jpeg_size)
		}
		mwc.ReadImageBlob(bytes)
		// mwc.SetFirstIterator()
		mwc.ResizeImage(imageOverlapWidth, imageOverlapHeight, imagick.FILTER_UNDEFINED, 1)

		mwc.SetImageMatte(true) // 透明度を有効にする
		mw.CompositeImage(mwc, imagick.COMPOSITE_OP_OVER, iox, ioy)
	}

	/*
	 *  アノテーション。(文字列を上書きする)
	 */
	if params.Text != "" {
		dw := imagick.NewDrawingWand()
		defer dw.Destroy()
		var japanese_font_list []string = nil
		if len(params.TextFont) > 0 {
			japanese_font_list = params.TextFont
		} else {
			japanese_font_list = []string{"Noto-Sans-CJK-JP-Medium", "IPA-Pゴシック-Regular", "IPAゴシック-Regular", "VL-Pゴシック-regular", "IPAGothic-Regular"}
		}
		// (日本語)フォントが見つけたら、それを適用する。
		for _, font := range japanese_font_list {
			found_fonts := mw.QueryFonts(font)
			if len(found_fonts) > 0 {
				dw.SetFont(font)
				break
			}
		}
		dw.SetFontSize(params.TextFontSize)

		TextGravity := getGravityValue(params.TextGravity)
		dw.SetGravity(TextGravity)

		// 太くする分はみ出たり、枠と文字がずれるので、
		// SetStrokeWidth に合わせて補正。
		var textX float64 = 0.0
		var textY float64 = 0.0
		var textX2 float64 = 0.0
		var textY2 float64 = 0.0
		switch getHorizontal(params.TextGravity) {
		case 0.0:
			textX = 3.5
			textX2 = 3
		case 0.5:
			textX = 0.5
			textX2 = 0
		case 1.0:
			textX = 2.5
			textX2 = 3
		}
		switch getVertical(params.TextGravity) {
		case 0.0:
			textY = 3.5
			textY2 = 3
		case 0.5:
			textY = 0.4
			textY2 = 0
		case 1.0:
			textY = 2.6
			textY2 = 3
		}
		//              fmt.Printf("textX:%f textY:%f\n", textX, textY)

		cw := imagick.NewPixelWand()
		defer cw.Destroy()
		if params.TextColor == "" {
			cw.SetColor("rgb(0,0,0)")
			dw.SetStrokeWidth(2.5)
			dw.SetStrokeColor(cw)
			dw.Annotation(textX2, textY2, params.Text)

			cw.SetColor("rgba(0,0,0,0)")
			dw.SetStrokeColor(cw)
			cw.SetColor("rgb(255,255,255)")
		} else {
			cw.SetColor(params.TextColor)
		}

		dw.SetFillColor(cw)
		dw.Annotation(textX, textY, params.Text)

		mw.DrawImage(dw)

	}

	// 座標情報をResetImagePageで落とす
	err = mw.ResetImagePage("")
	if err != nil {
		glog.Error("Upstream ResetImagePage failed: " + err.Error())
		log.Println("Upstream ResetImagePage failed: " + err.Error())
		return err
	}

	if params.CropMode < 2 {
		// 透明ピクセル背景色を適用する
		pw := imagick.NewPixelWand()
		defer pw.Destroy()
		pw.SetColor(params.Background)
		err := mw.SetImageBackgroundColor(pw) // 背景色の色
		if err != nil {
			glog.Error("SetImageBackgroundColor failed: " + err.Error())
			log.Println("SetImageBackgroundColor failed: " + err.Error())
			return err
		}
		// Flatten 処理
		mw = mw.MergeImageLayers(imagick.IMAGE_LAYER_FLATTEN)
		defer mw.Destroy()
	} else { // params.CropMode == 2
		// マージン方式の時は縦横サイズを拡張する。
		err := mw.ExtentImage(round(destWidth), round(destHeight), -int(mappedX), -int(mappedY))
		if err != nil {
			panic(err)
			return err
		}
		err = mw.ResetImagePage("") // +repage
		if err != nil {
			glog.Error("Upstream ResetImagePage failed: " + err.Error())
			log.Println("Upstream ResetImagePage failed: " + err.Error())
			return err
		}
	}
	// JPEG, WebP
	err = mw.SetImageCompressionQuality(uint(params.Quality))
	if err != nil {
		panic(err)
	}
	// HEIC
	err = mw.SetCompressionQuality(uint(params.Quality))
	if err != nil {
		panic(err)
	}
	//画像出力フォーマット指定
	switch params.FormatOutput {
	case "jpg", "jpeg":
		err = mw.SetImageFormat("jpeg")
	case "webp":
		err = mw.SetImageFormat("webp")
	case "png":
		err = mw.SetImageFormat("png")
	case "gif":
		err = mw.SetImageFormat("gif")
	case "heic", "heif":
		err = mw.SetImageFormat("heic")
	case "":
		// nothing
	}

	if err != nil {
		glog.Error("Upstream SetImageFormat failed: " + err.Error())
		log.Println("Upstream SetImageFormat failed: " + err.Error())
		return err
	}

	//画像出力
	blob := mw.GetImagesBlob()
	if params.HttpAvoidChunk {
		dst.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob)))
	}

	dst.Write(blob)

	return nil
}
