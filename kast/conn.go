package kast

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	pb "github.com/luckygeck/kast/proto"
)

type Conn struct {
	mu        sync.Mutex
	inflight  map[int]func(*Msg) // callback for response
	conn      net.Conn
	requestID int // request ID for the next message
}

// NewConn creates a new connection to a Cast device.
// addr is the address of the device in the format "192.168.1.100:8009" or "[2001:db8::1]:8009".
func NewConn(ctx context.Context, addr string) (*Conn, error) {
	conn, err := dial(ctx, addr)
	if err != nil {
		return nil, err
	}
	return &Conn{
		conn:      conn,
		requestID: 1,
		inflight:  make(map[int]func(*Msg)),
	}, nil
}

func dial(ctx context.Context, addr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   time.Second * 3,
		KeepAlive: time.Second * 30,
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // allow self-signed certificates
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS12,
		// Explicitly set cipher suites
		CipherSuites: []uint16{
			// NOTE: We allow less secure cipher suites to support older devices or the MiroCast app.
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
	}

	slog.DebugContext(ctx, "Attempting TLS connection", "addr", addr)
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %v", err)
	}

	tlsConn := tls.Client(conn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	slog.DebugContext(ctx, "TLS connection established successfully")
	return tlsConn, nil
}

func (c *Conn) Connect(receiverID string) error {
	return c.Send(Msg{
		Namespace:     ConnectionNamespace,
		DestinationID: receiverID,
		Type:          MsgTypeConnect,
	}, nil)
}

func Call[T any](c *Conn, m Msg) (T, error) {
	var ret T
	var parseErr error
	done := make(chan struct{})
	err := c.Send(m, func(msg *Msg) {
		parseErr = json.Unmarshal([]byte(msg.raw), &ret)
		close(done)
	})
	if err != nil {
		return ret, err
	}
	<-done
	return ret, parseErr
}

func (c *Conn) Send(m Msg, cb func(msg *Msg)) error {
	c.mu.Lock()
	m.RequestID = c.requestID
	c.requestID++

	if cb != nil {
		c.inflight[m.RequestID] = cb
	}
	c.mu.Unlock()

	if m.SourceID == "" {
		m.SourceID = "sender-0"
	}
	if m.DestinationID == "" {
		m.DestinationID = DefaultReceiverID
	}

	blob, err := m.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	slog.Debug("Send",
		"type", m.Type,
		"dst", m.DestinationID,
		"requestID", m.RequestID,
		"namespace", m.Namespace,
		"src", m.SourceID,
		"payload", m.Payload,
	)
	var indented bytes.Buffer
	json.Indent(&indented, blob, "", "  ")
	fmt.Println("Send", m.SourceID, m.DestinationID, m.Namespace, indented.String())

	pb := &pb.CastMessage{
		ProtocolVersion: pb.CastMessage_CASTV2_1_0.Enum(),
		SourceId:        proto.String(m.SourceID),
		DestinationId:   proto.String(m.DestinationID),
		Namespace:       proto.String(m.Namespace),
		PayloadType:     pb.CastMessage_STRING.Enum(),
		PayloadUtf8:     proto.String(string(blob)),
	}
	data, err := proto.Marshal(pb)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	if err := binary.Write(c.conn, binary.BigEndian, uint32(len(data))); err != nil {
		return fmt.Errorf("failed to write message length: %v", err)
	}

	if _, err := c.conn.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}

	return nil
}

// Run starts the connection and handles incoming messages.
func (c *Conn) Run() error {
	for {
		m, err := c.readMsg()
		if err != nil {
			c.conn.Close()
			return err
		}
		switch {
		case m.Type == "PING":
			c.Send(Msg{
				Namespace:     ConnectionNamespace,
				SourceID:      m.SourceID,
				DestinationID: m.DestinationID,
				Type:          MsgTypePong,
			}, nil)
		case m.RequestID != 0:
			c.mu.Lock()
			cb := c.inflight[m.RequestID]
			delete(c.inflight, m.RequestID)
			c.mu.Unlock()

			if cb != nil {
				cb(m)
			}
		}
	}
}

func (c *Conn) readMsg() (*Msg, error) {
	var length uint32
	if err := binary.Read(c.conn, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("failed to binary read payload: %v", err)
	}
	if length == 0 {
		return nil, fmt.Errorf("invalid payload length: %d", length)
	}

	blob := make([]byte, length)
	i, err := io.ReadFull(c.conn, blob)
	if err != nil {
		return nil, fmt.Errorf("failed to read payload: %v", err)
	}

	if i != int(length) {
		return nil, fmt.Errorf("invalid payload length: %d != %d", i, length)
	}

	m := &pb.CastMessage{}
	if err := proto.Unmarshal(blob, m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proto message: %v", err)
	}

	ret := &Msg{
		raw:           m.GetPayloadUtf8(),
		SourceID:      m.GetSourceId(),
		DestinationID: m.GetDestinationId(),
		Namespace:     m.GetNamespace(),
	}
	if err := json.Unmarshal([]byte(ret.raw), ret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal utf8 payload as json: %v", err)
	}

	slog.Debug("Recv",
		"type", ret.Type,
		"requestID", ret.RequestID,
		"dst", ret.DestinationID,
		"src", ret.SourceID,
		"namespace", ret.Namespace,
		"payload", ret.raw,
	)
	var indented bytes.Buffer
	json.Indent(&indented, []byte(ret.raw), "", "  ")
	fmt.Println("Recv", ret.SourceID, ret.DestinationID, ret.Namespace, indented.String())
	return ret, nil
}
