package main

import (
	"context"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pokernode/internal/mcpserver"
)

func main() {
	logger := log.New(os.Stderr, "pokernode-mcp: ", 0)
	config, err := mcpserver.ConfigFromEnv()
	if err != nil {
		logger.Fatal(err)
	}
	client, err := mcpserver.NewAPIClient(config, nil)
	if err != nil {
		logger.Fatal(err)
	}
	if err := mcpserver.New(client).Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		logger.Fatal(err)
	}
}
