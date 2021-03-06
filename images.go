package main

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io/ioutil"
	"math"
	"os"

	"github.com/nfnt/resize"
	_ "golang.org/x/image/webp"
)

var Tsizes = map[string]int{
	"s0": 1024,
	"s1": 4096,
	"s2": 9216,
	"s3": 25600,
	"s4": 65536,
	"s5": 193600,
	"s6": 577600,
	"s7": 1721344,
	"s8": 5062500,
	"s9": 14745600,
}

var Tprecalcs = [...]string{"s2", "s3", "s4", "s5", "s6"}

func (x *RequestsHandler) WriteToF(img image.Image) (error, string) {
	var w bytes.Buffer
	err := png.Encode(&w, img)
	if err != nil {
		return err, ""
	}
	body := w.Bytes()
	ID := GetMD5HashByte(body)
	return x.SaveWriteToFile(x.ImPathF+ID, body), ID
}

func (x *RequestsHandler) WriteToScaled(base string, img image.Image, rsize string) error {
	if size, ok := Tsizes[rsize]; ok {
		return x.SaveWriteToPNG(x.ImPathS[rsize]+base, DownSampleTo(img, size))
	}
	return errors.New("Not such size defined in the size map")

}

func (x *RequestsHandler) SaveWriteToPNG(fname string, img image.Image) error {
	tfilename := fname + ".tmp"

	w, err := os.Create(tfilename)
	if err != nil {
		return err
	}

	err = png.Encode(w, img)
	w.Close()
	if err != nil {
		os.Remove(tfilename)
		return err
	}
	if err = os.Rename(tfilename, fname); err != nil {
		os.Remove(tfilename)
		return err
	}
	return nil
}

func (x *RequestsHandler) SaveWriteToFile(fname string, data []byte) error {
	tfilename := fname + ".tmp"
	err := ioutil.WriteFile(tfilename, data, 0666)
	if err != nil {
		os.Remove(tfilename)
		return err
	}
	if err = os.Rename(tfilename, fname); err != nil {
		return err
	}
	return nil
}

func (x *RequestsHandler) ProcessImage(src []byte) (error, string) {
	img, format, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return err, ""
	}
	L().Info("Incoming image:", format)

	var ID string
	if format == "gif" {
		ID = GetMD5HashByte(src)
		err = x.SaveWriteToFile(x.ImPathF+ID, src)
	} else {
		err, ID = x.WriteToF(img)
	}

	if err != nil {
		return err, ""
	}

	for _, v := range Tprecalcs {
		if err = x.WriteToScaled(ID, img, v); err != nil {
			return err, ""
		}
	}

	L().Info("Hash:", ID)
	return err, ID

}

func DownSampleTo(img image.Image, NewPixelCount int) image.Image {
	ox := float64(img.Bounds().Max.X)
	oy := float64(img.Bounds().Max.Y)

	scl := math.Sqrt(float64(NewPixelCount) / (ox * oy))

	nx := uint(math.Round(ox * scl))
	ny := uint(math.Round(oy * scl))

	imgout := resize.Thumbnail(nx, ny, img, resize.Bilinear)
	return imgout
}
