package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	"image"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/riff"
	_ "golang.org/x/image/vp8"
	_ "golang.org/x/image/vp8l"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi"
	"github.com/h2non/filetype"
	"github.com/hashicorp/golang-lru"
)

type FrameRenderRequest struct {
	ID    *string
	Frame *FrameDesc
	wg    sync.WaitGroup
}

type MetaDef struct {
	H, W int
	mime string
}

func check_error(err error) bool {
	if err != nil {
		L().Error(err)
		sentry.CaptureException(err)
		return true
	}
	return false
}

type RequestsHandler struct {
	Fontpath  string
	Imagepath string
	Audiopath string
	ImPathF   string
	ImPathS   map[string]string
	ImageMapF *lru.Cache
	ImageMapS map[string]*lru.Cache
	// ImageMap         map[string]bool
	PresentMutex     sync.Mutex
	framesinprogress map[string]bool
	RenderQueue      chan *FrameRenderRequest
	RenderDone       chan *FrameRenderRequest
}

const defaultCacheSize = 1024

var API_PREFIX = "/api/v3"

func DefRequestsHandler(cfg *LocalConfig) *RequestsHandler {
	x := new(RequestsHandler)
	x.Imagepath = strings.TrimSuffix(cfg.Imagepath, "/") + "/"
	x.Audiopath = strings.TrimSuffix(cfg.Audiopath, "/") + "/"
	x.Fontpath = strings.TrimSuffix(cfg.Fontpath, "/") + "/"
	x.framesinprogress = make(map[string]bool)
	x.RenderQueue = make(chan *FrameRenderRequest, 512)
	x.RenderDone = make(chan *FrameRenderRequest, 512)
	os.MkdirAll(x.Imagepath, os.ModePerm)
	os.MkdirAll(x.Audiopath, os.ModePerm)

	os.MkdirAll(x.Imagepath+"F", os.ModePerm)
	x.ImPathF = x.Imagepath + "F/"

	x.ImageMapF, _ = lru.New(defaultCacheSize)
	x.ImageMapS = make(map[string]*lru.Cache)
	x.ImPathS = make(map[string]string)
	for rs := range Tsizes {
		os.MkdirAll(x.Imagepath+rs, os.ModePerm)
		x.ImPathS[rs] = x.Imagepath + rs + "/"
		x.ImageMapS[rs], _ = lru.New(defaultCacheSize)
	}

	os.MkdirAll(strings.TrimSuffix(x.ImPathF, "/"), 0775)
	go x.run()
	return x
}

func (x *RequestsHandler) run() {
	L().Info("Runner...")
	for {
		select {
		case req := <-x.RenderQueue:
			if !x.framesinprogress[*req.ID] {
				x.framesinprogress[*req.ID] = true
				go x.RenderFrame(req)
			}
			n := len(x.RenderQueue)
			for i := 0; i < n; i++ {
				req := <-x.RenderQueue
				if !x.framesinprogress[*req.ID] {
					x.framesinprogress[*req.ID] = true
					go x.RenderFrame(req)
				}
			}
		case req := <-x.RenderDone:
			delete(x.framesinprogress, *req.ID)
			n := len(x.RenderDone)
			for i := 0; i < n; i++ {
				req := <-x.RenderDone
				delete(x.framesinprogress, *req.ID)
			}
		}
	}
}

func (x *RequestsHandler) present(ID *string) (*MetaDef, *string) {
	fpath := x.ImPathF + *ID
	res, ok := x.ImageMapF.Get(*ID)

	if ok {
		L().Debug(*ID + " is already in the map")
		return res.(*MetaDef), &fpath
	}

	reader, err := os.Open(fpath)
	L().Debug(fpath)

	if err != nil {
		return nil, nil
	}

	defer reader.Close()
	im, format, err1 := image.DecodeConfig(reader)
	meta := new(MetaDef)
	if err1 != nil {
		L().Debug("%s: %v\n", *ID, err1)
		meta.mime = "image/png"
	} else {
		meta.H = im.Height
		meta.W = im.Width
		meta.mime = "image/" + format
		L().Debug("%s %d %d\n", *ID, im.Width, im.Height)
	}
	L().Debug("Mime:", meta.mime)
	x.ImageMapF.Add(*ID, meta)
	return meta, &fpath

}

