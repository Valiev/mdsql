package cache

import (
	"errors"
	"os"
	"strings"
)

// Stats reports the number and on-disk bytes of cache entries, excluding temps.
type Stats struct {
	Entries int64
	Bytes   int64
}

// StatsDefault inspects the private cache without creating it or reading payloads.
func StatsDefault() (Stats, error) {
	var stats Stats
	dir, err := defaultDir()
	if err != nil {
		return stats, err
	}
	c := &Cache{dir: dir}
	if err := c.checkDir(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return stats, nil
		}
		return stats, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return stats, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".gob") || !cacheName(e.Name()) || !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return stats, err
		}
		if !privateFile(fi) {
			continue
		}
		stats.Entries++
		stats.Bytes += fi.Size()
	}
	return stats, nil
}
