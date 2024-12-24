package main

import (
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

func init() {
	// GLFW event handling must run on the main thread
	runtime.LockOSThread()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Open Screen Protocol receiver...")

	// Initialize GLFW in the main thread
	if err := glfw.Init(); err != nil {
		log.Fatalf("Failed to initialize GLFW: %v", err)
	}
	defer glfw.Terminate()

	receiver := NewReceiver("OSP Receiver")

	// Start mDNS advertisement
	if err := receiver.startMDNSAdvertisement(); err != nil {
		log.Fatalf("Failed to start mDNS advertisement: %v", err)
	}

	// Start QUIC server
	go func() {
		if err := receiver.startQUICServer(); err != nil {
			log.Printf("QUIC server error: %v", err)
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		receiver.Stop()
		os.Exit(0)
	}()

	// Main event loop - this must run on the main thread
	for {
		select {
		case cmd := <-receiver.windowChan:
			var err error
			switch cmd.action {
			case "create":
				if receiver.window == nil {
					receiver.window, err = NewWindow(1280, 720, "Open Screen Receiver")
					if err == nil {
						receiver.window.SetURL(cmd.url)
					}
				}
			case "update":
				if receiver.window != nil {
					receiver.window.SetURL(cmd.url)
				}
			case "close":
				if receiver.window != nil {
					// Stop any active renderers before closing the window
					if receiver.window.renderer != nil {
						receiver.window.renderer.Stop()
						receiver.window.renderer = nil
					}
					receiver.window.Close()
					receiver.window = nil
					// Clear all presentations when window is closed
					receiver.presentationMu.Lock()
					receiver.presentations = make(map[string]*Presentation)
					receiver.presentationMu.Unlock()
				}
			}
			if cmd.result != nil {
				cmd.result <- err
			}

		default:
			// Render frame if window exists
			if receiver.window != nil {
				if receiver.window.ShouldClose() {
					// Window was closed by user, clean up
					result := make(chan error)
					receiver.windowChan <- windowCommand{
						action: "close",
						result: result,
					}
					<-result
					continue
				}

				receiver.window.window.MakeContextCurrent()

				// Clear the framebuffer
				gl.ClearColor(0.0, 0.0, 0.0, 1.0)
				gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

				// Draw content
				receiver.window.Draw()

				// Swap buffers and poll events
				receiver.window.window.SwapBuffers()
				glfw.PollEvents()
			}
		}
	}
}
