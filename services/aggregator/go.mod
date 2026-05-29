module github.com/Ajayendra2705/iicpc-platform/services/aggregator

go 1.25.0

require (
	github.com/Ajayendra2705/iicpc-platform/proto/gen/go v0.0.0-20260514201044-7b430a97b9ab
	github.com/HdrHistogram/hdrhistogram-go v1.1.2
	github.com/jackc/pgx/v5 v5.7.1
	github.com/segmentio/kafka-go v0.4.47
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/grpc v1.65.0 // indirect
)

// The proto module lives in this repo. Resolve it from the local path so the
// service builds in pure module mode (e.g. inside the Docker image, with no
// go.work and no GITHUB_TOKEN) without trying to fetch the private pseudo-version.
// In workspace builds (local dev / CI) the go.work `use` directive takes
// precedence and this replace is ignored.
replace github.com/Ajayendra2705/iicpc-platform/proto/gen/go => ../../proto/gen/go
