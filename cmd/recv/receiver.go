package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/quic-go/quic-go"
)

type Receiver struct {
	friendlyName   string
	mdns           *zeroconf.Server
	quicListener   *quic.Listener
	ctx            context.Context
	cancel         context.CancelFunc
	presentations  map[string]*Presentation
	presentationMu sync.RWMutex
	window         *Window
	windowChan     chan windowCommand
}

func NewReceiver(friendlyName string) *Receiver {
	ctx, cancel := context.WithCancel(context.Background())
	return &Receiver{
		friendlyName:  friendlyName,
		ctx:           ctx,
		cancel:        cancel,
		presentations: make(map[string]*Presentation),
		windowChan:    make(chan windowCommand),
	}
}

func (r *Receiver) startMDNSAdvertisement() error {
	// Find local IP address
	var localIP string
	var localIf *net.Interface
	ifaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to get network interfaces: %v", err)
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
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ip4 := ipnet.IP.To4(); ip4 != nil && !ip4.IsLoopback() {
					localIP = ip4.String()
					localIf = &iface
					break
				}
			}
		}
		if localIP != "" {
			break
		}
	}
	if localIP == "" {
		return fmt.Errorf("no suitable local IP address found")
	}

	// OSP TXT records as per spec
	txtRecords := []string{
		fmt.Sprintf("fn=%s", r.friendlyName),
		"rt=OpenScreen", // Receiver type
		"rs=Ready",      // Receiver state
	}

	server, err := zeroconf.Register(
		r.friendlyName,
		serviceName,
		domain,
		port,
		txtRecords,
		[]net.Interface{*localIf},
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %v", err)
	}

	r.mdns = server
	log.Printf("Advertising Open Screen service on %s:%d", localIP, port)
	return nil
}

// generateTLSConfig generates a self-signed certificate for QUIC
func generateTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 180), // 180 days
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"osp/1"},
	}, nil
}

func (r *Receiver) startQUICServer() error {
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to generate TLS config: %v", err)
	}

	listener, err := quic.ListenAddr(fmt.Sprintf(":%d", port), tlsConfig, &quic.Config{
		MaxIdleTimeout: time.Minute * 5,
	})
	if err != nil {
		return fmt.Errorf("failed to start QUIC listener: %v", err)
	}
	r.quicListener = listener

	log.Printf("QUIC server listening on port %d", port)

	go func() {
		<-r.ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept(r.ctx)
		if err != nil {
			if r.ctx.Err() != nil {
				return nil // Context cancelled
			}
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go r.handleQUICConnection(conn)
	}
}

func (r *Receiver) handleQUICConnection(conn quic.Connection) {
	defer conn.CloseWithError(0, "connection closed")
	log.Printf("New QUIC connection from %s", conn.RemoteAddr())

	// Accept the first stream, which will be used for control messages
	stream, err := conn.AcceptStream(r.ctx)
	if err != nil {
		log.Printf("Failed to accept stream: %v", err)
		return
	}
	defer stream.Close()

	decoder := json.NewDecoder(stream)
	encoder := json.NewEncoder(stream)

	for {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			log.Printf("Failed to decode message: %v", err)
			return
		}

		log.Printf("Received message: %+v", msg)

		var response Message
		var err error

		switch msg.Method {
		case "requestPresentation":
			response, err = r.handlePresentationRequest(msg)
		case "startPresentation":
			response, err = r.handleStartPresentation(msg)
		case "stopPresentation":
			response, err = r.handleStopPresentation(msg)
		default:
			err = fmt.Errorf("unknown method: %s", msg.Method)
		}

		if err != nil {
			response = Message{
				Type:      MsgTypeError,
				RequestID: msg.RequestID,
				Error: &ErrorResponse{
					Code:    500,
					Message: err.Error(),
				},
			}
		}

		if err := encoder.Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
			return
		}
	}
}

func (r *Receiver) handlePresentationRequest(msg Message) (Message, error) {
	// Validate the request and check if we can accept a new presentation
	r.presentationMu.RLock()
	numPresentations := len(r.presentations)
	r.presentationMu.RUnlock()

	if numPresentations > 0 {
		return Message{}, fmt.Errorf("already handling a presentation")
	}

	return Message{
		Type:      MsgTypeResponse,
		RequestID: msg.RequestID,
		Result:    map[string]interface{}{"available": true},
	}, nil
}

func (r *Receiver) handleStartPresentation(msg Message) (Message, error) {
	params, ok := msg.Params.(map[string]interface{})
	if !ok {
		return Message{}, fmt.Errorf("invalid parameters type")
	}

	url, ok := params["url"].(string)
	if !ok {
		return Message{}, fmt.Errorf("missing or invalid url parameter")
	}

	clientID, ok := params["clientId"].(string)
	if !ok {
		return Message{}, fmt.Errorf("missing or invalid clientId parameter")
	}

	r.presentationMu.Lock()
	defer r.presentationMu.Unlock()

	// Check if we already have this presentation
	if _, exists := r.presentations[clientID]; exists {
		return Message{}, fmt.Errorf("presentation already exists for client %s", clientID)
	}

	// Create window if it doesn't exist
	if r.window == nil {
		result := make(chan error)
		r.windowChan <- windowCommand{
			action: "create",
			url:    url,
			result: result,
		}
		if err := <-result; err != nil {
			return Message{}, fmt.Errorf("failed to create window: %v", err)
		}
	} else {
		// Update existing window with new URL
		result := make(chan error)
		r.windowChan <- windowCommand{
			action: "update",
			url:    url,
			result: result,
		}
		if err := <-result; err != nil {
			return Message{}, fmt.Errorf("failed to update window: %v", err)
		}
	}

	// Store the new presentation
	r.presentations[clientID] = &Presentation{
		URL:      url,
		ClientID: clientID,
	}

	log.Printf("Started presentation: %s for client %s", url, clientID)

	return Message{
		Type:      MsgTypeResponse,
		RequestID: msg.RequestID,
		Result:    map[string]interface{}{"success": true},
	}, nil
}

func (r *Receiver) handleStopPresentation(msg Message) (Message, error) {
	params, ok := msg.Params.(map[string]interface{})
	if !ok {
		return Message{}, fmt.Errorf("invalid parameters type")
	}

	clientID, ok := params["clientId"].(string)
	if !ok {
		return Message{}, fmt.Errorf("missing or invalid clientId parameter")
	}

	r.presentationMu.Lock()
	defer r.presentationMu.Unlock()

	if _, exists := r.presentations[clientID]; !exists {
		return Message{}, fmt.Errorf("no presentation found for client %s", clientID)
	}

	delete(r.presentations, clientID)

	// Close window if no more presentations
	if len(r.presentations) == 0 && r.window != nil {
		result := make(chan error)
		r.windowChan <- windowCommand{
			action: "close",
			result: result,
		}
		<-result
	}

	log.Printf("Stopped presentation for client %s", clientID)

	return Message{
		Type:      MsgTypeResponse,
		RequestID: msg.RequestID,
		Result:    map[string]interface{}{"success": true},
	}, nil
}

func (r *Receiver) Stop() {
	if r.mdns != nil {
		r.mdns.Shutdown()
	}
	if r.quicListener != nil {
		r.quicListener.Close()
	}
	if r.window != nil {
		r.window.Close()
	}
	r.cancel()
}
