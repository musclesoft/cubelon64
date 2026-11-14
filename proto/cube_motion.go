package main

import "math"

const (
	cubeCoefficients = 4
	cubeFitFirst     = 3
	cubeFitLast      = cubeStates - 4
	cubeMidState     = float64(cubeStates-1) / 2
)

// The cube's rotation: three rotations about fixed axes in the given order, each angle a polynomial
// of the state number, between a fixed rotation before and one after
type cubeMotion struct {
	Angles [3][cubeCoefficients]float64
	Axes   [3]int
	Post   cubeMatrix
	Pre    cubeMatrix
}

type cubeAxisOrder [3]int

type cubeAngles [3]float64

var cubeAxisOrders = []cubeAxisOrder{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}

func (motion cubeMotion) rotationAt(state float64) cubeMatrix {
	var angles cubeAngles
	for i := range angles {
		angles[i] = polynomialOfState(motion.Angles[i][:], state)
	}
	return multiplyCube(multiplyCube(motion.Pre, eulerMatrix(motion.Axes, angles)), motion.Post)
}

func polynomialOfState(coefficients []float64, state float64) float64 {
	return polynomialAt(coefficients, (state-cubeMidState)/cubeMidState)
}

func eulerMatrix(axes cubeAxisOrder, angles cubeAngles) cubeMatrix {
	rotation := cubeIdentity()
	for i, axis := range axes {
		rotation = multiplyCube(rotation, cubeAxisRotation(axis, angles[i]))
	}
	return rotation
}

func wrapAngle(angle float64) float64 {
	for angle > math.Pi {
		angle -= 2 * math.Pi
	}
	for angle <= -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

// The 24 rotations that map the cube's axes onto themselves
func cubeRotationGroup() []cubeMatrix {
	var group []cubeMatrix
	for _, order := range cubeAxisOrders {
		for signs := 0; signs < 8; signs++ {
			var matrix cubeMatrix
			for row, column := range order {
				matrix[row][column] = 1
				if signs>>row&1 == 1 {
					matrix[row][column] = -1
				}
			}
			if cubeDeterminant(matrix) > 0 {
				group = append(group, matrix)
			}
		}
	}
	return group
}

func cubeDeterminant(m cubeMatrix) float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) - m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) + m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// Fits the motion to the poses of the states: for every choice of fixed rotations (from the
// cube's group) and axis order, the angles of each pose follow in closed form and their
// rate and offset from a linear regression. The best choice then gets cubics with the
// face-on end states pinned exactly.
func fitCubeMotion(poses []cubeMatrix) cubeMotion {
	group := cubeRotationGroup()
	best := math.Inf(1)
	var motion cubeMotion
	var bestPath []cubeAngles
	for _, axes := range cubeAxisOrders {
		for _, pre := range group {
			for _, post := range group {
				path, ok := eulerPath(axes, pre, post, poses)
				if !ok {
					continue
				}
				linear := cubeMotion{Axes: axes, Post: post, Pre: pre}
				for i := range linear.Angles {
					copy(linear.Angles[i][:2], fitPolynomial(path, i, 1))
				}
				if deviation := motionError(linear, poses); deviation < best {
					best, motion, bestPath = deviation, linear, path
				}
			}
		}
	}
	for i, coefficients := range constrainedPolynomials(bestPath) {
		motion.Angles[i] = coefficients
	}
	return motion
}

func motionError(motion cubeMotion, poses []cubeMatrix) float64 {
	sum := 0.0
	for state := cubeFitFirst; state <= cubeFitLast; state++ {
		angle := cubeRotationAngle(motion.rotationAt(float64(state)), poses[state])
		sum += angle * angle
	}
	return math.Sqrt(sum / float64(cubeFitLast-cubeFitFirst+1))
}

