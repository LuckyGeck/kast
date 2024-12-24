module github.com/luckygeck/kast/cmd/recv

go 1.23.4

replace (
	github.com/luckygeck/kast/kast => ../../kast
	github.com/luckygeck/kast/proto => ../../proto
)

require (
	github.com/grandcat/zeroconf v1.0.0
	github.com/luckygeck/kast/kast v0.0.0-20241221201920-0ddd5898842a
	github.com/luckygeck/kast/proto v0.0.0-20241221201920-0ddd5898842a
)

require google.golang.org/protobuf v1.36.1

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/miekg/dns v1.1.62 // indirect
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/tools v0.28.0 // indirect
)
