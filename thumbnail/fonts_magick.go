// return the thumbnailed version.
package thumbnail

import (
	"encoding/json"
	"fmt"
	"gopkg.in/gographics/imagick.v2/imagick"
	"io"
)

// type ThumbnailParameters struct {
//thumbnail.goで定義済み
// }

func FontsMagick(dst io.Writer) error {
	// var err error
	mw := imagick.NewMagickWand()
	defer mw.Destroy()

	fonts := mw.QueryFonts("*")
	bytes, err := json.Marshal(fonts)
	if err != nil {
		return err
	}
	fmt.Fprintln(dst, string(bytes))
	return nil
}
