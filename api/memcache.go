package api

import (
	"fmt"
	"net/http"

	"google.golang.org/appengine/memcache"
)

func (s *Server) handleGetMemcache() http.HandlerFunc {
	return s.handler(func(r *http.Request) (int, interface{}) {
		key := r.URL.Query()["key"]
		if len(key) == 0 {
			return http.StatusBadRequest, fmt.Errorf("keys not found")
		}
		items, _ := memcache.GetMulti(r.Context(), key)
		return http.StatusOK, items
	})
}

func (s *Server) handlePostMemcache() http.HandlerFunc {
	type request struct {
		Items []*memcache.Item
	}
	return s.handler(func(r *http.Request) (int, interface{}) {
		var req request
		if err := s.readJSON(r, &req); err != nil {
			return http.StatusInternalServerError, err
		}
		if err := memcache.SetMulti(r.Context(), req.Items); err != nil {
			return http.StatusInternalServerError, err
		}
		return http.StatusCreated, nil
	})
}

func (s *Server) handleDeleteMemcache() http.HandlerFunc {
	return s.handler(func(r *http.Request) (int, interface{}) {
		key := r.URL.Query()["key"]
		if len(key) == 0 {
			return http.StatusBadRequest, fmt.Errorf("keys not found")
		}
		memcache.DeleteMulti(r.Context(), key)
		return http.StatusAccepted, nil
	})
}
