package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"io"
	"log"
	"math"
	"math/cmplx"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	segmentDuration = 10
	frameRate       = 60
	maxIterations   = 10
)

var frameRateStr = fmt.Sprintf("%d", frameRate)

var startTime = time.Now()

var segmentCache sync.Map

func generateFrame(segmentNumber int, shape shape, t float64) []byte {
	width, height := shape.width, shape.height
	img := make([]byte, width*height*3)

	zoom := math.Exp(math.Sin(t*0.5) * 2)
	rotation := t * 0.1
	cos, sin := math.Cos(rotation), math.Sin(rotation)
	centerX := -0.743643887037158704752191506114774
	centerY := 0.131825904205311970493132056385139
	scale := 2.0 / float64(min(width, height)) / zoom

	for py := 0; py < height; py++ {
		y0 := float64(py)*scale - float64(height)/2*scale
		for px := 0; px < width; px++ {
			x0 := float64(px)*scale - float64(width)/2*scale

			x := x0*cos - y0*sin + centerX
			y := x0*sin + y0*cos + centerY
			c := complex(x, y)
			z := complex(0, 0)

			var iteration float64
			for iteration = 0; iteration < maxIterations; iteration++ {
				z = z*z + c
				if cmplx.Abs(z) > 2 {
					break
				}
			}

			var r, g, b uint8
			if iteration == maxIterations {
				r, g, b = 0, 0, 0
			} else {
				smoothed := iteration + 1 - math.Log(math.Log(cmplx.Abs(z)))/math.Log(2)
				hue := math.Mod(smoothed*0.1+t*0.1, 1.0)
				sat := 1.0
				if iteration < 5 {
					sat = iteration / 5
				}
				r, g, b = hsvToRGB(hue, sat, 1.0)
			}

			i := (py*width + px) * 3
			img[i] = r
			img[i+1] = g
			img[i+2] = b
		}
	}

	text := fmt.Sprintf("SEG=%d FR=%0.3f", segmentNumber, t)
	face := basicfont.Face7x13

	startX := 5
	startY := height - 5 - face.Metrics().Descent.Round()

	for i, r := range text {
		dot := fixed.Point26_6{
			X: fixed.I(startX + (i * face.Width)),
			Y: fixed.I(startY),
		}
		dr, mask, maskp, _, ok := face.Glyph(dot, r)
		if !ok {
			continue
		}

		for y := dr.Min.Y; y < dr.Max.Y; y++ {
			for x := dr.Min.X; x < dr.Max.X; x++ {
				alpha := mask.At(x-dr.Min.X+maskp.X, y-dr.Min.Y+maskp.Y).(color.Alpha).A
				if alpha > 0 {
					i := (y*width + x) * 3
					// Purple: 255, 0, 255
					img[i] = 255
					img[i+1] = 0
					img[i+2] = 255
				}
			}
		}
	}

	return img
}

