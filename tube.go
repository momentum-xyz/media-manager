// Part of this code is borrowed from/inspired by https://github.com/iqhater/get-youtube-thumbnail, which is licensed by MIT license
//
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/draw"
	"net/http"
	"strings"

	_ "embed"

	"github.com/nfnt/resize"
)

type TubeDesc struct {
	Url string `json:"url"`
}

var playbutton image.Image

//go:embed play_button_1280x720.png
var playbuttonpng []byte

func init() {
	playbutton, _, _ = image.Decode(bytes.NewReader(playbuttonpng))
}

// findVideoID extract video id from raw input url
// and save it at Thumbnail struct. Also checks id length
// and bad symbols at id sequence.
func getThumbnail(urlVideo string) (*http.Response, error) {
	// two possible resolutions
	const (
		vi     = "https://i.ytimg.com/vi/"
		resMax = "/maxresdefault.jpg"
		resHQ  = "/hqdefault.jpg"
	)

	equalIndex := strings.Index(urlVideo, "=")
	ampIndex := strings.Index(urlVideo, "&")
	slash := strings.LastIndex(urlVideo, "/")
	questionIndex := strings.Index(urlVideo, "?")
	var id string

	if equalIndex != -1 {
		if ampIndex != -1 {
			id = urlVideo[equalIndex+1 : ampIndex]
		} else if questionIndex != -1 && strings.Contains(urlVideo, "?t=") {
			id = urlVideo[slash+1 : questionIndex]
		} else {
			id = urlVideo[equalIndex+1:]
		}
	} else {
		id = urlVideo[slash+1:]
	}

	if strings.ContainsAny(id, "?&/<%=") {
		return nil, errors.New("invalid characters in video id")
	}
	if len(id) < 10 {
		return nil, errors.New("the video id must be at least 10 characters long")
	}

	resp, err := http.Get(vi + id + resMax)

	if err != nil || resp.StatusCode != 200 {
		resp, err = http.Get(vi + id + resHQ)
		if err != nil || resp.StatusCode != 200 {
			L().Info("Response Status Code: %v\n", resp.StatusCode)
			return nil, err
		}
	}
	return resp, nil
}

func (x *RequestsHandler) ProcessTube(src []byte) (error, string) {
	url := src
	var payload TubeDesc
	if json.Unmarshal(src, &payload) == nil {
		url = []byte(payload.Url)
	}

	hash := GetMD5HashByte(url)
	meta, _ := x.present(&hash)
	if meta == nil {
		resp, err := getThumbnail(string(url))
		if check_error(err) {
			return err, ""
		}

		if thumb, _, err := image.Decode(resp.Body); !check_error(err) {
			nx := thumb.Bounds().Max.X
			ny := thumb.Bounds().Max.Y

			imgovl := resize.Resize(0, uint(ny), playbutton, resize.Bilinear)

			imout := image.NewRGBA(thumb.Bounds())
			draw.Draw(imout, thumb.Bounds(), thumb, image.ZP, draw.Src)
			offset := image.Pt((nx-imgovl.Bounds().Max.X)/2, 0)
			draw.Draw(imout, imgovl.Bounds().Add(offset), imgovl, image.ZP, draw.Over)
			err = x.SaveWriteToPNG(x.ImPathF+hash, imout)
			if err != nil {
				return err, ""
			}
			for _, v := range Tprecalcs {
				if err = x.WriteToScaled(hash, imout, v); err != nil {
					return err, ""
				}
			}
		}

	}
	return nil, hash
}
