package api

import (
	"fmt"
	"log"
	"net/http"

	"google.golang.org/appengine/memcache"
)

func (s *server) handleGetMemcache() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query()["key"]
		if len(key) == 0 {
			s.hasErr(w, http.StatusBadRequest, fmt.Errorf("keys not found"))
			return
		}
		items, _ := memcache.GetMulti(r.Context(), key)
		s.JSON(w, http.StatusOK, items)
	}
}

func (s *server) handlePostMemcache() http.HandlerFunc {
	type request struct {
		Items []*memcache.Item
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if s.hasErr(w, 0, s.readJSON(r, &req)) {
			return
		}
		log.Printf("Req: %v", req)
		if s.hasErr(w, 0, memcache.SetMulti(r.Context(), req.Items)) {
			return
		}
		s.JSON(w, http.StatusCreated, nil)
	}
}

func (s *server) handleDeleteMemcache() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query()["key"]
		if len(key) == 0 {
			s.hasErr(w, http.StatusBadRequest, fmt.Errorf("keys not found"))
			return
		}
		memcache.DeleteMulti(r.Context(), key)
		s.JSON(w, http.StatusAccepted, nil)
	}
}
