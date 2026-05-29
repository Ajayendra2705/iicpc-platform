module github.com/Ajayendra2705/iicpc-platform/services/bot-worker

go 1.25.0

require (
	github.com/Ajayendra2705/iicpc-platform/proto/gen/go v0.0.0-20260514201044-7b430a97b9ab
	github.com/HdrHistogram/hdrhistogram-go v1.1.2
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674
	github.com/quickfixgo/quickfix v0.9.10
	google.golang.org/grpc v1.65.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pires/go-proxyproto v0.7.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/quagmt/udecimal v1.8.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

// The proto module lives in this repo. Resolve it from the local path so the
// service builds in pure module mode (e.g. inside the Docker image, with no
// go.work and no GITHUB_TOKEN) without trying to fetch the private pseudo-version.
// In workspace builds (local dev / CI) the go.work `use` directive takes
// precedence and this replace is ignored.
replace github.com/Ajayendra2705/iicpc-platform/proto/gen/go => ../../proto/gen/go
