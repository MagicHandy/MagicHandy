package motion

import (
	"container/heap"
	"math"
)

// reversalFilter preserves the leftmost-first removal order of the original
// filter. Linked point/edge/anchor indexes let a removal update only its two
// neighboring edges and their reversal anchors, instead of rescanning and
// copying the entire remaining script. Original indexes give the heap a stable
// playback order even after points disappear.
type reversalFilter struct {
	points                     []CurvePoint
	previous, next             []int
	edgePrevious, edgeNext     []int
	anchorPrevious, anchorNext []int
	direction                  []int
	anchor, queued             []bool
	candidates                 reversalCandidates
	minimumProminence          float64
	flankMillis                int64
	preserveStrongReversals    bool
}

type reversalCandidates []int

func (q reversalCandidates) Len() int           { return len(q) }
func (q reversalCandidates) Less(i, j int) bool { return q[i] < q[j] }
func (q reversalCandidates) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *reversalCandidates) Push(value any)    { *q = append(*q, value.(int)) }
func (q *reversalCandidates) Pop() any {
	last := len(*q) - 1
	value := (*q)[last]
	*q = (*q)[:last]
	return value
}

func filterPatternReversals(points []CurvePoint, minimumProminence float64) []CurvePoint {
	if len(points) < 3 {
		return append([]CurvePoint(nil), points...)
	}
	return filterReversals(points, minimumProminence, patternChatterFlankMillis(points), false)
}

// SmoothMediaReversals removes only small excursions with two short flanks.
// A large authored reversal beside a small spike remains an anchor. The fixed
// wall-time window is independent of how much script remains after a seek.
func SmoothMediaReversals(points []CurvePoint, minimumProminence float64) []CurvePoint {
	return filterReversals(points, minimumProminence, 250, true)
}

func filterReversals(points []CurvePoint, minimumProminence float64, flank int64, preserveStrong bool) []CurvePoint {
	n := len(points)
	if n < 3 || minimumProminence <= 0 {
		return append([]CurvePoint(nil), points...)
	}
	// Most authored scripts have nothing to remove. Avoid the working indexes
	// entirely in that case (including media played with smoothing off).
	anchors := curveReversalAnchors(points)
	eligible := func(left, current, right int) bool {
		return reversalIsChatter(points[left], points[current], points[right], minimumProminence, flank, preserveStrong)
	}
	queue := make(reversalCandidates, 0)
	for i := 1; i < len(anchors)-1; i++ {
		if eligible(anchors[i-1], anchors[i], anchors[i+1]) {
			queue = append(queue, anchors[i])
		}
	}
	if len(queue) == 0 {
		return append([]CurvePoint(nil), points...)
	}
	f := reversalFilter{
		points: points, previous: make([]int, n), next: make([]int, n),
		edgePrevious: make([]int, n), edgeNext: make([]int, n),
		anchorPrevious: make([]int, n), anchorNext: make([]int, n),
		direction: make([]int, n), anchor: make([]bool, n), queued: make([]bool, n),
		candidates: queue, minimumProminence: minimumProminence, flankMillis: flank,
		preserveStrongReversals: preserveStrong,
	}
	lastEdge := -1
	for i := range points {
		f.previous[i], f.next[i] = i-1, i+1
		f.edgePrevious[i], f.edgeNext[i] = -1, -1
		if i == n-1 {
			f.next[i] = -1
			continue
		}
		f.direction[i] = curveDirection(points[i+1].PositionPercent - points[i].PositionPercent)
		if f.direction[i] != 0 {
			f.edgePrevious[i] = lastEdge
			if lastEdge >= 0 {
				f.edgeNext[lastEdge] = i
			}
			lastEdge = i
		}
	}
	for i, at := range anchors {
		f.anchor[at] = true
		f.anchorPrevious[at], f.anchorNext[at] = -1, -1
		if i > 0 {
			f.anchorPrevious[at] = anchors[i-1]
		}
		if i+1 < len(anchors) {
			f.anchorNext[at] = anchors[i+1]
		}
	}
	for _, at := range queue {
		f.queued[at] = true
	}
	heap.Init(&f.candidates)
	for len(f.candidates) > 0 {
		at := heap.Pop(&f.candidates).(int)
		f.queued[at] = false
		if f.eligible(at) {
			f.remove(at)
		}
	}
	result := make([]CurvePoint, 0, n)
	for at := 0; at >= 0; at = f.next[at] {
		result = append(result, points[at])
	}
	return result
}

