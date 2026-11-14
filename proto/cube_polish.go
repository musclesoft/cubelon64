package main

import (
	"fmt"
	"math"
	"sort"
)

const (
	cubePolishIterations = 700
	cubeRenderSoftLimit  = 4.0
)

var cubePolishScales = []float64{0.03, 0.01, 0.003}

var cubeRenderPolishScales = []float64{0.01, 0.003}

// What one unit of the initial simplex step means for each parameter: the 8 free cubic
// coefficients (radians), then the camera's centre, half size and perspective
var cubeParameterUnits = [12]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0.04}

// What the face-on end states fix: the middle angle at both ends, and the sum (or difference)
// of the outer two there
type cubeEnds struct {
	endSum, middleEnd, middleStart, outerSign, startSum float64
}

func cubeEndsOf(motion cubeMotion) cubeEnds {
	first, last := 0.0, float64(cubeStates-1)
	sign := 1.0
	if cubicAt(motion.Angles[1], first) < 0 {
		sign = -1
	}
	return cubeEnds{
		endSum:      cubicAt(motion.Angles[0], last) - sign*cubicAt(motion.Angles[2], last),
		middleEnd:   cubicAt(motion.Angles[1], last),
		middleStart: cubicAt(motion.Angles[1], first),
		outerSign:   sign,
		startSum:    cubicAt(motion.Angles[0], first) + sign*cubicAt(motion.Angles[2], first),
	}
}

// A cubic with the given values at the two ends and the given squared and cubed terms
func pinnedCubic(squared, cubed, start, end float64) [4]float64 {
	return [4]float64{(start+end)/2 - squared, (end-start)/2 - cubed, squared, cubed}
}

// The free parameters: the squared and cubed terms of the first two angles, all four
// coefficients of the third, and the camera
func cubeFitVector(fit cubeFit) []float64 {
	angles := fit.Motion.Angles
	vector := []float64{angles[0][2], angles[0][3], angles[1][2], angles[1][3]}
	vector = append(vector, angles[2][:]...)
	camera := fit.Camera
	return append(vector, camera.CentreX, camera.CentreY, camera.HalfSize, camera.Perspective)
}

func cubeFitOfVector(template cubeFit, ends cubeEnds, vector []float64) cubeFit {
	fit := template
	last := float64(cubeStates - 1)
	copy(fit.Motion.Angles[2][:], vector[4:8])
	third := fit.Motion.Angles[2]
	start := ends.startSum - ends.outerSign*cubicAt(third, 0)
	end := ends.endSum + ends.outerSign*cubicAt(third, last)
	fit.Motion.Angles[0] = pinnedCubic(vector[0], vector[1], start, end)
	fit.Motion.Angles[1] = pinnedCubic(vector[2], vector[3], ends.middleStart, ends.middleEnd)
	fit.Camera = cubeCamera{CentreX: vector[8], CentreY: vector[9], HalfSize: vector[10], Perspective: vector[11]}
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
		return cubeRenderCost(candidate, targets) + 0.5*cubeFitCost(candidate, targets)
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
			steps[i] = scale * cubeParameterUnits[i]
		}
		vector, value = nelderMead(cost, vector, steps, cubePolishIterations)
		fmt.Printf("Polish step %.3f: cost %.1f\n", scale, value)
	}
	return cubeFitOfVector(fit, ends, vector)
}

// The validator's mismatches of the C64 render summed over the states, the states above a
// few mismatches counting extra since the block limit applies to the worst one
func cubeRenderCost(fit cubeFit, targets []morphTile) float64 {
	weights := make([]float64, len(targets))
	parallelStates(len(targets), func(state int) {
		levels := fit.Camera.subpixelLevels(fit.Motion.rotationAt(float64(state)))
		weights[state] = cubeValidatorMismatches(&levels, &targets[state])
	})
	total := 0.0
	for _, weight := range weights {
		total += weight + 3*math.Max(0, weight-cubeRenderSoftLimit)
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