// The continuous angle triples of the poses between the fixed rotations, over the states
// that are not too close to the face-on ends, where the outer two angles are ill-defined
func eulerPath(axes cubeAxisOrder, pre, post cubeMatrix, poses []cubeMatrix) ([]cubeAngles, bool) {
	var solutions [][]cubeAngles
	for state := cubeFitFirst; state <= cubeFitLast; state++ {
		inner := multiplyCube(multiplyCube(transposeCube(pre), poses[state]), transposeCube(post))
		options := eulerSolutions(axes, inner)
		if len(options) == 0 {
			return nil, false
		}
		solutions = append(solutions, options)
	}
	return unwrapAngles(smoothestSolutions(solutions)), true
}

func eulerSolutions(axes cubeAxisOrder, target cubeMatrix) []cubeAngles {
	first, second, third := axes[0], axes[1], axes[2]
	var found []cubeAngles
	for _, middleSign := range []float64{1, -1} {
		base := math.Asin(math.Max(-1, math.Min(1, middleSign*target[first][third])))
		for _, middle := range []float64{base, math.Pi - base} {
			for _, firstSign := range []float64{1, -1} {
				for _, firstTurn := range []float64{0, math.Pi} {
					firstAngle := wrapAngle(math.Atan2(firstSign*target[second][third], target[third][third]) + firstTurn)
					for _, lastSign := range []float64{1, -1} {
						for _, lastTurn := range []float64{0, math.Pi} {
							lastAngle := wrapAngle(math.Atan2(lastSign*target[first][second], target[first][first]) + lastTurn)
							candidate := cubeAngles{firstAngle, wrapAngle(middle), lastAngle}
							if cubeRotationAngle(eulerMatrix(axes, candidate), target) < 1e-6 && !containsAngles(found, candidate) {
								found = append(found, candidate)
							}
						}
					}
				}
			}
		}
	}
	return found
}

func containsAngles(list []cubeAngles, angles cubeAngles) bool {
	for _, other := range list {
		distance := 0.0
		for i := range angles {
			distance += math.Abs(wrapAngle(other[i] - angles[i]))
		}
		if distance < 1e-6 {
			return true
		}
	}
	return false
}

func smoothestSolutions(solutions [][]cubeAngles) []cubeAngles {
	costs := make([][]float64, len(solutions))
	origins := make([][]int, len(solutions))
	for step := range solutions {
		costs[step] = make([]float64, len(solutions[step]))
		origins[step] = make([]int, len(solutions[step]))
		if step == 0 {
			continue
		}
		for i, angles := range solutions[step] {
			best := math.Inf(1)
			for j, previous := range solutions[step-1] {
				value := costs[step-1][j] + angleDistance(angles, previous)
				if value < best {
					best, origins[step][i] = value, j
				}
			}
			costs[step][i] = best
		}
	}
	last := len(solutions) - 1
	choice := 0
	for i, value := range costs[last] {
		if value < costs[last][choice] {
			choice = i
		}
	}
	path := make([]cubeAngles, len(solutions))
	for step := last; step >= 0; step-- {
		path[step] = solutions[step][choice]
		choice = origins[step][choice]
	}
	return path
}

func angleDistance(a, b cubeAngles) float64 {
	sum := 0.0
	for i := range a {
		difference := wrapAngle(a[i] - b[i])
		sum += difference * difference
	}
	return sum
}

func unwrapAngles(path []cubeAngles) []cubeAngles {
	for step := 1; step < len(path); step++ {
		for i := range path[step] {
			path[step][i] = path[step-1][i] + wrapAngle(path[step][i]-path[step-1][i])
		}
	}
	return path
}

func pathWeight(path []cubeAngles, step, angle int) float64 {
	if angle == 1 {
		return 1
	}
	return math.Pow(math.Cos(path[step][1]), 2)
}

func pathPosition(step int) float64 {
	return (float64(step+cubeFitFirst) - cubeMidState) / cubeMidState
}

// Weighted least squares of one angle: the outer angles count less near the middle angle's
// ±90 degrees, where they are ill-defined
func fitPolynomial(path []cubeAngles, angle, degree int) []float64 {
	size := degree + 1
	system := make([][]float64, size)
	for i := range system {
		system[i] = make([]float64, size+1)
	}
	for step := range path {
		weight, x := pathWeight(path, step, angle), pathPosition(step)
		for i := 0; i < size; i++ {
			for j := 0; j < size; j++ {
				system[i][j] += weight * math.Pow(x, float64(i+j))
			}
			system[i][size] += weight * math.Pow(x, float64(i)) * path[step][angle]
		}
	}
	return solveLinear(system)
}

