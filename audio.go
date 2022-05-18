package main

import (
	"crypto/md5"
	_ "embed"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"os"

	"github.com/h2non/filetype"
	"github.com/h2non/filetype/matchers"
	types "github.com/h2non/filetype/types"
)

var AllowedAudioTypes = map[types.Type]bool{
	matchers.TypeMp3:  true,
	matchers.TypeOgg:  true,
	matchers.TypeAac:  true,
	matchers.TypeWebm: true,
}

func (x *RequestsHandler) ProcessTrack(body io.ReadCloser) (error, string) {
	hasher := md5.New()

	bodyReader := io.TeeReader(body, hasher)

	buf := make([]byte, 265)

	n, err := bodyReader.Read(buf)

	if check_error(err) {
		return err, ""
	}

	t, err := filetype.Get(buf[:n])

	if _, ok := AllowedAudioTypes[t]; !ok {
		return errors.New("Not accepted audio type"), ""
	}

	file, err := ioutil.TempFile(x.Audiopath, "tmp")

	if err != nil {
		return err, ""
	}

	defer func() {
		_, err := os.Stat(file.Name())
		if err == nil {
			os.Remove(file.Name())
		}
	}()

	file.Write(buf[:n])
	_, err = io.Copy(file, bodyReader)

	if err != nil {
		return err, ""
	}
	file.Close()

	bhash := hasher.Sum(nil)
	hash := hex.EncodeToString(bhash[:])
	os.Rename(file.Name(), x.Audiopath+hash)
	return err, hash
}
