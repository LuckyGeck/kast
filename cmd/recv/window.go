package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

type Window struct {
	window   *glfw.Window
	url      string
	texture  uint32
	program  uint32
	vao      uint32
	renderer ContentRenderer
}

func NewWindow(width, height int, title string) (*Window, error) {
	glfw.WindowHint(glfw.Resizable, glfw.True)
	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.False)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLAnyProfile)
	glfw.WindowHint(glfw.Visible, glfw.True)
	glfw.WindowHint(glfw.Focused, glfw.True)
	glfw.WindowHint(glfw.Decorated, glfw.True)

	window, err := glfw.CreateWindow(width, height, title, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create window: %v", err)
	}

	window.MakeContextCurrent()
	window.Show()

	if err := gl.Init(); err != nil {
		window.Destroy()
		return nil, fmt.Errorf("failed to initialize OpenGL: %v", err)
	}

	// Set viewport
	gl.Viewport(0, 0, int32(width), int32(height))

	w := &Window{
		window: window,
	}

	if err := w.initGL(); err != nil {
		window.Destroy()
		return nil, fmt.Errorf("failed to initialize GL: %v", err)
	}

	// Set resize callback
	window.SetFramebufferSizeCallback(func(w *glfw.Window, width, height int) {
		gl.Viewport(0, 0, int32(width), int32(height))
	})

	return w, nil
}

func (w *Window) initGL() error {
	var err error

	// Enable blending
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// Compile shaders
	vertShader, err := compileShader(vertexShader, gl.VERTEX_SHADER)
	if err != nil {
		return fmt.Errorf("failed to compile vertex shader: %v", err)
	}
	fragShader, err := compileShader(fragmentShader, gl.FRAGMENT_SHADER)
	if err != nil {
		return fmt.Errorf("failed to compile fragment shader: %v", err)
	}

	// Create and link program
	w.program = gl.CreateProgram()
	gl.AttachShader(w.program, vertShader)
	gl.AttachShader(w.program, fragShader)
	gl.LinkProgram(w.program)

	var status int32
	gl.GetProgramiv(w.program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(w.program, gl.INFO_LOG_LENGTH, &logLength)
		log := make([]byte, logLength)
		gl.GetProgramInfoLog(w.program, logLength, nil, &log[0])
		return fmt.Errorf("failed to link program: %v", string(log))
	}

	// Create VAO and VBO
	vertices := []float32{
		// Position    // Texture coordinates
		-1.0, -1.0, 0.0, 1.0, // Bottom left
		1.0, -1.0, 1.0, 1.0, // Bottom right
		-1.0, 1.0, 0.0, 0.0, // Top left
		1.0, 1.0, 1.0, 0.0, // Top right
	}

	var vbo uint32
	gl.GenVertexArrays(1, &w.vao)
	gl.GenBuffers(1, &vbo)

	gl.BindVertexArray(w.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

	// Position attribute
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(0)
	// Texture coordinate attribute
	gl.VertexAttribPointer(1, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(2*4))
	gl.EnableVertexAttribArray(1)

	// Create texture
	gl.GenTextures(1, &w.texture)
	gl.BindTexture(gl.TEXTURE_2D, w.texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

	return nil
}

func (w *Window) SetURL(url string) {
	w.url = url

	// Stop current renderer if any
	if w.renderer != nil {
		w.renderer.Stop()
	}

	// Detect content type and create appropriate renderer
	contentType := detectContentType(url)
	switch contentType {
	case "video":
		w.renderer = NewVideoRenderer(w)
	case "image":
		w.renderer = NewImageRenderer(w)
	default:
		log.Printf("Unsupported content type: %s", contentType)
		return
	}

	if err := w.renderer.Start(url); err != nil {
		log.Printf("Failed to start content playback: %v", err)
	}
}

func (w *Window) Draw() {
	if w.renderer == nil {
		return
	}

	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	// Bind texture and program
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, w.texture)
	gl.UseProgram(w.program)

	// Set texture uniform
	textureUniform := gl.GetUniformLocation(w.program, gl.Str("tex\x00"))
	gl.Uniform1i(textureUniform, 0)

	// Draw content
	w.renderer.Draw()
}

func (w *Window) Close() {
	if w.renderer != nil {
		w.renderer.Stop()
		w.renderer = nil
	}
	if w.window != nil {
		w.window.Destroy()
		w.window = nil
	}
}

func (w *Window) ShouldClose() bool {
	return w.window.ShouldClose()
}

func detectContentType(url string) string {
	// Try to determine content type from URL extension first
	switch {
	case strings.HasSuffix(strings.ToLower(url), ".mp4"),
		strings.HasSuffix(strings.ToLower(url), ".webm"),
		strings.HasSuffix(strings.ToLower(url), ".mkv"):
		return "video"
	case strings.HasSuffix(strings.ToLower(url), ".jpg"),
		strings.HasSuffix(strings.ToLower(url), ".jpeg"),
		strings.HasSuffix(strings.ToLower(url), ".png"),
		strings.HasSuffix(strings.ToLower(url), ".gif"):
		return "image"
	}

	// If no extension, try HEAD request
	resp, err := http.Head(url)
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	default:
		return "unknown"
	}
}
