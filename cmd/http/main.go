// cmd/http/main.go
package main

import (
	"log"

	"github.com/antiloger/Gfy/core"
	"github.com/antiloger/Gfy/external/ginserver"
	"github.com/antiloger/Gfy/external/sqlite"
	"github.com/antiloger/Gfy/module/broker"
	"github.com/antiloger/Gfy/module/sse"
)

func main() {
	fw := core.New()

	server := ginserver.NewGinServer()
	if err := fw.RegisterExternal("gin", server); err != nil {
		log.Fatal("gin failed to register")
	}

	db := sqlite.New()
	if err := fw.RegisterExternal("db", db); err != nil {
		log.Fatal("sqlite db failed to register")
	}
	fw.StartExternal("db")

	broker := broker.NewBrokerModule(server.GetGin(), db.GetDB())
	if err := fw.RegisterModule("broker", broker); err != nil {
		log.Fatal("broker module failed to register")
	}
	fw.StartModule("broker")

	sseModule := sse.NewSSEModule(server.GetGin())
	if err := fw.RegisterModule("sse", sseModule); err != nil {
		log.Fatal("sse module failed to register")
		return
	}

	fw.StartExternal("gin")
}
