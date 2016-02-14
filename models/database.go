package models

import (
	"encoding/json"
	"io/ioutil"
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
	return buildings, nil
}

func SaveMapData(buildings []*Building) error {
	buf, err := json.Marshal(buildings)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(MapDataPath, buf, 0755)
}
