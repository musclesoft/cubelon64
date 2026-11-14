package main

import (
	"math"
	"math/rand"
	"sort"
	"sync"
)

const (
	cubeBestSeeds        = 40
	cubeDistinctPoses    = 12
	cubeFirstPassSeeds   = 20000
	cubeRefineIterations = 1500
	cubeRefineSigma      = 0.12
	cubeSecondPassSeeds  = 4000
	cubeSmoothness       = 40.0
	cubeSameAngle        = 0.12
	cubeWorkers          = 8
)

type cubeQuat [4]float64

type cubePoseCandidate struct {
	cost float64
	pose cubeQuat
}

type cubeSearch struct {
	camera  cubeCamera
	targets []morphTile
}

var cubeSymmetries = []cubeQuat{{1, 0, 0, 0}, {0, 1, 0, 0}, {0, 0, 1, 0}, {0, 0, 0, 1}}

var cubeQuarterRoll = cubeQuat{math.Sqrt(0.5), 0, 0, math.Sqrt(0.5)}

func quatMultiply(a, b cubeQuat) cubeQuat {
	return cubeQuat{
		a[0]*b[0] - a[1]*b[1] - a[2]*b[2] - a[3]*b[3],
		a[0]*b[1] + a[1]*b[0] + a[2]*b[3] - a[3]*b[2],
		a[0]*b[2] - a[1]*b[3] + a[2]*b[0] + a[3]*b[1],
		a[0]*b[3] + a[1]*b[2] - a[2]*b[1] + a[3]*b[0],
	}
}

func quatNormalise(q cubeQuat) cubeQuat {
	length := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	return cubeQuat{q[0] / length, q[1] / length, q[2] / length, q[3] / length}
}

func quatFromRotationVector(v cubeVector) cubeQuat {
	angle := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if angle < 1e-12 {
		return cubeQuat{1, 0, 0, 0}
	}
	scale := math.Sin(angle/2) / angle
	return cubeQuat{math.Cos(angle / 2), v[0] * scale, v[1] * scale, v[2] * scale}
}

func quatMatrix(q cubeQuat) cubeMatrix {
	w, x, y, z := q[0], q[1], q[2], q[3]
	return cubeMatrix{
		{1 - 2*(y*y+z*z), 2 * (x*y - z*w), 2 * (x*z + y*w)},
		{2 * (x*y + z*w), 1 - 2*(x*x+z*z), 2 * (y*z - x*w)},
		{2 * (x*z - y*w), 2 * (y*z + x*w), 1 - 2*(x*x+y*y)},
	}
}

func quatAngle(a, b cubeQuat) float64 {
	dot := math.Abs(a[0]*b[0] + a[1]*b[1] + a[2]*b[2] + a[3]*b[3])
	return 2 * math.Acos(math.Min(dot, 1))
}

func randomQuat(rng *rand.Rand) cubeQuat {
	return quatNormalise(cubeQuat{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()})
}

func cubeFaceOnPoses() []cubeQuat {
	var poses []cubeQuat
	for _, roll := range []cubeQuat{{1, 0, 0, 0}, cubeQuarterRoll} {
		for _, symmetry := range cubeSymmetries {
			poses = append(poses, quatNormalise(quatMultiply(roll, symmetry)))
		}
	}
	return poses
}

func parallelStates(count int, work func(state int)) {
	var group sync.WaitGroup
	slots := make(chan struct{}, cubeWorkers)
	for state := 0; state < count; state++ {
		group.Add(1)
		slots <- struct{}{}
		go func(state int) {
			defer group.Done()
			defer func() { <-slots }()
			work(state)
		}(state)
	}
	group.Wait()
}

// Finds one pose per state: the best poses of each state from a random and a refined search
// (with their shade-preserving equivalents), then the path through them that stays smooth.
// The face-on states have no pattern to pin them down, so their candidates are the face-on poses.
func (search cubeSearch) fitPoses() []cubeMatrix {
	first := search.candidatesOfStates(nil, cubeFirstPassSeeds)
	poses := cubeViterbi(first)
	var seeds [][]cubeQuat
	for state := range poses {
		var neighbours []cubeQuat
		for other := state - 2; other <= state+2; other++ {
			if other >= 0 && other < len(poses) {
				neighbours = append(neighbours, poses[other])
			}
		}
		seeds = append(seeds, neighbours)
	}
	poses = cubeViterbi(search.candidatesOfStates(seeds, cubeSecondPassSeeds))
	matrices := make([]cubeMatrix, len(poses))
	for state, pose := range poses {
		matrices[state] = quatMatrix(pose)
	}
	return matrices
}

