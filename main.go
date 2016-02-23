package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"google.golang.org/api/googleapi/transport"
	"google.golang.org/api/vision/v1"

	"github.com/BurntSushi/graphics-go/graphics"
	"github.com/PuerkitoBio/goquery"
	"github.com/abbot/go-http-auth"
	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/search"
	"github.com/d4l3k/campus/models"
	"github.com/golang/groupcache"
	"github.com/gorilla/mux"

	_ "net/http/pprof"
)

var (
	adminPassword      = flag.String("pass", "", "the md5 hash of the admin password")
	googleAPIKey       = flag.String("key", "", "the Google Vision API key")
	googleClientID     = flag.String("clientID", "", "the Google Vision API client id")
	googleClientSecret = flag.String("clientSecret", "", "the Google Vision API client secret")
	cacheToken         = flag.Bool("cachetoken", true, "cache the OAuth 2.0 token")
)

const TileSize = 256

type Server struct {
	r                *mux.Router
	buildings        []*models.Building
	zoomedFloorCache *groupcache.Group
	tileCache        *groupcache.Group
	index            bleve.Index
	idIndex          map[string]*models.Index
	authenticator    auth.AuthenticatorInterface

	mapTileReq chan *MapTileRequest
}

func NewServer() (*Server, error) {
	flag.Parse()

	if len(*adminPassword) == 0 {
		log.Println("Warning: no admin password; login impossible")
	}
	if len(*googleAPIKey) == 0 {
		log.Println("Warning: no google api key; OCR impossible")
	}

	s := &Server{}

	s.authenticator = auth.NewBasicAuthenticator("localhost", s.secret)

	s.r = mux.NewRouter()
	s.r.HandleFunc("/api/tiles/{zoom}_{x}_{y}_{floor}.png", s.tiles)
	s.r.HandleFunc("/api/view/{json}", s.view)
	s.r.HandleFunc("/api/schedule/{loc}", s.schedule)
	s.r.HandleFunc("/api/search/", s.search)
	s.r.HandleFunc("/api/item/{json}", s.item)
	s.r.HandleFunc("/api/dump/", s.dump)
	s.r.HandleFunc("/api/save_building/", s.authenticator.Wrap(s.saveBuilding))
	s.r.HandleFunc("/api/ocr/", s.authenticator.Wrap(s.ocrFloor))
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
	s.initTileBuilding()

	return s, nil
}

func (s Server) secret(user, realm string) string {
	if user == "admin" {
		return *adminPassword
	}
	return ""
}

// indexBuildings builds the search index as well as a way to look items up by SIS/room number.
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
			Id:          b.SIS,
			Name:        b.Name,
			Type:        "building",
			Description: b.Description,
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

// saveBuilding saves the changes made to a building.
func (s *Server) saveBuilding(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	b := &models.Building{}
	if err := json.NewDecoder(r.Body).Decode(b); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	for i, b2 := range s.buildings {
		if b2.SIS != b.SIS {
			continue
		}
		for _, f := range b.Floors {
			dx, dy := newDimentions(1, 1, -f.Rotation)
			_ = dx
			_ = dy
			affine := graphics.I.Rotate(-f.Rotation)
			for _, r := range f.Rooms {
				if r.RelPosition == nil {
					continue
				}
				rotated := affine.Mul(graphics.Affine{
					(r.RelPosition.Lng - 0.5), 0, 0,
					(r.RelPosition.Lat - 0.5), 0, 0,
					1, 1, 1,
				})
				px := rotated[0] / dx
				py := rotated[3] / dy

				log.Printf("PX %f PY %f", px, py)

				lat := (-py+0.5)*f.Coords.DLat() + f.Coords.South
				lng := (px+0.5)*f.Coords.DLng() + f.Coords.West
				r.Position = &models.LatLng{
					Lat: lat,
					Lng: lng,
				}
			}
		}
		s.buildings[i] = b
		if err := models.SaveMapData(s.buildings); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.buildings[i])
		return
	}
	http.Error(w, "building SIS not found", 404)
}

// ocrFloor runs google OCR on the floor image for text analysis.
func (s *Server) ocrFloor(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	floor := &models.Floor{}
	if err := json.NewDecoder(r.Body).Decode(floor); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	bytes, err := ioutil.ReadFile("static/" + floor.Image)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var config = &oauth2.Config{
		ClientID:     *googleClientID,
		ClientSecret: *googleClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{vision.CloudPlatformScope},
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
		Transport: &transport.APIKey{Key: *googleAPIKey},
	})
	client := newOAuthClient(ctx, config)
	service, err := vision.New(client)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	req := &vision.BatchAnnotateImagesRequest{
		Requests: []*vision.AnnotateImageRequest{
			{
				Features: []*vision.Feature{
					{
						Type: "TEXT_DETECTION",
					},
				},
				Image: &vision.Image{
					Content: base64.StdEncoding.EncodeToString(bytes),
				},
			},
		},
	}
	log.Printf("calling annotate")
	call := service.Images.Annotate(req)
	resp, err := call.Do()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	log.Printf("OCR %+v", resp)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp.Responses)
}

// item returns a specific search result item.
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

// schedule returns the schedule for a UBC food services location.
func (s *Server) schedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	args := vars["loc"]

	doc, err := goquery.NewDocument("http://www.food.ubc.ca/place/" + args + "/")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	schedule, err := doc.Find(".location-hours").Html()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(schedule))
}

// search executes a search for rooms or buildings.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	q := query.Get("q")
	typeFilter := query.Get("type")

	results := []*models.Index{}
	if idx, ok := s.idIndex[q]; ok {
		results = append(results, idx)
	} else {
		var queryShould []bleve.Query
		if len(q) > 0 {
			/*fuzzy_query := bleve.NewFuzzyQuery(q)
			fuzzy_query.FuzzinessVal = 3
			queryShould = append(queryShould, fuzzy_query)
			queryShould = append(queryShould, bleve.NewRegexpQuery("[a-zA-Z0-9_]*"+q+"[a-zA-Z0-9_]*"))
			queryShould = append(queryShould, bleve.NewQueryStringQuery(q))*/
			queryShould = append(queryShould, bleve.NewQueryStringQuery(q))
		}

		var queryMust []bleve.Query
		if typeFilter != "all" {
			termQuery := bleve.NewTermQuery(typeFilter)
			queryMust = append(queryMust, termQuery)
		}

		query := bleve.NewBooleanQuery(queryMust, queryShould, nil)

		searchRequest := bleve.NewSearchRequest(query)
		searchRequest.Size = 25
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

// view returns the items that should be displayed to the user.
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

// dump just dumps the entire database.
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
