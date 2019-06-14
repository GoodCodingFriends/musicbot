module github.com/acomagu/musicbot

go 1.12

require (
	cloud.google.com/go v0.39.0
	github.com/bwmarrin/discordgo v0.19.0
	github.com/djherbis/buffer v1.0.0
	github.com/djherbis/nio v2.0.3+incompatible
	github.com/golang/protobuf v1.3.1 // indirect
	github.com/jonas747/dca v0.0.0-20180225204759-bf5d11669cdb
	github.com/jonas747/ogg v0.0.0-20161220051205-b4f6f4cf3757 // indirect
	github.com/matryer/is v1.2.0
	github.com/pkg/errors v0.8.1
	golang.org/x/net v0.0.0-20190328230028-74de082e2cca // indirect
	golang.org/x/oauth2 v0.0.0-20190319182350-c85d3e98c914 // indirect
	google.golang.org/api v0.5.0
	google.golang.org/appengine v1.5.0 // indirect
	google.golang.org/grpc v1.19.0
)

replace github.com/bwmarrin/discordgo v0.19.0 => github.com/acomagu/discordgo v0.19.1-0.20190614141415-9d0cdedfa4da
