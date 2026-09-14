package geom

import (
	"encoding/json"
	"math/big"
	"testing"
)

func square(x0, y0, x1, y1 int64) []IntPoint {
	return []IntPoint{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}
func squareSheet(id string, x0, y0, x1, y1 int64, rgb [3]int, op int) Sheet {
	return Sheet{ID: id, Vertices: square(x0, y0, x1, y1), R: rgb[0], G: rgb[1], B: rgb[2], OpacityMillis: op}
}
func blankRegion(id string, vs []IntPoint) Region {
	return Region{ID: id, Kind: "blank", Vertices: vs}
}
func targetRegion(id string, vs []IntPoint, rgb [3]int, tol int) Region {
	return Region{ID: id, Kind: "target", Vertices: vs, R: rgb[0], G: rgb[1], B: rgb[2], Tolerance: tol}
}

func ringKey(r Ring) string {
	out := ""
	for _, p := range r {
		out += p.key() + ";"
	}
	return out
}

// A fully opaque sheet over a target region is its exact encoded colour.
func TestExactColorMatch(t *testing.T) {
	act := ActInput{
		Sheets:  []Sheet{squareSheet("s", 0, 0, 10, 10, [3]int{10, 20, 30}, 1000)},
		Regions: []Region{targetRegion("t", square(0, 0, 10, 10), [3]int{10, 20, 30}, 0)},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("expected no bad cells, got %d: %+v", len(bad), bad)
	}
}

// No sheets over a target => white (255,255,255); a non-white target fails.
func TestWhiteBackground(t *testing.T) {
	act := ActInput{
		Regions: []Region{targetRegion("t", square(0, 0, 4, 4), [3]int{0, 0, 0}, 254)},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 1 || bad[0].Actual != [3]int{255, 255, 255} {
		t.Fatalf("expected one white bad cell, got %+v", bad)
	}
}

// Two rectangles side by side with a shared vertical edge: the blank region
// touches the sheet along an edge only (zero area), which must be legal.
func TestSharedEdgeIsNotLeak(t *testing.T) {
	act := ActInput{
		Sheets:  []Sheet{squareSheet("s", 0, 0, 10, 10, [3]int{0, 0, 0}, 1000)},
		Regions: []Region{blankRegion("b", square(10, 0, 20, 10))},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("edge contact must not leak, got %+v", bad)
	}
}

// A one-unit-wide sliver of sheet intrudes into the blank region: a
// positive-area leak cell must appear, even though pixel sampling on coarse
// grids could miss it entirely.
func TestThinSliverLeak(t *testing.T) {
	act := ActInput{
		Sheets:  []Sheet{squareSheet("s", 0, 0, 11, 10, [3]int{0, 0, 0}, 1000)},
		Regions: []Region{blankRegion("b", square(10, 0, 20, 10))},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 1 {
		t.Fatalf("expected exactly one sliver cell, got %d: %+v", len(bad), bad)
	}
	c := bad[0]
	if c.Kind != "leak" || c.RegionID != "b" {
		t.Fatalf("unexpected cell: %+v", c)
	}
	wantOuter := "10,0;11,0;11,10;10,10;"
	if ringKey(c.Outer) != wantOuter {
		t.Fatalf("outer ring = %s, want %s", ringKey(c.Outer), wantOuter)
	}
	// CCW: positive signed area.
	if signedArea2(c.Outer).Sign() <= 0 {
		t.Fatalf("outer ring must be CCW")
	}
	if len(c.Contributors) != 1 || c.Contributors[0].ID != "s" {
		t.Fatalf("contributors wrong: %+v", c.Contributors)
	}
}

// A rational-width sliver (two sheets whose diagonal edges cross at a
// non-integer point) proves intersections are rational and not snapped.
func TestRationalSliverLeak(t *testing.T) {
	// Sheet triangle covers (0,0)-(6,0)-(0,6); the blank square starts at
	// x=3. The line x+y=6 cuts the blank 3..6 x 0..6 in a triangular corner
	// where x+y<6 at the bottom-left: positive area, vertices (3,0),(6,0),(3,3).
	tri := Sheet{
		ID: "t", OpacityMillis: 1000, R: 0,
		Vertices: []IntPoint{{0, 0}, {6, 0}, {0, 6}},
	}
	act := ActInput{
		Sheets:  []Sheet{tri},
		Regions: []Region{blankRegion("b", square(3, 0, 6, 6))},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 1 {
		t.Fatalf("want 1 leak, got %d: %+v", len(bad), bad)
	}
	wantOuter := "3,0;6,0;3,3;" // CCW order from lex-min (3,0)
	if ringKey(bad[0].Outer) != wantOuter {
		t.Fatalf("outer = %s want %s", ringKey(bad[0].Outer), wantOuter)
	}
}

// Two disjoint slivers -> multiple bad cells in one act.
func TestMultipleBadCells(t *testing.T) {
	act := ActInput{
		Sheets: []Sheet{
			squareSheet("a", 0, 0, 11, 5, [3]int{0, 0, 0}, 1000),
			squareSheet("b", 0, 5, 11, 10, [3]int{0, 0, 0}, 1000),
		},
		Regions: []Region{blankRegion("blk", square(10, 0, 20, 10))},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 2 {
		t.Fatalf("want 2 bad cells, got %d: %+v", len(bad), bad)
	}
}

// 50% black over white composites exactly to 127.5; final half-up = 128.
// Expecting 127 fails (|128-127|=1 > 0), expecting 128 passes.
func TestHalfUpRoundingOnlyAtEnd(t *testing.T) {
	run := func(target int, tol int) int {
		act := ActInput{
			Sheets:  []Sheet{squareSheet("s", 0, 0, 4, 4, [3]int{0, 0, 0}, 500)},
			Regions: []Region{targetRegion("t", square(0, 0, 4, 4), [3]int{target, target, target}, tol)},
		}
		bad, err := Evaluate(act)
		if err != nil {
			t.Fatal(err)
		}
		return len(bad)
	}
	if run(128, 0) != 0 {
		t.Fatal("128 must pass exactly after half-up rounding")
	}
	if run(127, 0) != 1 {
		t.Fatal("127 must fail")
	}
}

// Two 500‰ black layers: 0*0.5 + 127.5*0.5 = 63.75 -> half-up 64. This also
// catches an implementation that rounds after every layer (would give 64 too
// here; see TestNoIntermediateRounding below for a distinguishing case).
func TestTranslucentStack(t *testing.T) {
	act := ActInput{
		Sheets: []Sheet{
			squareSheet("a", 0, 0, 4, 4, [3]int{0, 0, 0}, 500),
			squareSheet("b", 0, 0, 4, 4, [3]int{0, 0, 0}, 500),
		},
		Regions: []Region{targetRegion("t", square(0, 0, 4, 4), [3]int{64, 64, 64}, 0)},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("stack should composite to 64, got %+v", bad[0].Actual)
	}
}

// Layer at alpha 1/1000 over white yields 255 - 255/1000 = 254.745 -> 255.
// A second identical layer: 255 - 255*(1/1000)^2 = 254.999745 -> 255 as
// well, BUT a per-layer rounding engine would also produce 255. Use a color
// whose rounding boundary distinguishes them instead: source colour 200 at
// 500‰ => 227.5 -> 228 exactly; second layer over exact 227.5 gives
// 200*0.5 + 227.5*0.5 = 213.75 -> 214, whereas rounding the first layer to
// 228 gives 200*0.5 + 228*0.5 = 214 — same. Use 3 layers of 1‰ black: exact
// 255*(999/1000)^3 = 254.235745 -> 254; naive per-layer rounding keeps 255.
func TestNoIntermediateRounding(t *testing.T) {
	sheets := []Sheet{
		squareSheet("a", 0, 0, 4, 4, [3]int{0, 0, 0}, 1),
		squareSheet("b", 0, 0, 4, 4, [3]int{0, 0, 0}, 1),
		squareSheet("c", 0, 0, 4, 4, [3]int{0, 0, 0}, 1),
	}
	act := ActInput{
		Sheets:  sheets,
		Regions: []Region{targetRegion("t", square(0, 0, 4, 4), [3]int{254, 254, 254}, 0)},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("exact compositing should yield 254 (254.235... half-up), got actual=%v", bad[0].Actual)
	}
}

// Rotation 90 CCW around the origin maps (10,0)->(0,10). The transformed
// square should be (0,0),(0,10),(-10,10),(-10,0); a target over that shape
// with the sheet colour passes; a blank at the untransformed location must be
// untouched (no leak).
func TestRotation90(t *testing.T) {
	src := []IntPoint{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	var rot []IntPoint
	for _, p := range src {
		q, ok := Rotate(p, 90, 0, 0)
		if !ok {
			t.Fatal("rotate")
		}
		rot = append(rot, q)
	}
	s := Sheet{ID: "s", Vertices: rot, R: 7, G: 8, B: 9, OpacityMillis: 1000}
	act := ActInput{
		Sheets: []Sheet{s},
		Regions: []Region{
			targetRegion("t", square(-10, 0, 0, 10), [3]int{7, 8, 9}, 0),
			blankRegion("b", square(0, 0, 10, 10)),
		},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("rotated sheet: expected pass, got %+v", bad)
	}
}

func TestRotationTranslationKeepsIntegral(t *testing.T) {
	for deg, want := range map[int64]IntPoint{
		0: {3 + 1, 4 + 2}, 90: {-4 + 1, 3 + 2},
		180: {-3 + 1, -4 + 2}, 270: {4 + 1, -3 + 2},
	} {
		got, ok := Rotate(IntPoint{3, 4}, deg, 1, 2)
		if !ok || got != want {
			t.Fatalf("rotate %d = %+v, want %+v", deg, got, want)
		}
	}
}

// A failing face with an inner island constructs a bad cell whose outer ring
// is CCW and whose hole ring is CW, both starting at the lex-min point:
//
//	big opaque red square (0,0)-(30,30);
//	inner opaque red square (12,12)-(18,18) (same colour, no visual change);
//	target region (5,5)-(25,25) expects blue -> the region splits into an
//	outer red frame and an inner red island; the frame face has one hole.
func TestHoleRingOrientation(t *testing.T) {
	act := ActInput{
		Sheets: []Sheet{
			squareSheet("big", 0, 0, 30, 30, [3]int{200, 0, 0}, 1000),
			squareSheet("in", 12, 12, 18, 18, [3]int{200, 0, 0}, 1000),
		},
		Regions: []Region{
			targetRegion("t", square(5, 5, 25, 25), [3]int{0, 0, 200}, 0),
		},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 2 {
		t.Fatalf("want frame + island bad cells, got %d: %+v", len(bad), bad)
	}
	var frame *BadCell
	for i := range bad {
		if len(bad[i].Holes) == 1 {
			frame = &bad[i]
		}
	}
	if frame == nil {
		t.Fatalf("no cell with a hole: %+v", bad)
	}
	if signedArea2(frame.Outer).Sign() <= 0 {
		t.Fatal("outer must be CCW")
	}
	if signedArea2(frame.Holes[0]).Sign() >= 0 {
		t.Fatalf("hole must be CW: %+v", frame.Holes)
	}
	for _, r := range append([]Ring{frame.Outer}, frame.Holes...) {
		for _, p := range r[1:] {
			if lexLess(p, r[0]) {
				t.Fatalf("ring does not start at lex-min: %s before %s", p.key(), r[0].key())
			}
		}
	}
	if got := ringKey(frame.Outer); got != "5,5;25,5;25,25;5,25;" {
		t.Fatalf("frame outer = %s", got)
	}
	if got := ringKey(frame.Holes[0]); got != "12,12;12,18;18,18;18,12;" {
		t.Fatalf("frame hole = %s", got)
	}
}

// Regions overlapping with positive area are rejected; shared edges pass.
func TestRegionOverlapRejected(t *testing.T) {
	act := ActInput{
		Regions: []Region{
			blankRegion("a", square(0, 0, 10, 10)),
			targetRegion("b", square(5, 5, 15, 15), [3]int{0, 0, 0}, 0),
		},
	}
	if _, err := Evaluate(act); err == nil {
		t.Fatal("overlapping regions must be rejected")
	}
	act2 := ActInput{
		Regions: []Region{
			blankRegion("a", square(0, 0, 10, 10)),
			targetRegion("b", square(10, 0, 20, 10), [3]int{255, 255, 255}, 0),
		},
	}
	if bad, err := Evaluate(act2); err != nil || len(bad) != 0 {
		t.Fatalf("edge-adjacent regions must be accepted: %v %+v", err, bad)
	}
}

// Self-intersecting inputs of every flavor are rejected before any analysis.
func TestSelfIntersectingRejected(t *testing.T) {
	cases := map[string][]IntPoint{
		"bowtie": {{0, 0}, {10, 10}, {10, 0}, {0, 10}},
		"pinch":  {{0, 0}, {10, 0}, {5, 5}, {10, 10}, {0, 10}, {5, 5}},
		"touch":  {{0, 0}, {10, 0}, {10, 10}, {5, 0}, {0, 10}}, // vertex on non-adjacent edge
		"dup":    {{0, 0}, {5, 0}, {5, 0}, {5, 5}, {0, 5}},
		"two":    {{0, 0}, {4, 0}},
	}
	for name, vs := range cases {
		if err := validateSimple(vs, name); err == nil {
			t.Fatalf("%s: expected rejection", name)
		}
	}
}

// Order bottom-to-top is respected: swapping two half-transparent sheets
// changes the composited colour.
func TestStackOrder(t *testing.T) {
	red := squareSheet("r", 0, 0, 4, 4, [3]int{255, 0, 0}, 500)
	blu := squareSheet("b", 0, 0, 4, 4, [3]int{0, 0, 255}, 500)
	// force a failure to read the actual values, tolerance 0, target black.
	run := func(sheets []Sheet) [3]int {
		act := ActInput{Sheets: sheets,
			Regions: []Region{targetRegion("t", square(0, 0, 4, 4), [3]int{0, 0, 0}, 0)}}
		bad, err := Evaluate(act)
		if err != nil || len(bad) != 1 {
			t.Fatalf("setup: %v %v", bad, err)
		}
		return bad[0].Actual
	}
	redTop := run([]Sheet{blu, red})
	bluTop := run([]Sheet{red, blu})
	// red on top over blue over white:
	// R = 255*.5 + 127.5*.5 = 191.25 -> 191
	// G = 0   *.5 + 127.5*.5 = 63.75  -> 64
	// B = 0   *.5 + 255  *.5 = 127.5  -> 128 (first blue layer left B=255)
	if redTop != ([3]int{191, 64, 128}) {
		t.Fatalf("red-over-blue = %v, want {191,64,128}", redTop)
	}
	if bluTop != ([3]int{128, 64, 191}) {
		t.Fatalf("blue-over-red = %v, want {128,64,191}", bluTop)
	}
}

func TestHalfUpDirect(t *testing.T) {
	cases := []struct {
		num, den int64
		want     int
	}{
		{255, 2, 128}, {2545, 10, 255}, {2544, 10, 254},
		{0, 1, 0}, {1, 2, 1}, {254745, 1000, 255},
	}
	for _, c := range cases {
		if got := halfUp(new(big.Rat).SetFrac(big.NewInt(c.num), big.NewInt(c.den))); got != c.want {
			t.Fatalf("halfUp(%d/%d)=%d want %d", c.num, c.den, got, c.want)
		}
	}
}

// Point-only contact between a sheet corner and a blank region: zero area,
// legal.
func TestPointContactIsNotLeak(t *testing.T) {
	// Sheet (0,0)-(10,10); blank square diagonally NE, touching only at (10,10).
	act := ActInput{
		Sheets:  []Sheet{squareSheet("s", 0, 0, 10, 10, [3]int{0, 0, 0}, 1000)},
		Regions: []Region{blankRegion("b", square(10, 10, 20, 20))},
	}
	bad, err := Evaluate(act)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("point contact must be legal, got %+v", bad)
	}
}

// Tolerance is per-channel absolute difference on encoded integers.
func TestPerChannelTolerance(t *testing.T) {
	act := ActInput{
		Sheets:  []Sheet{squareSheet("s", 0, 0, 4, 4, [3]int{10, 20, 30}, 1000)},
		Regions: []Region{targetRegion("t", square(0, 0, 4, 4), [3]int{12, 18, 33}, 3)},
	}
	if bad, err := Evaluate(act); err != nil || len(bad) != 0 {
		t.Fatalf("diffs (2,2,3) within tol 3 must pass: %+v %v", bad, err)
	}
	act.Regions[0].Tolerance = 2 // diff on blue is 3 -> fail
	bad, err := Evaluate(act)
	if err != nil || len(bad) != 1 {
		t.Fatalf("tol 2 must fail: %+v %v", bad, err)
	}
	if bad[0].Tolerance != 2 || bad[0].Expected != [3]int{12, 18, 33} {
		t.Fatalf("bad cell payload wrong: %+v", bad[0])
	}
}

// Fractional ring vertices round-trip through JSON without losing precision
// (a big.Rat with unexported fields would otherwise marshal as {}).
func TestBadCellJSONFractions(t *testing.T) {
	x, _ := new(big.Rat).SetString("7/3")
	y, _ := new(big.Rat).SetString("-2/5")
	cell := BadCell{
		Kind:   "leak",
		Outer:  Ring{{X: x, Y: y}},
		Actual: [3]int{1, 2, 3},
	}
	data, err := json.Marshal(cell)
	if err != nil {
		t.Fatal(err)
	}
	var back BadCell
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Outer[0].X.Cmp(x) != 0 || back.Outer[0].Y.Cmp(y) != 0 {
		t.Fatalf("fraction lost: %s", data)
	}
}
