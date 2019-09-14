package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

type server struct {
	secret string
	router *mux.Router
}

// Router returns a handler that implements the Handler interface
func Router() *mux.Router {
	r := mux.NewRouter()

	srv := &server{router: r, secret: os.Getenv("SECRET")}
	if srv.secret == "" {
		panic("SECRET in app.yaml is empty")
	}
	sr := r.PathPrefix("/api").Subrouter()
	sr.Use(srv.auth)
	// memcache
	srm := sr.PathPrefix("/memcache").Subrouter()
	srm.HandleFunc("", srv.handleGetMemcache()).Methods("GET")
	srm.HandleFunc("", srv.handlePostMemcache()).Methods("POST")
	srm.HandleFunc("", srv.handleDeleteMemcache()).Methods("DELETE")

	// search
	srs := sr.PathPrefix("/search").Subrouter()
	srs.HandleFunc("/{index}", srv.handleGetSearch()).Methods("GET")

	return r
}

func (s *server) hasErr(w http.ResponseWriter, code int, err error) bool {
	if err == nil {
		return false
	}
	if code == 0 {
		code = http.StatusInternalServerError
	}
	msg := http.StatusText(code)
	log.Printf("APIError: %v", err)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		panic(err)
	}
	return true
}

func (s *server) JSON(w http.ResponseWriter, code int, resp interface{}) {
	w.WriteHeader(code)
	if resp == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		panic(err)
	}
}

func (s *server) readJSON(r *http.Request, out interface{}) error {
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		return err
	}
	if err := r.Body.Close(); err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Secret") != s.secret {
			s.hasErr(w, http.StatusForbidden, fmt.Errorf("Secret does not match"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
