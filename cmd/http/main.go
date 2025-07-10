// cmd/http/main.go
package main

import (
	"log"

	"github.com/antiloger/Gfy/core"
	"github.com/antiloger/Gfy/external/ginserver"
	"github.com/antiloger/Gfy/external/sqlite"
	"github.com/antiloger/Gfy/module/broker"
)

func main() {
	fw := core.New()

	server := ginserver.NewGinServer()
	db := sqlite.New()
	broker := broker.NewBrokerModule(server.GetGin(), db.GetDB())

	if err := fw.RegisterExternal("gin", server); err != nil {
		log.Fatal("gin failed to register")
	}

	if err := fw.RegisterExternal("db", db); err != nil {
		log.Fatal("sqlite db failed to register")
	}

	if err := fw.RegisterModule("broker", broker); err != nil {
		log.Fatal("broker module failed to register")
	}

	fw.StartExternal("db")
	fw.StartModule("broker")
	fw.StartExternal("gin")
}
