package main

import (
	"github.com/alecthomas/units"
	"github.com/golang/groupcache"
)

func (s *Server) initCache() {
	s.zoomedFloorCache = groupcache.GetGroup("zoomedImages")
	if s.zoomedFloorCache == nil {
		g := zoomedImageGetter{s}
		s.zoomedFloorCache = groupcache.NewGroup("zoomedImages", int64(256*units.MiB), g)
	}

}
