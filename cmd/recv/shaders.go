package main

import (
	"fmt"

	"github.com/go-gl/gl/v2.1/gl"
)

const vertexShader = `
#version 120
attribute vec2 position;
attribute vec2 texCoord;
varying vec2 fragTexCoord;
void main() {
    gl_Position = vec4(position, 0.0, 1.0);
    fragTexCoord = texCoord;
}
` + "\x00"

const fragmentShader = `
#version 120
varying vec2 fragTexCoord;
uniform sampler2D tex;
void main() {
    gl_FragColor = texture2D(tex, fragTexCoord);
}
` + "\x00"

func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)
	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		log := make([]byte, logLength)
		gl.GetShaderInfoLog(shader, logLength, nil, &log[0])
		return 0, fmt.Errorf("failed to compile shader: %v", string(log))
	}

	return shader, nil
}
