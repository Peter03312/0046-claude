package geom

import (
	"fmt"
	"math/big"
)

// validateSimple rejects any polygon that is not a strict simple polygon:
// at least 3 vertices, no zero-length edge, no non-adjacent edge pair sharing
// any point (intersection or touching), and adjacent edges may only meet at
// their common corner.
//
// Arithmetic is exact and overflow-free (big.Int). Self-touching polygons
// (pinch points, slits) are rejected outright so each polygon has one
// well-defined interior.
func validateSimple(vs []IntPoint, label string) error {
	if len(vs) < 3 {
		return fmt.Errorf("%s: polygon needs at least 3 vertices, got %d", label, len(vs))
	}
	for i, v := range vs {
		if i > 0 && v == vs[0] {
			return fmt.Errorf("%s: vertex %d duplicates the first vertex (do not close the ring)", label, i)
		}
		if i > 0 && v == vs[i-1] {
			return fmt.Errorf("%s: consecutive vertices %d and %d coincide", label, i-1, i)
		}
	}
	n := len(vs)
	for i := 0; i < n; i++ {
		a, b := vs[i], vs[(i+1)%n]
		if a == b {
			return fmt.Errorf("%s: edge %d has zero length", label, i)
		}
		for j := i + 1; j < n; j++ {
			c, d := vs[j], vs[(j+1)%n]
			adjacent := (j == i+1) || (i == 0 && j == n-1)
			rel := intSegRelation(a, b, c, d)
			switch rel {
			case segDisjoint:
				continue
			case segShareEndpoint:
				if adjacent {
					continue
				}
				return fmt.Errorf("%s: non-adjacent edges %d and %d share an endpoint", label, i, j)
			case segNonAdjacentEndpointTouch:
				if adjacent {
					return fmt.Errorf("%s: adjacent edges %d and %d pinch at a non-corner point", label, i, j)
				}
				return fmt.Errorf("%s: edges %d and %d touch at a non-vertex point (self-intersecting input)", label, i, j)
			case segProperCross, segOverlap, segCollinearTouch:
				return fmt.Errorf("%s: edges %d and %d intersect or overlap (self-intersecting input)", label, i, j)
			}
		}
	}
	if polygonArea2(vs).Sign() == 0 {
		return fmt.Errorf("%s: polygon area is zero", label)
	}
	return nil
}

type segRelation int

const (
	segDisjoint segRelation = iota
	segShareEndpoint
	segProperCross
	segNonAdjacentEndpointTouch
	segOverlap
	segCollinearTouch
)

func delta(a, b IntPoint) (x, y *big.Int) {
	return big.NewInt(b.X - a.X), big.NewInt(b.Y - a.Y)
}

// bigCross returns (bx-ax)*(cy-ay) - (by-ay)*(cx-ax) exactly.
func bigCross(a, b, c IntPoint) *big.Int {
	bx, by := delta(a, b)
	cx, cy := delta(a, c)
	return new(big.Int).Sub(new(big.Int).Mul(bx, cy), new(big.Int).Mul(by, cx))
}

// intSegRelation classifies how two closed integer segments relate, exactly.
func intSegRelation(a, b, c, d IntPoint) segRelation {
	// den = cross(b-a, d-c)
	abx, aby := delta(a, b)
	cdx, cdy := delta(c, d)
	den := new(big.Int).Sub(new(big.Int).Mul(abx, cdy), new(big.Int).Mul(aby, cdx))
	// numT = cross(c-a, d-c); numU = cross(c-a, b-a)
	acx, acy := delta(a, c)
	numT := new(big.Int).Sub(new(big.Int).Mul(acx, cdy), new(big.Int).Mul(acy, cdx))
	numU := new(big.Int).Sub(new(big.Int).Mul(acx, aby), new(big.Int).Mul(acy, abx))

	if den.Sign() == 0 {
		if numT.Sign() != 0 || numU.Sign() != 0 {
			return segDisjoint // parallel, distinct supporting lines
		}
		// Collinear. Parameterise a + t*(b-a), evaluate c and d.
		var axis int64
		var tC, tD *big.Rat
		if abx.Sign() != 0 {
			axis = b.X - a.X
			tC = big.NewRat(c.X-a.X, axis)
			tD = big.NewRat(d.X-a.X, axis)
		} else {
			axis = b.Y - a.Y
			tC = big.NewRat(c.Y-a.Y, axis)
			tD = big.NewRat(d.Y-a.Y, axis)
		}
		lo, hi := tC, tD
		if lo.Cmp(hi) > 0 {
			lo, hi = hi, lo
		}
		zero, one := big.NewRat(0, 1), big.NewRat(1, 1)
		if hi.Cmp(zero) < 0 || lo.Cmp(one) > 0 {
			return segDisjoint
		}
		lmax := maxRat(lo, zero)
		hmin := minRat(hi, one)
		if lmax.Cmp(hmin) == 0 {
			if endpointsCoincident(a, b, c, d) {
				return segShareEndpoint
			}
			return segCollinearTouch
		}
		return segOverlap
	}

	if !ratioInUnit(numT, den) || !ratioInUnit(numU, den) {
		return segDisjoint
	}
	tEnd := ratioEndType(numT, den) // 0: t=0, 1: strictly inside, 2: t=1
	uEnd := ratioEndType(numU, den)
	if tEnd == 1 || uEnd == 1 {
		if tEnd != 1 && uEnd != 1 {
			return segNonAdjacentEndpointTouch
		}
		return segProperCross
	}
	return segShareEndpoint
}

func endpointsCoincident(a, b, c, d IntPoint) bool {
	return a == c || a == d || b == c || b == d
}

// ratioInUnit reports whether n/d ∈ [0,1] exactly.
func ratioInUnit(n, d *big.Int) bool {
	zero := big.NewInt(0)
	if d.Sign() > 0 {
		return n.Cmp(zero) >= 0 && n.Cmp(d) <= 0
	}
	return n.Cmp(zero) <= 0 && n.Cmp(d) >= 0
}

// ratioEndType classifies n/d as 0 (==0), 1 (strictly inside), 2 (==1).
func ratioEndType(n, d *big.Int) int {
	q := new(big.Rat).SetFrac(n, d)
	if q.Sign() == 0 {
		return 0
	}
	if q.Cmp(big.NewRat(1, 1)) == 0 {
		return 2
	}
	return 1
}

func maxRat(a, b *big.Rat) *big.Rat {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}

func minRat(a, b *big.Rat) *big.Rat {
	if a.Cmp(b) <= 0 {
		return a
	}
	return b
}

// polygonArea2 returns twice the signed area; sign encodes orientation.
func polygonArea2(vs []IntPoint) *big.Int {
	s := big.NewInt(0)
	n := len(vs)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		x1, y1 := big.NewInt(vs[i].X), big.NewInt(vs[i].Y)
		x2, y2 := big.NewInt(vs[j].X), big.NewInt(vs[j].Y)
		s.Add(s, new(big.Int).Sub(new(big.Int).Mul(x1, y2), new(big.Int).Mul(x2, y1)))
	}
	return s
}
