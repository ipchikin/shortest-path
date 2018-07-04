package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/ipchikin/shortest-path/api"
	"github.com/julienschmidt/httprouter"
)

func Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

func main() {
	// Open the Badger database located in the /tmp/badger directory.
	// It will be created if it doesn't exist.
	opts := badger.DefaultOptions
	opts.Dir = "/tmp/badger"
	opts.ValueDir = "/tmp/badger"
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	client := &http.Client{Timeout: time.Second * 2}
	s := api.Server{db, client}

	router := httprouter.New()
	router.GET("/", Index)
	router.POST("/route", api.GenTokenHandler)
	router.GET("/route/:token", s.GetRouteHandler)

	log.Fatal(http.ListenAndServe(":8080", router))
}
