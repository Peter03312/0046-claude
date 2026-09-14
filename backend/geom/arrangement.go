package geom

import (
	"math/big"
	"sort"
)

// rSeg is an input edge of the planar arrangement with rational endpoints
// (for transformed sheets/regions every endpoint is actually an integer).
type rSeg struct {
	a, b RatPoint
}

// atomicSeg is a sub-segment between two arrangement nodes that contains no
// node in its relative interior. owners indexes the original polygons whose
// boundary contains this atomic edge.
type atomicSeg struct {
	a, b   RatPoint
	owners []int
}

type neighbor struct {
	to     string
	point  RatPoint
	dx, dy *big.Rat
	owners []int
}

// arrangement is the full node-edge incidence structure of all polygon
// boundaries after exact splitting.
type arrangement struct {
	points map[string]RatPoint
	// adjacency: node key -> neighbors in CCW angular order
	adj  map[string][]neighbor
	segs []atomicSeg
}

// buildArrangement splits every input edge at all exact intersection points
// and builds the ordered incidence graph. Collinear overlaps are merged and
// their owner sets unioned.
func buildArrangement(segs []rSeg, owners [][]int) (*arrangement, error) {
	// perSeg[i] holds the parameters t ∈ [0,1] where segment i must be cut.
	cuts := make(map[int][]*big.Rat, len(segs))
	for i := range segs {
		cuts[i] = []*big.Rat{big.NewRat(0, 1), big.NewRat(1, 1)}
	}
	cutAt := func(i int, t *big.Rat) {
		for _, u := range cuts[i] {
			if u.Cmp(t) == 0 {
				return
			}
		}
		cuts[i] = append(cuts[i], new(big.Rat).Set(t))
	}
	for i := 0; i < len(segs); i++ {
		for j := i + 1; j < len(segs); j++ {
			ts, us, ok := ratSegIntersections(segs[i].a, segs[i].b, segs[j].a, segs[j].b)
			if !ok {
				continue
			}
			for _, t := range ts {
				cutAt(i, t)
			}
			for _, u := range us {
				cutAt(j, u)
			}
		}
	}

	// Emit atomic edges, keyed by unordered endpoint pair so collinear
	// coincident pieces merge and owner sets combine.
	type edgeRec struct {
		a, b   string
		pa, pb RatPoint
		owners map[int]struct{}
	}
	edgeMap := map[string]*edgeRec{}
	points := map[string]RatPoint{}
	addPoint := func(p RatPoint) string {
		k := p.key()
		points[k] = p
		return k
	}
	for i := range segs {
		ts := cuts[i]
		sort.Slice(ts, func(a, b int) bool { return ts[a].Cmp(ts[b]) < 0 })
		for k := 0; k+1 < len(ts); k++ {
			p1 := pointAt(segs[i].a, segs[i].b, ts[k])
			p2 := pointAt(segs[i].a, segs[i].b, ts[k+1])
			if pointsEqual(p1, p2) {
				continue
			}
			k1, k2 := addPoint(p1), addPoint(p2)
			lo, hi := k1, k2
			if lo > hi {
				lo, hi = hi, lo
			}
			ek := lo + "|" + hi
			rec := edgeMap[ek]
			if rec == nil {
				pl, ph := p1, p2
				if k1 != lo {
					pl, ph = p2, p1
				}
				rec = &edgeRec{a: lo, b: hi, pa: pl, pb: ph, owners: map[int]struct{}{}}
				edgeMap[ek] = rec
			}
			for _, o := range owners[i] {
				rec.owners[o] = struct{}{}
			}
		}
	}

	ar := &arrangement{points: points, adj: map[string][]neighbor{}}
	for _, rec := range edgeMap {
		ow := make([]int, 0, len(rec.owners))
		for o := range rec.owners {
			ow = append(ow, o)
		}
		sort.Ints(ow)
		ar.segs = append(ar.segs, atomicSeg{a: rec.pa, b: rec.pb, owners: ow})
		ar.adj[rec.a] = append(ar.adj[rec.a], neighbor{
			to: rec.b, point: rec.pb,
			dx: new(big.Rat).Sub(rec.pb.X, rec.pa.X), dy: new(big.Rat).Sub(rec.pb.Y, rec.pa.Y),
			owners: ow,
		})
		ar.adj[rec.b] = append(ar.adj[rec.b], neighbor{
			to: rec.a, point: rec.pa,
			dx: new(big.Rat).Sub(rec.pa.X, rec.pb.X), dy: new(big.Rat).Sub(rec.pa.Y, rec.pb.Y),
			owners: ow,
		})
	}
	for k := range ar.adj {
		ns := ar.adj[k]
		sort.Slice(ns, func(i, j int) bool { return angleLess(ns[i], ns[j]) })
		ar.adj[k] = ns
	}
	return ar, nil
}

