package thumbnail

import (
	"errors"
	"math"
)

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
