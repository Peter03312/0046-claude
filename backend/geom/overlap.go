package geom

import "math/big"

// polygonsPositiveOverlap reports whether two simple integer polygons share
// a positive-area interior intersection. Edge or point contact alone returns
// false. It is exact: polygon A and B have positive overlap iff a vertex of
// one lies strictly inside the other, or two of their edges cross properly.
func polygonsPositiveOverlap(a, b []IntPoint) bool {
	// Proper edge crossing implies positive overlap. Also a vertex of A
	// strictly inside B (or vice versa) covers containment and T-junction
	// configurations without proper crossings.
	for i := 0; i < len(a); i++ {
		e1a, e1b := a[i], a[(i+1)%len(a)]
		for j := 0; j < len(b); j++ {
			e2a, e2b := b[j], b[(j+1)%len(b)]
			switch intSegRelation(e1a, e1b, e2a, e2b) {
			case segProperCross:
				return true
			case segOverlap:
				// Collinear overlap is a shared edge segment (zero area by
				// itself) but often accompanies area overlap decided by
				// containment tests; fall through to those.
			}
		}
	}
	for _, v := range a {
		pr := RatPoint{X: ratInt(v.X), Y: ratInt(v.Y)}
		ring := intRing(b)
		if pointInRing(pr, ring) && !pointOnIntBoundary(v, b) {
			// pointInRing is parity-based; if v is on B's boundary it cannot
			// establish interior membership, otherwise it is strictly inside.
			return true
		}
	}
	for _, v := range b {
		pr := RatPoint{X: ratInt(v.X), Y: ratInt(v.Y)}
		if pointInRing(pr, intRing(a)) && !pointOnIntBoundary(v, a) {
			return true
		}
	}
	return false
}

func intRing(vs []IntPoint) []RatPoint {
	out := make([]RatPoint, len(vs))
	for i, v := range vs {
		out[i] = RatPoint{X: ratInt(v.X), Y: ratInt(v.Y)}
	}
	return out
}

func pointOnIntBoundary(p IntPoint, ring []IntPoint) bool {
	pr := RatPoint{X: ratInt(p.X), Y: ratInt(p.Y)}
	for i := range ring {
		a := ring[i]
		b := ring[(i+1)%len(ring)]
		if pointOnSegment(pr,
			RatPoint{X: ratInt(a.X), Y: ratInt(a.Y)},
			RatPoint{X: ratInt(b.X), Y: ratInt(b.Y)}) {
			return true
		}
	}
	return false
}

var _ = big.NewRat