// Polynomials for the three angles by least squares under four linear conditions: the middle
// angle is ±90 degrees at both ends, and the outer two sum (or differ) by a multiple of 180
// degrees there, which is exactly face-on
func constrainedPolynomials(path []cubeAngles) [3][cubeCoefficients]float64 {
	var free [3][]float64
	for i := range free {
		free[i] = fitPolynomial(path, i, cubeCoefficients-1)
	}
	startSign := 1.0
	if path[0][1] < path[len(path)-1][1] {
		startSign = -1
	}
	const conditions = 4
	variables := cubeAxes * cubeCoefficients
	size := variables + conditions
	system := make([][]float64, size)
	for i := range system {
		system[i] = make([]float64, size+1)
	}
	for step := range path {
		x := pathPosition(step)
		for angle := 0; angle < cubeAxes; angle++ {
			weight := pathWeight(path, step, angle)
			for i := 0; i < cubeCoefficients; i++ {
				for j := 0; j < cubeCoefficients; j++ {
					system[cubeCoefficients*angle+i][cubeCoefficients*angle+j] += 2 * weight * math.Pow(x, float64(i+j))
				}
				system[cubeCoefficients*angle+i][size] += 2 * weight * math.Pow(x, float64(i)) * path[step][angle]
			}
		}
	}
	addCondition := func(index int, terms map[int]float64, value float64) {
		for variable, factor := range terms {
			system[variables+index][variable] = factor
			system[variable][variables+index] = factor
		}
		system[variables+index][size] = value
	}
	endTerms := func(x, outerSign float64) map[int]float64 {
		terms := map[int]float64{}
		for i := 0; i < cubeCoefficients; i++ {
			terms[i] = math.Pow(x, float64(i))
			terms[2*cubeCoefficients+i] = outerSign * math.Pow(x, float64(i))
		}
		return terms
	}
	middleTerms := func(x float64) map[int]float64 {
		terms := map[int]float64{}
		for i := 0; i < cubeCoefficients; i++ {
			terms[cubeCoefficients+i] = math.Pow(x, float64(i))
		}
		return terms
	}
	startTurns := math.Round((polynomialAt(free[0], -1) + startSign*polynomialAt(free[2], -1)) / math.Pi)
	endTurns := math.Round((polynomialAt(free[0], 1) - startSign*polynomialAt(free[2], 1)) / math.Pi)
	addCondition(0, middleTerms(-1), startSign*math.Pi/2)
	addCondition(1, middleTerms(1), -startSign*math.Pi/2)
	addCondition(2, endTerms(-1, startSign), startTurns*math.Pi)
	addCondition(3, endTerms(1, -startSign), endTurns*math.Pi)
	solution := solveLinear(system)
	var polynomials [3][cubeCoefficients]float64
	for angle := range polynomials {
		copy(polynomials[angle][:], solution[cubeCoefficients*angle:cubeCoefficients*(angle+1)])
	}
	return polynomials
}

func polynomialAt(coefficients []float64, x float64) float64 {
	value := 0.0
	for i := len(coefficients) - 1; i >= 0; i-- {
		value = value*x + coefficients[i]
	}
	return value
}

func solveLinear(system [][]float64) []float64 {
	size := len(system)
	for column := 0; column < size; column++ {
		pivot := column
		for row := column + 1; row < size; row++ {
			if math.Abs(system[row][column]) > math.Abs(system[pivot][column]) {
				pivot = row
			}
		}
		system[column], system[pivot] = system[pivot], system[column]
		for row := 0; row < size; row++ {
			if row == column {
				continue
			}
			factor := system[row][column] / system[column][column]
			for k := column; k <= size; k++ {
				system[row][k] -= factor * system[column][k]
			}
		}
	}
	solution := make([]float64, size)
	for i := range solution {
		solution[i] = system[i][size] / system[i][i]
	}
	return solution
}
