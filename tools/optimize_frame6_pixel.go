package main

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"runtime"
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

// Rotate with truncation
func rotateTruncated(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)

	rxTemp := dx*cos256 - dy*sin256
	ryTemp := dx*sin256 + dy*cos256

	rx := rxTemp >> 8
	ry := ryTemp >> 8

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

// Check if pixel 12,6 is correct in frame 6
func hasFrame6IsolatedPixel(size, cx, cy, px, py Fixed88, targets [11][][]bool) bool {
	angle := 5.0 * (math.Pi / 20.0) // Frame 6 = 45°
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	cos256 := int32(cos * 256)
	sin256 := int32(sin * 256)

	half := size / 2
	initial := [4]Point{
		{cx - half, cy - half},
		{cx + half, cy - half},
		{cx + half, cy + half},
		{cx - half, cy + half},
	}
	pivot := Point{px, py}

	var rotated [4]Point
	for i := 0; i < 4; i++ {
		rotated[i] = rotateTruncated(initial[i], pivot, cos256, sin256)
	}

	rendered := renderQuad(rotated)
	target := targets[5] // Frame 6

	// Check if pixel 12,6 matches
	return rendered[6][12] == target[6][12]
}

// Evaluate parameters
func evaluateParams(size, cx, cy, px, py Fixed88, targets [11][][]bool) (float64, int, bool) {
	half := size / 2
	angleStep := math.Pi / 20.0

	totalPixel := 0.0
	perfect := 0

	for frame := 0; frame < 11; frame++ {
		angle := float64(frame) * angleStep
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		cos256 := int32(cos * 256)
		sin256 := int32(sin * 256)

		initial := [4]Point{
			{cx - half, cy - half},
			{cx + half, cy - half},
			{cx + half, cy + half},
			{cx - half, cy + half},
		}
		pivot := Point{px, py}

		var rotated [4]Point
		for i := 0; i < 4; i++ {
			rotated[i] = rotateTruncated(initial[i], pivot, cos256, sin256)
		}

		rendered := renderQuad(rotated)
		pixel := comparePatterns(targets[frame], rendered)

		totalPixel += pixel
		if pixel == 100.0 {
			perfect++
		}
	}

	frame6Fixed := hasFrame6IsolatedPixel(size, cx, cy, px, py, targets)

	return totalPixel / 11.0, perfect, frame6Fixed
}

