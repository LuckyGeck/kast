package video

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestStreamMPEGTS(t *testing.T) {
	width, height := 640, 480

	// Create a test HTTP response recorder
	w := httptest.NewRecorder()

	// Stream the MPEGTS data
	err := streamMPEGTS(w, width, height)
	if err != nil {
		t.Fatalf("Failed to stream MPEGTS: %v", err)
	}

	// Verify the output using ffprobe
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-i", "pipe:0",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,codec_name,r_frame_rate",
		"-of", "default=noprint_wrappers=1",
	)
	cmd.Stdin = bytes.NewReader(w.Body.Bytes())
	probeOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe failed: %v\nOutput: %s", err, probeOutput)
	}

	// Basic checks on the ffprobe output
	probeStr := string(probeOutput)
	expectedChecks := []struct {
		name     string
		contains string
	}{
		{"width", fmt.Sprintf("width=%d", width)},
		{"height", fmt.Sprintf("height=%d", height)},
		{"codec", "codec_name=h264"},
		{"frame_rate", "r_frame_rate=30/1"},
	}

	for _, check := range expectedChecks {
		if !strings.Contains(probeStr, check.contains) {
			t.Errorf("Expected %s to contain %q, got: %s", check.name, check.contains, probeStr)
		}
	}

	// Check that we got some output
	if w.Body.Len() == 0 {
		t.Error("No data was written to the response")
	}
}
