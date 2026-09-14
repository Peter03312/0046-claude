package geom

import "math/big"

func ratInt(v int64) *big.Rat { return new(big.Rat).SetFrac64(v, 1) }

func ratCopy(v *big.Rat) *big.Rat { return new(big.Rat).Set(v) }

func rp(x, y *big.Rat) RatPoint {
	return RatPoint{X: new(big.Rat).Set(x), Y: new(big.Rat).Set(y)}
}

// key renders a rational point into a canonical, collision-free string.
func (p RatPoint) key() string {
	return p.X.RatString() + "," + p.Y.RatString()
}

func lexLess(a, b RatPoint) bool {
	if c := a.X.Cmp(b.X); c != 0 {
		return c < 0
	}
	return a.Y.Cmp(b.Y) < 0
}

func pointsEqual(a, b RatPoint) bool {
	return a.X.Cmp(b.X) == 0 && a.Y.Cmp(b.Y) == 0
}

// Rotate applies a 0/90/180/270 degree counter-clockwise rotation around
// the origin followed by an integer translation. Because both operations
// only permute and negate the integer coordinates, the result stays integral.
func Rotate(p IntPoint, deg, tx, ty int64) (IntPoint, bool) {
	switch deg {
	case 0:
		return IntPoint{X: p.X + tx, Y: p.Y + ty}, true
	case 90:
		return IntPoint{X: -p.Y + tx, Y: p.X + ty}, true
	case 180:
		return IntPoint{X: -p.X + tx, Y: -p.Y + ty}, true
	case 270:
		return IntPoint{X: p.Y + tx, Y: -p.X + ty}, true
	default:
		return IntPoint{}, false
	}
}
