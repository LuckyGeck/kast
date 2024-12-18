package kast

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/grandcat/zeroconf"
)

func FindDevice(ctx context.Context, name string) (*Device, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	r, err := zeroconf.NewResolver()
	if err != nil {
		return nil, err
	}
	found := make(chan *zeroconf.ServiceEntry)
	go r.Browse(ctx, "_googlecast._tcp", "local", found)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case d := <-found:
			if d == nil {
				continue
			}
			dev, err := newDevice(d)
			if err != nil {
				continue
			}
			slog.DebugContext(ctx, "Cast device", "device", dev)
			if dev.Name == name {
				return dev, nil
			}
		}
	}
}

// Device is a Cast device found via mDNS.
type Device struct {
	Name  string // e.g. "My Chromecast"
	Model string // e.g. "Google Home Mini"

	Addr net.IP // e.g. 192.168.1.100 or [2001:db8::1]
	Port int    // e.g. 8009
}

func newDevice(d *zeroconf.ServiceEntry) (*Device, error) {
	ret := &Device{
		Port: d.Port,
	}
	switch {
	case len(d.AddrIPv4) > 0:
		ret.Addr = d.AddrIPv4[0]
	case len(d.AddrIPv6) > 0:
		ret.Addr = d.AddrIPv6[0]
	default:
		return nil, fmt.Errorf("no address found")
	}
	for _, text := range d.Text {
		k, v, _ := strings.Cut(text, "=")
		switch k {
		case "fn":
			ret.Name = v
		case "md":
			ret.Model = v
		}
	}
	return ret, nil
}
