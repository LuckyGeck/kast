package video

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

func StartServer(port int) (videoURL string, err error) {
	// Listen on all interfaces instead of just localhost
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("failed to create listener: %v", err)
	}

	// Get local network IP address
	ip, err := getLocalIP()
	if err != nil {
		return "", fmt.Errorf("failed to get local IP: %v", err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	videoURL = fmt.Sprintf("http://%s:%d/playlist.m3u8", ip.String(), actualPort)

	// Start HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "video/static/index.html")
	})
	mux.HandleFunc("/video.ts", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Serving video stream", "path", r.URL.Path)
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Transfer-Encoding", "chunked")

		if err := streamMPEGTS(w, 1280, 720); err != nil {
			http.Error(w, "Streaming failed", http.StatusInternalServerError)
			slog.Error("Streaming failed", "error", err)
			return
		}
	})
	server := &http.Server{Handler: mux}

	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	slog.Info("Video server started",
		"port", actualPort,
		"video", videoURL)
	return videoURL, nil
}

func getLocalIP() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			// Skip loopback addresses
			if ipnet.IP.IsLoopback() {
				continue
			}
			// We want IPv4 addresses
			if ipnet.IP.To4() != nil {
				return ipnet.IP, nil
			}
		}
	}
	return nil, fmt.Errorf("no suitable local IP address found")
}
