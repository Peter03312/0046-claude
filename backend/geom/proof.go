package geom

import (
	"fmt"
	"math/big"
	"sort"
)

// ActInput is one scene ("act") after all per-sheet transformations have
// been applied: vertices here are already rotated and translated and stay
// integral. sheets are listed strictly bottom-to-top.
type ActInput struct {
	Name    string
	Sheets  []Sheet
	Regions []Region
}

// Evaluate checks a single act by exact planar subdivision. It never samples
// pixels: every positive-area cell of the arrangement induced by the
// transformed sheet boundaries and the region boundaries is classified.
//
// Rules:
//   - Every positive-area cell inside a blank region must not be inside any
//     sheet; shared edges or points produce zero-area contact and are legal.
//   - Every positive-area cell inside a target region must composite, over a
//     white background with source-over, to RGB values whose final
//     half-up-rounded channels each differ from the target by ≤ tolerance.
//
// All violations of the act are returned (deterministic order); the API layer
// stops at the earliest failing act.
func Evaluate(act ActInput) ([]BadCell, error) {
	for i := range act.Sheets {
		s := &act.Sheets[i]
		label := "sheet " + strung(s.ID, s.Name, i)
		if err := validateSimple(s.Vertices, label); err != nil {
			return nil, err
		}
		if err := validateColor(s.R, s.G, s.B); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		if s.OpacityMillis < 0 || s.OpacityMillis > 1000 {
			return nil, fmt.Errorf("%s: opacityMillis must be 0..1000", label)
		}
	}
	for i := range act.Regions {
		r := &act.Regions[i]
		label := "region " + strung(r.ID, r.Name, i)
		if err := validateSimple(r.Vertices, label); err != nil {
			return nil, err
		}
		switch r.Kind {
		case "target":
			if err := validateColor(r.R, r.G, r.B); err != nil {
				return nil, fmt.Errorf("%s: %w", label, err)
			}
			if r.Tolerance < 0 || r.Tolerance > 255 {
				return nil, fmt.Errorf("%s: tolerance must be 0..255", label)
			}
		case "blank":
		default:
			return nil, fmt.Errorf("%s: region kind must be target or blank", label)
		}
	}

	// Gather every boundary edge. Polygon indices 0..S-1 are sheets, the rest
	// are regions; coincident edges merge and union their owner sets.
	var segs []rSeg
	var owners [][]int
	addPoly := func(vs []IntPoint, owner int) {
		for i := 0; i < len(vs); i++ {
			a := vs[i]
			b := vs[(i+1)%len(vs)]
			segs = append(segs, rSeg{
				a: RatPoint{X: ratInt(a.X), Y: ratInt(a.Y)},
				b: RatPoint{X: ratInt(b.X), Y: ratInt(b.Y)},
			})
			owners = append(owners, []int{owner})
		}
	}
	for i := range act.Sheets {
		addPoly(act.Sheets[i].Vertices, i)
	}
	regionBase := len(act.Sheets)
	for i := range act.Regions {
		addPoly(act.Regions[i].Vertices, regionBase+i)
	}
	// Duplicate identical edges (e.g. a sheet edge shared by two sheets) are
	// merged below anyway; buildArrangement treats owner lists per edge but
	// exact duplicates without intersection still emit parallel records keyed
	// by the same endpoint pair, which merge in edgeMap.
	ar, err := buildArrangement(segs, owners)
	if err != nil {
		return nil, err
	}
	cycles := ar.extractCycles()
	faces := buildFaces(cycles)

	// Rational polygon copies for parity tests.
	sheetRings := make([][]RatPoint, len(act.Sheets))
	for i := range act.Sheets {
		for _, v := range act.Sheets[i].Vertices {
			sheetRings[i] = append(sheetRings[i], RatPoint{X: ratInt(v.X), Y: ratInt(v.Y)})
		}
	}
	regionRings := make([][]RatPoint, len(act.Regions))
	for i := range act.Regions {
		for _, v := range act.Regions[i].Vertices {
			regionRings[i] = append(regionRings[i], RatPoint{X: ratInt(v.X), Y: ratInt(v.Y)})
		}
	}

	// Exact positive-area overlap test via the subdivision itself: two
	// distinct regions overlap with positive area iff some arrangement face
	// lies strictly inside both (the representative point is guaranteed to be
	// off every boundary, so shared edges and shared points never count).
	// This catches containment, identical boundaries and edge-aligned partial
	// overlaps — configurations a vertex/edge-classification heuristic misses,
	// and which would otherwise make a cell obey two possibly contradictory
	// target colours with an order-dependent outcome.
	overlapA, overlapB := -1, -1
outer:
	for _, f := range faces {
		var inside []int
		for ri, ring := range regionRings {
			if pointInRing(f.outer.rep, ring) {
				inside = append(inside, ri)
				if len(inside) == 2 {
					overlapA, overlapB = inside[0], inside[1]
					break outer
				}
			}
		}
	}
	if overlapA >= 0 {
		return nil, fmt.Errorf("regions %s and %s overlap with positive area",
			strung(act.Regions[overlapA].ID, act.Regions[overlapA].Name, overlapA),
			strung(act.Regions[overlapB].ID, act.Regions[overlapB].Name, overlapB))
	}

	var bad []BadCell
	for _, f := range faces {
		probe := f.outer.rep
		var covering []int // sheet indices covering the cell, bottom to top
		for si, ring := range sheetRings {
			if pointInRing(probe, ring) {
				covering = append(covering, si)
			}
		}
		var containingRegions []int
		for ri, ring := range regionRings {
			if pointInRing(probe, ring) {
				containingRegions = append(containingRegions, ri)
			}
		}
		if len(containingRegions) == 0 {
			continue
		}

		outer := normalizeRing(f.outer.verts, true)
		holes := make([]Ring, 0, len(f.holes))
		for _, h := range f.holes {
			holes = append(holes, normalizeRing(h.verts, false))
		}

		actual := composite(act.Sheets, covering)

		// Blank regions win: any positive-area coverage of a blank cell is a
		// leak regardless of the colour it produces.
		blankIdx := -1
		targetIdx := -1
		for _, ri := range containingRegions {
			switch act.Regions[ri].Kind {
			case "blank":
				if blankIdx == -1 {
					bl := ri
					blankIdx = bl
				}
			case "target":
				if targetIdx == -1 {
					targetIdx = ri
				}
			}
		}
		if blankIdx != -1 && len(covering) > 0 {
			rg := act.Regions[blankIdx]
			bad = append(bad, BadCell{
				Kind:         "leak",
				RegionID:     rg.ID,
				RegionName:   rg.Name,
				Outer:        outer,
				Holes:        holes,
				Actual:       actual,
				Contributors: contributors(act.Sheets, covering),
			})
			continue
		}
		if targetIdx != -1 {
			rg := act.Regions[targetIdx]
			expected := [3]int{rg.R, rg.G, rg.B}
			if abs(actual[0]-rg.R) > rg.Tolerance ||
				abs(actual[1]-rg.G) > rg.Tolerance ||
				abs(actual[2]-rg.B) > rg.Tolerance {
				bad = append(bad, BadCell{
					Kind:         "color",
					RegionID:     rg.ID,
					RegionName:   rg.Name,
					Outer:        outer,
					Holes:        holes,
					Actual:       actual,
					Expected:     expected,
					Tolerance:    rg.Tolerance,
					Contributors: contributors(act.Sheets, covering),
				})
			}
		}
	}

	sort.SliceStable(bad, func(i, j int) bool {
		return ringSortKey(bad[i].Outer) < ringSortKey(bad[j].Outer)
	})
	return bad, nil
}