type Result struct {
	size, cx, cy, px, py  Fixed88
	accuracy              float64
	perfect               int
	frame6Fixed           bool
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("🔍 Searching for Parameter Tweak to Fix Frame 6 Isolated Pixel")
	fmt.Println()

	// Load target patterns
	var targets [11][][]bool
	for i := 0; i < 11; i++ {
		pattern, err := loadPattern(fmt.Sprintf("../emulator/frames/cycle_%02d.png", i+1))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		targets[i] = pattern
	}

	// Current truncation-optimized parameters
	baseSize := Fixed88(3072)
	baseCX := Fixed88(3072)
	baseCY := Fixed88(2944)
	basePX := Fixed88(4608)
	basePY := Fixed88(4480)

	baseAcc, basePerfect, baseFrame6 := evaluateParams(baseSize, baseCX, baseCY, basePX, basePY, targets)
	fmt.Printf("Current parameters:\n")
	fmt.Printf("  Size:   %d (%.3f)\n", baseSize, baseSize.toFloat())
	fmt.Printf("  Center: (%d, %d) = (%.3f, %.3f)\n", baseCX, baseCY, baseCX.toFloat(), baseCY.toFloat())
	fmt.Printf("  Pivot:  (%d, %d) = (%.3f, %.3f)\n", basePX, basePY, basePX.toFloat(), basePY.toFloat())
	fmt.Printf("  Accuracy: %.2f%% (%d/11 perfect)\n", baseAcc, basePerfect)
	fmt.Printf("  Frame 6 pixel (12,6) fixed: %v\n", baseFrame6)
	fmt.Println()

	fmt.Println("Searching small adjustments (±16 units = ±1/16 pixel)...")
	fmt.Println()

	var mu sync.Mutex
	var wg sync.WaitGroup
	bestResult := Result{
		size:        baseSize,
		cx:          baseCX,
		cy:          baseCY,
		px:          basePX,
		py:          basePY,
		accuracy:    baseAcc,
		perfect:     basePerfect,
		frame6Fixed: baseFrame6,
	}

	// Search adjustments in parallel
	tasks := make(chan struct {
		sizeOff, cxOff, cyOff, pxOff, pyOff int
	}, 1000)

	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				size := baseSize + Fixed88(task.sizeOff)
				cx := baseCX + Fixed88(task.cxOff)
				cy := baseCY + Fixed88(task.cyOff)
				px := basePX + Fixed88(task.pxOff)
				py := basePY + Fixed88(task.pyOff)

				acc, perf, frame6 := evaluateParams(size, cx, cy, px, py, targets)

				// Only consider if frame 6 pixel is fixed
				if frame6 {
					mu.Lock()
					// Better if: higher accuracy, or same accuracy with more perfect frames
					better := acc > bestResult.accuracy ||
						(acc == bestResult.accuracy && perf > bestResult.perfect) ||
						(!bestResult.frame6Fixed && frame6)

					if better {
						bestResult = Result{
							size:        size,
							cx:          cx,
							cy:          cy,
							px:          px,
							py:          py,
							accuracy:    acc,
							perfect:     perf,
							frame6Fixed: frame6,
						}
						fmt.Printf("  Found: acc=%.2f%% (%d/11) frame6=%v  ", acc, perf, frame6)
						fmt.Printf("size=%d cx=%d cy=%d px=%d py=%d\n",
							size, cx, cy, px, py)
					}
					mu.Unlock()
				}
			}
		}()
	}

	// Generate tasks - small adjustments only
	count := 0
	for sizeOff := -16; sizeOff <= 16; sizeOff += 4 {
		for cxOff := -16; cxOff <= 16; cxOff += 4 {
			for cyOff := -16; cyOff <= 16; cyOff += 4 {
				for pxOff := -16; pxOff <= 16; pxOff += 4 {
					for pyOff := -16; pyOff <= 16; pyOff += 4 {
						tasks <- struct {
							sizeOff, cxOff, cyOff, pxOff, pyOff int
						}{sizeOff, cxOff, cyOff, pxOff, pyOff}
						count++
					}
				}
			}
		}
	}
	close(tasks)
	wg.Wait()

	fmt.Printf("\nSearched %d combinations\n", count)
	fmt.Println()

	if bestResult.frame6Fixed && (bestResult.accuracy > baseAcc || bestResult.perfect > basePerfect) {
		fmt.Println("✓ Found improved parameters:")
		fmt.Printf("  Size:   %d (%.6f) [%+d]\n", bestResult.size, bestResult.size.toFloat(), int(bestResult.size-baseSize))
		fmt.Printf("  Center: (%d, %d) = (%.6f, %.6f) [%+d, %+d]\n",
			bestResult.cx, bestResult.cy,
			bestResult.cx.toFloat(), bestResult.cy.toFloat(),
			int(bestResult.cx-baseCX), int(bestResult.cy-baseCY))
		fmt.Printf("  Pivot:  (%d, %d) = (%.6f, %.6f) [%+d, %+d]\n",
			bestResult.px, bestResult.py,
			bestResult.px.toFloat(), bestResult.py.toFloat(),
			int(bestResult.px-basePX), int(bestResult.py-basePY))
		fmt.Printf("  Accuracy: %.2f%% (%d/11 perfect) [%+.2f%%, %+d frames]\n",
			bestResult.accuracy, bestResult.perfect,
			bestResult.accuracy-baseAcc, bestResult.perfect-basePerfect)
		fmt.Printf("  Frame 6 pixel (12,6) fixed: %v\n", bestResult.frame6Fixed)
	} else if bestResult.frame6Fixed {
		fmt.Println("✓ Found parameters that fix frame 6 pixel:")
		fmt.Printf("  Size:   %d (%.6f) [%+d]\n", bestResult.size, bestResult.size.toFloat(), int(bestResult.size-baseSize))
		fmt.Printf("  Center: (%d, %d) = (%.6f, %.6f) [%+d, %+d]\n",
			bestResult.cx, bestResult.cy,
			bestResult.cx.toFloat(), bestResult.cy.toFloat(),
			int(bestResult.cx-baseCX), int(bestResult.cy-baseCY))
		fmt.Printf("  Pivot:  (%d, %d) = (%.6f, %.6f) [%+d, %+d]\n",
			bestResult.px, bestResult.py,
			bestResult.px.toFloat(), bestResult.py.toFloat(),
			int(bestResult.px-basePX), int(bestResult.py-basePY))
		fmt.Printf("  Accuracy: %.2f%% (%d/11 perfect) [%+.2f%%, %+d frames]\n",
			bestResult.accuracy, bestResult.perfect,
			bestResult.accuracy-baseAcc, bestResult.perfect-basePerfect)
		fmt.Printf("  Frame 6 pixel (12,6) fixed: %v\n", bestResult.frame6Fixed)
		fmt.Println()
		fmt.Println("  ⚠ Warning: Fixes frame 6 but reduces overall accuracy")
	} else {
		fmt.Println("✗ No improvement found within search range")
		fmt.Println("  Frame 6 pixel cannot be fixed without degrading other frames")
	}
}
