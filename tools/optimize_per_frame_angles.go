package main

import (
	"fmt"
	"image/png"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 8.8 fixed-point type
type Fixed88 int16

func toFixed88(f float64) Fixed88 {
	return Fixed88(f * 256.0)
}

func (f Fixed88) toFloat() float64 {
	return float64(f) / 256.0
}

// Point in 8.8 fixed-point
type Point struct {
	X, Y Fixed88
}

// Parameter vector with per-frame angles
type Params struct {
	Size    Fixed88
	CenterX Fixed88
	CenterY Fixed88
	PivotX  Fixed88
	PivotY  Fixed88
	Angles  [11]float64 // Per-frame angles (not fixed steps)
}

// Particle for PSO
type Particle struct {
	Position     Params
	Velocity     Params
	BestPosition Params
	BestScore    float64
}

// Load pattern from PNG
func loadPattern(filename string) ([][]bool, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	scale := bounds.Dx() / 24

	pattern := make([][]bool, 24)
	for y := 0; y < 24; y++ {
		pattern[y] = make([]bool, 24)
		for x := 0; x < 24; x++ {
			r, _, _, _ := img.At(x*scale+scale/2, y*scale+scale/2).RGBA()
			pattern[y][x] = (r < 32768)
		}
	}
	return pattern, nil
}

// Rotate point around pivot
func rotateFixed88(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)

	rxTemp := dx*cos256 - dy*sin256
	ryTemp := dx*sin256 + dy*cos256

	var rx, ry int32
	if rxTemp >= 0 {
		rx = (rxTemp + 128) >> 8
	} else {
		rx = (rxTemp - 128) >> 8
	}
	if ryTemp >= 0 {
		ry = (ryTemp + 128) >> 8
	} else {
		ry = (ryTemp - 128) >> 8
	}

	return Point{
		X: pivot.X + Fixed88(rx),
		Y: pivot.Y + Fixed88(ry),
	}
}

// Render quadrilateral
func renderQuad(corners [4]Point) [][]bool {
	grid := make([][]bool, 24)
	for i := range grid {
		grid[i] = make([]bool, 24)
	}

	minY, maxY := 24*256, -1
	for _, c := range corners {
		y := int(c.Y)
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	for y := minY >> 8; y <= (maxY>>8) && y < 24; y++ {
		if y < 0 {
			continue
		}
		yFixed := Fixed88(y << 8)

		var intersections []int
		for i := 0; i < 4; i++ {
			p1 := corners[i]
			p2 := corners[(i+1)%4]

			if (p1.Y <= yFixed && p2.Y > yFixed) || (p2.Y <= yFixed && p1.Y > yFixed) {
				dy := int32(p2.Y - p1.Y)
				if dy != 0 {
					t := int32(yFixed-p1.Y) * 256 / dy
					x := int32(p1.X) + (int32(p2.X-p1.X)*t)/256
					intersections = append(intersections, int(x))
				}
			}
		}

		for i := 0; i < len(intersections)-1; i++ {
			for j := i + 1; j < len(intersections); j++ {
				if intersections[i] > intersections[j] {
					intersections[i], intersections[j] = intersections[j], intersections[i]
				}
			}
		}

		for i := 0; i+1 < len(intersections); i += 2 {
			x1 := (intersections[i] + 128) >> 8
			x2 := (intersections[i+1] + 128) >> 8
			for x := x1; x < x2; x++ {
				if x >= 0 && x < 24 {
					grid[y][x] = true
				} else if x >= 24 {
					grid[y][x%24] = true
				}
			}
		}
	}

	return grid
}

// Compare patterns
func comparePatterns(a, b [][]bool) float64 {
	matches := 0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if a[y][x] == b[y][x] {
				matches++
			}
		}
	}
	return float64(matches) / 576.0 * 100.0
}

