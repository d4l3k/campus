package main

import (
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gorilla/mux"
	"github.com/kellydunn/golang-geo"
)

type Building struct {
	Floors []*Floor `json:"floors"`
}
type Floor struct {
	Name   string  `json:"floor"`
	Coords *Coords `json:"coords"`
	Image  string  `json:"image"`
	Rooms  []*Room `json:"rooms"`

	NativeImage image.Image
	ImageWG     sync.WaitGroup
	ImageOnce   sync.Once
}

func (f *Floor) LoadImage() (image.Image, error) {
	var err error
	var img image.Image
	f.ImageOnce.Do(func() {
		f.ImageWG.Add(1)
		defer f.ImageWG.Done()
		log.Printf("Loading image: %s", f.Image)
		fImg, err2 := os.Open("static/" + f.Image)
		if err2 != nil {
			err = err2
			return
		}
		defer fImg.Close()
		img, _, err = image.Decode(fImg)
		if err != nil {
			return
		}
		f.NativeImage = img
	})
	f.ImageWG.Wait()
	if f.NativeImage != nil {
		return f.NativeImage, nil
	}
	return img, err
}

type Coords struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
}

type ZoomableCoord struct {
	*Coords

	Zoom int `json:"zoom"`
}

func (c Coords) Overlap(c2 *Coords) bool {
	return c.West < c2.East && c.East > c2.West && c.North > c2.South && c.South < c2.North
}

type Room struct {
	Id       string  `json:"id"`
	Name     string  `json:"name"`
	Position *LatLng `json:"position"`
}

type LatLng struct {
	Lat float64 `json:"H"`
	Lng float64 `json:"L"`
}

type Server struct {
	r         *mux.Router
	buildings []*Building
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

	return s, nil
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

func (s *Server) tiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	z, err := strconv.Atoi(vars["zoom"])
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	x, err := strconv.Atoi(vars["x"])
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	y, err := strconv.Atoi(vars["y"])
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	floorName := vars["floor"]

	point := tileToPoint(x, y, z)
	pointBottom := tileToPoint(x+1, y+1, z)
	log.Printf("Map tile %d %d %d %+v %+v", z, x, y, point, pointBottom)
	coords := &Coords{
		North: point.Lat(),
		South: pointBottom.Lat(),
		West:  point.Lng(),
		East:  pointBottom.Lng(),
	}
	buildings := s.overlappingPeers(coords)
	log.Printf("Buildings len = %d", len(buildings))

	m := image.NewNRGBA(image.Rect(0, 0, 256, 256))

	for _, building := range buildings {
		for _, floor := range building.Floors {
			if floor.Name != floorName {
				continue
			}
			img, err := floor.LoadImage()
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			draw.Draw(m, image.Rect(0, 0, 256, 256), img, image.ZP, draw.Over)
		}
	}

	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, m); err != nil {
		log.Println(err)
	}
}

func main() {
	s, err := NewServer()
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(s.Listen())
}

func tileToPoint(x, y, z int) *geo.Point {
	xf := float64(x)
	yf := float64(y)
	zf := float64(z)

	long := xf/math.Pow(2, zf)*360 - 180
	n := math.Pi - 2*math.Pi*yf/math.Pow(2, zf)
	lat := (180 / math.Pi * math.Atan(0.5*(math.Exp(n)-math.Exp(-n))))

	return geo.NewPoint(lat, long)
}

/*
func tile2long(x, z) { return (x/Math.pow(2, z)*360 - 180) }

func tile2lat(y, z) {
	var n = Math.PI - 2*Math.PI*y/Math.pow(2, z)
	return (180 / Math.PI * Math.atan(0.5*(Math.exp(n)-Math.exp(-n))))
}
*/
