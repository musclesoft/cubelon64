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

// Parameter vector (using integer 8.8 values internally)
type Params struct {
	Size    Fixed88 // 8.8 fixed-point
	CenterX Fixed88
	CenterY Fixed88
	PivotX  Fixed88
	PivotY  Fixed88
	Angle0  float64 // Keep as float for now
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

// Rotate point around pivot using 8.8 fixed-point with rounding
func rotateFixed88(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)

	rxTemp := dx*cos256 - dy*sin256
	ryTemp := dx*sin256 + dy*cos256

	// Round to nearest
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

// Render quadrilateral to 24x24 grid
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
			// Round intersections to nearest pixel
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
	angleStep := (math.Pi / 2.0) / 10.0

	for frame := 0; frame < 11; frame++ {
		angle := p.Angle0 + float64(frame)*angleStep
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

// Particle Swarm Optimization constrained to 8.8 fixed-point
func particleSwarmOptimization(numParticles int, maxIterations int) Params {
	particles := make([]Particle, numParticles)

	// Initialize particles with 8.8 fixed-point values
	for i := range particles {
		particles[i].Position = Params{
			Size:    Fixed88(2900 + rand.Intn(300)),   // ~11.3-12.5
			CenterX: Fixed88(2900 + rand.Intn(300)),   // ~11.3-12.5
			CenterY: Fixed88(2800 + rand.Intn(400)),   // ~10.9-12.5
			PivotX:  Fixed88(4500 + rand.Intn(250)),   // ~17.6-18.6
			PivotY:  Fixed88(4500 + rand.Intn(250)),   // ~17.6-18.6
			Angle0:  (rand.Float64() - 0.5) * 0.1,     // -0.05 to 0.05 radians
		}
		particles[i].Velocity = Params{
			Size:    Fixed88((rand.Intn(21) - 10) * 2),  // -20 to +20 in 8.8 units
			CenterX: Fixed88((rand.Intn(21) - 10) * 2),
			CenterY: Fixed88((rand.Intn(21) - 10) * 2),
			PivotX:  Fixed88((rand.Intn(21) - 10) * 2),
			PivotY:  Fixed88((rand.Intn(21) - 10) * 2),
			Angle0:  (rand.Float64() - 0.5) * 0.02,
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
	w := 0.7  // inertia
	c1 := 1.4 // cognitive
	c2 := 1.4 // social

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

			// Update velocity (in 8.8 units for fixed-point params)
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
			particles[i].Velocity.Angle0 = w*particles[i].Velocity.Angle0 +
				c1*r1*(particles[i].BestPosition.Angle0-particles[i].Position.Angle0) +
				c2*r2*(globalBest.Angle0-particles[i].Position.Angle0)

			// Update position (keep as 8.8 integers)
			particles[i].Position.Size += particles[i].Velocity.Size
			particles[i].Position.CenterX += particles[i].Velocity.CenterX
			particles[i].Position.CenterY += particles[i].Velocity.CenterY
			particles[i].Position.PivotX += particles[i].Velocity.PivotX
			particles[i].Position.PivotY += particles[i].Velocity.PivotY
			particles[i].Position.Angle0 += particles[i].Velocity.Angle0

			// Clamp to reasonable bounds
			if particles[i].Position.Size < 2700 {
				particles[i].Position.Size = 2700
			} else if particles[i].Position.Size > 3400 {
				particles[i].Position.Size = 3400
			}
			if particles[i].Position.CenterX < 2700 {
				particles[i].Position.CenterX = 2700
			} else if particles[i].Position.CenterX > 3400 {
				particles[i].Position.CenterX = 3400
			}
			if particles[i].Position.CenterY < 2600 {
				particles[i].Position.CenterY = 2600
			} else if particles[i].Position.CenterY > 3400 {
				particles[i].Position.CenterY = 3400
			}
			if particles[i].Position.PivotX < 4300 {
				particles[i].Position.PivotX = 4300
			} else if particles[i].Position.PivotX > 4900 {
				particles[i].Position.PivotX = 4900
			}
			if particles[i].Position.PivotY < 4300 {
				particles[i].Position.PivotY = 4300
			} else if particles[i].Position.PivotY > 4900 {
				particles[i].Position.PivotY = 4900
			}
			if particles[i].Position.Angle0 < -0.1 {
				particles[i].Position.Angle0 = -0.1
			} else if particles[i].Position.Angle0 > 0.1 {
				particles[i].Position.Angle0 = 0.1
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
			fmt.Printf("Iteration %3d: best=%.4f%% (size=%d/%.6f center=(%d,%d)/%.6f,%.6f pivot=(%d,%d)/%.6f,%.6f angle=%.6f)\n",
				iter+1, globalBestScore,
				globalBest.Size, globalBest.Size.toFloat(),
				globalBest.CenterX, globalBest.CenterY, globalBest.CenterX.toFloat(), globalBest.CenterY.toFloat(),
				globalBest.PivotX, globalBest.PivotY, globalBest.PivotX.toFloat(), globalBest.PivotY.toFloat(),
				globalBest.Angle0)
		}

		// Adaptive inertia
		w = 0.8 - (0.3 * float64(iter) / float64(maxIterations))
	}

	return globalBest
}

func main() {
	rand.Seed(time.Now().UnixNano())
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🎯 8.8 Fixed-Point Optimization (PSO)")
	fmt.Printf("   Using %d CPU cores\n", runtime.NumCPU())
	fmt.Println("   Target: 11 frames (cycle_01.png - cycle_11.png)")
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

	numParticles := runtime.NumCPU() * 16
	maxIterations := 500

	fmt.Printf("Running PSO with %d particles for %d iterations...\n\n", numParticles, maxIterations)
	bestParams := particleSwarmOptimization(numParticles, maxIterations)

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("🎉 OPTIMAL 8.8 FIXED-POINT PARAMETERS")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Size:    8.8 value %d → %.10f pixels\n", bestParams.Size, bestParams.Size.toFloat())
	fmt.Printf("CenterX: 8.8 value %d → %.10f\n", bestParams.CenterX, bestParams.CenterX.toFloat())
	fmt.Printf("CenterY: 8.8 value %d → %.10f\n", bestParams.CenterY, bestParams.CenterY.toFloat())
	fmt.Printf("PivotX:  8.8 value %d → %.10f\n", bestParams.PivotX, bestParams.PivotX.toFloat())
	fmt.Printf("PivotY:  8.8 value %d → %.10f\n", bestParams.PivotY, bestParams.PivotY.toFloat())
	fmt.Printf("Angle0:  %.10f radians (%.6f degrees)\n", bestParams.Angle0, bestParams.Angle0*180/math.Pi)

	finalScore := evaluateParams(bestParams)
	fmt.Printf("Overall accuracy: %.6f%%\n", finalScore)
	fmt.Println()

	fmt.Println("Per-frame accuracy:")
	half := bestParams.Size / 2
	initial := [4]Point{
		{bestParams.CenterX - half, bestParams.CenterY - half},
		{bestParams.CenterX + half, bestParams.CenterY - half},
		{bestParams.CenterX + half, bestParams.CenterY + half},
		{bestParams.CenterX - half, bestParams.CenterY + half},
	}
	pivot := Point{bestParams.PivotX, bestParams.PivotY}
	angleStep := (math.Pi / 2.0) / 10.0

	for frame := 0; frame < 11; frame++ {
		angle := bestParams.Angle0 + float64(frame)*angleStep
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
		status := " "
		if accuracy >= 99.0 {
			status = "✓"
		}
		fmt.Printf("  Frame %02d: %6.2f%% (angle: %7.3f°) %s\n",
			frame+1, accuracy, angle*180/math.Pi, status)
	}
}