// Count perfect frames
func countPerfectFrames(p Params, targets [11][][]bool) int {
	half := p.Size / 2
	cx := p.CenterX
	cy := p.CenterY
	initial := [4]Point{
		{cx - half, cy - half},
		{cx + half, cy - half},
		{cx + half, cy + half},
		{cx - half, cy + half},
	}

	pivot := Point{p.PivotX, p.PivotY}
	perfectCount := 0

	for frame := 0; frame < 11; frame++ {
		angle := p.Angles[frame]
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFixed88(initial[i], pivot, cos256, sin256)
		}

		rendered := renderQuad(rotated)
		accuracy := comparePatterns(rendered, targets[frame])
		if accuracy == 100.0 {
			perfectCount++
		}
	}

	return perfectCount
}

// Global target patterns
var targets [11][][]bool

// Evaluate parameters
func evaluateParams(p Params) float64 {
	half := p.Size / 2
	cx := p.CenterX
	cy := p.CenterY
	initial := [4]Point{
		{cx - half, cy - half},
		{cx + half, cy - half},
		{cx + half, cy + half},
		{cx - half, cy + half},
	}

	pivot := Point{p.PivotX, p.PivotY}
	totalAccuracy := 0.0

	for frame := 0; frame < 11; frame++ {
		angle := p.Angles[frame]
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFixed88(initial[i], pivot, cos256, sin256)
		}

		rendered := renderQuad(rotated)
		accuracy := comparePatterns(rendered, targets[frame])
		totalAccuracy += accuracy
	}

	return totalAccuracy / 11.0
}

