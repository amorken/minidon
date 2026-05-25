package api

import (
	"fmt"
	"net/http"
	"strconv"
)

// queryInt parses a bounded integer query parameter with standard boundaries and defaults.
func queryInt(r *http.Request, key string, defaultVal, min, max int) (int, error) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < min {
		return 0, fmt.Errorf("invalid %s parameter", key)
	}
	if v > max {
		v = max
	}
	return v, nil
}
