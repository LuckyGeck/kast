package kast

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

type Msg struct {
	// raw is the original message payload.
	raw string `json:"-"`
	// SourceID is the ID of the sender of the message.
	// If empty, the message is sent from the default sender.
	SourceID string `json:"-"`
	// DestinationID is the ID of the receiver to send the message to.
	// If empty, the message is sent to the default receiver.
	DestinationID string `json:"-"`

	Namespace string `json:"-"`

	Type      MsgType `json:"type"`
	RequestID int     `json:"requestId,omitempty"`

	Payload []KeyVal `json:"-"`
}

func (r Msg) With(key string, value any) Msg {
	r.Payload = append(slices.Clip(r.Payload), KeyVal{Key: key, Value: value})
	return r
}

func (r Msg) WithMany(args ...any) Msg {
	if len(args)%2 != 0 {
		panic("WithMany requires an even number of arguments")
	}
	r.Payload = slices.Clip(r.Payload)
	for i := 0; i < len(args); i += 2 {
		key := args[i].(string)
		value := args[i+1]
		r.Payload = append(r.Payload, KeyVal{Key: key, Value: value})
	}
	return r
}

type KeyVal struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func (r *Msg) MarshalJSON() ([]byte, error) {
	var ret bytes.Buffer
	fmt.Fprintf(&ret, `{"type":%q,"requestId":%d`, r.Type, r.RequestID)
	for _, kv := range r.Payload {
		if kv.Key == "type" || kv.Key == "requestId" {
			continue
		}
		fmt.Fprintf(&ret, `,"%s":`, kv.Key)
		if err := json.NewEncoder(&ret).Encode(kv.Value); err != nil {
			return nil, fmt.Errorf("failed to marshal value %T for key %q: %v", kv.Value, kv.Key, err)
		}
	}
	fmt.Fprintf(&ret, "}")
	return ret.Bytes(), nil
}

type MsgType string

const (
	MsgTypeConnect MsgType = "CONNECT"

	MsgTypeMediaStatus    MsgType = "MEDIA_STATUS"
	MsgTypeReceiverStatus MsgType = "RECEIVER_STATUS"
	MsgTypeLoadFailed     MsgType = "LOAD_FAILED"
	MsgTypeClose          MsgType = "CLOSE"
	MsgTypeLaunch         MsgType = "LAUNCH"
	MsgTypePing           MsgType = "PING"
	MsgTypePong           MsgType = "PONG"
	MsgTypeLoad           MsgType = "LOAD"
)

var (
	Launch = Msg{
		Namespace: ReceiverNamespace,
		Type:      MsgTypeLaunch,
	}
	Load = Msg{
		Namespace: MediaNamespace,
		Type:      MsgTypeLoad,
	}
)

const (
	ConnectionNamespace = "urn:x-cast:com.google.cast.tp.connection"
	ReceiverNamespace   = "urn:x-cast:com.google.cast.receiver"
	MediaNamespace      = "urn:x-cast:com.google.cast.media"
)
