package main

import (
	"context"
	"log"
	"os"

	"github.com/dibou/mcp-obsidian/internal/config"
	mcpserver "github.com/dibou/mcp-obsidian/internal/mcp"
	syncer "github.com/dibou/mcp-obsidian/internal/sync"
	s3sync "github.com/dibou/mcp-obsidian/internal/sync/s3"
	"github.com/dibou/mcp-obsidian/internal/vault"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.1.0"

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.LUTC | log.Lmsgprefix)
	log.SetPrefix("mcp-obsidian: ")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	v, err := vault.New(cfg.VaultPath, cfg.AllowDelete)
	if err != nil {
		log.Fatal(err)
	}

	var s syncer.Syncer = syncer.NewNoop()
	if cfg.S3.Enabled {
		s, err = s3sync.New(context.Background(), cfg.S3, v)
		if err != nil {
			log.Fatal(err)
		}
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-obsidian", Version: version}, nil)
	mcpserver.Register(server, mcpserver.Dependencies{
		Config: cfg,
		Vault:  v,
		Sync:   s,
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
