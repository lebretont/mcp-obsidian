package mcp

import (
	"testing"

	"github.com/dibou/mcp-obsidian/internal/config"
	syncapi "github.com/dibou/mcp-obsidian/internal/sync"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterToolsDoesNotPanic(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "test"}, nil)

	Register(server, Dependencies{
		Config: config.Config{},
		Sync:   syncapi.NewNoop(),
	})
}
