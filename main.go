package main

import (
	"net/http"

	"github.com/faisalraja/aefts/v1/api"
	"google.golang.org/appengine"
)

func main() {
	http.Handle("/", api.NewServer())
	appengine.Main()
}