func (search cubeSearch) candidatesOfStates(seeds [][]cubeQuat, randomCount int) [][]cubePoseCandidate {
	candidates := make([][]cubePoseCandidate, len(search.targets))
	parallelStates(len(search.targets), func(state int) {
		var extra []cubeQuat
		if seeds != nil {
			extra = seeds[state]
		}
		candidates[state] = search.candidatesOfState(state, extra, randomCount)
	})
	return candidates
}

func (search cubeSearch) candidatesOfState(state int, extra []cubeQuat, randomCount int) []cubePoseCandidate {
	if state == 0 || state == len(search.targets)-1 {
		return search.costed(state, cubeFaceOnPoses())
	}
	rng := rand.New(rand.NewSource(int64(state) + 77))
	seeds := append([]cubeQuat(nil), extra...)
	for i := 0; i < randomCount; i++ {
		seeds = append(seeds, randomQuat(rng))
	}
	pool := search.costed(state, seeds)
	var refined []cubePoseCandidate
	for _, seed := range pool[:cubeBestSeeds] {
		refined = append(refined, search.refine(rng, state, seed.pose))
	}
	sort.Slice(refined, func(i, j int) bool { return refined[i].cost < refined[j].cost })
	var variants []cubeQuat
	for _, base := range distinctPoses(refined) {
		for _, roll := range []cubeQuat{{1, 0, 0, 0}, cubeQuarterRoll} {
			for _, symmetry := range cubeSymmetries {
				variants = append(variants, quatNormalise(quatMultiply(quatMultiply(base, roll), symmetry)))
			}
		}
	}
	return search.costed(state, variants)
}

func (search cubeSearch) costed(state int, poses []cubeQuat) []cubePoseCandidate {
	candidates := make([]cubePoseCandidate, len(poses))
	for i, pose := range poses {
		candidates[i] = cubePoseCandidate{cost: search.camera.tolerantCost(quatMatrix(pose), &search.targets[state]), pose: pose}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].cost < candidates[j].cost })
	return candidates
}

func (search cubeSearch) refine(rng *rand.Rand, state int, start cubeQuat) cubePoseCandidate {
	cost := func(pose cubeQuat) float64 {
		return search.camera.tolerantCost(quatMatrix(pose), &search.targets[state])
	}
	current := cubePoseCandidate{cost: cost(start), pose: start}
	best := current
	for iteration := 0; iteration < cubeRefineIterations; iteration++ {
		progress := float64(iteration) / cubeRefineIterations
		sigma := cubeRefineSigma * (1 - 0.95*progress)
		step := quatFromRotationVector(cubeVector{rng.NormFloat64() * sigma, rng.NormFloat64() * sigma, rng.NormFloat64() * sigma})
		pose := quatNormalise(quatMultiply(step, current.pose))
		value := cost(pose)
		if value <= current.cost || rng.Float64() < math.Exp((current.cost-value)/(0.5*(1-progress)+0.01)) {
			current = cubePoseCandidate{cost: value, pose: pose}
			if value < best.cost {
				best = current
			}
		}
	}
	return best
}

func distinctPoses(sorted []cubePoseCandidate) []cubeQuat {
	var distinct []cubeQuat
	for _, candidate := range sorted {
		if !sameUpToSymmetry(distinct, candidate.pose) {
			distinct = append(distinct, candidate.pose)
		}
		if len(distinct) == cubeDistinctPoses {
			break
		}
	}
	return distinct
}

func sameUpToSymmetry(poses []cubeQuat, pose cubeQuat) bool {
	for _, other := range poses {
		for _, symmetry := range cubeSymmetries {
			if quatAngle(pose, quatNormalise(quatMultiply(other, symmetry))) < cubeSameAngle {
				return true
			}
		}
	}
	return false
}

func cubeViterbi(candidates [][]cubePoseCandidate) []cubeQuat {
	costs := make([][]float64, len(candidates))
	origins := make([][]int, len(candidates))
	for state := range candidates {
		costs[state] = make([]float64, len(candidates[state]))
		origins[state] = make([]int, len(candidates[state]))
		for i, candidate := range candidates[state] {
			if state == 0 {
				costs[state][i] = candidate.cost
				continue
			}
			best, origin := math.Inf(1), 0
			for j, previous := range candidates[state-1] {
				value := costs[state-1][j] + cubeSmoothness*quatAngle(previous.pose, candidate.pose)
				if value < best {
					best, origin = value, j
				}
			}
			costs[state][i] = best + candidate.cost
			origins[state][i] = origin
		}
	}
	last := len(candidates) - 1
	choice := 0
	for i, value := range costs[last] {
		if value < costs[last][choice] {
			choice = i
		}
	}
	poses := make([]cubeQuat, len(candidates))
	for state := last; state >= 0; state-- {
		poses[state] = candidates[state][choice].pose
		choice = origins[state][choice]
	}
	return poses
}
