package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	"log"
	"net/http"
	"strconv"

	"github.com/d4l3k/campus/models"
	"github.com/golang/groupcache"
	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
)

const TileWorkers = 4

type zoomedImageGetter struct {
	s *Server
}

func (g zoomedImageGetter) Get(ctx groupcache.Context, key string, dest groupcache.Sink) error {
	bfz := &BuildingFloorZoom{}
	if err := json.Unmarshal([]byte(key), bfz); err != nil {
		return err
	}

	floor := g.s.GetBuildingFloor(bfz.Building, bfz.Floor)
	img, err := floor.LoadImage()
	if err != nil {
		return err
	}
	coords := ctx.(*models.Coords)
	latDiff := coords.North - coords.South
	lngDiff := coords.East - coords.West
	pixelsPerLongitude := TileSize / (lngDiff)
	pixelsPerLatitude := TileSize / (latDiff)
	newWidth := (floor.Coords.East - floor.Coords.West) * pixelsPerLongitude
	newHeight := (floor.Coords.North - floor.Coords.South) * pixelsPerLatitude

	log.Printf("Generating resized image %f %f", newWidth, newHeight)

	resizedImg := resize.Resize(uint(newWidth), uint(newHeight), img, resize.NearestNeighbor)
	var buf bytes.Buffer
	if err := png.Encode(&buf, resizedImg); err != nil {
		return err
	}
	return dest.SetBytes(buf.Bytes())
}

type mapTileGetter struct {
	s *Server
}

func (g mapTileGetter) Get(ctx groupcache.Context, key string, dest groupcache.Sink) error {
	req := &MapTileRequest{}
	if err := json.Unmarshal([]byte(key), req); err != nil {
		return err
	}

	point := tileToPoint(req.X, req.Y, req.Z)
	pointBottom := tileToPoint(req.X+1, req.Y+1, req.Z)
	log.Printf("Map tile req %+v %+v %+v", req, point, pointBottom)
	coords := &models.Coords{
		North: point.Lat(),
		South: pointBottom.Lat(),
		West:  point.Lng(),
		East:  pointBottom.Lng(),
	}
	buildings := g.s.OverlappingBuildings(coords)

	m := image.NewNRGBA(image.Rect(0, 0, TileSize, TileSize))

	for _, building := range buildings {
		for _, floor := range building.Floors {
			if floor.Name != req.Floor {
				continue
			}
			bfz := &BuildingFloorZoom{building.Name, floor.Name, req.Z}
			buf, err := json.Marshal(bfz)
			if err != nil {
				return err
			}
			var resp []byte
			if err := g.s.zoomedFloorCache.Get(coords, string(buf), groupcache.AllocatingByteSliceSink(&resp)); err != nil {
				return err
			}
			resizedImg, _, err := image.Decode(bytes.NewBuffer(resp))
			if err != nil {
				return err
			}
			rect := resizedImg.Bounds()
			x := float64(rect.Dx()) - float64(rect.Dx())/(floor.Coords.East-floor.Coords.West)*(floor.Coords.East-coords.East) - TileSize
			y := float64(rect.Dy()) / (floor.Coords.North - floor.Coords.South) * (floor.Coords.North - coords.North)
			sp := image.Pt(int(x), int(y))
			draw.Draw(m, image.Rect(0, 0, TileSize, TileSize), resizedImg, sp, draw.Over)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		return err
	}
	return dest.SetBytes(buf.Bytes())
}

type BuildingFloorZoom struct {
	Building, Floor string
	Zoom            int
}

type MapTileRequest struct {
	X, Y, Z int
	Floor   string

	resp chan []byte
	err  chan error
}

func (s *Server) initTileBuilding() {
	s.mapTileReq = make(chan *MapTileRequest)
	for i := 0; i < TileWorkers; i++ {
		go s.tileWorker()
	}
}

func (s *Server) tileWorker() {
	for req := range s.mapTileReq {
		buf, err := json.Marshal(req)
		if err != nil {
			req.err <- err
			continue
		}

		var resp []byte
		if err := s.tileCache.Get(nil, string(buf), groupcache.AllocatingByteSliceSink(&resp)); err != nil {
			req.err <- err
			continue
		}
		req.resp <- resp
	}
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

	req := &MapTileRequest{
		x, y, z, floorName,
		make(chan []byte, 1),
		make(chan error, 1),
	}
	defer close(req.resp)
	defer close(req.err)
	s.mapTileReq <- req

	select {
	case err := <-req.err:
		http.Error(w, err.Error(), 500)
		return
	case resp := <-req.resp:
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(resp); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}

}
