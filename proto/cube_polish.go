package main

import (
	"fmt"
	"math"
	"sort"
)

const (
	cubeTileCentre       = morphTileSize / 2
	cubeTolerantWeight   = 0.5
	cubePolishIterations = 700
	cubeRenderHardLimit  = 7.0
	cubeRenderSoftLimit  = 4.0
)

var cubePolishScales = []float64{0.03, 0.01, 0.003}

var cubeRenderPolishScales = []float64{0.03, 0.01, 0.003}

// What one unit of the initial simplex step means for each parameter: the free polynomial
// coefficients (radians), then the camera's half size and perspective. The camera's centre stays
// at the tile's centre, which the second half of the animation, the first turned by 180
// degrees, is symmetric about.
func cubeParameterUnit(index int) float64 {
	if index == cubeParameters-1 {
		return 0.04
	}
	return 1
}

const (
	cubeFreePinned = cubeCoefficients - 2
	cubeParameters = 2*cubeFreePinned + cubeCoefficients + 2
)

// What the face-on end states fix: the middle angle at both ends, and the sum (or difference)
// of the outer two there
type cubeEnds struct {
	endSum, middleEnd, middleStart, outerSign, startSum float64
}

func cubeEndsOf(motion cubeMotion) cubeEnds {
	first, last := 0.0, float64(cubeStates-1)
	sign := 1.0
	if polynomialOfState(motion.Angles[1][:], first) < 0 {
		sign = -1
	}
	return cubeEnds{
		endSum:      polynomialOfState(motion.Angles[0][:], last) - sign*polynomialOfState(motion.Angles[2][:], last),
		middleEnd:   polynomialOfState(motion.Angles[1][:], last),
		middleStart: polynomialOfState(motion.Angles[1][:], first),
		outerSign:   sign,
		startSum:    polynomialOfState(motion.Angles[0][:], first) + sign*polynomialOfState(motion.Angles[2][:], first),
	}
}

// A polynomial with the given values at the two ends and the given terms from the square
// up: the even terms must add up to the mean of the end values, the odd ones to half their
// difference
func pinnedPolynomial(higher []float64, start, end float64) [cubeCoefficients]float64 {
	var polynomial [cubeCoefficients]float64
	copy(polynomial[2:], higher)
	even, odd := (start+end)/2, (end-start)/2
	for i := 2; i < cubeCoefficients; i++ {
		if i%2 == 0 {
			even -= polynomial[i]
		} else {
			odd -= polynomial[i]
		}
	}
	polynomial[0], polynomial[1] = even, odd
	return polynomial
}

// The free parameters: the terms from the square up of the first two angles, all
// coefficients of the third, and the camera's size and perspective
func cubeFitVector(fit cubeFit) []float64 {
	angles := fit.Motion.Angles
	vector := append([]float64(nil), angles[0][2:]...)
	vector = append(vector, angles[1][2:]...)
	vector = append(vector, angles[2][:]...)
	camera := fit.Camera
	return append(vector, camera.HalfSize, camera.Perspective)
}

func cubeFitOfVector(template cubeFit, ends cubeEnds, vector []float64) cubeFit {
	fit := template
	last := float64(cubeStates - 1)
	first := vector[:cubeFreePinned]
	second := vector[cubeFreePinned : 2*cubeFreePinned]
	third := vector[2*cubeFreePinned : 2*cubeFreePinned+cubeCoefficients]
	camera := vector[2*cubeFreePinned+cubeCoefficients:]
	copy(fit.Motion.Angles[2][:], third)
	start := ends.startSum - ends.outerSign*polynomialOfState(third, 0)
	end := ends.endSum + ends.outerSign*polynomialOfState(third, last)
	fit.Motion.Angles[0] = pinnedPolynomial(first, start, end)
	fit.Motion.Angles[1] = pinnedPolynomial(second, ends.middleStart, ends.middleEnd)
	fit.Camera = cubeCamera{CentreX: cubeTileCentre, CentreY: cubeTileCentre, HalfSize: camera[0], Perspective: camera[1]}
	return fit
}

// Nelder-Mead on the tolerant cost of all states, in rounds of shrinking initial steps. The
// face-on end states stay exactly face-on: the coefficients the ends fix are not searched.
func polishCube(fit cubeFit, targets []morphTile) cubeFit {
	return polishCubeWith(fit, cubePolishScales, func(candidate cubeFit) float64 { return cubeFitCost(candidate, targets) })
}