// PSO with per-frame angle optimization
func particleSwarmOptimization(numParticles int, maxIterations int) Params {
	particles := make([]Particle, numParticles)

	// Best known spatial parameters
	baseSize := Fixed88(3065)
	baseCenterX := Fixed88(3010)
	baseCenterY := Fixed88(2954)
	basePivotX := Fixed88(4578)
	basePivotY := Fixed88(4552)

	// Initialize particles with small variations
	baseAngleStep := math.Pi / 20.0 // 9 degrees
	for i := range particles {
		particles[i].Position = Params{
			Size:    baseSize + Fixed88(rand.Intn(21)-10),
			CenterX: baseCenterX + Fixed88(rand.Intn(21)-10),
			CenterY: baseCenterY + Fixed88(rand.Intn(21)-10),
			PivotX:  basePivotX + Fixed88(rand.Intn(21)-10),
			PivotY:  basePivotY + Fixed88(rand.Intn(21)-10),
		}
		// Initialize angles with small random deviations from 9° steps
		for frame := 0; frame < 11; frame++ {
			idealAngle := float64(frame) * baseAngleStep
			particles[i].Position.Angles[frame] = idealAngle + (rand.Float64()-0.5)*0.05
		}

		// Initialize velocity
		particles[i].Velocity = Params{
			Size:    Fixed88(rand.Intn(5) - 2),
			CenterX: Fixed88(rand.Intn(5) - 2),
			CenterY: Fixed88(rand.Intn(5) - 2),
			PivotX:  Fixed88(rand.Intn(5) - 2),
			PivotY:  Fixed88(rand.Intn(5) - 2),
		}
		for frame := 0; frame < 11; frame++ {
			particles[i].Velocity.Angles[frame] = (rand.Float64() - 0.5) * 0.01
		}

		particles[i].BestPosition = particles[i].Position
		particles[i].BestScore = evaluateParams(particles[i].Position)
	}

	// Find global best
	globalBest := particles[0].BestPosition
	globalBestScore := particles[0].BestScore
	for i := 1; i < numParticles; i++ {
		if particles[i].BestScore > globalBestScore {
			globalBest = particles[i].BestPosition
			globalBestScore = particles[i].BestScore
		}
	}

	fmt.Printf("Initial best: %.4f%%\n", globalBestScore)

	// PSO parameters
	w := 0.7
	c1 := 1.4
	c2 := 1.4

	type EvalJob struct {
		id     int
		params Params
	}
	type EvalResult struct {
		id    int
		score float64
	}

	numWorkers := runtime.NumCPU()

	for iter := 0; iter < maxIterations; iter++ {
		jobs := make(chan EvalJob, numParticles)
		results := make(chan EvalResult, numParticles)

		var wg sync.WaitGroup
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					score := evaluateParams(job.params)
					results <- EvalResult{job.id, score}
				}
			}()
		}

		// Update velocities and positions
		for i := range particles {
			r1, r2 := rand.Float64(), rand.Float64()

			// Update spatial parameters
			particles[i].Velocity.Size = Fixed88(float64(particles[i].Velocity.Size)*w +
				c1*r1*float64(particles[i].BestPosition.Size-particles[i].Position.Size) +
				c2*r2*float64(globalBest.Size-particles[i].Position.Size))
			particles[i].Velocity.CenterX = Fixed88(float64(particles[i].Velocity.CenterX)*w +
				c1*r1*float64(particles[i].BestPosition.CenterX-particles[i].Position.CenterX) +
				c2*r2*float64(globalBest.CenterX-particles[i].Position.CenterX))
			particles[i].Velocity.CenterY = Fixed88(float64(particles[i].Velocity.CenterY)*w +
				c1*r1*float64(particles[i].BestPosition.CenterY-particles[i].Position.CenterY) +
				c2*r2*float64(globalBest.CenterY-particles[i].Position.CenterY))
			particles[i].Velocity.PivotX = Fixed88(float64(particles[i].Velocity.PivotX)*w +
				c1*r1*float64(particles[i].BestPosition.PivotX-particles[i].Position.PivotX) +
				c2*r2*float64(globalBest.PivotX-particles[i].Position.PivotX))
			particles[i].Velocity.PivotY = Fixed88(float64(particles[i].Velocity.PivotY)*w +
				c1*r1*float64(particles[i].BestPosition.PivotY-particles[i].Position.PivotY) +
				c2*r2*float64(globalBest.PivotY-particles[i].Position.PivotY))

			// Update angle velocities
			for frame := 0; frame < 11; frame++ {
				particles[i].Velocity.Angles[frame] = w*particles[i].Velocity.Angles[frame] +
					c1*r1*(particles[i].BestPosition.Angles[frame]-particles[i].Position.Angles[frame]) +
					c2*r2*(globalBest.Angles[frame]-particles[i].Position.Angles[frame])
			}

			// Update positions
			particles[i].Position.Size += particles[i].Velocity.Size
			particles[i].Position.CenterX += particles[i].Velocity.CenterX
			particles[i].Position.CenterY += particles[i].Velocity.CenterY
			particles[i].Position.PivotX += particles[i].Velocity.PivotX
			particles[i].Position.PivotY += particles[i].Velocity.PivotY
			for frame := 0; frame < 11; frame++ {
				particles[i].Position.Angles[frame] += particles[i].Velocity.Angles[frame]
			}

			// Clamp spatial parameters
			if particles[i].Position.Size < 3000 {
				particles[i].Position.Size = 3000
			} else if particles[i].Position.Size > 3100 {
				particles[i].Position.Size = 3100
			}
			if particles[i].Position.CenterX < 2950 {
				particles[i].Position.CenterX = 2950
			} else if particles[i].Position.CenterX > 3050 {
				particles[i].Position.CenterX = 3050
			}
			if particles[i].Position.CenterY < 2900 {
				particles[i].Position.CenterY = 2900
			} else if particles[i].Position.CenterY > 3000 {
				particles[i].Position.CenterY = 3000
			}
			if particles[i].Position.PivotX < 4530 {
				particles[i].Position.PivotX = 4530
			} else if particles[i].Position.PivotX > 4630 {
				particles[i].Position.PivotX = 4630
			}
			if particles[i].Position.PivotY < 4500 {
				particles[i].Position.PivotY = 4500
			} else if particles[i].Position.PivotY > 4600 {
				particles[i].Position.PivotY = 4600
			}

			// Clamp angles to reasonable range (0-90 degrees ± some tolerance)
			for frame := 0; frame < 11; frame++ {
				minAngle := float64(frame)*baseAngleStep - 0.1
				maxAngle := float64(frame)*baseAngleStep + 0.1
				if particles[i].Position.Angles[frame] < minAngle {
					particles[i].Position.Angles[frame] = minAngle
				} else if particles[i].Position.Angles[frame] > maxAngle {
					particles[i].Position.Angles[frame] = maxAngle
				}
			}

			jobs <- EvalJob{i, particles[i].Position}
		}
		close(jobs)

		go func() {
			wg.Wait()
			close(results)
		}()

		scores := make([]float64, numParticles)
		for result := range results {
			scores[result.id] = result.score
		}

		improved := false
		for i := range particles {
			if scores[i] > particles[i].BestScore {
				particles[i].BestScore = scores[i]
				particles[i].BestPosition = particles[i].Position

				if scores[i] > globalBestScore {
					globalBest = particles[i].Position
					globalBestScore = scores[i]
					improved = true
				}
			}
		}

		if improved || iter%10 == 0 {
			perfectCount := countPerfectFrames(globalBest, targets)
			fmt.Printf("Iteration %3d: best=%.4f%% (%d/11 perfect)\n",
				iter+1, globalBestScore, perfectCount)
		}

		w = 0.9 - (0.4 * float64(iter) / float64(maxIterations))
	}

	return globalBest
}

