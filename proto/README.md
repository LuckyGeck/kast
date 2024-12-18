# cast_channel.proto

This is the protocol for the Chromecast device-to-device communication.

https://source.chromium.org/chromium/chromium/src/+/main:third_party/openscreen/src/cast/common/channel/proto/cast_channel.proto;drc=37a17677e5ded963fc41a3d8dee7a59484e5ec13


Compile it with:

```
protoc --go_out=. --go_opt=Mcast_channel.proto=. cast_channel.proto
```

You might need to install the protoc compiler and the Go plugin:

```
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```