func (x *RequestsHandler) presentTexture(ID *string, rsize string) (*MetaDef, *string) {
	fpath := x.ImPathS[rsize] + *ID
	meta0, ok := x.ImageMapS[rsize].Get(*ID)
	if ok {
		L().Debug(*ID + " is already in the map")
		return meta0.(*MetaDef), &fpath
	}

	L().Debug(fpath)
	reader, err := os.Open(fpath)
	if err != nil {
		converted := false
		L().Debug(*ID + " : converting from full")
		if meta, filepath := x.present(ID); meta != nil {
			if reader, err = os.Open(*filepath); !check_error(err) {
				if img, _, errl := image.Decode(reader); !check_error(errl) {
					if err = x.WriteToScaled(*ID, img, rsize); !check_error(err) {
						if reader, err = os.Open(fpath); err == nil {
							converted = true
						}
					}
				}
			}
		}

		if !converted {
			return nil, nil
		}
	}

	defer reader.Close()
	im, format, err1 := image.DecodeConfig(reader)
	meta := new(MetaDef)
	if err1 != nil {
		L().Debug("%s: %v\n", *ID, err1)
		meta.mime = "image/png"
	} else {
		meta.H = im.Height
		meta.W = im.Width
		meta.mime = "image/" + format
		L().Debug("%s %d %d\n", *ID, im.Width, im.Height)
	}
	L().Debug("Mime:", meta.mime)
	x.ImageMapS[rsize].Add(*ID, meta)
	return meta, &fpath
}

func homePage(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Endpoint Hit: homePage")
}

func makeTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func (x *RequestsHandler) getImage(w http.ResponseWriter, r *http.Request) {
	tm1 := makeTimestamp()
	filename := chi.URLParam(r, "file")

	L().Debug("Endpoint Hit: Image Get:", filename)

	res, filepath := x.present(&(filename))
	if res == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", res.mime)
	w.Header().Set("x-height", strconv.Itoa(res.H))
	w.Header().Set("x-width", strconv.Itoa(res.W))
	http.ServeFile(w, r, *filepath)
	L().Info("Endpoint Hit: Image served: %s %d", filename, (makeTimestamp() - tm1))
}

func (x *RequestsHandler) getTrack(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "file")

	L().Debug("Endpoint Hit: Track Get:", filename)

	// res, filepath := x.present(&(filename))
	filepath := x.Audiopath + filename

	buf := make([]byte, 264)
	f, err := os.Open(filepath)
	if check_error(err) {
		w.WriteHeader(http.StatusBadRequest)
		sentry.CaptureException(err)
		L().Error(err)
		return
	}
	defer f.Close()

	n, err := f.Read(buf)
	if check_error(err) {
		w.WriteHeader(http.StatusBadRequest)
		sentry.CaptureException(err)
		L().Error(err)
		return
	}

	ftype, err := filetype.Get(buf[:n])

	w.Header().Set("Content-Type", ftype.MIME.Value)

	http.ServeFile(w, r, filepath)
	L().Info("Endpoint Hit: Track served: %s", filename)
}

func (x *RequestsHandler) getTexture(w http.ResponseWriter, r *http.Request) {
	tm1 := makeTimestamp()
	rsize := chi.URLParam(r, "rsize")
	filename := chi.URLParam(r, "file")

	if _, ok := Tsizes[rsize]; !ok {
		http.NotFound(w, r)
		return
	}

	L().Debug("Endpoint Hit: Texture Get:", filename, rsize)

	res, filepath := x.presentTexture(&(filename), rsize)
	if res == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", res.mime)
	w.Header().Set("x-height", strconv.Itoa(res.H))
	w.Header().Set("x-width", strconv.Itoa(res.W))
	http.ServeFile(w, r, *filepath)
	L().Info("Endpoint Hit: Texture served: %s %d", filename, (makeTimestamp() - tm1))
}

func (x *RequestsHandler) addFrame(w http.ResponseWriter, r *http.Request) {
	L().Debug("Endpoint Hit: Add Frame")

	IDi := chi.URLParam(r, "ID")
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("{\"Error\":\"body\"}"))
		sentry.CaptureException(err)
		L().Error("{\"Error\":\"body\"}")
		return
	}

	var payload FrameDesc
	err = json.Unmarshal(body, &payload)

	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte("{\"Error\":\"json\"}"))
		sentry.CaptureException(err)
		L().Error("{\"Error\":\"json\"}")
		return
	}

	ID := GetMD5HashByte(body)

	if IDi != "" && IDi != ID {
		w.WriteHeader(http.StatusNotAcceptable)
		w.Write([]byte("{\"Error\":\"hash\"}"))
		L().Error("{\"Error\":\"hash\"}")
		sentry.CaptureException(errors.New("MD5 hash doesn't match"))
		return
	}

	L().Debug(ID)
	// func() {
	str, err := json.MarshalIndent(payload, "", "    ")
	if err != nil {
		sentry.CaptureException(err)
	}
	L().Debug(string(str))
	res, _ := x.present(&ID)
	// ok := false
	if res == nil {
		req := &FrameRenderRequest{ID: &ID, Frame: &payload}
		req.wg.Add(1)
		x.RenderQueue <- req
		req.wg.Wait()
	}

	// }()
	// time.Sleep(time.Millisecond * 10)
	L().Debug("responding with hash")
	w.Write([]byte("{\"hash\":\"" + ID + "\"}"))

}