func generateSegment(ctx context.Context, shape shape, segmentNumber int, out io.Writer) error {
	numWorkers := runtime.NumCPU()
	frameCount := segmentDuration * frameRate

	type work struct {
		img  []byte
		pts  float64
		dts  float64
		done chan struct{}
	}
	works := make([]*work, frameCount)
	for i := range works {
		works[i] = &work{
			done: make(chan struct{}),
		}
	}

	var wg sync.WaitGroup
	var jobIdx atomic.Int32

	baseTime := float64(segmentNumber * segmentDuration)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for frameIndex := jobIdx.Add(1); frameIndex <= int32(frameCount); frameIndex = jobIdx.Add(1) {
				if ctx.Err() != nil {
					return
				}
				w := works[frameIndex-1]
				t := baseTime + (float64(frameIndex) / float64(frameRate))
				w.img = generateFrame(segmentNumber, shape, t)
				w.pts = t
				w.dts = t
				close(w.done)
			}
		}()
	}

	for _, w := range works {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.done:
			// Write complete frame (all planes together)
			pts := int64(w.pts * 1000000000.0)
			dts := int64(w.dts * 1000000000.0)
			out.Write([]byte{
				byte((pts >> 56) & 0xFF),
				byte((pts >> 48) & 0xFF),
				byte((pts >> 40) & 0xFF),
				byte((pts >> 32) & 0xFF),
				byte((pts >> 24) & 0xFF),
				byte((pts >> 16) & 0xFF),
				byte((pts >> 8) & 0xFF),
				byte((pts >> 0) & 0xFF),
				byte((dts >> 56) & 0xFF),
				byte((dts >> 48) & 0xFF),
				byte((dts >> 40) & 0xFF),
				byte((dts >> 32) & 0xFF),
				byte((dts >> 24) & 0xFF),
				byte((dts >> 16) & 0xFF),
				byte((dts >> 8) & 0xFF),
				byte((dts >> 0) & 0xFF),
			})
			out.Write(w.img)
		}
	}

	wg.Wait()
	return nil
}

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	h = math.Mod(h, 1)
	if h < 0 {
		h += 1
	}

	h *= 6
	i := math.Floor(h)
	f := h - i
	p := v * (1 - s)
	q := v * (1 - (s * f))
	t := v * (1 - (s * (1 - f)))

	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}

	return uint8(r * 255), uint8(g * 255), uint8(b * 255)
}

func streamSegment(ctx context.Context, w io.Writer, shape shape, segmentNumber int) error {
	startFrameId := segmentNumber * frameRate * segmentDuration
	fmt.Printf("! %dx%d segment %v - -start_number %v\n", shape.width, shape.height, segmentNumber, startFrameId)

	cmd := exec.Command("ffmpeg",
		"-y",             // overwrite existing file
		"-f", "rawvideo", // input format
		"-pixel_format", "rgb24", // input pixel format
		// "-pix_fmt", "yuv420p", // output pixel format
		"-video_size", fmt.Sprintf("%dx%d", shape.width, shape.height), // video size
		"-framerate", frameRateStr, // frame rate
		"-i", "-", // input from pipe
		"-start_number", fmt.Sprintf("%d", startFrameId), // Set the initial frame number
		"-c:v", "libx264", // video codec
		"-preset", "ultrafast", // encoding speed
		"-tune", "zerolatency", // encoding tuning
		"-g", frameRateStr, // GOP size (match frame rate)
		"-keyint_min", frameRateStr, // minimum GOP size
		"-sc_threshold", "0", // disable scene change detection
		"-level", "4.1", // level
		"-x264-params", "force-cfr=1", // force constant frame rate for smoother playback
		"-output_ts_offset", fmt.Sprintf("%f", float64(startFrameId)/float64(frameRate)), // set initial timestamp
		"-mpegts_copyts", "1", // preserve original timestamps
		"-mpegts_flags", "+initial_discontinuity", // signal initial discontinuity
		"-pcr_period", "30", // PCR period in ms (increased for stability)
		"-muxrate", "8M", // muxing rate
		"-f", "mpegts", // output format
		"-flush_packets", "1", // flush packets
		"pipe:1", // output to pipe
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	cmd.Stdout = w

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	if err := generateSegment(ctx, shape, segmentNumber, stdin); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("failed to write frames: %w", err)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg error: %w", err)
	}

	return nil
}

