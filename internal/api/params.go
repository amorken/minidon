package api

import (
	"fmt"
	"net/http"
	"strconv"
)

type intParam struct {
	defaultVal int
	minVal     int
	maxVal     int
}

// queryInt parses a bounded integer query parameter with standard boundaries and defaults.
func queryInt(r *http.Request, key string, bounds intParam) (int, error) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return bounds.defaultVal, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < bounds.minVal {
		return 0, fmt.Errorf("invalid %q parameter", key)
	}
	if v > bounds.maxVal {
		v = bounds.maxVal
	}
	return v, nil
}
