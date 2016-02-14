package models

import (
	"image"
	"log"
	"os"
	"sync"
)

type Building struct {
	Floors   []*Floor `json:"floors"`
	Name     string   `json:"name"`
	SIS      string   `json:"sis"`
	Position *LatLng  `json:"position"`
	Address  string
	Image    string
}

func (b Building) Meta() *Building {
	return &Building{
		Name:     b.Name,
		SIS:      b.SIS,
		Position: b.Position,
		Address:  b.Address,
		Image:    b.Image,
	}
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

	Zoom  int    `json:"zoom"`
	Floor string `json:"floor"`
}

func (c Coords) Overlap(c2 *Coords) bool {
	return c.West < c2.East && c.East > c2.West && c.North > c2.South && c.South < c2.North
}

func (c Coords) OverlapLatLng(p *LatLng) bool {
	return c.West < p.Lng && c.East > p.Lng && c.North > p.Lat && c.South < p.Lat
}

type Room struct {
	Id       string  `json:"id"`
	SIS      string  `json:"sis"`
	Name     string  `json:"name"`
	Position *LatLng `json:"position"`
	Type     string  `json:"type"`
	Floor    string  `json:"floor"`
}

type LatLng struct {
	Lat float64 `json:"H"`
	Lng float64 `json:"L"`
}

type Index struct {
	Id    string
	Name  string
	Type  string
	Image string

	Item interface{} `json:"-"`
}
