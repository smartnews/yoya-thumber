// This function comes from yoya's gist. For more details, see: https://github.com/yoya/misc/blob/master/go/heifsetimagesize.go

package thumbnail

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type HEIF struct {
	Boxes []*HEIFBox
}
type HEIFBox struct {
	Offset uint32
	Size   uint32
	Name   string
	Data   []byte
	Boxes  []*HEIFBox
}

func HEIFParse(bytes []byte) (*HEIF, error) {
	heif := &HEIF{}
	boxes, err := HEIFParseBoxes(bytes, 0)
	if err != nil {
		return nil, err
	}
	heif.Boxes = boxes
	return heif, nil
}

func HEIFParseBoxes(bytes []byte, baseOffset uint32) ([]*HEIFBox, error) {
	var boxes []*HEIFBox
	var bin = binary.BigEndian
	var boxOffset uint32 = 0
	length := uint32(len(bytes))
	for boxOffset < length {
		offset := boxOffset
		boxSize := bin.Uint32(bytes[offset:])
		if boxSize < 8 {
			boxSize = length - boxOffset
		}
		nextOffset := boxOffset + boxSize
		if length < nextOffset {
			return nil, fmt.Errorf("length:%d < nextOffset:%d",
				length, nextOffset)
		}
		boxName := string(bytes[offset+4 : offset+8])
		box := &HEIFBox{
			Offset: baseOffset + boxOffset,
			Size:   boxSize, Name: boxName,
			Data: bytes[offset+8 : nextOffset],
		}
		isContainer := false
		offset += 8
		switch boxName {
		case "meta":
			offset += 4 // version & flags
			isContainer = true
		case "dinf", "iprp", "ipco":
			isContainer = true
		case "iinf":
			version := bytes[offset]
			if version <= 1 {
				offset += 4 + 2 // version & flags, count
			} else {
				offset += 4 + 4 // version & flags, count
			}
			isContainer = true
		}
		if isContainer {
			box.Data = nil
			boxes, err := HEIFParseBoxes(bytes[offset:nextOffset], baseOffset+offset)
			if err != nil {
				return nil, err
			}
			box.Boxes = boxes
		}
		boxes = append(boxes, box)
		boxOffset = nextOffset
	}
	return boxes, nil
}

func (box *HEIFBox) GetBoxesByName(name string) ([]*HEIFBox, error) {
	var boxes []*HEIFBox
	if box.Name == name {
		boxes = append(boxes, box)
	}
	for _, box2 := range box.Boxes {
		bs, err := box2.GetBoxesByName(name)
		if err != nil {
			return nil, err
		}
		if len(bs) > 0 {
			boxes = append(boxes, bs...)
		}
	}
	return boxes, nil
}

func (heif *HEIF) GetBoxesByName(name string) ([]*HEIFBox, error) {
	var boxes []*HEIFBox
	for _, box := range heif.Boxes {
		bs, err := box.GetBoxesByName(name)
		if err != nil {
			return nil, err
		}
		if len(bs) > 0 {
			boxes = append(boxes, bs...)
		}
	}
	return boxes, nil
}

func (heif *HEIF) GetPrimaryItemId() (uint16, error) {
	var bin = binary.BigEndian
	boxes, err := heif.GetBoxesByName("pitm")
	if err != nil {
		return 0, err
	}
	if len(boxes) < 1 {
		return 0, errors.New("not found pitm box")
	}
	primaryId := bin.Uint16(boxes[0].Data[4:])
	return primaryId, nil
}

func (heif *HEIF) SetImageSize(bytes []byte, itemId uint16, width, height uint32) error {
	var bin = binary.BigEndian
	ipmaBoxes, err := heif.GetBoxesByName("ipma")
	if err != nil {
		return err
	}
	if len(ipmaBoxes) < 1 {
		return errors.New("not found ipma box")
	}
	ipma := ipmaBoxes[0]
	//
	ipcoBoxes, err := heif.GetBoxesByName("ipco")
	if err != nil {
		return err
	}
	if len(ipcoBoxes) < 1 {
		return errors.New("not found ipco box")
	}
	ipco := ipcoBoxes[0]
	//
	flags3 := ipma.Data[3]
	entryCount := bin.Uint32(ipma.Data[4:])
	offset := 8
	for i := uint32(0); i < entryCount; i++ {
		id := bin.Uint16(ipma.Data[offset:])
		offset += 2
		essenCount := int(ipma.Data[offset])
		offset += 1
		if id != itemId {
			if (flags3 & 1) == 1 {
				offset += essenCount * 2
			} else {
				offset += essenCount
			}
		} else {
			for j := 0; j < essenCount; j++ {
				propertyIndex := int(ipma.Data[offset]) & 0x7F
				offset += 1
				if (flags3 & 1) == 1 {
					propertyIndex = (propertyIndex << 7) +
						int(ipma.Data[offset])
					offset += 1
				}
				if len(ipco.Boxes) < propertyIndex {
					return fmt.Errorf("len(ipco.Boxes):%d <= propertyIndex:%d", len(ipco.Boxes), propertyIndex)
				}
				if ipco.Boxes[propertyIndex-1].Name == "ispe" {
					ispe := ipco.Boxes[propertyIndex-1]
					ispeOffset := ispe.Offset
					bin.PutUint32(bytes[ispeOffset+12:], width)
					bin.PutUint32(bytes[ispeOffset+16:], height)
				}
			}
		}
	}
	return nil
}

func HEIFSetImageSize(bytes []byte, width, height uint32) error {
	heif, err := HEIFParse(bytes)
	if err != nil {
		return err
	}
	primaryId, err := heif.GetPrimaryItemId()
	if err != nil {
		return err
	}
	err = heif.SetImageSize(bytes, primaryId, width, height)
	return err
}
