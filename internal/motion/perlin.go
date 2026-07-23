package motion

import (
	"math"
	"math/rand"
)

// PerlinNoise is a lightweight 2D gradient noise generator for organic motion.
type PerlinNoise struct {
	perm [512]int
}

// NewPerlinNoise seeds a deterministic permutation table.
func NewPerlinNoise(seed int64) *PerlinNoise {
	p := &PerlinNoise{}
	perm := make([]int, 256)
	for i := range perm {
		perm[i] = i
	}
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- procedural motion requires seeded variance.
	for i := 255; i > 0; i-- {
		j := rng.Intn(i + 1)
		perm[i], perm[j] = perm[j], perm[i]
	}
	for i := 0; i < 512; i++ {
		p.perm[i] = perm[i&255]
	}
	return p
}

// Noise2D returns smooth noise in [-1, 1] for the given coordinates.
func (p *PerlinNoise) Noise2D(x, y float64) float64 {
	xi := int(math.Floor(x)) & 255
	yi := int(math.Floor(y)) & 255
	xf := x - math.Floor(x)
	yf := y - math.Floor(y)

	u := fade(xf)
	v := fade(yf)

	aa := p.grad(p.perm[xi]+p.perm[yi], xf, yf)
	ab := p.grad(p.perm[xi+1]+p.perm[yi], xf-1, yf)
	ba := p.grad(p.perm[xi]+p.perm[yi+1], xf, yf-1)
	bb := p.grad(p.perm[xi+1]+p.perm[yi+1], xf-1, yf-1)

	x1 := lerp(u, aa, ab)
	x2 := lerp(u, ba, bb)
	return lerp(v, x1, x2)
}

func fade(t float64) float64 {
	return t * t * t * (t*(t*6-15) + 10)
}

func lerp(t, a, b float64) float64 {
	return a + t*(b-a)
}

func (p *PerlinNoise) grad(hash int, x, y float64) float64 {
	h := hash & 7
	u, v := x, y
	if h >= 4 {
		u, v = y, x
	}
	signU, signV := 1.0, 1.0
	if h&1 != 0 {
		signU = -1
	}
	if h&2 != 0 {
		signV = -2
	}
	return signU*u + signV*v
}
