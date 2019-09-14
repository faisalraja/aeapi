package main

import (
	"net/http"

	"github.com/faisalraja/aeapi/v1/api"
	"google.golang.org/appengine"
)

func main() {
	http.Handle("/", api.Router())
	appengine.Main()
}
