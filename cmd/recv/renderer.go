package main

import (
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"log"
	"net/http"
	"os/exec"
	"sync"

	"github.com/go-gl/gl/v2.1/gl"
	"golang.org/x/image/draw"
)

// VideoRenderer renders video content using FFmpeg
type VideoRenderer struct {
	window       *Window
	ffmpegCmd    *exec.Cmd
	ffmpegDone   chan struct{}
	currentFrame *image.RGBA
	frameMutex   sync.Mutex
}

func NewVideoRenderer(window *Window) *VideoRenderer {
	return &VideoRenderer{
		window:     window,
		ffmpegDone: make(chan struct{}),
	}
}

func (r *VideoRenderer) Start(url string) error {
	if r.ffmpegCmd != nil {
		r.ffmpegCmd.Process.Kill()
		<-r.ffmpegDone
	}

	// Create a new RGBA frame buffer
	r.currentFrame = image.NewRGBA(image.Rect(0, 0, 1280, 720))

	// FFmpeg command with proper scaling and format
	cmd := exec.Command("ffmpeg",
		"-i", url,
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-s", "1280x720",
		"-r", "30", // 30 fps
		"-vf", "scale=1280:720:force_original_aspect_ratio=1,pad=1280:720:(ow-iw)/2:(oh-ih)/2:black,format=rgba",
		"-an", // Disable audio
		"-sn", // Disable subtitles
		"-")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %v", err)
	}

	r.ffmpegCmd = cmd

	// Handle stderr in a separate goroutine
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				log.Printf("FFmpeg: %s", string(buf[:n]))
			}
		}
	}()

	// Handle video frames
	go func() {
		defer close(r.ffmpegDone)

		frameSize := 1280 * 720 * 4 // RGBA = 4 bytes per pixel
		buffer := make([]byte, frameSize)

		for {
			n, err := io.ReadFull(stdout, buffer)
			if err != nil {
				if err != io.EOF {
					log.Printf("FFmpeg read error: %v", err)
				}
				return
			}

			if n != frameSize {
				log.Printf("Incomplete frame received: %d bytes", n)
				continue
			}

			r.frameMutex.Lock()
			copy(r.currentFrame.Pix, buffer)
			r.frameMutex.Unlock()
		}
	}()

	return nil
}

func (r *VideoRenderer) Stop() {
	if r.ffmpegCmd != nil && r.ffmpegCmd.Process != nil {
		r.ffmpegCmd.Process.Kill()
		<-r.ffmpegDone
		r.ffmpegCmd = nil
	}
	r.frameMutex.Lock()
	r.currentFrame = nil
	r.frameMutex.Unlock()
}

func (r *VideoRenderer) Draw() {
	r.frameMutex.Lock()
	defer r.frameMutex.Unlock()

	if r.currentFrame != nil {
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, r.window.texture)

		// Update texture with new frame
		gl.TexImage2D(
			gl.TEXTURE_2D,
			0,
			gl.RGBA,
			int32(r.currentFrame.Rect.Dx()),
			int32(r.currentFrame.Rect.Dy()),
			0,
			gl.RGBA,
			gl.UNSIGNED_BYTE,
			gl.Ptr(r.currentFrame.Pix),
		)

		// Use shader program
		gl.UseProgram(r.window.program)

		// Set texture uniform
		textureUniform := gl.GetUniformLocation(r.window.program, gl.Str("tex\x00"))
		gl.Uniform1i(textureUniform, 0)

		// Draw the quad
		gl.BindVertexArray(r.window.vao)
		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	}
}

// ImageRenderer renders static images
type ImageRenderer struct {
	window *Window
	image  *image.RGBA
}

func NewImageRenderer(window *Window) *ImageRenderer {
	return &ImageRenderer{
		window: window,
	}
}

func (r *ImageRenderer) Start(url string) error {
	// Download and decode the image
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to decode image: %v", err)
	}

	// Convert to RGBA and scale to window size
	bounds := img.Bounds()
	r.image = image.NewRGBA(image.Rect(0, 0, 1280, 720))
	draw.BiLinear.Scale(r.image, r.image.Bounds(), img, bounds, draw.Over, nil)

	return nil
}

func (r *ImageRenderer) Stop() {
	r.image = nil
}

func (r *ImageRenderer) Draw() {
	if r.image != nil {
		gl.BindTexture(gl.TEXTURE_2D, r.window.texture)
		gl.TexImage2D(
			gl.TEXTURE_2D,
			0,
			gl.RGBA,
			1280,
			720,
			0,
			gl.RGBA,
			gl.UNSIGNED_BYTE,
			gl.Ptr(r.image.Pix),
		)

		gl.UseProgram(r.window.program)
		gl.BindVertexArray(r.window.vao)
		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	}
}
