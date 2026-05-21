// Command minidon is the binary entrypoint for the minidon server.
//
// It is responsible for:
//   - Parsing configuration from environment variables and flags (via internal/config).
//   - Wiring together the Mastodon streaming client, the ingest pipeline,
//     the ring buffer, the MeiliSearch index adapter, and the HTTP API server.
//   - Handling OS signals for graceful shutdown.
//
// TODO: implement flag/env parsing, dependency wiring, and graceful shutdown.
package main

import "fmt"

func main() {
	// TODO: load config, wire components, start server.
	fmt.Println("minidon starting… (not yet implemented)")
}
