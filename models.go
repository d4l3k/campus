package main

import (
	"image"
	"log"
	"os"
	"sync"
)

type Building struct {
	Floors []*Floor `json:"floors"`
	Name   string   `json:"name"`
	SIS    string   `json:"sis"`
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
