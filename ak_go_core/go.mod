module example.com/ak_go_core

go 1.25.0

toolchain go1.25.7

require (
	example.com/pisag_go v0.0.0
	github.com/go-chi/chi/v5 v5.2.3
	github.com/jackc/pgx/v5 v5.8.0
	github.com/oklog/ulid/v2 v2.1.1
	github.com/redis/go-redis/v9 v9.17.3
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/net v0.29.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace example.com/pisag_go => ../pisag_go
