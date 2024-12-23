package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/luckygeck/kast/kast"
	"github.com/luckygeck/kast/video"
)

var (
	videoURL   = flag.String("video", "", "URL of the video to play")
	deviceName = flag.String("device", "", "Name of the Chromecast device")
	debug      = flag.Bool("debug", false, "Enable debug logging")
	port       = flag.Int("port", 19091, "Port to start the video server on (0 for random)")
	cast       = flag.Bool("cast", false, "Cast to the Chromecast device")
)

func main() {
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Fatal("Shutting down...")
	}()

	flag.Parse()

	if *deviceName == "" {
		log.Fatal("device name is required")
	}

	// Start the video server
	servedVideoURL, err := video.StartServer(*port)
	if err != nil {
		log.Fatalf("Failed to start video server: %v", err)
	}
	slog.Info("Started video server", "video", servedVideoURL)

	if !*cast {
		select {}
	}

	appID := kast.DefaultMediaReceiverAppID

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

	launch, err := kast.Call[kast.ReceiverStatusMsg](c, kast.Launch.With("appId", appID))
	if err != nil {
		log.Fatalf("LAUNCH: %v", err)
	}
	if *debug {
		blob, _ := json.MarshalIndent(launch, "", "  ")
		fmt.Printf("LAUNCH: %s\n", string(blob))
	}

	if launch.Type != kast.MsgTypeReceiverStatus {
		log.Fatalf("Expected RECEIVER_STATUS, got %s", launch.Type)
	}
	idx := slices.IndexFunc(launch.Status.Applications, func(app kast.Application) bool { return app.AppID == appID })
	if idx == -1 {
		log.Fatalf("Expected Default Media Receiver, got %s", launch.Status.Applications[0].AppID)
	}
	a := launch.Status.Applications[idx]

	if err := c.Connect(a.TransportID); err != nil {
		log.Fatalf("CONNECT(%s): %v", a.TransportID, err)
	}

	contentID := servedVideoURL
	if *videoURL != "" {
		contentID = *videoURL
	}

	load, err := kast.Call[kast.MediaStatusMsg](c, kast.Msg{
		DestinationID: a.TransportID,
		Namespace:     kast.MediaNamespace,
		Type:          kast.MsgTypeLoad,
		Payload: []kast.KeyVal{
			{Key: "media", Value: kast.MediaInformation{
				ContentID:   contentID,
				StreamType:  "LIVE",
				ContentType: "application/vnd.apple.mpegurl",
				Metadata: &kast.Metadata{
					MetadataType: 0,
					Title:        "Live Mandelbrot Animation",
					Subtitle:     "Mandelbrot set animation",
				},
			}},
		},
	})
	if err != nil {
		log.Fatalf("LOAD(%s): %v", servedVideoURL, err)
	}
	if *debug {
		blob, _ := json.MarshalIndent(load, "", "  ")
		fmt.Printf("LOAD: %T: %s\n", load, string(blob))
	}

	<-ctx.Done()
}
