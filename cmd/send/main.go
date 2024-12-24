package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"log"
	"time"

	"github.com/quic-go/quic-go"
)

type Message struct {
	Type      string         `json:"type"`
	RequestID int            `json:"requestId"`
	Method    string         `json:"method,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
}

func main() {
	url := flag.String("url", "", "URL to present (e.g., http://example.com/video.mp4)")
	flag.Parse()

	if *url == "" {
		log.Fatal("Please provide a URL to present using -url flag")
	}

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"osp/1"},
	}

	conn, err := quic.DialAddr(context.Background(), "localhost:3333", tlsConf, nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.CloseWithError(0, "bye")

	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}
	defer stream.Close()

	// First, request presentation availability
	reqMsg := Message{
		Type:      "request",
		RequestID: 1,
		Method:    "requestPresentation",
	}

	if err := json.NewEncoder(stream).Encode(reqMsg); err != nil {
		log.Fatalf("Failed to send request: %v", err)
	}

	// Read response
	var resp map[string]interface{}
	if err := json.NewDecoder(stream).Decode(&resp); err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}
	log.Printf("Availability response: %+v", resp)

	// Start presentation
	startMsg := Message{
		Type:      "request",
		RequestID: 2,
		Method:    "startPresentation",
		Params: map[string]any{
			"url":      *url,
			"clientId": "test-client",
		},
	}

	if err := json.NewEncoder(stream).Encode(startMsg); err != nil {
		log.Fatalf("Failed to send start request: %v", err)
	}

	// Read response
	if err := json.NewDecoder(stream).Decode(&resp); err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}
	log.Printf("Start presentation response: %+v", resp)

	// Wait for a while
	log.Println("Presentation started. Press Ctrl+C to stop...")

	// Send stop presentation before exiting
	defer func() {
		stopMsg := Message{
			Type:      "request",
			RequestID: 3,
			Method:    "stopPresentation",
			Params: map[string]any{
				"clientId": "test-client",
			},
		}
		if err := json.NewEncoder(stream).Encode(stopMsg); err != nil {
			log.Printf("Failed to send stop request: %v", err)
			return
		}
		var stopResp map[string]interface{}
		if err := json.NewDecoder(stream).Decode(&stopResp); err != nil {
			log.Printf("Failed to read stop response: %v", err)
			return
		}
		log.Printf("Stop presentation response: %+v", stopResp)
	}()

	time.Sleep(time.Hour)
}
