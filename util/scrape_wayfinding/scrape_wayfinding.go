package main

import (
	"flag"
	"log"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/PuerkitoBio/goquery"
	"github.com/d4l3k/campus/models"

	"googlemaps.github.io/maps"
)

var (
	apiKey  = flag.String("key", "", "the google maps api key for geocoding")
	scrape  = flag.Bool("scrape", false, "whether to scrape or not")
	geocode = flag.Bool("geocode", false, "whether to geocode or not")

	customSIS = map[string]string{
		"Wayne and William White Engineering Design Centre": "EDC",
	}
)

func fetchDetails(rel <-chan string, out chan *models.Building) {
	for rel := range rel {
		href := "http://www.maps.ubc.ca/PROD/" + rel + "&show=n,n,n,y,n,n"
		log.Printf("Fetching: %s", href)
		doc, err := goquery.NewDocument(href)
		if err != nil {
			log.Println(err)
			continue
		}
		name := strings.TrimSpace(doc.Find("#divSiteContent h1").Nodes[0].FirstChild.Data)
		addr := strings.TrimSpace(doc.Find("#divSiteContent h2").First().Text())
		sisBits := strings.Split(doc.Find("#divSiteContent blockquote").Text(), "(SIS) Name:")
		sis := ""
		if len(sisBits) > 1 {
			sis = strings.TrimSpace(sisBits[1])
		}
		img := ""
		doc.Find("#divSiteContent img").Each(func(i int, s *goquery.Selection) {
			src := s.AttrOr("src", "")
			if strings.HasPrefix(src, "images/photos/") {
				img = "http://www.maps.ubc.ca/PROD/" + src
			}
		})
		desc := strings.TrimSpace(doc.Find(".showDetailTD01").Text())
		log.Printf("Name: %s, Addr: %s, SIS: %s", name, addr, sis)
		out <- &models.Building{
			Name:        name,
			Address:     addr,
			SIS:         sis,
			Image:       img,
			Description: desc,
		}
	}
}

func scrapeBuildings() error {
	buildings, err := models.LoadMapData()
	if err != nil {
		return err
	}
	doc, err := goquery.NewDocument("http://www.maps.ubc.ca/PROD/buildingsListAll.php")
	if err != nil {
		return err
	}

	detailsDup := make(map[string]bool)
	var details []string
	doc.Find("a[href]").Each(func(i int, sel *goquery.Selection) {
		href, exists := sel.Attr("href")
		if !exists || !strings.HasPrefix(href, "index_detail.php?") || detailsDup[href] {
			return
		}
		detailsDup[href] = true
		details = append(details, href)
	})
	relChan := make(chan string, 1)
	defer close(relChan)
	outChan := make(chan *models.Building, 1)
	defer close(outChan)
	for _ = range make([]interface{}, 20) {
		go fetchDetails(relChan, outChan)
	}
	go func() {
		for _, rel := range details {
			relChan <- rel
		}
	}()
	var scrapedBuildings []*models.Building
	for b := range outChan {
		if len(b.SIS) == 0 {
			b.SIS = customSIS[b.Name]
		}
		scrapedBuildings = append(scrapedBuildings, b)
		if len(scrapedBuildings) == len(details) {
			break
		}
	}
	buildingIndex := make(map[string]*models.Building)
	for _, building := range buildings {
		buildingIndex[building.SIS] = building
	}
	for _, b := range scrapedBuildings {
		if len(b.SIS) == 0 {
			continue
		}
		if b2, ok := buildingIndex[b.SIS]; ok {
			b2.Address = b.Address
			b2.Image = b.Image
			b2.Description = b.Description
			continue
		}
		buildingIndex[b.SIS] = b
		buildings = append(buildings, b)
	}
	return models.SaveMapData(buildings)
}

func geocodeBuildings(c *maps.Client) error {
	buildings, err := models.LoadMapData()
	if err != nil {
		return err
	}
	for _, b := range buildings {
		if b.Position != nil || len(b.Address) == 0 {
			continue
		}
		log.Printf("geocoding %s", b.Name)
		req := &maps.GeocodingRequest{
			Address: b.Address,
			Region:  "ca",
		}
		result, err := c.Geocode(context.TODO(), req)
		if err != nil {
			return err
		}
		if len(result) == 0 {
			continue
		}
		loc := result[0].Geometry.Location
		b.Position = &models.LatLng{
			Lat: loc.Lat,
			Lng: loc.Lng,
		}
		time.Sleep(100 * time.Millisecond)
		if err := models.SaveMapData(buildings); err != nil {
			return err
		}
	}
	for _, b := range buildings {
		if len(b.Floors) == 0 {
			continue
		}
		var lat, lng float64
		c := float64(len(b.Floors) * 2)
		for _, f := range b.Floors {
			lat += f.Coords.North
			lat += f.Coords.South
			lng += f.Coords.West
			lng += f.Coords.East
			for _, r := range f.Rooms {
				lat += r.Position.Lat
				lng += r.Position.Lng
				c += 1
			}
		}
		b.Position.Lat = lat / c
		b.Position.Lng = lng / c
	}
	return models.SaveMapData(buildings)
}

func main() {
	flag.Parse()
	if len(*apiKey) == 0 {
		log.Fatal("-key is required")
	}
	c, err := maps.NewClient(maps.WithAPIKey(*apiKey))
	if err != nil {
		log.Fatalf("fatal error: %s", err)
	}

	if *scrape {
		if err := scrapeBuildings(); err != nil {
			log.Fatal(err)
		}
	}
	if *geocode {
		if err := geocodeBuildings(c); err != nil {
			log.Fatal(err)
		}
	}
}
