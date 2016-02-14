package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/search"
	"github.com/d4l3k/campus/models"
	"github.com/golang/groupcache"
	"github.com/gorilla/mux"
)

const TileSize = 256

type Server struct {
	r                *mux.Router
	buildings        []*models.Building
	zoomedFloorCache *groupcache.Group
	tileCache        *groupcache.Group
	index            bleve.Index
	idIndex          map[string]*models.Index
}

func NewServer() (*Server, error) {
	s := &Server{}
	s.r = mux.NewRouter()
	s.r.HandleFunc("/tiles/{zoom}_{x}_{y}_{floor}.png", s.tiles)
	s.r.HandleFunc("/view/{json}", s.view)
	s.r.HandleFunc("/search/", s.search)
	s.r.HandleFunc("/item/{json}", s.item)
	s.r.HandleFunc("/dump/", s.dump)
	s.r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
	http.Handle("/", s.r)

	log.Println("Loading existing map data...")
	buildings, err := models.LoadMapData()
	if err != nil {
		return nil, err
	}
	s.buildings = buildings

	s.indexBuildings()

	s.initCache()

	return s, nil
}

func (s *Server) indexBuildings() {
	s.idIndex = make(map[string]*models.Index)
	dir, err := ioutil.TempDir("", "campus")
	if err != nil {
		log.Fatal(err)
	}
	file := dir + "/index.bleve"
	log.Printf("Index file %s", file)
	mapping := bleve.NewIndexMapping()
	index, err := bleve.New(file, mapping)
	if err != nil {
		log.Fatal(err)
	}
	s.index = index
	for _, b := range s.buildings {
		idx := &models.Index{
			Id:   b.SIS,
			Name: b.Name,
			Type: "building",
		}
		index.Index(b.SIS, idx)
		idx.Item = b.Meta()
		idx.Image = b.Image
		s.idIndex[b.SIS] = idx
		for _, f := range b.Floors {
			for _, r := range f.Rooms {
				id := b.SIS + " " + r.Id
				idx := &models.Index{
					Id:   id,
					Name: r.Name,
					Type: r.Type,
				}
				index.Index(id, idx)
				idx.Item = r
				r.Floor = f.Name
				r.SIS = b.SIS
				s.idIndex[id] = idx
			}
		}
	}
}

func (s *Server) Listen() error {
	log.Println("Listening on :8383...")
	return http.ListenAndServe(":8383", nil)
}

// GetBuildingFloor returns the specified floor from building and floor name.
func (s *Server) GetBuildingFloor(b string, f string) *models.Floor {
	for _, building := range s.buildings {
		if building.Name != b {
			continue
		}
		for _, floor := range building.Floors {
			if floor.Name == f {
				return floor
			}
		}
	}
	return nil
}

// OverlappingBuildings returns overlapping buildings with the coords.
func (s *Server) OverlappingBuildings(c *models.Coords) []*models.Building {
	var buildings []*models.Building
Building:
	for _, building := range s.buildings {
		if c.OverlapLatLng(building.Position) {
			buildings = append(buildings, building)
			continue Building
		}
		for _, floor := range building.Floors {
			if c.Overlap(floor.Coords) {
				buildings = append(buildings, building)
				continue Building
			}
		}
	}
	return buildings
}

type ViewResp struct {
	Floors    []string
	Rooms     []*models.Room
	Buildings []*models.Building
}

func (s *Server) item(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	args := vars["json"]

	results, ok := s.idIndex[args]
	if !ok {
		http.Error(w, "item not found", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results.Item)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	q := query.Get("q")
	typeFilter := query.Get("type")

	results := []*models.Index{}
	if idx, ok := s.idIndex[q]; ok {
		results = append(results, idx)
	} else {
		query_list := make([]bleve.Query, 2)
		fuzzy_query := bleve.NewFuzzyQuery(q)
		fuzzy_query.FuzzinessVal = 3
		query_list[0] = fuzzy_query
		query_list[1] = bleve.NewRegexpQuery("[a-zA-Z0-9_]*" + q + "[a-zA-Z0-9_]*")

		var query_must []bleve.Query

		if typeFilter != "all" {
			termQuery := bleve.NewTermQuery(typeFilter)
			query_must = append(query_must, termQuery)
		}

		query := bleve.NewBooleanQuery(query_must, query_list, nil)

		searchRequest := bleve.NewSearchRequest(query)
		searchResult, err := s.index.Search(searchRequest)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		for _, result := range []*search.DocumentMatch(searchResult.Hits) {
			results = append(results, s.idIndex[result.ID])
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) view(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	args := vars["json"]
	coords := &models.ZoomableCoord{}
	if err := json.Unmarshal([]byte(args), coords); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	log.Printf("floors coords %+v", coords)
	buildings := s.OverlappingBuildings(coords.Coords)
	var names []string
	var rooms []*models.Room
	var buildingMeta []*models.Building
	nameDup := make(map[string]bool)
	for _, building := range buildings {
		if coords.Coords.OverlapLatLng(building.Position) {
			buildingMeta = append(buildingMeta, building.Meta())
		}
		for _, floor := range building.Floors {
			if coords.Zoom < 19 {
				continue
			}
			if floor.Name == coords.Floor {
				for _, room := range floor.Rooms {
					if coords.Coords.OverlapLatLng(room.Position) {
						rooms = append(rooms, room)
					}
				}
			}
			if nameDup[floor.Name] {
				continue
			}
			names = append(names, floor.Name)
			nameDup[floor.Name] = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&ViewResp{
		Floors:    names,
		Rooms:     rooms,
		Buildings: buildingMeta,
	})
}

func (s *Server) dump(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.buildings)
}

func main() {
	s, err := NewServer()
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(s.Listen())
}