// angleLess orders rays counter-clockwise starting on the +x axis, using only
// half-plane tests and exact cross products (no atan2, no floats).
func angleLess(u, v neighbor) bool {
	hu := halfPlane(u.dy, u.dx)
	hv := halfPlane(v.dy, v.dx)
	if hu != hv {
		return hu < hv
	}
	cr := new(big.Rat).Sub(
		new(big.Rat).Mul(u.dx, v.dy),
		new(big.Rat).Mul(u.dy, v.dx),
	)
	if cr.Sign() != 0 {
		return cr.Sign() > 0 // u strictly clockwise-before v within the same half
	}
	return false
}

// halfPlane: 0 for upper half [0,π) (dy>0 or dy==0 && dx>0), 1 for lower.
func halfPlane(dy, dx *big.Rat) int {
	if dy.Sign() > 0 || (dy.Sign() == 0 && dx.Sign() > 0) {
		return 0
	}
	return 1
}

func pointAt(a, b RatPoint, t *big.Rat) RatPoint {
	x := new(big.Rat).Add(a.X, new(big.Rat).Mul(t, new(big.Rat).Sub(b.X, a.X)))
	y := new(big.Rat).Add(a.Y, new(big.Rat).Mul(t, new(big.Rat).Sub(b.Y, a.Y)))
	return RatPoint{X: x, Y: y}
}

// ratSegIntersections returns the parameters t on AB and u on CD of every
// contact point of the two closed segments: a crossing gives one pair;
// collinear overlap gives the t/u parameters of the shared interval ends.
func ratSegIntersections(a, b, c, d RatPoint) (ts, us []*big.Rat, ok bool) {
	abx := new(big.Rat).Sub(b.X, a.X)
	aby := new(big.Rat).Sub(b.Y, a.Y)
	cdx := new(big.Rat).Sub(d.X, c.X)
	cdy := new(big.Rat).Sub(d.Y, c.Y)
	acx := new(big.Rat).Sub(c.X, a.X)
	acy := new(big.Rat).Sub(c.Y, a.Y)

	den := new(big.Rat).Sub(new(big.Rat).Mul(abx, cdy), new(big.Rat).Mul(aby, cdx))
	numT := new(big.Rat).Sub(new(big.Rat).Mul(acx, cdy), new(big.Rat).Mul(acy, cdx))
	numU := new(big.Rat).Sub(new(big.Rat).Mul(acx, aby), new(big.Rat).Mul(acy, abx))

	if den.Sign() == 0 {
		if numT.Sign() != 0 || numU.Sign() != 0 {
			return nil, nil, false
		}
		// Collinear: project c,d onto AB.
		var tC, tD *big.Rat
		if abx.Sign() != 0 {
			tC = new(big.Rat).Quo(new(big.Rat).Sub(c.X, a.X), abx)
			tD = new(big.Rat).Quo(new(big.Rat).Sub(d.X, a.X), abx)
		} else {
			tC = new(big.Rat).Quo(new(big.Rat).Sub(c.Y, a.Y), aby)
			tD = new(big.Rat).Quo(new(big.Rat).Sub(d.Y, a.Y), aby)
		}
		add := func(t, u *big.Rat) {
			ts = append(ts, clampUnit(t))
			us = append(us, clampUnit(u))
		}
		add(tC, big.NewRat(0, 1))
		add(tD, big.NewRat(1, 1))
		// The overlap interval endpoints also correspond to AB's own
		// endpoints when they lie inside CD: parameters u at t=0,1.
		uAt0 := big.NewRat(0, 1)
		uAt1 := big.NewRat(1, 1)
		if cdx.Sign() != 0 {
			uAt0 = new(big.Rat).Quo(new(big.Rat).Sub(a.X, c.X), cdx)
			uAt1 = new(big.Rat).Quo(new(big.Rat).Sub(b.X, c.X), cdx)
		} else {
			uAt0 = new(big.Rat).Quo(new(big.Rat).Sub(a.Y, c.Y), cdy)
			uAt1 = new(big.Rat).Quo(new(big.Rat).Sub(b.Y, c.Y), cdy)
		}
		add(big.NewRat(0, 1), uAt0)
		add(big.NewRat(1, 1), uAt1)
		// Keep only pairs inside both unit intervals.
		var fts, fus []*big.Rat
		for i := range ts {
			if inUnit(ts[i]) && inUnit(us[i]) {
				fts = append(fts, ts[i])
				fus = append(fus, us[i])
			}
		}
		if len(fts) == 0 {
			return nil, nil, false
		}
		return fts, fus, true
	}

	t := new(big.Rat).Quo(numT, den)
	u := new(big.Rat).Quo(numU, den)
	if !inUnit(t) || !inUnit(u) {
		return nil, nil, false
	}
	return []*big.Rat{t}, []*big.Rat{u}, true
}

func inUnit(q *big.Rat) bool {
	return q.Cmp(big.NewRat(0, 1)) >= 0 && q.Cmp(big.NewRat(1, 1)) <= 0
}

func clampUnit(q *big.Rat) *big.Rat {
	if q.Cmp(big.NewRat(0, 1)) < 0 {
		return big.NewRat(0, 1)
	}
	if q.Cmp(big.NewRat(1, 1)) > 0 {
		return big.NewRat(1, 1)
	}
	return q
}
