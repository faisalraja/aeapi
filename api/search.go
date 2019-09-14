package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"google.golang.org/appengine/search"
)

func (s *server) handleGetSearch() http.HandlerFunc {
	type response struct {
		Result []map[string]interface{}
		Cursor string
	}
	return func(w http.ResponseWriter, r *http.Request) {
		index := mux.Vars(r)["index"]
		query := r.URL.Query()
		if len(query["q"]) == 0 {
			s.hasErr(w, http.StatusBadRequest, fmt.Errorf("Query string missing"))
			return
		}
		opt := &search.SearchOptions{}
		if len(query["Limit"]) == 1 {
			limit, _ := strconv.Atoi(query["Limit"][0])
			opt.Limit = limit
		}
		if len(query["IDsOnly"]) == 1 {
			opt.IDsOnly = query["IDsOnly"][0] == "true"
		}
		if len(query["Fields"]) > 0 {
			opt.Fields = query["Fields"]
		}
		if len(query["Cursor"]) == 1 {
			opt.Cursor = search.Cursor(query["Cursor"][0])
		}

		var resp response
		idx, err := search.Open(index)
		if s.hasErr(w, http.StatusInternalServerError, err) {
			return
		}

		for t := idx.Search(r.Context(), query["q"][0], opt); ; {
			var d map[string]interface{}
			id, err := t.Next(&d)
			if err == search.Done {
				resp.Cursor = string(t.Cursor())
				break
			}
			if s.hasErr(w, http.StatusInternalServerError, err) {
				return
			}
			if id != "" {
				d["__id__"] = id
				resp.Result = append(resp.Result, d)
			}
		}
		if resp.Result == nil {
			resp.Result = make([]map[string]interface{}, 0)
		}
		s.JSON(w, http.StatusOK, resp)
	}
}
