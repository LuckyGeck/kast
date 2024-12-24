package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/grandcat/zeroconf"
	"google.golang.org/protobuf/proto"

	"github.com/luckygeck/kast/kast"
	pb "github.com/luckygeck/kast/proto"
)

func startMDNSAdvertisement() (*zeroconf.Server, error) {
	// Find local IP address
	var localIP string
	var localIf *net.Interface
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %v", err)
	}

	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			// Check if this is an IP network address
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ip4 := ipnet.IP.To4(); ip4 != nil {
					// Skip loopback addresses
					if !ip4.IsLoopback() {
						localIP = ip4.String()
						localIf = &iface
						break
					}
				}
			}
		}
		if localIP != "" {
			break
		}
	}
	if localIP == "" {
		return nil, fmt.Errorf("no suitable local IP address found")
	}

	server, err := zeroconf.Register(
		"LuckyKast",        // Instance name (this is what shows up in discovery)
		"_googlecast._tcp", // Service type
		"local.",           // Domain
		port,               // Port
		[]string{
			"id=beefdead148d492c9e940db5cddbaf49",
			"cd=beefdead148d492c9e940db5cddbaf49",
			"rm=",
			"ve=05",
			"md=LuckyKast Mac",
			"ic=/setup/icon.png",
			"fn=LuckyKast",
			"rmodel=LuckyKast",
			"ca=4101",
			"st=0",
			"bs=FA8FCA94D933",
			"rs=",
			"nf=1",
		},
		[]net.Interface{*localIf},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register mDNS service: %v", err)
	}

	log.Printf("Advertising Chromecast service on %s:%d", localIP, port)
	return server, nil
}

func readCastMessage(r io.Reader) (*pb.CastMessage, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read message data: %w", err)
	}

	var msg pb.CastMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

func writeCastMessage(w io.Writer, msg *pb.CastMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write message data: %w", err)
	}

	return nil
}

type connectionMessage struct {
	Type       string `json:"type"`
	UserAgent  string `json:"userAgent,omitempty"`
	ConnType   int    `json:"connType,omitempty"`
	SenderInfo struct {
		SdkType        int    `json:"sdkType"`
		Version        string `json:"version"`
		BrowserVersion string `json:"browserVersion"`
		Platform       int    `json:"platform"`
		ConnectionType int    `json:"connectionType"`
	} `json:"senderInfo,omitempty"`
}

type receiverStatus struct {
	Type   string `json:"type"`
	Status struct {
		Applications []struct {
			AppID        string   `json:"appId"`
			DisplayName  string   `json:"displayName"`
			StatusText   string   `json:"statusText"`
			IsIdleScreen bool     `json:"isIdleScreen"`
			Namespaces   []string `json:"namespaces"`
		} `json:"applications"`
		Volume struct {
			Level float64 `json:"level"`
			Muted bool    `json:"muted"`
		} `json:"volume"`
		IsActiveInput bool `json:"isActiveInput"`
	} `json:"status"`
}

func handleAuthMessage(conn net.Conn, msg *pb.CastMessage) error {
	// First, unmarshal the incoming challenge
	var challenge pb.DeviceAuthMessage
	if err := proto.Unmarshal(msg.GetPayloadBinary(), &challenge); err != nil {
		return fmt.Errorf("failed to unmarshal auth challenge: %v", err)
	}

	log.Printf("Received auth challenge: %v", challenge.String())

	// Create auth response
	authResponse := &pb.DeviceAuthMessage{
		Response: &pb.AuthResponse{
			// For testing purposes, we'll send empty values
			// In a production environment, these would need to be properly signed/certified
			Signature:               []byte{},
			ClientAuthCertificate:   []byte{},
			IntermediateCertificate: [][]byte{},
			SenderNonce:             []byte{},
			HashAlgorithm:           pb.HashAlgorithm_SHA1.Enum(),
		},
	}

	// Marshal the auth response
	authData, err := proto.Marshal(authResponse)
	if err != nil {
		return fmt.Errorf("failed to marshal auth response: %v", err)
	}

	response := &pb.CastMessage{
		ProtocolVersion: pb.CastMessage_CASTV2_1_0.Enum(),
		SourceId:        proto.String(kast.DefaultReceiverID),
		DestinationId:   msg.SourceId,
		Namespace:       msg.Namespace,
		PayloadType:     pb.CastMessage_BINARY.Enum(),
		PayloadBinary:   authData,
	}

	return writeCastMessage(conn, response)
}

