// Package geom implements an exact, sampling-free planar proof engine.
//
// All input coordinates are integers. Rotations are 90-degree multiples
// around the origin followed by integer translations, so every transformed
// vertex remains an integer pair. Subdivision intersections are computed as
// arbitrary-precision rationals (math/big), never as floats, which guarantees
// that shared edges and single points cannot be mistaken for coloured leaks.
package geom

import (
	"encoding/json"
	"fmt"
	"math/big"
)

// RatPoint is a point with exact rational coordinates.
type RatPoint struct {
	X *big.Rat
	Y *big.Rat
}

// ratJSON is the on-wire form: exact fractions such as "3/2" (integers as
// "5"), never lossy decimals.
type ratJSON struct {
	X string `json:"x"`
	Y string `json:"y"`
}

// MarshalJSON encodes both coordinates as exact fraction strings.
func (p RatPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(ratJSON{X: p.X.RatString(), Y: p.Y.RatString()})
}

// UnmarshalJSON parses exact fraction strings back into big.Rat.
func (p *RatPoint) UnmarshalJSON(data []byte) error {
	var j ratJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	x, err := parseRat(j.X)
	if err != nil {
		return err
	}
	y, err := parseRat(j.Y)
	if err != nil {
		return err
	}
	p.X, p.Y = x, y
	return nil
}

func parseRat(s string) (*big.Rat, error) {
	if s == "" {
		return nil, fmt.Errorf("empty rational coordinate")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid rational coordinate %q", s)
	}
	return r, nil
}

// IntPoint is an integer coordinate pair.
type IntPoint struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

// Sheet is one coloured transparent piece after transformation.
type Sheet struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Vertices      []IntPoint `json:"vertices"`
	R             int        `json:"r"`
	G             int        `json:"g"`
	B             int        `json:"b"`
	OpacityMillis int        `json:"opacityMillis"` // 0..1000, integer thousandths
}

// Region is a target (expected colour) or blank (must stay empty) region.
type Region struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"` // "target" or "blank"
	Vertices  []IntPoint `json:"vertices"`
	R         int        `json:"r"`
	G         int        `json:"g"`
	B         int        `json:"b"`
	Tolerance int        `json:"tolerance"` // target only, per channel
}

// Contributor names one covering piece and its source parameters.
type Contributor struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Order         int    `json:"order"` // 0-based, bottom to top
	R             int    `json:"r"`
	G             int    `json:"g"`
	B             int    `json:"b"`
	OpacityMillis int    `json:"opacityMillis"`
}

// Ring is an ordered list of exact rational points.
type Ring []RatPoint

// BadCell is one positive-area cell of the earliest failing act that
// violates a rule: a colour mismatch in a target region ("color") or a
// positive-area overlap between a sheet and a blank region ("leak").
type BadCell struct {
	Kind         string        `json:"kind"`
	RegionID     string        `json:"regionId"`
	RegionName   string        `json:"regionName"`
	Outer        Ring          `json:"outer"` // counter-clockwise
	Holes        []Ring        `json:"holes"` // each clockwise
	Actual       [3]int        `json:"actual"`
	Expected     [3]int        `json:"expected"`
	Tolerance    int           `json:"tolerance"`
	Contributors []Contributor `json:"contributors"` // bottom to top
}
