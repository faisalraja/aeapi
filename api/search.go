package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/appengine"

	"github.com/araddon/dateparse"
	"github.com/gorilla/mux"
	"google.golang.org/appengine/search"
)

type docIndex struct {
	ID     string                   `json:"id"`
	Fields []search.Field           `json:"fields,omitempty"`
	Meta   *search.DocumentMetadata `json:"meta,omitempty"`
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
	type facet struct {
		Value interface{} `json:"value"`
		Start float64     `json:"start"`
		End   float64     `json:"end"`
		Count int         `json:"count"`
	}
	type response struct {
		Result []docIndex         `json:"result"`
		Facets map[string][]facet `json:"facets"`
		Cursor string             `json:"cursor"`
	}
	return s.handler(func(r *http.Request) interface{} {
		vars := mux.Vars(r)
		index := vars["index"]
		ctx := r.Context()
		ns := prefixNS(ctx, vars["ns"])
		ctx, err := appengine.Namespace(ctx, ns)
		if err != nil {
			return err
		}
		query := r.URL.Query()

		type cachedDocs struct {
			Found   bool
			IDsOnly bool
			IDs     []string
			Data    []byte
		}
		var resp response
		cd := &cachedDocs{}
		cKey := searchCacheKey(ctx, ns, index, query)
		if err := getCache(ctx, cKey, cd); err != nil {
			log.Println("GetCache: error", err)
		}
		if cd.Found {
			log.Println("CacheHit")
			if cd.IDsOnly {
				if cd.IDs == nil {
					return make([]string, 0)
				}
				return cd.IDs
			}
			if err := json.Unmarshal(cd.Data, &resp); err != nil {
				log.Println("UnmarshalErr", err)
			} else {
				return resp
			}
		}

		if len(query["q"]) == 0 {
			return badErr{m: "Query string missing"}
		}
		opt := &search.SearchOptions{}
		withMeta := false
		if len(query["limit"]) == 1 {
			limit, _ := strconv.Atoi(query["limit"][0])
			opt.Limit = limit
		}
		if len(query["ids"]) == 1 {
			opt.IDsOnly = query["ids"][0] == "true"
		}
		if len(query["fields"]) > 0 {
			opt.Fields = query["fields"]
		}
		if len(query["meta"]) > 0 {
			withMeta = true
		}
		if len(query["cursor"]) == 1 {
			opt.Cursor = search.Cursor(query["cursor"][0])
		}
		if len(query["facets"]) > 0 {
			if query["facets"][0] == "auto" {
				opt.Facets = append(opt.Facets, search.AutoFacetDiscovery(0, 0))
			} else {
				for _, facet := range query["facets"] {
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

		idx, err := search.Open(index)
		if err != nil {
			return err
		}
		var ids []string
		it := idx.Search(ctx, query["q"][0], opt)
		for {
			var d docIndex
			id, err := it.Next(&d)
			if err == search.Done {
				resp.Cursor = string(it.Cursor())
				break
			}
			if err != nil {
				log.Println("SearchError", err)
				break
			}
			if id != "" {
				d.ID = id
				ids = append(ids, id)
				if len(opt.Fields) == 0 {
					d.Fields = nil
				}
				if !withMeta {
					d.Meta = nil
				}
				resp.Result = append(resp.Result, d)
			}
		}
		if resp.Result == nil {
			resp.Result = make([]docIndex, 0)
			ids = make([]string, 0)
		}
		facets, err := it.Facets()
		if err == nil {
			resp.Facets = make(map[string][]facet)
			for _, results := range facets {
				for _, f := range results {
					fr := facet{Count: f.Count}
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
					resp.Facets[f.Name] = append(resp.Facets[f.Name], fr)
				}
			}
		} else {
			log.Println("FacetsErr", err)
		}
		cd.Found = true
		cd.IDsOnly = opt.IDsOnly
		if opt.IDsOnly {
			cd.IDs = ids
		} else {
			cd.Data, err = json.Marshal(resp)
			if err != nil {
				log.Println("MarshalErr", err)
				cd.Found = false
			}
		}
		if cd.Found {
			if err := setCache(ctx, cKey, cd, time.Hour*12); err != nil {
				log.Println("SetCache error", err)
			}
		}
		if opt.IDsOnly {
			return cd.IDs
		}
		return resp
	})
}

func (s *Server) handlePutSearch() http.HandlerFunc {
	type field struct {
		Name     string      `json:"name"`
		Value    interface{} `json:"value"`
		Type     string      `json:"type"`
		Facet    bool        `json:"facet"`
		Rank     int         `json:"rank"`
		Derived  bool        `json:"derived"`
		Language string      `json:"language"`
	}
	type doc struct {
		ID     string  `json:"id"`
		Fields []field `json:"fields"`
	}
	type request struct {
		Docs []doc `json:"docs"`
	}
	return s.handler(func(r *http.Request) interface{} {
		vars := mux.Vars(r)
		idx := vars["index"]
		ctx := r.Context()
		ns := prefixNS(ctx, vars["ns"])
		ctx, err := appengine.Namespace(ctx, ns)
		if err != nil {
			return err
		}
		index, err := search.Open(idx)
		if err != nil {
			return err
		}
		var req request
		if err := s.readJSON(r, &req); err != nil {
			return err
		}
		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			errs   []error
			err1   error
			err2   error
			ids    []string
			docs   []interface{}
			docIds []string
		)
		putDocs := func(dIDs []string, ds []interface{}) {
			defer wg.Done()
			log.Println("Indexing", len(dIDs), "docs")
			pIDs, err := index.PutMulti(ctx, dIDs, ds)
			mu.Lock()
			if err != nil {
				errs = append(errs, err)
			} else {
				ids = append(ids, pIDs...)
			}
			mu.Unlock()
		}
		strVal := func(d interface{}) string {
			v, ok := d.(string)
			if !ok {
				v = fmt.Sprint(d)
			}
			return v
		}
		for _, doc := range req.Docs {
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
				f := search.Field{Name: field.Name, Derived: field.Derived, Language: field.Language}
				switch field.Type {
				case "html":
					f.Value = search.HTML(strVal(field.Value))
				case "atom":
					f.Value = search.Atom(strVal(field.Value))
				case "date", "datetime":
					v := strVal(field.Value)
					if t, err := dateparse.ParseAny(v); err != nil {
						f.Value = search.Atom(v)
					} else {
						f.Value = t
					}
				case "geopoint":
					v := strVal(field.Value)
					ll := strings.Split(v, ",")
					gp := appengine.GeoPoint{}
					gp.Lat, _ = strconv.ParseFloat(ll[0], 64)
					gp.Lng, _ = strconv.ParseFloat(ll[1], 64)
					f.Value = gp
				default:
					f.Value = field.Value
				}
				d.Fields[j] = f
			}
			docIds = append(docIds, doc.ID)
			docs = append(docs, d)
			if len(docIds) >= 200 {
				wg.Add(1)
				go putDocs(docIds, docs)
				docIds = nil
				docs = nil
			}
		}
		if len(docIds) > 0 {
			wg.Add(1)
			go putDocs(docIds, docs)
		}
		wg.Add(2)
		go func() {
			defer wg.Done()
			err2 = resetSearchCacheKey(ctx, ns, idx)
		}()
		go func() {
			defer wg.Done()
			err1 = newDelayedReset(ctx, ns, idx)
		}()
		wg.Wait()
		if len(errs) > 0 {
			return fmt.Errorf("put errs: %v", errs)
		}
		if err1 != nil {
			return err1
		}
		if err2 != nil {
			return err2
		}
		return ids
	})
}

func (s *Server) handleDeleteSearch() http.HandlerFunc {
	return s.handler(func(r *http.Request) interface{} {
		q := r.URL.Query()
		ids := q["id"]
		if len(ids) == 0 {
			if dd := q["ids"]; len(dd) > 0 {
				ids = strings.Split(dd[0], ",")
			}
			if len(ids) == 0 {
				return badErr{m: "ids not found"}
			}
		}
		vars := mux.Vars(r)
		idx := vars["index"]
		ctx := r.Context()
		ns := prefixNS(ctx, vars["ns"])
		ctx, err := appengine.Namespace(ctx, ns)
		if err != nil {
			return err
		}
		index, err := search.Open(idx)
		if err != nil {
			return err
		}
		var (
			wg   sync.WaitGroup
			err1 error
			err2 error
			err3 error
		)
		wg.Add(3)
		go func() {
			defer wg.Done()
			err1 = index.DeleteMulti(ctx, ids)
		}()
		go func() {
			defer wg.Done()
			err2 = resetSearchCacheKey(ctx, ns, idx)
		}()
		go func() {
			defer wg.Done()
			err3 = newDelayedReset(ctx, ns, idx)
		}()
		wg.Wait()
		if err1 != nil {
			return err1
		}
		if err2 != nil {
			return err2
		}
		if err3 != nil {
			return err3
		}
		return nil
	})
}

func (s *Server) handleDropIndex() http.HandlerFunc {
	return s.handler(func(r *http.Request) interface{} {
		vars := mux.Vars(r)
		idx := vars["index"]
		ctx := r.Context()
		ns := prefixNS(ctx, vars["ns"])
		ctx, err := appengine.Namespace(ctx, ns)
		if err != nil {
			return err
		}
		index, err := search.Open(idx)
		if err != nil {
			return err
		}
		var (
			wg   sync.WaitGroup
			mu   sync.Mutex
			errs []error
			err1 error
			err2 error
			ids  []string
		)

		it := index.List(ctx, &search.ListOptions{IDsOnly: true})
		delBatch := func(batch []string) {
			if err := index.DeleteMulti(ctx, batch); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}
		for {
			id, err := it.Next(nil)
			if err == search.Done {
				break
			}
			if err != nil {
				return err
			}
			if id != "" {
				ids = append(ids, id)
			}
			if len(ids) > 200 {
				wg.Add(1)
				go delBatch(ids)
				ids = nil
			}
		}
		if len(ids) > 0 {
			wg.Add(1)
			go delBatch(ids)
		}
		wg.Add(2)
		go func() {
			defer wg.Done()
			err1 = newDelayedReset(ctx, ns, idx)
		}()
		go func() {
			defer wg.Done()
			err2 = resetSearchCacheKey(ctx, ns, idx)
		}()
		wg.Wait()
		if err1 != nil {
			return err1
		}
		if err2 != nil {
			return err2
		}
		if len(errs) > 0 {
			return fmt.Errorf("delete errs: %v", errs)
		}
		return nil
	})
}

func (s *Server) handleResetSearch() http.HandlerFunc {
	return s.handler(func(r *http.Request) interface{} {
		ctx := r.Context()
		ns := r.FormValue("ns")
		index := r.FormValue("index")
		return resetSearchCacheKey(ctx, ns, index)
	})
}
