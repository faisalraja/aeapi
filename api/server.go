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
	"google.golang.org/appengine/memcache"
)

type server struct {
	secret string
	router *mux.Router
}

// Routes returns a mux instance for handler
func Routes() *mux.Router {
	r := mux.NewRouter()

	srv := &server{router: r, secret: os.Getenv("SECRET")}
	if srv.secret == "" {
		panic("SECRET in app.yaml is empty")
	}

	r.Use(srv.auth)
	r.HandleFunc("/memcache", srv.handleGetMemcache()).Methods("GET")
	r.HandleFunc("/memcache", srv.handlePostMemcache()).Methods("POST")

	return r
}

func (s *server) err(w http.ResponseWriter, code int, err error) bool {
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

func (s *server) json(w http.ResponseWriter, code int, resp interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
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
		next.ServeHTTP(w, r)
	})
}

func (s *server) handleGetMemcache() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query()["key"]
		if len(key) == 0 {
			s.err(w, http.StatusBadRequest, fmt.Errorf("keys not found"))
			return
		}
		items, _ := memcache.GetMulti(r.Context(), key)
		s.json(w, http.StatusOK, items)
	}
}

func (s *server) handlePostMemcache() http.HandlerFunc {
	type request struct {
		Items []*memcache.Item
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if s.err(w, 0, s.readJSON(r, &req)) {
			return
		}
		log.Printf("Req: %v", req)
		if s.err(w, 0, memcache.SetMulti(r.Context(), req.Items)) {
			return
		}
		s.json(w, http.StatusOK, "OK")
	}
}