func handleConnectionMessage(conn net.Conn, msg *pb.CastMessage) error {
	var connMsg connectionMessage
	if err := json.Unmarshal([]byte(msg.GetPayloadUtf8()), &connMsg); err != nil {
		return fmt.Errorf("failed to unmarshal connection message: %v", err)
	}

	response := &pb.CastMessage{
		ProtocolVersion: pb.CastMessage_CASTV2_1_0.Enum(),
		SourceId:        proto.String(kast.DefaultReceiverID),
		DestinationId:   msg.SourceId,
		Namespace:       msg.Namespace,
		PayloadType:     pb.CastMessage_STRING.Enum(),
		PayloadUtf8:     proto.String(`{"type":"CONNECT"}`),
	}

	return writeCastMessage(conn, response)
}

func handleReceiverMessage(conn net.Conn, msg *pb.CastMessage) error {
	// Create a basic receiver status response
	status := receiverStatus{
		Type: "RECEIVER_STATUS",
		Status: struct {
			Applications []struct {
				AppID        string   `json:"appId"`
				DisplayName  string   `json:"displayName"`
				StatusText   string   `json:"statusText"`
				IsIdleScreen bool     `json:"isIdleScreen"`
				Namespaces   []string `json:"namespaces"`
			} `json:"applications"`
			Volume struct {
				Level float64 `json:"level"`
				Muted bool    `json:"muted"`
			} `json:"volume"`
			IsActiveInput bool `json:"isActiveInput"`
		}{
			Applications: []struct {
				AppID        string   `json:"appId"`
				DisplayName  string   `json:"displayName"`
				StatusText   string   `json:"statusText"`
				IsIdleScreen bool     `json:"isIdleScreen"`
				Namespaces   []string `json:"namespaces"`
			}{},
			Volume: struct {
				Level float64 `json:"level"`
				Muted bool    `json:"muted"`
			}{
				Level: 1.0,
				Muted: false,
			},
			IsActiveInput: true,
		},
	}

	statusJSON, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal receiver status: %v", err)
	}

	response := &pb.CastMessage{
		ProtocolVersion: pb.CastMessage_CASTV2_1_0.Enum(),
		SourceId:        proto.String(kast.DefaultReceiverID),
		DestinationId:   msg.SourceId,
		Namespace:       msg.Namespace,
		PayloadType:     pb.CastMessage_STRING.Enum(),
		PayloadUtf8:     proto.String(string(statusJSON)),
	}

	return writeCastMessage(conn, response)
}

func handleMediaMessage(conn net.Conn, msg *pb.CastMessage) error {
	// For now, just acknowledge media messages
	response := &pb.CastMessage{
		ProtocolVersion: pb.CastMessage_CASTV2_1_0.Enum(),
		SourceId:        proto.String(kast.DefaultReceiverID),
		DestinationId:   msg.SourceId,
		Namespace:       msg.Namespace,
		PayloadType:     pb.CastMessage_STRING.Enum(),
		PayloadUtf8:     proto.String(`{"type":"MEDIA_STATUS","status":[]}`),
	}

	return writeCastMessage(conn, response)
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("New connection from %s", conn.RemoteAddr())

	for {
		msg, err := readCastMessage(conn)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("Failed to read message: %v", err)
			}
			return
		}

		log.Printf("Received message: namespace=%s source=%s destination=%s type=%v payload=%v",
			msg.GetNamespace(), msg.GetSourceId(), msg.GetDestinationId(), msg.GetPayloadType(), msg)

		var handleErr error
		switch msg.GetNamespace() {
		case "urn:x-cast:com.google.cast.tp.deviceauth":
			handleErr = handleAuthMessage(conn, msg)
		case "urn:x-cast:com.google.cast.tp.connection":
			handleErr = handleConnectionMessage(conn, msg)
		case "urn:x-cast:com.google.cast.receiver":
			handleErr = handleReceiverMessage(conn, msg)
		case "urn:x-cast:com.google.cast.media":
			handleErr = handleMediaMessage(conn, msg)
		default:
			log.Printf("Unhandled namespace: %s", msg.GetNamespace())
		}

		if handleErr != nil {
			log.Printf("Failed to handle message: %v", handleErr)
			return
		}
	}
}

const port = 8009

func startTLSServer(ctx context.Context) error {
	// TODO: Generate proper TLS certificate
	cert, err := tls.LoadX509KeyPair("cmd/recv/server.crt", "cmd/recv/server.key")
	if err != nil {
		return fmt.Errorf("failed to load TLS certificate: %v", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), config)
	if err != nil {
		return fmt.Errorf("failed to start TLS listener: %v", err)
	}
	defer listener.Close()

	log.Printf("TLS server listening on port %d", port)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // Context cancelled
			}
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go handleConnection(conn)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting virtual Chromecast receiver...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start mDNS advertisement
	server, err := startMDNSAdvertisement()
	if err != nil {
		log.Fatalf("Failed to start mDNS advertisement: %v", err)
	}
	defer server.Shutdown()

	// Start TLS server
	go func() {
		if err := startTLSServer(ctx); err != nil {
			log.Printf("TLS server error: %v", err)
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down...")
}