func strung(id, name string, i int) string {
	if id != "" {
		return id
	}
	if name != "" {
		return name
	}
	return fmt.Sprintf("#%d", i)
}

func validateColor(r, g, b int) error {
	for name, v := range map[string]int{"r": r, "g": g, "b": b} {
		if v < 0 || v > 255 {
			return fmt.Errorf("%s channel must be 0..255", name)
		}
	}
	return nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// composite performs source-over on a white background in exact rationals.
// Channels stay rational through every layer; only the final result is
// rounded half-up to an 8-bit integer. There is deliberately no intermediate
// rounding.
func composite(sheets []Sheet, covering []int) [3]int {
	mil := big.NewRat(1000, 1)
	var cur [3]*big.Rat
	for c := 0; c < 3; c++ {
		cur[c] = big.NewRat(255, 1)
	}
	for _, si := range covering {
		s := sheets[si]
		if s.OpacityMillis == 0 {
			continue
		}
		a := new(big.Rat).Quo(big.NewRat(int64(s.OpacityMillis), 1), mil)
		oneMinus := new(big.Rat).Sub(big.NewRat(1, 1), a)
		src := [3]int64{int64(s.R), int64(s.G), int64(s.B)}
		for c := 0; c < 3; c++ {
			term := new(big.Rat).Mul(a, big.NewRat(src[c], 1))
			cur[c] = new(big.Rat).Add(term, new(big.Rat).Mul(oneMinus, cur[c]))
		}
	}
	var out [3]int
	for c := 0; c < 3; c++ {
		out[c] = halfUp(cur[c])
	}
	return out
}

// halfUp rounds a non-negative rational to the nearest integer, halves going
// up: floor(q + 1/2).
func halfUp(q *big.Rat) int {
	s := new(big.Rat).Add(q, big.NewRat(1, 2))
	v := new(big.Int).Quo(s.Num(), s.Denom()) // s ≥ 0: truncation == floor
	return int(v.Int64())
}

func contributors(sheets []Sheet, covering []int) []Contributor {
	out := make([]Contributor, 0, len(covering))
	for order, si := range covering {
		s := sheets[si]
		out = append(out, Contributor{
			ID: s.ID, Name: s.Name, Order: order,
			R: s.R, G: s.G, B: s.B, OpacityMillis: s.OpacityMillis,
		})
	}
	return out
}

// normalizeRing rotates a ring so it starts at its lexicographically smallest
// vertex. wantCCW=true reverses a clockwise ring and vice versa as needed.
func normalizeRing(in []RatPoint, wantCCW bool) Ring {
	vs := make([]RatPoint, len(in))
	copy(vs, in)
	ccw := signedArea2(vs).Sign() > 0
	if ccw != wantCCW {
		for i, j := 0, len(vs)-1; i < j; i, j = i+1, j-1 {
			vs[i], vs[j] = vs[j], vs[i]
		}
	}
	min := 0
	for i := 1; i < len(vs); i++ {
		if lexLess(vs[i], vs[min]) {
			min = i
		}
	}
	out := make(Ring, len(vs))
	for i := range vs {
		out[i] = vs[(min+i)%len(vs)]
	}
	return out
}

func ringSortKey(r Ring) string {
	if len(r) == 0 {
		return ""
	}
	return r[0].key()
}
