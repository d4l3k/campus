package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/BurntSushi/graphics-go/graphics"
	"github.com/d4l3k/campus/models"
	"github.com/golang/groupcache"
	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
)

const TileWorkers = 4

var tileEncoder = &png.Encoder{CompressionLevel: png.NoCompression}
var blankTile []byte

func init() {
	img := image.NewNRGBA(image.Rect(0, 0, TileSize, TileSize))

	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	blankTile = buf.Bytes()
}

type zoomedImageGetter struct {
	s *Server
}

func newImageDimentions(img draw.Image, angle float64) (int, int) {
	bounds := img.Bounds()
	width := float64(bounds.Max.X - bounds.Min.X)
	height := float64(bounds.Max.Y - bounds.Min.Y)

	w, h := newDimentions(width, height, angle)
	return int(w), int(h)
}

func newDimentions(width, height, angle float64) (float64, float64) {
	affine := graphics.I.Rotate(angle)
	rotated := affine.Mul(graphics.Affine{
		width, width, 0,
		height, 0, height,
		1, 1, 1,
	})
	log.Printf("Rotation matrix %+v", rotated)

	// Compute new bounding coordinates
	left := math.Min(math.Min(0, rotated[0]), math.Min(rotated[1], rotated[2]))
	right := math.Max(math.Max(0, rotated[0]), math.Max(rotated[1], rotated[2]))
	bottom := math.Min(math.Min(0, rotated[3]), math.Min(rotated[4], rotated[5]))
	top := math.Max(math.Max(0, rotated[3]), math.Max(rotated[4], rotated[5]))

	return (math.Abs(right - left)), (math.Abs(top - bottom))
}

func (g zoomedImageGetter) Get(ctx groupcache.Context, key string, dest groupcache.Sink) error {
	bfz := &BuildingFloorZoom{}
	if err := json.Unmarshal([]byte(key), bfz); err != nil {
		return err
	}

	floor := g.s.GetBuildingFloor(bfz.Building, bfz.Floor)
	var err error
	floor.ImageOnce.Do(func() {
		var img, origImg draw.Image
		floor.ImageWG.Add(1)
		defer floor.ImageWG.Done()
		origImg, err = floor.LoadImage()
		if err != nil {
			return
		}

		img = origImg

		if floor.Rotation != 0 {
			rotatedWidth, rotatedHeight := newImageDimentions(origImg, floor.Rotation)
			img = image.NewNRGBA64(image.Rect(0, 0, rotatedWidth, rotatedHeight))
			if err = graphics.Rotate(img, origImg, &graphics.RotateOptions{Angle: floor.Rotation}); err != nil {
				return
			}
		}
		floor.RotatedImage = img
	})
	floor.ImageWG.Wait()
	if err != nil {
		return err
	}

	img := floor.RotatedImage

	coords := ctx.(*models.Coords)
	pixelsPerLongitude := TileSize / coords.DLng()
	pixelsPerLatitude := TileSize / coords.DLat()
	newWidth := floor.Coords.DLng() * pixelsPerLongitude
	newHeight := floor.Coords.DLat() * pixelsPerLatitude

	log.Printf("Generating resized image %f %f", newWidth, newHeight)

	resizedImg := resize.Resize(uint(newWidth), uint(newHeight), img, resize.NearestNeighbor)
	var buf bytes.Buffer
	if err := tileEncoder.Encode(&buf, resizedImg); err != nil {
		return err
	}
	return dest.SetBytes(buf.Bytes())
}

func (s *Server) generateTile(req *MapTileRequest) ([]byte, error) {
	url := fmt.Sprintf("static/api/tiles/%d_%d_%d_%s.png", req.Z, req.X, req.Y, req.Floor)
	if _, err := os.Stat(url); err == nil {
		if *debug {
			log.Printf("file exists, but not serving due to debug; %s", url)
		} else {
			return ioutil.ReadFile(url)
		}
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
	buildings := s.OverlappingBuildings(coords)

	if len(buildings) == 0 {
		return blankTile, nil
	}

	m := image.NewNRGBA(image.Rect(0, 0, TileSize, TileSize))
	for _, building := range buildings {
		for _, floor := range building.Floors {
			if floor.Name != req.Floor {
				continue
			}
			bfz := &BuildingFloorZoom{building.Name, floor.Name, req.Z}
			buf, err := json.Marshal(bfz)
			if err != nil {
				return nil, err
			}
			var resp []byte
			if err := s.zoomedFloorCache.Get(coords, string(buf), groupcache.AllocatingByteSliceSink(&resp)); err != nil {
				return nil, err
			}
			resizedImg, _, err := image.Decode(bytes.NewBuffer(resp))
			if err != nil {
				return nil, err
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
		return nil, err
	}
	bytes := buf.Bytes()
	if err := ioutil.WriteFile(url, bytes, 0755); err != nil {
		return nil, err
	}
	return bytes, nil
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
		resp, err := s.generateTile(req)
		if err != nil {
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
