package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"slices"

	"github.com/luckygeck/kast/kast"
)

var (
	videoURL   = flag.String("video", "", "URL of the video to cast")
	deviceName = flag.String("device", "", "Name of the Chromecast device")
	debug      = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx := context.Background()
	flag.Parse()

	if *videoURL == "" {
		log.Fatal("Please provide a video URL using -video flag")
	}

	device, err := kast.FindDevice(ctx, *deviceName)
	if err != nil {
		log.Fatalf("Error finding device %q: %v", *deviceName, err)
	}

	c, err := kast.NewConn(ctx, fmt.Sprintf("%s:%d", device.Addr, device.Port))
	if err != nil {
		log.Fatalf("Error creating connection: %v", err)
	}
	// Create and send the CONNECT message
	if err := c.Connect(kast.DefaultReceiverID); err != nil {
		log.Fatalf("CONNECT(%s): %v", kast.DefaultReceiverID, err)
	}

	go c.Run()

	launch, err := kast.Call[kast.ReceiverStatusMsg](c, kast.Launch.With("appId", kast.DefaultMediaReceiverAppID))
	if err != nil {
		log.Fatalf("LAUNCH: %v", err)
	}
	blob, err := json.MarshalIndent(launch, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal launch: %v", err)
	}
	fmt.Printf("LAUNCH: %s\n", string(blob))

	if launch.Type != kast.MsgTypeReceiverStatus {
		log.Fatalf("Expected RECEIVER_STATUS, got %s", launch.Type)
	}
	idx := slices.IndexFunc(launch.Status.Applications, func(app kast.Application) bool {
		return app.AppID == kast.DefaultMediaReceiverAppID
	})
	if idx == -1 {
		log.Fatalf("Expected Default Media Receiver, got %s", launch.Status.Applications[0].AppID)
	}
	app := launch.Status.Applications[idx]

	if err := c.Connect(app.TransportID); err != nil {
		log.Fatalf("CONNECT(%s): %v", app.TransportID, err)
	}

	load, err := kast.Call[kast.MediaStatusMsg](c, kast.Msg{
		DestinationID: app.TransportID,
		Namespace:     kast.MediaNamespace,
		Type:          kast.MsgTypeLoad,
		Payload: []kast.KeyVal{
			{Key: "media", Value: kast.MediaInformation{
				ContentID:   *videoURL,
				StreamType:  "BUFFERED",
				ContentType: "video/mp4",
				Metadata: &kast.Metadata{
					MetadataType: 0, // GenericMediaMetadata
					Title:        "Test Video",
					Subtitle:     "Test Subtitle",
				},
			}},
		},
	})
	if err != nil {
		log.Fatalf("LOAD(%s): %v", *videoURL, err)
	}
	blob, err = json.MarshalIndent(load, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal load: %v", err)
	}
	fmt.Printf("LOAD: %T: %s\n", load, string(blob))
}
