package video

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/cmplx"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	segmentDurationSec = 30
	frameRate          = 30
	segmentDuration    = segmentDurationSec * time.Second
	framesPerSegment   = frameRate * segmentDurationSec
	totalDuration      = 30 * time.Second
	totalSegments      = int(totalDuration / segmentDuration)
	maxIterations      = 100
)

func writeFramesToPipe(writer *os.File, width, height, segmentID int) error {
	start := time.Now()
	startFrame := (segmentID - 1) * framesPerSegment
	numWorkers := runtime.NumCPU()

	type work struct {
		img  *image.RGBA
		done chan struct{}
	}
	works := make([]*work, framesPerSegment)
	for i := range works {
		works[i] = &work{
			done: make(chan struct{}),
		}
	}

	var wg sync.WaitGroup
	// Start workers
	var jobIdx atomic.Int32
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for frameIndex := jobIdx.Add(1); frameIndex <= framesPerSegment; frameIndex = jobIdx.Add(1) {
				w := works[frameIndex-1]
				w.img = generateFrame(width, height, startFrame-1+int(frameIndex))
				close(w.done)
			}
		}()
	}
	out := bufio.NewWriter(writer)
	// Write frames to pipe in order
	for i, w := range works {
		<-w.done
		// RGB24 format expects 3 bytes per pixel in RGB order
		size := w.img.Bounds().Size()
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				r, g, b, _ := w.img.At(x, y).RGBA()
				out.WriteByte(uint8(r >> 8))
				out.WriteByte(uint8(g >> 8))
				out.WriteByte(uint8(b >> 8))
			}
		}

		works[i] = nil // free up memory

		// Show progress
		progress := (i + 1) * 100 / framesPerSegment
		if i%frameRate == 0 {
			out.Flush()
			fmt.Printf("Segment %d: %d%% complete - elapsed: %s\n", segmentID, progress, time.Since(start))
		}
	}
	out.Flush()
	fmt.Printf("Segment %d: 100%% complete - elapsed: %s\n", segmentID, time.Since(start))

	wg.Wait()
	return nil
}

func generateVideoSegment(width, height, segmentID int) ([]byte, error) {
	if segmentID < 1 || segmentID > totalSegments {
		return nil, fmt.Errorf("invalid segment ID: %d, must be between 1 and %d", segmentID, totalSegments)
	}

	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found: %v", err)
	}

	// Create a pipe for streaming to ffmpeg
	ffmpegReader, ffmpegWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer ffmpegWriter.Close()
	defer ffmpegReader.Close()

	// Create a buffer for the output
	var outputBuffer bytes.Buffer

	// Start ffmpeg command
	ffmpegCmd := exec.Command("ffmpeg",
		"-y",
		"-f", "rawvideo",
		"-vcodec", "rawvideo",
		"-pixel_format", "rgb24",
		"-video_size", fmt.Sprintf("%dx%d", width, height),
		"-framerate", fmt.Sprintf("%d", frameRate),
		"-i", "pipe:0",
		"-c:v", "h264",
		"-preset", "slow",
		"-pix_fmt", "yuv420p",
		"-an",
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4",
		"-crf", "17",
		"-profile:v", "high",
		"-level", "4.1",
		"-maxrate", "8M",
		"-bufsize", "16M",
		"-x264-params", "aq-mode=3:deblock=-1,-1",
		"pipe:1",
	)
	ffmpegCmd.Stdin = ffmpegReader
	ffmpegCmd.Stdout = &outputBuffer
	ffmpegCmd.Stderr = os.Stderr

	// Start the command before writing frames
	if err := ffmpegCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %v", err)
	}

	// Write frames in a goroutine
	var writeErr error
	go func() {
		defer ffmpegWriter.Close()
		writeErr = writeFramesToPipe(ffmpegWriter, width, height, segmentID)
	}()

	// Wait for ffmpeg to finish
	if err := ffmpegCmd.Wait(); err != nil {
		if writeErr != nil {
			return nil, fmt.Errorf("failed to write frames: %v", writeErr)
		}
		return nil, fmt.Errorf("failed to finish ffmpeg: %v", err)
	}

	return outputBuffer.Bytes(), nil
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
