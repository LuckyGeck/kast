module github.com/luckygeck/kast/cmd/recv

go 1.23.4

replace (
	github.com/luckygeck/kast/kast => ../../kast
	github.com/luckygeck/kast/proto => ../../proto
)

require (
	github.com/grandcat/zeroconf v1.0.0
	github.com/quic-go/quic-go v0.48.2
	golang.org/x/image v0.23.0
)

require github.com/go-gl/gl v0.0.0-20231021071112-07e5d0ea2e71

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20240506104042-037f3cc74f2a
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/google/pprof v0.0.0-20210407192527-94a9f03dee38 // indirect
	github.com/miekg/dns v1.1.62 // indirect
	github.com/onsi/ginkgo/v2 v2.9.5 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842 // indirect
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/tools v0.28.0 // indirect
	google.golang.org/protobuf v1.36.1 // indirect
)
