package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/araddon/dateparse"
	"github.com/gorilla/mux"
	"google.golang.org/appengine/search"
)

type docIndex struct {
	ID     string
	Fields []search.Field
	Meta   *search.DocumentMetadata
}

// Load document index
func (di *docIndex) Load(fields []search.Field, meta *search.DocumentMetadata) error {
	di.Fields = fields
	di.Meta = meta
	return nil
}

// Save document index
func (di *docIndex) Save() ([]search.Field, *search.DocumentMetadata, error) {
	return di.Fields, di.Meta, nil
}

func (s *Server) handleGetSearch() http.HandlerFunc {
	type facetResult struct {
		Value interface{}
		Start float64
		End   float64
		Count int
	}
	type facet struct {
		Name   string
		Result []facetResult
	}
	type response struct {
		Result []docIndex
		Facets []facet
		Cursor string
	}
	return s.handler(func(r *http.Request) (int, interface{}) {
		index := mux.Vars(r)["index"]
		query := r.URL.Query()
		if len(query["q"]) == 0 {
			return http.StatusBadRequest, fmt.Errorf("Query string missing")
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
		if len(query["Facets"]) > 0 {
			if query["Facets"][0] == "auto" {
				opt.Facets = append(opt.Facets, search.AutoFacetDiscovery(0, 0))
			} else {
				for _, facet := range query["Facets"] {
					ff := strings.Split(facet, "|")
					vals := make([]interface{}, 0)
					if len(ff) >= 2 {
						for _, fv := range strings.Split(ff[1], ",") {
							if strings.Contains(fv, "---") {
								rn := strings.Split(fv, "---")
								sr := search.Range{}
								sr.Start, _ = strconv.ParseFloat(rn[0], 64)
								sr.End, _ = strconv.ParseFloat(rn[1], 64)
								vals = append(vals, sr)
							} else {
								vals = append(vals, search.Atom(fv))
							}
						}
					}
					if len(ff) >= 3 {
						if l, err := strconv.Atoi(ff[2]); err == nil {
							vals = append(vals, search.AtLeast(float64(l)))
						}
					}
					f := search.FacetDiscovery(ff[0], vals...)
					opt.Facets = append(opt.Facets, f)
				}
			}
		}

		var resp response
		idx, err := search.Open(index)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		var ids []string
		it := idx.Search(r.Context(), query["q"][0], opt)
		for {
			var d docIndex
			id, err := it.Next(&d)
			if err == search.Done {
				resp.Cursor = string(it.Cursor())
				break
			}
			if err != nil {
				return http.StatusInternalServerError, err
			}
			if id != "" {
				d.ID = id
				ids = append(ids, id)
				resp.Result = append(resp.Result, d)
			}
		}
		if resp.Result == nil {
			resp.Result = make([]docIndex, 0)
			ids = make([]string, 0)
		}
		facets, err := it.Facets()
		if err != nil {
			return http.StatusInternalServerError, err
		}
		resp.Facets = make([]facet, len(facets))
		for i, results := range facets {
			for j, f := range results {
				if j == 0 {
					resp.Facets[i] = facet{Name: f.Name, Result: make([]facetResult, len(results))}
				}
				fr := facetResult{Count: f.Count}
				if r, ok := f.Value.(search.Range); ok {
					if !math.IsInf(r.Start, 0) {
						fr.Start = r.Start
					}
					if !math.IsInf(r.End, 0) {
						fr.End = r.End
					}
				} else {
					fr.Value = f.Value
				}
				resp.Facets[i].Result = append(resp.Facets[i].Result, fr)
			}
		}
		if opt.IDsOnly {
			return http.StatusOK, ids
		}
		return http.StatusOK, resp
	})
}

func (s *Server) handlePutSearch() http.HandlerFunc {
	type field struct {
		Name  string
		Value interface{}
		Type  string
		Facet bool
		Rank  int
	}
	type doc struct {
		ID     string
		Fields []field
	}
	type request struct {
		Docs []doc
	}
	return s.handler(func(r *http.Request) (int, interface{}) {
		idx := mux.Vars(r)["index"]
		index, err := search.Open(idx)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		var req request
		if err := s.readJSON(r, &req); err != nil {
			return http.StatusInternalServerError, err
		}
		docsLen := len(req.Docs)
		docIds := make([]string, docsLen)
		docs := make([]interface{}, docsLen)
		for i, doc := range req.Docs {
			d := &docIndex{
				ID:     doc.ID,
				Fields: make([]search.Field, len(doc.Fields)),
				Meta:   &search.DocumentMetadata{},
			}
			for j, field := range doc.Fields {
				if field.Facet {
					facet := search.Facet{Name: field.Name}
					if v, ok := field.Value.(string); ok {
						facet.Value = search.Atom(v)
					} else {
						facet.Value = field.Value
					}
					d.Meta.Facets = append(d.Meta.Facets, facet)
				}
				f := search.Field{Name: field.Name}
				switch field.Type {
				case "atom":
					f.Value = search.Atom(field.Value.(string))
				case "date", "datetime":
					v := field.Value.(string)
					if t, err := dateparse.ParseAny(v); err != nil {
						f.Value = search.Atom(v)
					} else {
						f.Value = t
					}
				default:
					f.Value = field.Value
				}
				d.Fields[j] = f
			}
			docIds[i] = doc.ID
			docs[i] = d
		}
		ids, err := index.PutMulti(r.Context(), docIds, docs)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		return http.StatusOK, ids
	})
}