func (x *RequestsHandler) addImage(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Endpoint Hit: addImage")
	// IDi := chi.URLParam(r, "ID")

	body0, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		sentry.CaptureException(err)
		L().Error(fmt.Errorf("error during reading body: %v", err))
		return
	}
	err, hash := x.ProcessImage(body0)
	if err != nil {
		sentry.CaptureException(err)
		L().Error(fmt.Errorf("error during writing image: %v", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write([]byte("{\"hash\":\"" + hash + "\"}"))
	return
}

func (x *RequestsHandler) renderTube(w http.ResponseWriter, r *http.Request) {
	L().Info("Endpoint Hit: Add Tube")
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		sentry.CaptureException(err)
		L().Error(fmt.Errorf("error during reading body: %v", err))
		return
	}
	defer r.Body.Close()
	err, hash := x.ProcessTube(body)
	if err != nil {
		sentry.CaptureException(err)
		L().Error(fmt.Errorf("error during writing image: %v", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write([]byte("{\"hash\":\"" + hash + "\"}"))
	return
}

func (x *RequestsHandler) addTrack(w http.ResponseWriter, r *http.Request) {
	L().Info("Endpoint Hit: Add Track")

	defer r.Body.Close()

	// err, hash := x.ProcessTrack(file.Name())
	err, hash := x.ProcessTrack(r.Body)

	if err != nil {
		sentry.CaptureException(err)
		L().Error(fmt.Errorf("error during writing audio track: %v", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write([]byte("{\"hash\":\"" + hash + "\"}"))
	return
}

func (x *RequestsHandler) delTrack(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "file")

	L().Info("Endpoint Hit: Track Delete:", filename)

	filepath := x.Audiopath + filename
	err := os.Remove(filepath)
	if err != nil {
		sentry.CaptureException(err)
		L().Error(fmt.Errorf("error during deletion of audio track: %v", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	L().Info("Endpoint Hit: Delete Track served:")
}
func handle_http(cfg *LocalConfig) {
	myRouter := chi.NewRouter()
	// myRouter.Use(middleware.Logger)
	myRequestsHandler := DefRequestsHandler(cfg)

	myRouter.HandleFunc("/", homePage)
	myRouter.MethodFunc("POST", "/render/addimage", myRequestsHandler.addImage)
	myRouter.MethodFunc("POST", "/render/addframe", myRequestsHandler.addFrame)
	myRouter.MethodFunc("POST", "/render/addtube", myRequestsHandler.renderTube)
	myRouter.MethodFunc("POST", "/addtrack", myRequestsHandler.addTrack)
	myRouter.MethodFunc("DELETE", "/deltrack/{file:[a-zA-Z0-9]+}", myRequestsHandler.delTrack)
	myRouter.MethodFunc("GET", API_PREFIX+"/render/get/{file:[a-zA-Z0-9]+}", myRequestsHandler.getImage)
	myRouter.MethodFunc("GET", API_PREFIX+"/render/texture/{rsize:s[0-9]}/{file:[a-zA-Z0-9]+}", myRequestsHandler.getTexture)
	myRouter.MethodFunc("GET", API_PREFIX+"/render/track/{file:[a-zA-Z0-9]+}", myRequestsHandler.getTrack)

	if err := http.ListenAndServe(cfg.Address+":"+strconv.FormatUint(uint64(cfg.Port), 10), myRouter); err != nil {
		L().Fatal(errors.WithMessage(err, "failed to start service"))
	}
}

func GetMD5HashString(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func GetMD5HashByte(text []byte) string {
	hash := md5.Sum(text)
	return hex.EncodeToString(hash[:])
}

func GetMD5HashFile(name string) (string, error) {
	file, err := os.Open(name)

	if err != nil {
		panic(err)
	}

	defer file.Close()

	hasher := md5.New()
	_, err = io.Copy(hasher, file)

	if err != nil {
		return "", errors.New("Can not calc a hash")
	}
	hash := hasher.Sum(nil)
	return hex.EncodeToString(hash[:]), nil
}

func main() {
	cfg := GetConfig()
	SetLogLevel(zapcore.Level(cfg.Settings.LogLevel))
	defer CloseLogger()
	if cfg.Sentry.Enable {
		dsn := cfg.Sentry.DSN
		fmt.Println(dsn)
		err := sentry.Init(sentry.ClientOptions{
			// Either set your DSN here or set the SENTRY_DSN environment variable.
			Dsn: dsn,
			// Either set environment and release here or set the SENTRY_ENVIRONMENT
			// and SENTRY_RELEASE environment variables.
			Environment: cfg.Sentry.Environment,
			Release:     "media-manager@v0.0.9a",
			// Enable printing of SDK debug messages.
			// Useful when getting started or trying to figure something out.
			Debug:            false,
			TracesSampleRate: 0.2,
		})
		if err != nil {
			log.Fatalf("sentry.Init: %s", err)
		}
		// Flush buffered events before the program terminates.
		// Set the timeout to the maximum duration the program can afford to wait.
		defer sentry.Flush(2 * time.Second)
		sentry.CaptureMessage("Started!")
	}
	handle_http(&cfg.Settings)
}
