package api

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"google.golang.org/appengine/memcache"
)

type (
	// Server defines how api request is handled
	Server struct {
		liveSecret string
		testSecret string
		router     *mux.Router
	}

	badErr struct {
		m string
	}

	ctxKey string
)

var (
	errAuth     = fmt.Errorf("Unauthorized")
	errNotFound = fmt.Errorf("Not found")
)

const (
	// KeyEnv added to namespace prefix
	KeyEnv ctxKey = "env"
)

// Error bad request
func (be badErr) Error() string {
	return be.m
}

// NewServer returns the instance of api server that implements the Handler interface
func NewServer() *Server {
	r := mux.NewRouter()

	srv := &Server{router: r, liveSecret: os.Getenv("LIVE_SECRET"), testSecret: os.Getenv("TEST_SECRET")}
	if srv.liveSecret == "" || srv.testSecret == "" {
		panic("LIVE_SECRET or TEST_SECRET in app.yaml is empty")
	}
	sr := r.PathPrefix("/api").Subrouter()
	sr.Use(srv.auth)

	// search
	srs := sr.PathPrefix("/search").Subrouter()
	srs.HandleFunc("/{ns}/{index}", srv.handleGetSearch()).Methods("GET")
	srs.HandleFunc("/{ns}/{index}", srv.handlePutSearch()).Methods("PUT")
	srs.HandleFunc("/{ns}/{index}", srv.handleDeleteSearch()).Methods("DELETE")

	// catch all
	r.PathPrefix("/").HandlerFunc(srv.handleCatchAll())

	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handler(f func(r *http.Request) interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := f(r)
		if result != nil {
			if err, ok := result.(error); ok {
				s.writeError(w, err)
				return
			}
		}
		s.writeJSON(w, result)
	}
}

func (s *Server) writeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	code := http.StatusInternalServerError
	var msg string
	if err == errAuth {
		code = http.StatusForbidden
	} else if err == errNotFound {
		code = http.StatusNotFound
	} else if errB, ok := err.(badErr); ok {
		msg = errB.m
		code = http.StatusBadRequest
	}

	if msg == "" {
		msg = http.StatusText(code)
	}

	log.Printf("APIError: %v", err)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		panic(err)
	}
	return true
}

func (s *Server) writeJSON(w http.ResponseWriter, resp interface{}) {
	w.WriteHeader(http.StatusOK)
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
		ctx := r.Context()
		secret := r.Header.Get("X-Secret")
		if secret == s.liveSecret {
			r = r.WithContext(context.WithValue(ctx, KeyEnv, "live"))
		} else if secret == s.testSecret {
			r = r.WithContext(context.WithValue(ctx, KeyEnv, "test"))
		} else {
			s.writeError(w, errAuth)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) prefixNS(ctx context.Context, ns string) string {
	if env, ok := ctx.Value(KeyEnv).(string); ok {
		ns = env + "_" + ns
	}
	return ns
}

func (s *Server) handleCatchAll() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.writeError(w, errNotFound)
	}
}

// SetCache creates a memcache cache
func (s *Server) SetCache(ctx context.Context, key string, value interface{}, expire time.Duration) error {

	item := &memcache.Item{
		Key:        key,
		Object:     value,
		Expiration: expire,
	}
	if err := memcache.Gob.Set(ctx, item); err != nil {
		return err
	}

	return nil
}

// GetCache gets the cache from memcache
func (s *Server) GetCache(ctx context.Context, key string, out interface{}) error {
	if _, err := memcache.Gob.Get(ctx, key, out); err != nil && err != memcache.ErrCacheMiss {
		return err
	}

	return nil
}

// DeleteCache deletes a cache
func (s *Server) DeleteCache(ctx context.Context, key string) error {
	err := memcache.Delete(ctx, key)
	if err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}

// SearchCacheKey returns cache key for current context etc
func (s *Server) SearchCacheKey(ctx context.Context, ns string, index string, query url.Values) string {
	var prefix string
	cKey := "search:" + ns + ":index:" + index
	s.GetCache(ctx, cKey, &prefix)
	if prefix == "" {
		prefix = cKey + strconv.FormatInt(time.Now().Unix(), 10)
		if err := s.SetCache(ctx, cKey, prefix, time.Hour*12); err != nil {
			log.Println("SearchCacheKey: failed to cache prefix", err)
		}
	}
	b, err := json.Marshal(query)
	if err != nil {
		panic(err)
	}
	return hashSha1(prefix + string(b))
}

// ResetSearchCacheKey renews cache key to invalidate old cache
func (s *Server) ResetSearchCacheKey(ctx context.Context, ns string, index string) error {
	cKey := "search:" + ns + ":index:" + index
	return s.SetCache(ctx, cKey, cKey+strconv.FormatInt(time.Now().Unix(), 10), time.Hour*24)
}

func hashSha1(text string) string {
	hasher := sha1.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}
