package main

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
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

// Parameter vector
type Params struct {
	Size    Fixed88
	CenterX Fixed88
	CenterY Fixed88
	PivotX  Fixed88
	PivotY  Fixed88
	Angle0  float64
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
	angleStep := (math.Pi / 2.0) / 10.0
	perfectCount := 0

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
		if accuracy == 100.0 {
			perfectCount++
		}
	}

	return perfectCount
}

// Evaluate parameters
func evaluateParams(p Params, targets [11][][]bool) float64 {
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

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🔍 Exhaustive Grid Search (8.8 Fixed-Point)")
	fmt.Printf("   Using %d CPU cores\n", runtime.NumCPU())
	fmt.Println()

	// Load target patterns
	fmt.Println("Loading target patterns...")
	var targets [11][][]bool
	for i := 0; i < 11; i++ {
		pattern, err := loadPattern(fmt.Sprintf("../emulator/frames/cycle_%02d.png", i+1))
		if err != nil {
			fmt.Printf("Error loading pattern %d: %v\n", i+1, err)
			return
		}
		targets[i] = pattern
	}
	fmt.Println("✓ Loaded 11 patterns\n")

	// Best known parameters as starting point
	baseSize := Fixed88(3075)
	baseCenterX := Fixed88(3019)
	baseCenterY := Fixed88(2945)
	basePivotX := Fixed88(4579)
	basePivotY := Fixed88(4542)

	// Search ranges (fine-grained around best known)
	sizeRange := 10       // ±10 in 8.8 units = ±0.039 pixels
	centerRange := 10
	pivotRange := 10
	angleSteps := 21      // -0.01 to +0.01 radians

	fmt.Printf("Search space:\n")
	fmt.Printf("  Size: %d ±%d\n", baseSize, sizeRange)
	fmt.Printf("  CenterX: %d ±%d\n", baseCenterX, centerRange)
	fmt.Printf("  CenterY: %d ±%d\n", baseCenterY, centerRange)
	fmt.Printf("  PivotX: %d ±%d\n", basePivotX, pivotRange)
	fmt.Printf("  PivotY: %d ±%d\n", basePivotY, pivotRange)
	fmt.Printf("  Angle: %d steps from -0.01 to +0.01 rad\n", angleSteps)

	totalCombinations := (2*sizeRange+1) * (2*centerRange+1) * (2*centerRange+1) *
		(2*pivotRange+1) * (2*pivotRange+1) * angleSteps
	fmt.Printf("  Total: %d combinations\n\n", totalCombinations)

	// Parallel search
	type Result struct {
		params       Params
		accuracy     float64
		perfectCount int
	}

	results := make(chan Result, 1000)
	var wg sync.WaitGroup

	numWorkers := runtime.NumCPU()
	jobs := make(chan Params, numWorkers*4)

	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				accuracy := evaluateParams(p, targets)
				perfectCount := countPerfectFrames(p, targets)
				results <- Result{p, accuracy, perfectCount}
			}
		}()
	}

	// Generate jobs
	go func() {
		checked := 0
		for dSize := -sizeRange; dSize <= sizeRange; dSize++ {
			for dCX := -centerRange; dCX <= centerRange; dCX++ {
				for dCY := -centerRange; dCY <= centerRange; dCY++ {
					for dPX := -pivotRange; dPX <= pivotRange; dPX++ {
						for dPY := -pivotRange; dPY <= pivotRange; dPY++ {
							for aIdx := 0; aIdx < angleSteps; aIdx++ {
								angle := -0.01 + float64(aIdx)*0.02/float64(angleSteps-1)
								p := Params{
									Size:    baseSize + Fixed88(dSize),
									CenterX: baseCenterX + Fixed88(dCX),
									CenterY: baseCenterY + Fixed88(dCY),
									PivotX:  basePivotX + Fixed88(dPX),
									PivotY:  basePivotY + Fixed88(dPY),
									Angle0:  angle,
								}
								jobs <- p
								checked++
								if checked%100000 == 0 {
									fmt.Printf("Queued %d/%d combinations...\n", checked, totalCombinations)
								}
							}
						}
					}
				}
			}
		}
		close(jobs)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	bestAccuracy := 0.0
	bestPerfect := 0
	var bestParams Params
	processed := 0

	for result := range results {
		processed++
		if processed%100000 == 0 {
			fmt.Printf("Processed %d/%d (best so far: %.4f%%, %d/11 perfect)...\n",
				processed, totalCombinations, bestAccuracy, bestPerfect)
		}

		// Prioritize perfect frames, then accuracy
		if result.perfectCount > bestPerfect ||
			(result.perfectCount == bestPerfect && result.accuracy > bestAccuracy) {
			bestPerfect = result.perfectCount
			bestAccuracy = result.accuracy
			bestParams = result.params

			if result.perfectCount == 11 {
				fmt.Printf("\n🎉 FOUND 11/11 PERFECT! Accuracy: %.6f%%\n", result.accuracy)
				fmt.Printf("Size=%d CenterX=%d CenterY=%d PivotX=%d PivotY=%d Angle=%.6f\n",
					result.params.Size, result.params.CenterX, result.params.CenterY,
					result.params.PivotX, result.params.PivotY, result.params.Angle0)
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("🎉 BEST PARAMETERS FOUND")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Perfect frames: %d/11\n", bestPerfect)
	fmt.Printf("Overall accuracy: %.6f%%\n", bestAccuracy)
	fmt.Printf("Size:    %d → %.10f\n", bestParams.Size, bestParams.Size.toFloat())
	fmt.Printf("CenterX: %d → %.10f\n", bestParams.CenterX, bestParams.CenterX.toFloat())
	fmt.Printf("CenterY: %d → %.10f\n", bestParams.CenterY, bestParams.CenterY.toFloat())
	fmt.Printf("PivotX:  %d → %.10f\n", bestParams.PivotX, bestParams.PivotX.toFloat())
	fmt.Printf("PivotY:  %d → %.10f\n", bestParams.PivotY, bestParams.PivotY.toFloat())
	fmt.Printf("Angle0:  %.10f rad (%.6f°)\n", bestParams.Angle0, bestParams.Angle0*180/math.Pi)
}