func serveHLSPlaylist(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	setCORSHeaders(w)
	w.WriteHeader(http.StatusOK)

	// Add segments for ids from unix timestamp to unix timestamp + segmentCount
	segmentCount := 2 + int(time.Since(startTime).Seconds()/segmentDuration)
	fmt.Fprintf(w, "#EXTM3U\n")
	fmt.Fprintf(w, "#EXT-X-VERSION:3\n")
	fmt.Fprintf(w, "#EXT-X-PLAYLIST-TYPE:VOD\n")
	fmt.Fprintf(w, "#EXT-X-MEDIA-SEQUENCE:1\n")
	fmt.Fprintf(w, "#EXT-X-TARGETDURATION:10\n")
	fmt.Fprintf(w, "#EXT-X-ALLOW-CACHE:YES\n")
	var names []string
	for i := 0; i < segmentCount; i++ {
		uri := fmt.Sprintf("/segment/%s/%s/%d", r.PathValue("width"), r.PathValue("height"), i)
		names = append(names, uri)
		fmt.Fprintf(w, "#EXTINF:%d,\n%s\n", segmentDuration, uri)
	}
	fmt.Println("Playlist to ", r.RemoteAddr)
}

func serveHLSSegment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-cache")
	setCORSHeaders(w)

	ctx := r.Context()
	widthStr := r.PathValue("width")
	heightStr := r.PathValue("height")
	width, err := strconv.Atoi(widthStr)
	if err != nil {
		http.Error(w, "Invalid width", http.StatusBadRequest)
		return
	}
	height, err := strconv.Atoi(heightStr)
	if err != nil {
		http.Error(w, "Invalid height", http.StatusBadRequest)
		return
	}
	segmentNumberStr := r.PathValue("segment")
	segmentNumber, err := strconv.Atoi(segmentNumberStr)
	if err != nil {
		http.Error(w, "Invalid segment number", http.StatusBadRequest)
		return
	}
	getOrComputeSegment(ctx, w, shape{width, height}, segmentNumber)
}

func getOrComputeSegment(ctx context.Context, w http.ResponseWriter, shape shape, segmentNumber int) {
	key := cacheKey{shape, segmentNumber}

	// Try to get from cache first
	if cachedData, ok := segmentCache.Load(key); ok {
		w.WriteHeader(http.StatusOK)
		w.Write(cachedData.([]byte))
		fmt.Printf("! served #%d from cache\n", segmentNumber)
		return
	}

	// If not in cache, create a buffer to store the segment
	var buf bytes.Buffer
	if err := streamSegment(ctx, &buf, shape, segmentNumber); err != nil {
		log.Printf("Error streaming segment: %v", err)
		return
	}

	// Store in cache
	segmentData := buf.Bytes()
	segmentCache.Store(key, segmentData)

	// Serve the response
	w.WriteHeader(http.StatusOK)
	w.Write(segmentData)
}

func main() {
	ctx := context.Background()
	go prepareCache(ctx)
	startTime = time.Now()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "cmd/hls/index.html")
	})
	http.HandleFunc("/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		setCORSHeaders(w)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `#EXTM3U
#EXT-X-STREAM-INF:PROGRAM-ID=1,BANDWIDTH=2149280,RESOLUTION=1280x720,NAME=Big
playlist/1280/720/p.m3u8
#EXT-X-STREAM-INF:PROGRAM-ID=1,BANDWIDTH=2149280,RESOLUTION=640x360,NAME=Smol
playlist/640/360/p.m3u8`)
	})
	http.HandleFunc("/playlist/{width}/{height}/p.m3u8", serveHLSPlaylist)
	http.HandleFunc("/segment/{width}/{height}/{segment}", serveHLSSegment)

	fmt.Println("Server listening on :8080...")
	err := http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatalln(err)
	}
}

type shape struct {
	width  int
	height int
}

type cacheKey struct {
	shape
	index int
}

func prepareCache(ctx context.Context) {
	shapes := []shape{
		{640, 360},
		{1280, 720},
	}
	var wg sync.WaitGroup
	for _, shape := range shapes {
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// If not in cache, create a buffer to store the segment
				var buf bytes.Buffer
				if err := streamSegment(ctx, &buf, shape, i); err != nil {
					log.Printf("Error streaming segment: %v", err)
					return
				}
				segmentCache.Store(cacheKey{shape, i}, buf.Bytes())
			}()
		}
	}
	wg.Wait()
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}