func main() {
	rand.Seed(time.Now().UnixNano())
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🎯 Per-Frame Angle Optimization")
	fmt.Printf("   Using %d CPU cores\n", runtime.NumCPU())
	fmt.Println("   Optimizing each frame's angle independently")
	fmt.Println()

	fmt.Println("Loading target patterns...")
	for i := 0; i < 11; i++ {
		pattern, err := loadPattern(fmt.Sprintf("../emulator/frames/cycle_%02d.png", i+1))
		if err != nil {
			fmt.Printf("Error loading pattern %d: %v\n", i+1, err)
			return
		}
		targets[i] = pattern
	}
	fmt.Println("✓ Loaded 11 patterns\n")

	numParticles := runtime.NumCPU() * 8
	maxIterations := 200

	fmt.Printf("Running PSO with %d particles for %d iterations...\n\n", numParticles, maxIterations)
	bestParams := particleSwarmOptimization(numParticles, maxIterations)

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("🎉 OPTIMAL PARAMETERS WITH PER-FRAME ANGLES")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Size:    %d → %.6f\n", bestParams.Size, bestParams.Size.toFloat())
	fmt.Printf("CenterX: %d → %.6f\n", bestParams.CenterX, bestParams.CenterX.toFloat())
	fmt.Printf("CenterY: %d → %.6f\n", bestParams.CenterY, bestParams.CenterY.toFloat())
	fmt.Printf("PivotX:  %d → %.6f\n", bestParams.PivotX, bestParams.PivotX.toFloat())
	fmt.Printf("PivotY:  %d → %.6f\n", bestParams.PivotY, bestParams.PivotY.toFloat())
	fmt.Println()

	finalScore := evaluateParams(bestParams)
	perfectCount := countPerfectFrames(bestParams, targets)
	fmt.Printf("Overall accuracy: %.6f%% (%d/11 perfect frames)\n", finalScore, perfectCount)
	fmt.Println()

	fmt.Println("Per-frame angles:")
	baseAngleStep := math.Pi / 20.0
	for frame := 0; frame < 11; frame++ {
		idealAngle := float64(frame) * baseAngleStep
		deviation := bestParams.Angles[frame] - idealAngle
		fmt.Printf("  Frame %02d: %.6f rad (%.3f°) [ideal: %.3f°, deviation: %+.3f°]\n",
			frame+1,
			bestParams.Angles[frame],
			bestParams.Angles[frame]*180/math.Pi,
			idealAngle*180/math.Pi,
			deviation*180/math.Pi)
	}
}
