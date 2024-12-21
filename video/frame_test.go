package video

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
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
		"-show_entries", "stream=width,height,codec_name,r_frame_rate,pix_fmt,profile,level",
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
		{"frame_rate", "r_frame_rate=30/1"},
		{"pixel_format", "pix_fmt=yuv420p"},
		{"profile", "profile=High"},
		{"level", "level=41"}, // 4.1 is represented as 41 in ffprobe output
	}

	for _, check := range expectedChecks {
		if !strings.Contains(probeStr, check.contains) {
			t.Errorf("Expected %s to contain %q, got: %s", check.name, check.contains, probeStr)
		}
	}

	// Additional quality checks using ffprobe
	qualityCmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=bit_rate",
		"-of", "default=noprint_wrappers=1",
		"-f", "mp4",
		"pipe:0",
	)
	qualityCmd.Stdin = bytes.NewReader(output)
	qualityOutput, err := qualityCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe quality check failed: %v\nOutput: %s", err, qualityOutput)
	}

	// Check if bitrate is within expected range (should be less than maxrate)
	bitRateStr := strings.TrimSpace(string(qualityOutput))
	if strings.HasPrefix(bitRateStr, "bit_rate=") {
		bitRate := strings.TrimPrefix(bitRateStr, "bit_rate=")
		if bitRate != "N/A" {
			// Convert to integer and check if it's less than maxrate (8M)
			var bitRateInt int
			if _, err := fmt.Sscanf(bitRate, "%d", &bitRateInt); err == nil {
				maxBitRate := 8 * 1024 * 1024 // 8M in bits/second
				if bitRateInt > maxBitRate {
					t.Errorf("Bitrate %d exceeds maximum allowed %d", bitRateInt, maxBitRate)
				}
			}
		}
	}
}
