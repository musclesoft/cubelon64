package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Point in pure floating-point
type Point struct {
	X, Y float64
}

// Parameter vector
type Params struct {
	Size    float64
	CenterX float64
	CenterY float64
	PivotX  float64
	PivotY  float64
	Angle0  float64
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

// Rotate point around pivot using pure floating-point
func rotateFloat(p Point, pivot Point, cos, sin float64) Point {
	dx := p.X - pivot.X
	dy := p.Y - pivot.Y
	return Point{
		X: pivot.X + dx*cos - dy*sin,
		Y: pivot.Y + dx*sin + dy*cos,
	}
}

// Render quadrilateral to 24x24 grid using floating-point scanline
func renderQuad(corners [4]Point) [][]bool {
	grid := make([][]bool, 24)
	for i := range grid {
		grid[i] = make([]bool, 24)
	}

	minY, maxY := 1e9, -1e9
	for _, c := range corners {
		if c.Y < minY {
			minY = c.Y
		}
		if c.Y > maxY {
			maxY = c.Y
		}
	}

	for y := int(math.Floor(minY)); y <= int(math.Floor(maxY)) && y < 24; y++ {
		if y < 0 {
			continue
		}
		yFloat := float64(y) + 0.5 // Center of pixel row

		var intersections []float64
		for i := 0; i < 4; i++ {
			p1 := corners[i]
			p2 := corners[(i+1)%4]

			if (p1.Y <= yFloat && p2.Y > yFloat) || (p2.Y <= yFloat && p1.Y > yFloat) {
				dy := p2.Y - p1.Y
				if math.Abs(dy) > 1e-10 {
					t := (yFloat - p1.Y) / dy
					x := p1.X + (p2.X-p1.X)*t
					intersections = append(intersections, x)
				}
			}
		}

		// Sort intersections
		for i := 0; i < len(intersections)-1; i++ {
			for j := i + 1; j < len(intersections); j++ {
				if intersections[i] > intersections[j] {
					intersections[i], intersections[j] = intersections[j], intersections[i]
				}
			}
		}

		// Fill between pairs
		for i := 0; i+1 < len(intersections); i += 2 {
			x1 := int(math.Round(intersections[i]))
			x2 := int(math.Round(intersections[i+1]))
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
	half := p.Size / 2.0
	initial := [4]Point{
		{p.CenterX - half, p.CenterY - half},
		{p.CenterX + half, p.CenterY - half},
		{p.CenterX + half, p.CenterY + half},
		{p.CenterX - half, p.CenterY + half},
	}

	pivot := Point{p.PivotX, p.PivotY}
	totalAccuracy := 0.0
	angleStep := (math.Pi / 2.0) / 10.0 // 10 steps for 11 frames

	for frame := 0; frame < 11; frame++ {
		angle := p.Angle0 + float64(frame)*angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFloat(initial[i], pivot, cos, sin)
		}

		rendered := renderQuad(rotated)
		accuracy := comparePatterns(rendered, targets[frame])
		totalAccuracy += accuracy
	}

	return totalAccuracy / 11.0
}

// Particle Swarm Optimization with parallel evaluation
func particleSwarmOptimization(numParticles int, maxIterations int) Params {
	particles := make([]Particle, numParticles)

	// Initialize particles with reasonable ranges
	for i := range particles {
		particles[i].Position = Params{
			Size:    11.5 + rand.Float64(),      // 11.5-12.5
			CenterX: 11.5 + rand.Float64(),      // 11.5-12.5
			CenterY: 11.5 + rand.Float64(),      // 11.5-12.5
			PivotX:  18.0 + rand.Float64(),      // 18.0-19.0
			PivotY:  17.5 + rand.Float64(),      // 17.5-18.5
			Angle0:  (rand.Float64() - 0.5) * 0.2, // -0.1 to 0.1 radians
		}
		particles[i].Velocity = Params{
			Size:    (rand.Float64() - 0.5) * 0.2,
			CenterX: (rand.Float64() - 0.5) * 0.5,
			CenterY: (rand.Float64() - 0.5) * 0.5,
			PivotX:  (rand.Float64() - 0.5) * 0.5,
			PivotY:  (rand.Float64() - 0.5) * 0.5,
			Angle0:  (rand.Float64() - 0.5) * 0.1,
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
	c1 := 1.5 // cognitive
	c2 := 1.5 // social

	// Parallel worker function
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
		// Parallel evaluation
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

			// Update velocity
			particles[i].Velocity.Size = w*particles[i].Velocity.Size +
				c1*r1*(particles[i].BestPosition.Size-particles[i].Position.Size) +
				c2*r2*(globalBest.Size-particles[i].Position.Size)
			particles[i].Velocity.CenterX = w*particles[i].Velocity.CenterX +
				c1*r1*(particles[i].BestPosition.CenterX-particles[i].Position.CenterX) +
				c2*r2*(globalBest.CenterX-particles[i].Position.CenterX)
			particles[i].Velocity.CenterY = w*particles[i].Velocity.CenterY +
				c1*r1*(particles[i].BestPosition.CenterY-particles[i].Position.CenterY) +
				c2*r2*(globalBest.CenterY-particles[i].Position.CenterY)
			particles[i].Velocity.PivotX = w*particles[i].Velocity.PivotX +
				c1*r1*(particles[i].BestPosition.PivotX-particles[i].Position.PivotX) +
				c2*r2*(globalBest.PivotX-particles[i].Position.PivotX)
			particles[i].Velocity.PivotY = w*particles[i].Velocity.PivotY +
				c1*r1*(particles[i].BestPosition.PivotY-particles[i].Position.PivotY) +
				c2*r2*(globalBest.PivotY-particles[i].Position.PivotY)
			particles[i].Velocity.Angle0 = w*particles[i].Velocity.Angle0 +
				c1*r1*(particles[i].BestPosition.Angle0-particles[i].Position.Angle0) +
				c2*r2*(globalBest.Angle0-particles[i].Position.Angle0)

			// Update position
			particles[i].Position.Size += particles[i].Velocity.Size
			particles[i].Position.CenterX += particles[i].Velocity.CenterX
			particles[i].Position.CenterY += particles[i].Velocity.CenterY
			particles[i].Position.PivotX += particles[i].Velocity.PivotX
			particles[i].Position.PivotY += particles[i].Velocity.PivotY
			particles[i].Position.Angle0 += particles[i].Velocity.Angle0

			// Clamp to reasonable bounds
			particles[i].Position.Size = math.Max(10.0, math.Min(14.0, particles[i].Position.Size))
			particles[i].Position.CenterX = math.Max(10.0, math.Min(14.0, particles[i].Position.CenterX))
			particles[i].Position.CenterY = math.Max(10.0, math.Min(14.0, particles[i].Position.CenterY))
			particles[i].Position.PivotX = math.Max(16.0, math.Min(20.0, particles[i].Position.PivotX))
			particles[i].Position.PivotY = math.Max(16.0, math.Min(20.0, particles[i].Position.PivotY))
			particles[i].Position.Angle0 = math.Max(-0.2, math.Min(0.2, particles[i].Position.Angle0))

			// Send for evaluation
			jobs <- EvalJob{i, particles[i].Position}
		}
		close(jobs)

		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results
		scores := make([]float64, numParticles)
		for result := range results {
			scores[result.id] = result.score
		}

		// Update personal and global bests
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
			fmt.Printf("Iteration %3d: best=%.4f%% (size=%.6f center=(%.6f,%.6f) pivot=(%.6f,%.6f) angle=%.6f)\n",
				iter+1, globalBestScore,
				globalBest.Size, globalBest.CenterX, globalBest.CenterY,
				globalBest.PivotX, globalBest.PivotY, globalBest.Angle0)
		}

		// Adaptive inertia
		w = 0.9 - (0.5 * float64(iter) / float64(maxIterations))
	}

	return globalBest
}

func main() {
	rand.Seed(time.Now().UnixNano())
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🎯 Pure Floating-Point Optimization (PSO)")
	fmt.Printf("   Using %d CPU cores\n", runtime.NumCPU())
	fmt.Println("   Target: 11 frames (cycle_01.png - cycle_11.png)")
	fmt.Println()

	// Load all target patterns
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

	// Run PSO
	numParticles := runtime.NumCPU() * 8
	maxIterations := 300

	fmt.Printf("Running PSO with %d particles for %d iterations...\n\n", numParticles, maxIterations)
	bestParams := particleSwarmOptimization(numParticles, maxIterations)

	// Final evaluation
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("🎉 OPTIMAL FLOATING-POINT PARAMETERS")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Square size:      %.10f pixels\n", bestParams.Size)
	fmt.Printf("Center position:  (%.10f, %.10f)\n", bestParams.CenterX, bestParams.CenterY)
	fmt.Printf("Pivot position:   (%.10f, %.10f)\n", bestParams.PivotX, bestParams.PivotY)
	fmt.Printf("Pivot offset:     (+%.10f, +%.10f) from center\n",
		bestParams.PivotX-bestParams.CenterX, bestParams.PivotY-bestParams.CenterY)
	fmt.Printf("Initial angle:    %.10f radians (%.6f degrees)\n",
		bestParams.Angle0, bestParams.Angle0*180/math.Pi)

	finalScore := evaluateParams(bestParams)
	fmt.Printf("Overall accuracy: %.6f%%\n", finalScore)
	fmt.Println()

	// Show per-frame accuracy
	fmt.Println("Per-frame accuracy:")
	half := bestParams.Size / 2.0
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

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFloat(initial[i], pivot, cos, sin)
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

	// Show suggestions for 8.8 fixed-point encoding
	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("SUGGESTED 8.8 FIXED-POINT ENCODINGS")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Size:    %.10f → 8.8: %d (%.10f)\n",
		bestParams.Size, int(bestParams.Size*256), float64(int(bestParams.Size*256))/256.0)
	fmt.Printf("CenterX: %.10f → 8.8: %d (%.10f)\n",
		bestParams.CenterX, int(bestParams.CenterX*256), float64(int(bestParams.CenterX*256))/256.0)
	fmt.Printf("CenterY: %.10f → 8.8: %d (%.10f)\n",
		bestParams.CenterY, int(bestParams.CenterY*256), float64(int(bestParams.CenterY*256))/256.0)
	fmt.Printf("PivotX:  %.10f → 8.8: %d (%.10f)\n",
		bestParams.PivotX, int(bestParams.PivotX*256), float64(int(bestParams.PivotX*256))/256.0)
	fmt.Printf("PivotY:  %.10f → 8.8: %d (%.10f)\n",
		bestParams.PivotY, int(bestParams.PivotY*256), float64(int(bestParams.PivotY*256))/256.0)
	fmt.Printf("Angle0:  %.10f rad → scale to fit range (%.6f°)\n",
		bestParams.Angle0, bestParams.Angle0*180/math.Pi)

	// Generate debug PNG
	fmt.Println()
	fmt.Println("Generating debug PNG...")

	scale := 8
	margin := 2
	frameWidth := 24 * scale
	frameHeight := 24 * scale
	cols := 3 // target, rendered, diff
	rows := 11

	imgWidth := cols*frameWidth + (cols+1)*margin
	imgHeight := rows*frameHeight + (rows+1)*margin

	debugImg := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	// Fill background with dark gray
	bgColor := color.RGBA{40, 40, 40, 255}
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			debugImg.Set(x, y, bgColor)
		}
	}

	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}
	red := color.RGBA{255, 0, 0, 255}
	green := color.RGBA{0, 255, 0, 255}
	yellow := color.RGBA{255, 255, 0, 255}

	pivotXInt := int(math.Round(pivot.X))
	pivotYInt := int(math.Round(pivot.Y))

	for frame := 0; frame < 11; frame++ {
		angle := bestParams.Angle0 + float64(frame)*angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateFloat(initial[i], pivot, cos, sin)
		}

		rendered := renderQuad(rotated)
		target := targets[frame]

		rowY := margin + frame*(frameHeight+margin)

		// Column 0: Target
		col0X := margin
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := white
				if target[y][x] {
					c = black
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						debugImg.Set(col0X+x*scale+dx, rowY+y*scale+dy, c)
					}
				}
			}
		}

		// Column 1: Rendered
		col1X := margin + frameWidth + margin
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				c := white
				if rendered[y][x] {
					c = black
				}
				// Mark pivot
				if x == pivotXInt && y == pivotYInt {
					c = yellow
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						debugImg.Set(col1X+x*scale+dx, rowY+y*scale+dy, c)
					}
				}
			}
		}

		// Column 2: Diff
		col2X := margin + 2*(frameWidth+margin)
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				var c color.RGBA
				if x == pivotXInt && y == pivotYInt {
					c = yellow
				} else if rendered[y][x] == target[y][x] {
					// Match - show as gray
					c = color.RGBA{100, 100, 100, 255}
				} else if rendered[y][x] && !target[y][x] {
					// Extra pixel
					c = red
				} else {
					// Missing pixel
					c = green
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						debugImg.Set(col2X+x*scale+dx, rowY+y*scale+dy, c)
					}
				}
			}
		}
	}

	// Save debug image
	debugFile, err := os.Create("debug_float_optimization.png")
	if err != nil {
		fmt.Printf("Error creating debug PNG: %v\n", err)
		return
	}
	defer debugFile.Close()

	if err := png.Encode(debugFile, debugImg); err != nil {
		fmt.Printf("Error encoding debug PNG: %v\n", err)
		return
	}

	fmt.Println("✓ Saved debug_float_optimization.png")
	fmt.Println("  Layout: Target | Rendered (with pivot) | Diff (red=extra, green=missing, yellow=pivot)")
}
