package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/golang/groupcache"
	"github.com/gorilla/mux"
)

type Server struct {
	r                *mux.Router
	buildings        []*Building
	zoomedFloorCache *groupcache.Group
	tileCache        *groupcache.Group
}

func NewServer() (*Server, error) {
	s := &Server{}
	s.r = mux.NewRouter()
	s.r.HandleFunc("/tiles/{zoom}_{x}_{y}_{floor}.png", s.tiles)
	s.r.HandleFunc("/floors/{json}", s.floors)
	s.r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
	http.Handle("/", s.r)

	log.Println("Loading existing map data...")
	buf, err := ioutil.ReadFile("static/maps/map.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(buf, &s.buildings); err != nil {
		return nil, err
	}

	s.initCache()

	return s, nil
}

func (s *Server) GetBuildingFloor(b string, f string) *Floor {
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

func (s *Server) Listen() error {
	log.Println("Listening on :8383...")
	return http.ListenAndServe(":8383", nil)
}

func (s *Server) overlappingPeers(c *Coords) []*Building {
	var buildings []*Building
Building:
	for _, building := range s.buildings {
		for _, floor := range building.Floors {
			if c.Overlap(floor.Coords) {
				buildings = append(buildings, building)
				continue Building
			}
		}
	}
	return buildings
}

func (s *Server) floors(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	args := vars["json"]
	coords := &ZoomableCoord{}
	if err := json.Unmarshal([]byte(args), coords); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	log.Printf("floors coords %+v", coords)
	buildings := s.overlappingPeers(coords.Coords)
	var names []string
	nameDup := make(map[string]bool)
	for _, building := range buildings {
		for _, floor := range building.Floors {
			if nameDup[floor.Name] {
				continue
			}
			names = append(names, floor.Name)
			nameDup[floor.Name] = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

func main() {
	s, err := NewServer()
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(s.Listen())
}
