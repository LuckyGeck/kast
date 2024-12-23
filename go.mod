module github.com/luckygeck/kast

go 1.23.4

replace (
	github.com/luckygeck/kast/kast => ./kast
	github.com/luckygeck/kast/proto => ./proto
	github.com/luckygeck/kast/video => ./video
)

require (
	github.com/luckygeck/kast/kast v0.0.0-20241219172613-8ea4ed62e7ae
	github.com/luckygeck/kast/video v0.0.0-00010101000000-000000000000
)

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/grafov/m3u8 v0.12.1 // indirect
	github.com/grandcat/zeroconf v1.0.0 // indirect
	github.com/luckygeck/kast/proto v0.0.0-20241219172613-8ea4ed62e7ae // indirect
	github.com/miekg/dns v1.1.62 // indirect
	golang.org/x/image v0.23.0 // indirect
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/tools v0.28.0 // indirect
	google.golang.org/protobuf v1.36.0 // indirect
)