// The same on what the validator counts for the C64 render, plus the tolerant cost so that no
// pixel is explained by no shade at all
func polishCubeRender(fit cubeFit, targets []morphTile) cubeFit {
	return polishCubeWith(fit, cubeRenderPolishScales, func(candidate cubeFit) float64 {
		return cubeRenderCost(candidate, targets) + cubeTolerantWeight*cubeFitCost(candidate, targets)
	})
}

func polishCubeWith(fit cubeFit, scales []float64, fitCost func(cubeFit) float64) cubeFit {
	ends := cubeEndsOf(fit.Motion)
	cost := func(vector []float64) float64 {
		return fitCost(cubeFitOfVector(fit, ends, vector))
	}
	vector := cubeFitVector(fit)
	for _, scale := range scales {
		var value float64
		steps := make([]float64, len(vector))
		for i := range steps {
			steps[i] = scale * cubeParameterUnit(i)
		}
		vector, value = nelderMead(cost, vector, steps, cubePolishIterations)
		fmt.Printf("Polish step %.3f: cost %.1f\n", scale, value)
	}
	return cubeFitOfVector(fit, ends, vector)
}

// The validator's mismatches of the C64 render summed over the states, the states above a
// few mismatches counting extra, and those near the block limit a lot more, since the limit
// applies to the worst one
func cubeRenderCost(fit cubeFit, targets []morphTile) float64 {
	weights := make([]float64, len(targets))
	parallelStates(len(targets), func(state int) {
		levels := fit.Camera.renderLevels(fit.Motion.rotationAt(float64(state)))
		weights[state] = cubeValidatorMismatches(&levels, &targets[state])
	})
	total := 0.0
	for _, weight := range weights {
		total += weight + 3*math.Max(0, weight-cubeRenderSoftLimit) + 30*math.Max(0, weight-cubeRenderHardLimit)
	}
	return total
}

func nelderMead(cost func([]float64) float64, start, steps []float64, iterations int) ([]float64, float64) {
	size := len(start)
	points := make([][]float64, size+1)
	values := make([]float64, size+1)
	for i := range points {
		points[i] = append([]float64(nil), start...)
		if i > 0 {
			points[i][i-1] += steps[i-1]
		}
		values[i] = cost(points[i])
	}
	for iteration := 0; iteration < iterations; iteration++ {
		sortSimplex(points, values)
		centre := simplexCentre(points[:size])
		reflected := moveFromCentre(centre, points[size], -1)
		reflectedValue := cost(reflected)
		switch {
		case reflectedValue < values[0]:
			expanded := moveFromCentre(centre, points[size], -2)
			if expandedValue := cost(expanded); expandedValue < reflectedValue {
				points[size], values[size] = expanded, expandedValue
			} else {
				points[size], values[size] = reflected, reflectedValue
			}
		case reflectedValue < values[size-1]:
			points[size], values[size] = reflected, reflectedValue
		default:
			factor := 0.5
			if reflectedValue < values[size] {
				factor = -0.5
			}
			contracted := moveFromCentre(centre, points[size], factor)
			if contractedValue := cost(contracted); contractedValue < math.Min(reflectedValue, values[size]) {
				points[size], values[size] = contracted, contractedValue
			} else {
				for i := 1; i <= size; i++ {
					for k := range points[i] {
						points[i][k] = points[0][k] + 0.5*(points[i][k]-points[0][k])
					}
					values[i] = cost(points[i])
				}
			}
		}
	}
	sortSimplex(points, values)
	return points[0], values[0]
}

func sortSimplex(points [][]float64, values []float64) {
	order := make([]int, len(points))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return values[order[a]] < values[order[b]] })
	sortedPoints := make([][]float64, len(points))
	sortedValues := make([]float64, len(points))
	for i, index := range order {
		sortedPoints[i], sortedValues[i] = points[index], values[index]
	}
	copy(points, sortedPoints)
	copy(values, sortedValues)
}

func simplexCentre(points [][]float64) []float64 {
	centre := make([]float64, len(points[0]))
	for _, point := range points {
		for k := range centre {
			centre[k] += point[k] / float64(len(points))
		}
	}
	return centre
}

func moveFromCentre(centre, worst []float64, factor float64) []float64 {
	moved := make([]float64, len(centre))
	for k := range moved {
		moved[k] = centre[k] + factor*(worst[k]-centre[k])
	}
	return moved
}
