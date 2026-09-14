package geom

import (
	"math/big"
	"sort"
)

// cycle is one directed boundary loop extracted from the arrangement. Positive
// signed area means counter-clockwise (an outer boundary of a bounded face or
// the outer boundary of an island region); negative means clockwise (either
// the boundary of the unbounded face or a hole of a bounded face).
type cycle struct {
	verts []RatPoint
	area2 *big.Rat
	rep   RatPoint // a point strictly on the cycle's left (inside) side
}

// extractCycles traces every directed atomic edge exactly once. At each node
// we take the outgoing edge immediately clockwise of the reversed incoming
// ray (index +1 in CCW order), which keeps the face interior on the left.
func (ar *arrangement) extractCycles() []cycle {
	type dirEdge struct{ from, to string }
	used := map[dirEdge]bool{}
	var cycles []cycle
	for start := range ar.adj {
		for idx := range ar.adj[start] {
			de := dirEdge{start, ar.adj[start][idx].to}
			if used[de] {
				continue
			}
			var verts []RatPoint
			cur, edgeIdx := start, idx
			for {
				ns := ar.adj[cur]
				n := ns[edgeIdx]
				used[dirEdge{cur, n.to}] = true
				verts = append(verts, ar.points[cur])
				// Reverse ray from n back to cur; find its position among n's
				// CCW-ordered outgoing rays, then take its successor.
				back := neighbor{
					dx: new(big.Rat).Neg(n.dx), dy: new(big.Rat).Neg(n.dy),
				}
				at := sort.Search(len(ar.adj[n.to]), func(i int) bool {
					return !angleLess(ar.adj[n.to][i], back)
				})
				// back itself exists; its immediate CCW predecessor keeps the
				// traced face on the left (standard planar-face walk).
				nextIdx := (at - 1 + len(ar.adj[n.to])) % len(ar.adj[n.to])
				cur = n.to
				edgeIdx = nextIdx
				if cur == start && edgeIdx == idx {
					break
				}
			}
			cyc := cycle{verts: verts, area2: signedArea2(verts)}
			cyc.rep = ar.interiorPoint(cyc)
			cycles = append(cycles, cyc)
		}
	}
	return cycles
}

// signedArea2 returns twice the signed area of a rational ring.
func signedArea2(vs []RatPoint) *big.Rat {
	s := big.NewRat(0, 1)
	n := len(vs)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		s.Add(s, new(big.Rat).Sub(
			new(big.Rat).Mul(vs[i].X, vs[j].Y),
			new(big.Rat).Mul(vs[j].X, vs[i].Y),
		))
	}
	return s
}

// interiorPoint returns a point guaranteed to lie on the left side of the
// directed cycle and on no arrangement edge. For an edge p->q the midpoint
// shifted along its left normal by 1/(2·5^k) of the edge vector stays inside
// the wedge the edge bounds; shrinking k makes the offset smaller than the
// minimum clearance to every unrelated edge, so a valid k always exists.
// The construction works identically for CCW and CW cycles: the face that a
// directed cycle keeps on its left is the one we need.
func (ar *arrangement) interiorPoint(c cycle) RatPoint {
	for i := 0; i < len(c.verts); i++ {
		p := c.verts[i]
		q := c.verts[(i+1)%len(c.verts)]
		dx := new(big.Rat).Sub(q.X, p.X)
		dy := new(big.Rat).Sub(q.Y, p.Y)
		if dx.Sign() == 0 && dy.Sign() == 0 {
			continue
		}
		mx := new(big.Rat).Mul(big.NewRat(1, 2), new(big.Rat).Add(p.X, q.X))
		my := new(big.Rat).Mul(big.NewRat(1, 2), new(big.Rat).Add(p.Y, q.Y))
		leftX := new(big.Rat).Neg(dy) // left normal = (-dy, dx)
		leftY := dx
		for k := int64(1); k <= 60; k++ {
			den := new(big.Int).Mul(big.NewInt(2), new(big.Int).Exp(big.NewInt(5), big.NewInt(k), nil))
			ox := new(big.Rat).Quo(leftX, new(big.Rat).SetInt(den))
			oy := new(big.Rat).Quo(leftY, new(big.Rat).SetInt(den))
			pt := RatPoint{X: new(big.Rat).Add(mx, ox), Y: new(big.Rat).Add(my, oy)}
			if ar.pointOnAnyEdge(pt) {
				continue
			}
			// Confirm the point is strictly on the edge's left and not
			// beyond the edge's perpendicular slab; the normal offset makes
			// the slab check automatic, but enforce the side exactly.
			wx := new(big.Rat).Sub(pt.X, p.X)
			wy := new(big.Rat).Sub(pt.Y, p.Y)
			cr := new(big.Rat).Sub(new(big.Rat).Mul(dx, wy), new(big.Rat).Mul(dy, wx))
			if cr.Sign() <= 0 {
				continue
			}
			return pt
		}
	}
	// Unreachable for a well-formed planar subdivision; panic rather than
	// silently produce an unsound proof.
	panic("geom: could not construct a strictly interior point")
}

