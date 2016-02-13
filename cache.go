package main

import (
	"github.com/alecthomas/units"
	"github.com/golang/groupcache"
)

func (s *Server) initCache() {
	s.zoomedFloorCache = groupcache.GetGroup("zoomedImages")
	if s.zoomedFloorCache == nil {
		g := zoomedImageGetter{s}
		s.zoomedFloorCache = groupcache.NewGroup("zoomedImages", int64(500*units.MiB), g)
	}

	s.tileCache = groupcache.GetGroup("mapTiles")
	if s.tileCache == nil {
		g := mapTileGetter{s}
		s.tileCache = groupcache.NewGroup("mapTiles", int64(500*units.MiB), g)
	}
}