func reversalIsChatter(left, current, right CurvePoint, prominence float64, flank int64, preserveStrong bool) bool {
	if preserveStrong {
		return math.Max(math.Abs(current.PositionPercent-left.PositionPercent), math.Abs(current.PositionPercent-right.PositionPercent)) <= prominence &&
			max(current.TimeMillis-left.TimeMillis, right.TimeMillis-current.TimeMillis) <= flank
	}
	return math.Min(math.Abs(current.PositionPercent-left.PositionPercent), math.Abs(current.PositionPercent-right.PositionPercent)) <= prominence &&
		min(current.TimeMillis-left.TimeMillis, right.TimeMillis-current.TimeMillis) <= flank
}

func (f *reversalFilter) eligible(at int) bool {
	if at < 0 || !f.anchor[at] {
		return false
	}
	left, right := f.anchorPrevious[at], f.anchorNext[at]
	return left >= 0 && right >= 0 && reversalIsChatter(f.points[left], f.points[at], f.points[right], f.minimumProminence, f.flankMillis, f.preserveStrongReversals)
}

func (f *reversalFilter) removeAnchor(at int) {
	left, right := f.anchorPrevious[at], f.anchorNext[at]
	if left >= 0 {
		f.anchorNext[left] = right
	}
	if right >= 0 {
		f.anchorPrevious[right] = left
	}
	f.anchor[at] = false
}

func (f *reversalFilter) insertAnchor(at, left, right int) {
	f.anchorPrevious[at], f.anchorNext[at], f.anchor[at] = left, right, true
	if left >= 0 {
		f.anchorNext[left] = at
	}
	if right >= 0 {
		f.anchorPrevious[right] = at
	}
}

func (f *reversalFilter) isAnchor(at int) bool {
	if at < 0 {
		return false
	}
	if at == 0 || at == len(f.points)-1 {
		return true
	}
	before := f.edgePrevious[at]
	return f.direction[at] != 0 && before >= 0 && f.direction[before] != f.direction[at]
}

func (f *reversalFilter) remove(at int) {
	p, next := f.previous[at], f.next[at]
	left, right := f.anchorPrevious[at], f.anchorNext[at]
	f.removeAnchor(at)
	f.next[p], f.previous[next] = next, p
	q := f.replaceEdges(at, p, next)
	// Only p's outgoing direction and q's preceding nonzero direction changed.
	// There can be no other anchor between p, the removed point, and q.
	if f.anchor[p] && !f.isAnchor(p) {
		left = f.anchorPrevious[p]
		f.removeAnchor(p)
	} else if !f.anchor[p] && f.isAnchor(p) {
		f.insertAnchor(p, left, right)
		left = p
	}
	if q >= 0 {
		if f.anchor[q] && !f.isAnchor(q) {
			right = f.anchorNext[q]
			f.removeAnchor(q)
		} else if !f.anchor[q] && f.isAnchor(q) {
			f.insertAnchor(q, left, right)
			right = q
		}
	}
	for _, candidate := range []int{left, right, p, q} {
		if candidate < 0 || !f.anchor[candidate] {
			continue
		}
		for _, neighbor := range []int{candidate, f.anchorPrevious[candidate], f.anchorNext[candidate]} {
			if f.eligible(neighbor) && !f.queued[neighbor] {
				f.queued[neighbor] = true
				heap.Push(&f.candidates, neighbor)
			}
		}
	}
}

func (f *reversalFilter) replaceEdges(at, p, next int) int {
	// An interior anchor always has a nonzero outgoing edge. Replace p->at
	// and at->next by p->next in the nonzero-edge list, retaining plateau
	// behavior: the last equal-position point before a reversal is its anchor.
	edgeLeft, q := f.edgePrevious[at], f.edgeNext[at]
	if edgeLeft == p {
		edgeLeft = f.edgePrevious[p]
	}
	f.direction[at] = 0
	f.direction[p] = curveDirection(f.points[next].PositionPercent - f.points[p].PositionPercent)
	if f.direction[p] != 0 {
		f.edgePrevious[p], f.edgeNext[p] = edgeLeft, q
		if edgeLeft >= 0 {
			f.edgeNext[edgeLeft] = p
		}
		if q >= 0 {
			f.edgePrevious[q] = p
		}
	} else {
		if edgeLeft >= 0 {
			f.edgeNext[edgeLeft] = q
		}
		if q >= 0 {
			f.edgePrevious[q] = edgeLeft
		}
	}
	return q
}
