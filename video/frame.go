package video

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"math/cmplx"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	frameRate     = 30
	maxIterations = 100
)

func streamMPEGTS(w http.ResponseWriter, width, height int) error {
	ffmpegReader, ffmpegWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer ffmpegWriter.Close()
	defer ffmpegReader.Close()

	ffmpegCmd := exec.Command("ffmpeg",
		"-y",
		"-f", "rawvideo",
		"-vcodec", "rawvideo",
		"-pixel_format", "rgb24",
		"-video_size", fmt.Sprintf("%dx%d", width, height),
		"-framerate", fmt.Sprintf("%d", frameRate),
		"-i", "pipe:0",
		"-c:v", "h264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-f", "mpegts",
		"-flush_packets", "1",
		"pipe:1",
	)
	ffmpegCmd.Stdout = bufio.NewWriter(w)
	ffmpegCmd.Stderr = os.Stderr
	ffmpegCmd.Stdin = ffmpegReader

	// Write frames continuously
	go func() {
		out := bufio.NewWriter(ffmpegWriter)
		writeFramesToPipe(out, width, height)
		out.Flush()
		ffmpegWriter.Close()
	}()

	if err := ffmpegCmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %v", err)
	}

	ffmpegCmd.Wait()
	return nil
}

func writeFramesToPipe(out io.ByteWriter, width, height int) error {
	numWorkers := runtime.NumCPU()
	frameCount := int32(frameRate) // Generate 1 seconds worth of frames

	type work struct {
		img  *image.RGBA
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

	// Use current time for animation
	startTime := time.Now()

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for frameIndex := jobIdx.Add(1); frameIndex <= frameCount; frameIndex = jobIdx.Add(1) {
				w := works[frameIndex-1]
				// Use elapsed time instead of frame number for animation
				elapsedSeconds := time.Since(startTime).Seconds()
				w.img = generateFrame(width, height, int(elapsedSeconds*float64(frameRate)))
				close(w.done)
			}
		}()
	}

	for _, w := range works {
		<-w.done
		// Write frame data
		size := w.img.Bounds().Size()
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				r, g, b, _ := w.img.At(x, y).RGBA()
				if err := out.WriteByte(uint8(r >> 8)); err != nil {
					return err
				}
				if err := out.WriteByte(uint8(g >> 8)); err != nil {
					return err
				}
				if err := out.WriteByte(uint8(b >> 8)); err != nil {
					return err
				}
			}
		}
	}
	wg.Wait()
	return nil
}

// Generate a frame of the Mandelbrot set.
//
// The frame number is used to animate the zoom and rotation.
func generateFrame(width, height int, frameNum int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Calculate zoom level based on frame number
	t := float64(frameNum) / frameRate
	zoom := math.Exp(math.Sin(t*0.5) * 2)

	// Add rotation for extra effect
	rotation := t * 0.1
	cos, sin := math.Cos(rotation), math.Sin(rotation)

	// Center point for the zoom
	centerX := -0.743643887037158704752191506114774
	centerY := 0.131825904205311970493132056385139

	scale := 2.0 / float64(min(width, height)) / zoom

	for py := 0; py < height; py++ {
		y0 := float64(py)*scale - float64(height)/2*scale
		for px := 0; px < width; px++ {
			x0 := float64(px)*scale - float64(width)/2*scale

			// Apply rotation
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

			if iteration == maxIterations {
				img.Set(px, py, color.Black)
			} else {
				// Smooth coloring with time-based hue shift
				smoothed := iteration + 1 - math.Log(math.Log(cmplx.Abs(z)))/math.Log(2)
				hue := math.Mod(smoothed*0.1+t*0.1, 1.0)
				sat := 1.0
				val := 1.0
				if iteration < 5 {
					sat = iteration / 5
				}

				r, g, b := hsvToRGB(hue, sat, val)
				img.Set(px, py, color.RGBA{r, g, b, 255})
			}
		}
	}

	return img
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Convert HSV to RGB colors
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
