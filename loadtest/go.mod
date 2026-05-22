module github.com/Haruncakir/snakeio_clone/loadtest

go 1.24.0

replace github.com/Haruncakir/snakeio_clone/pkg => ../pkg

require (
	github.com/Haruncakir/snakeio_clone/pkg v0.0.0-00010101000000-000000000000
	github.com/gorilla/websocket v1.5.3
	google.golang.org/protobuf v1.36.11
)
