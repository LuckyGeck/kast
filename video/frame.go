package video

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"time"
)

const (
	segmentDurationSec = 30
	frameRate          = 30
	segmentDuration    = segmentDurationSec * time.Second
	framesPerSegment   = frameRate * segmentDurationSec
	totalDuration      = 30 * time.Second
	totalSegments      = int(totalDuration / segmentDuration)
)

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
		"-f", "mjpeg",
		"-framerate", fmt.Sprintf("%d", frameRate),
		"-i", "pipe:0",
		"-c:v", "h264",
		"-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		"-an",
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4",
		"-crf", "23",
		"-profile:v", "baseline",
		"-level", "3.0",
		"-maxrate", "2M",
		"-bufsize", "4M",
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

		startFrame := (segmentID - 1) * framesPerSegment
		for i := 0; i < framesPerSegment; i++ {
			frameNum := startFrame + i
			x, y := getCirclePosition(frameNum, width, height)
			frame := generateFrame(width, height, x, y, float64(height)*0.1)

			if err := jpeg.Encode(ffmpegWriter, frame, &jpeg.Options{Quality: 90}); err != nil {
				writeErr = err
				return
			}
		}
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

func generateFrame(width, height int, circleX, circleY, radius float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background with white
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255   // R
		img.Pix[i+1] = 255 // G
		img.Pix[i+2] = 255 // B
		img.Pix[i+3] = 255 // A
	}

	// Draw black circle
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx := float64(x) - circleX
			dy := float64(y) - circleY
			if dx*dx+dy*dy <= radius*radius {
				offset := (y*width + x) * 4
				img.Pix[offset] = 0     // R
				img.Pix[offset+1] = 0   // G
				img.Pix[offset+2] = 0   // B
				img.Pix[offset+3] = 255 // A
			}
		}
	}

	return img
}

// getCirclePosition calculates the circle position for a given frame number
func getCirclePosition(frameNum int, width, height int) (x, y float64) {
	// Make the circle jump in a figure-8 pattern
	t := float64(frameNum) / frameRate
	scale := 0.4 // How much of the screen to use
	centerX := float64(width) / 2
	centerY := float64(height) / 2

	x = centerX + float64(width)*scale*math.Sin(t*2)
	y = centerY + float64(height)*scale*math.Sin(t*4)/2
	return x, y
}
