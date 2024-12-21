package video

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestGenerateVideoSegment(t *testing.T) {
	width, height := 640, 480
	segmentID := 1

	// Generate video segment
	output, err := generateVideoSegment(width, height, segmentID)
	if err != nil {
		t.Fatalf("Failed to generate video segment: %v", err)
	}
	if len(output) == 0 {
		t.Fatal("Generated video segment is empty")
	}

	// Use ffprobe to verify the video
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,codec_name,duration",
		"-of", "default=noprint_wrappers=1",
		"-f", "mp4",
		"pipe:0",
	)
	cmd.Stdin = bytes.NewReader(output)
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
		{"codec", "codec_name=h264"},
		{"width", "width=640"},
		{"height", "height=480"},
	}

	for _, check := range expectedChecks {
		if !bytes.Contains(probeOutput, []byte(check.contains)) {
			t.Errorf("Expected %s to contain %q, got: %s", check.name, check.contains, probeStr)
		}
	}
}
