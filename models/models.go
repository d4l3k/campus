package models

import (
	"fmt"
	"image"
	"image/draw"
	"log"
	"os"
	"sync"
)

type Building struct {
	Floors      []*Floor `json:"floors,omitempty"`
	Name        string   `json:"name,omitempty"`
	SIS         string   `json:"sis,omitempty"`
	Position    *LatLng  `json:"position,omitempty"`
	Address     string
	Image       string
	Description string
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
	Name     string  `json:"floor,omitempty"`
	Coords   *Coords `json:"coords,omitempty"`
	Image    string  `json:"image,omitempty"`
	Rooms    []*Room `json:"rooms,omitempty"`
	Rotation float64 `json:"rotation,omitempty"`

	RotatedImage draw.Image     `json:"-"`
	ImageWG      sync.WaitGroup `json:"-"`
	ImageOnce    sync.Once      `json:"-"`
}

func (f *Floor) LoadImage() (draw.Image, error) {
	log.Printf("Loading image: %s", f.Image)
	fImg, err := os.Open("static/" + f.Image)
	if err != nil {
		return nil, err
	}
	defer fImg.Close()
	img, _, err := image.Decode(fImg)
	if err != nil {
		return nil, err
	}
	image, err := imageToDraw(img)
	if err != nil {
		return nil, err
	}
	return image, nil
}

type Coords struct {
	North float64 `json:"north,omitempty"`
	South float64 `json:"south,omitempty"`
	East  float64 `json:"east,omitempty"`
	West  float64 `json:"west,omitempty"`
}

type ZoomableCoord struct {
	*Coords

	Zoom  int    `json:"zoom,omitempty"`
	Floor string `json:"floor,omitempty"`
}

func (c Coords) Overlap(c2 *Coords) bool {
	return c.West < c2.East && c.East > c2.West && c.North > c2.South && c.South < c2.North
}

func (c Coords) OverlapLatLng(p *LatLng) bool {
	return c.West < p.Lng && c.East > p.Lng && c.North > p.Lat && c.South < p.Lat
}

func (c Coords) DLat() float64 {
	return c.North - c.South
}
func (c Coords) DLng() float64 {
	return c.East - c.West
}

type Room struct {
	Id          string  `json:"id,omitempty"`
	SIS         string  `json:"sis,omitempty"`
	Name        string  `json:"name,omitempty"`
	Position    *LatLng `json:"position,omitempty"`
	RelPosition *LatLng `json:"rel_position,omitempty"`
	Type        string  `json:"type,omitempty"`
	Floor       string  `json:"floor,omitempty"`
}

type LatLng struct {
	Lat float64 `json:"H,omitempty"`
	Lng float64 `json:"L,omitempty"`
}

type Index struct {
	Id          string
	Name        string
	Type        string
	Image       string
	Description string

	Item interface{} `json:"-"`
}

func imageToDraw(i image.Image) (draw.Image, error) {
	switch i := i.(type) {
	case *image.Alpha:
		return draw.Image(i), nil
	case *image.Alpha16:
		return draw.Image(i), nil
	case *image.CMYK:
		return draw.Image(i), nil
	case *image.Gray:
		return draw.Image(i), nil
	case *image.Gray16:
		return draw.Image(i), nil
	case *image.NRGBA:
		return draw.Image(i), nil
	case *image.NRGBA64:
		return draw.Image(i), nil
	case *image.Paletted:
		return draw.Image(i), nil
	case *image.RGBA:
		return draw.Image(i), nil
	case *image.RGBA64:
		return draw.Image(i), nil
	default:
		return nil, fmt.Errorf("invalid image type %T", i)
	}
}
