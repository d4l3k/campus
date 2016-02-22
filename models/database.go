package models

import (
	"encoding/json"
	"io/ioutil"
	"sort"
)

var MapDataPath = "./static/maps/map.json"

func LoadMapData() ([]*Building, error) {
	buf, err := ioutil.ReadFile(MapDataPath)
	if err != nil {
		return nil, err
	}
	var buildings []*Building
	if err := json.Unmarshal(buf, &buildings); err != nil {
		return nil, err
	}
	sort.Sort(BySIS(buildings))
	return buildings, nil
}

func SaveMapData(buildings []*Building) error {
	buf, err := json.MarshalIndent(buildings, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(MapDataPath, buf, 0755)
}

type BySIS []*Building

func (a BySIS) Len() int           { return len(a) }
func (a BySIS) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a BySIS) Less(i, j int) bool { return a[i].SIS < a[j].SIS }