func (ar *arrangement) pointOnAnyEdge(pt RatPoint) bool {
	for _, s := range ar.segs {
		if pointOnSegment(pt, s.a, s.b) {
			return true
		}
	}
	return false
}

// pointOnSegment tests exact collinearity and bounding-box containment.
func pointOnSegment(p, a, b RatPoint) bool {
	cx := new(big.Rat).Sub(p.X, a.X)
	cy := new(big.Rat).Sub(p.Y, a.Y)
	dx := new(big.Rat).Sub(b.X, a.X)
	dy := new(big.Rat).Sub(b.Y, a.Y)
	cr := new(big.Rat).Sub(new(big.Rat).Mul(cx, dy), new(big.Rat).Mul(cy, dx))
	if cr.Sign() != 0 {
		return false
	}
	return between(p.X, a.X, b.X) && between(p.Y, a.Y, b.Y)
}

func between(v, lo, hi *big.Rat) bool {
	if lo.Cmp(hi) > 0 {
		lo, hi = hi, lo
	}
	return v.Cmp(lo) >= 0 && v.Cmp(hi) <= 0
}

// pointInRing is the parity (even-odd) test, orientation independent; it
// never counts a point on the boundary because callers guarantee that.
func pointInRing(p RatPoint, ring []RatPoint) bool {
	inside := false
	n := len(ring)
	for i := 0; i < n; i++ {
		a, b := ring[i], ring[(i+1)%n]
		if (a.Y.Cmp(p.Y) > 0) != (b.Y.Cmp(p.Y) > 0) {
			// xIntersect = a.x + (b.x-a.x)*(p.y-a.y)/(b.y-a.y)
			t := new(big.Rat).Quo(new(big.Rat).Sub(p.Y, a.Y), new(big.Rat).Sub(b.Y, a.Y))
			xi := new(big.Rat).Add(a.X, new(big.Rat).Mul(t, new(big.Rat).Sub(b.X, a.X)))
			if p.X.Cmp(xi) < 0 {
				inside = !inside
			}
		}
	}
	return inside
}

// face pairs one CCW outer ring with its directly-contained CW hole rings.
type face struct {
	outer cycle
	holes []cycle
}

// buildFaces groups extracted cycles into bounded faces. CW cycles whose
// representative point lies inside no CCW cycle belong to the unbounded face
// and are dropped (they carry the uncovered white background).
func buildFaces(cycles []cycle) []face {
	var outers, holes []cycle
	for _, c := range cycles {
		if c.area2.Sign() > 0 {
			outers = append(outers, c)
		} else {
			holes = append(holes, c)
		}
	}
	// Sort outers by area ascending so "innermost containing" is found by
	// scanning small to large and keeping the last containing cycle.
	sortedOuters := make([]cycle, len(outers))
	copy(sortedOuters, outers)
	sort.Slice(sortedOuters, func(i, j int) bool {
		return sortedOuters[i].area2.Cmp(sortedOuters[j].area2) < 0
	})
	facesByOuter := map[int][]cycle{}
	for _, h := range holes {
		// The hole belongs to the innermost (smallest-area) CCW cycle that
		// contains the face on the hole's left; scanning ascending and
		// stopping at the first match yields exactly that.
		innermost := -1
		for oi, o := range sortedOuters {
			if pointInRing(h.rep, o.verts) {
				innermost = oi
				break
			}
		}
		if innermost == -1 {
			continue // bounds the unbounded face: white background, discarded
		}
		facesByOuter[innermost] = append(facesByOuter[innermost], h)
	}
	var faces []face
	for oi, o := range sortedOuters {
		faces = append(faces, face{outer: o, holes: facesByOuter[oi]})
	}
	return faces
}
