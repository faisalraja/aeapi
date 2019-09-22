package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
)

// Server defines how api request is handled
type Server struct {
	secret string
	router *mux.Router
}

// NewServer returns the instance of api server that implements the Handler interface
func NewServer() *Server {
	r := mux.NewRouter()

	srv := &Server{router: r, secret: os.Getenv("SECRET")}
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
	srs.HandleFunc("/{index}", srv.handlePutSearch()).Methods("PUT")
	srs.HandleFunc("/{index}", srv.handleDeleteSearch()).Methods("DELETE")

	// catch all
	r.PathPrefix("/").HandlerFunc(srv.handleCatchAll())

	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	st := time.Now()
	s.router.ServeHTTP(w, r)
	if tt := time.Now().Sub(st).Seconds(); tt >= 1 {
		log.Printf("[SLOW] warning request: %s %s took %f seconds", r.Method, r.RequestURI, tt)
	}
}

func (s *Server) handler(f func(r *http.Request) (int, interface{})) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code, result := f(r)
		if result != nil {
			if err, ok := result.(error); ok {
				s.writeError(w, code, err)
				return
			}
		}
		s.writeJSON(w, code, result)
	}
}

func (s *Server) writeError(w http.ResponseWriter, code int, err error) bool {
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

func (s *Server) writeJSON(w http.ResponseWriter, code int, resp interface{}) {
	w.WriteHeader(code)
	if resp == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		panic(err)
	}
}

func (s *Server) readJSON(r *http.Request, out interface{}) error {
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

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Secret") != s.secret {
			s.writeError(w, http.StatusForbidden, fmt.Errorf("Secret does not match"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleCatchAll() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("Catch all not found"))
	}
}
