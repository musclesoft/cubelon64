package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Fixed-point 8.8 math
const (
	fpShift = 8
	fpScale = 1 << fpShift
)

type fp88 int32

func floatToFP(f float64) fp88 {
	return fp88(f * fpScale)
}

func fpMul(a, b fp88) fp88 {
	return fp88((int64(a) * int64(b)) >> fpShift)
}

func fpToInt(f fp88) int {
	return int(f >> fpShift)
}

// Edge for scanline fill
type Edge struct {
	yStart float64
	yEnd   float64
	xStart float64
	dxDy   fp88
}

func newEdge(x1, y1, x2, y2 float64) *Edge {
	if y1 > y2 {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}

	e := &Edge{
		yStart: y1,
		yEnd:   y2,
		xStart: x1,
	}

	dy := y2 - y1
	if dy > 0.001 {
		e.dxDy = floatToFP((x2 - x1) / dy)
	}

	return e
}

func (e *Edge) getXAtY(y float64) int {
	dy := y - e.yStart
	dx := fpMul(floatToFP(dy), e.dxDy)
	x := floatToFP(e.xStart) + dx
	return fpToInt(x)
}

// Render square using 4-edge scanline fill
// size: Distance from center to corner (half-diagonal), like Python
func renderSquare(cx, cy, size, angleDeg float64, width, height int) [][]bool {
	bitmap := make([][]bool, height)
	for i := range bitmap {
		bitmap[i] = make([]bool, width)
	}

	angleRad := angleDeg * math.Pi / 180.0

	// Calculate 4 corners (Python style: size is center-to-corner distance)
	vertices := [4][2]float64{}
	for i := 0; i < 4; i++ {
		cornerAngle := angleRad + float64(i)*math.Pi/2.0
		x := cx + size*math.Cos(cornerAngle)
		y := cy + size*math.Sin(cornerAngle)
		vertices[i] = [2]float64{x, y}
	}

	// Create 4 edges
	edges := make([]*Edge, 4)
	for i := 0; i < 4; i++ {
		next := (i + 1) % 4
		edges[i] = newEdge(vertices[i][0], vertices[i][1], vertices[next][0], vertices[next][1])
	}

	// Scanline fill
	for y := 0; y < height; y++ {
		yf := float64(y)

		// Find active edges
		var activeEdges []*Edge
		for _, e := range edges {
			if e.yStart <= yf && yf < e.yEnd {
				activeEdges = append(activeEdges, e)
			}
		}

		if len(activeEdges) < 2 {
			continue
		}

		// Get X intersections
		xIntersects := make([]int, len(activeEdges))
		for i, e := range activeEdges {
			xIntersects[i] = e.getXAtY(yf)
		}

		// Sort intersections
		for i := 0; i < len(xIntersects)-1; i++ {
			for j := i + 1; j < len(xIntersects); j++ {
				if xIntersects[i] > xIntersects[j] {
					xIntersects[i], xIntersects[j] = xIntersects[j], xIntersects[i]
				}
			}
		}

		// Fill from first to last intersection (like Python)
		if len(xIntersects) >= 2 {
			x1 := xIntersects[0]
			x2 := xIntersects[len(xIntersects)-1]

			if x1 < 0 {
				x1 = 0
			}
			if x2 >= width {
				x2 = width - 1
			}

			for x := x1; x <= x2; x++ {
				bitmap[y][x] = true
			}
		}
	}

	return bitmap
}

// Compare bitmaps
func compareBitmaps(target, rendered [][]bool) int {
	matches := 0
	for y := 0; y < len(target); y++ {
		for x := 0; x < len(target[y]); x++ {
			if target[y][x] == rendered[y][x] {
				matches++
			}
		}
	}
	return matches
}

// Parse frame from text
func parseFrame(lines []string) ([][]bool, error) {
	bitmap := make([][]bool, 24)

	lineIdx := 0
	for lineIdx < len(lines) && !strings.HasPrefix(lines[lineIdx], "===") {
		lineIdx++
	}
	if lineIdx >= len(lines) {
		return nil, fmt.Errorf("no frame header found")
	}
	lineIdx++ // Skip header

	for y := 0; y < 24; y++ {
		for lineIdx < len(lines) && strings.TrimSpace(lines[lineIdx]) == "" {
			lineIdx++
		}
		if lineIdx >= len(lines) {
			return nil, fmt.Errorf("unexpected end of frame data")
		}

		line := lines[lineIdx]
		lineIdx++

		bitmap[y] = make([]bool, 24)
		x := 0
		for _, ch := range line {
			if ch == '█' || ch == '▓' {
				bitmap[y][x] = true
				x++
			} else if ch == ' ' || ch == '░' || ch == '·' {
				bitmap[y][x] = false
				x++
			}
			if x >= 24 {
				break
			}
		}
	}

	return bitmap, nil
}

// Load frame from animation file
func loadFrame(content string, targetFrame int) ([][]bool, error) {
	lines := strings.Split(content, "\n")
	framePattern := regexp.MustCompile(`=== Frame (\d+)`)

	currentFrameNum := -1
	var currentFrameLines []string

	for _, line := range lines {
		match := framePattern.FindStringSubmatch(line)
		if match != nil {
			if currentFrameNum == targetFrame && len(currentFrameLines) > 0 {
				return parseFrame(currentFrameLines)
			}
			currentFrameNum, _ = strconv.Atoi(match[1])
			currentFrameLines = []string{line}
		} else {
			currentFrameLines = append(currentFrameLines, line)
		}
	}

	if currentFrameNum == targetFrame {
		return parseFrame(currentFrameLines)
	}

	return nil, fmt.Errorf("frame %d not found", targetFrame)
}

// Test parameters
type TestParams struct {
	cx, cy, size, angle float64
}

type TestResult struct {
	params  TestParams
	matches int
}

// Worker function
func worker(frameNum int, bitmap [][]bool, tasks <-chan TestParams, results chan<- TestResult, tested *int64, wg *sync.WaitGroup) {
	defer wg.Done()

	for params := range tasks {
		rendered := renderSquare(params.cx, params.cy, params.size, params.angle, 24, 24)
		matches := compareBitmaps(bitmap, rendered)

		results <- TestResult{params: params, matches: matches}

		count := atomic.AddInt64(tested, 1)
		if count%10000 == 0 {
			fmt.Printf("  Tested: %d\n", count)
		}
	}
}

func main() {
	frameNum := flag.Int("frame", 326, "Frame number to optimize")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of parallel workers")
	flag.Parse()

	fmt.Printf("Parallel best-match search (Go)\n")
	fmt.Printf("Frame: %d, Workers: %d\n", *frameNum, *workers)
	fmt.Println(strings.Repeat("=", 70))

	// Load animation
	content, err := os.ReadFile("../emulator/frames/animation_318-336.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading animation file: %v\n", err)
		os.Exit(1)
	}

	bitmap, err := loadFrame(string(content), *frameNum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading frame: %v\n", err)
		os.Exit(1)
	}

	// Base parameters from previous best
	baseParams := map[int]TestParams{
		320: {12.0, 11.0, 8.0, 61.1},
		322: {12.269531, 11.359375, 8.085938, 70.9},
		324: {11.5, 11.0, 8.0, 77.3},
		326: {12.0, 11.0, 7.5, -4.2},
		328: {12.5, 11.0, 8.0, 9.1},
		330: {11.664062, 11.410156, 8.066406, 17.3},
		332: {12.0, 11.0, 8.0, 22.0},
		334: {12.0, 11.5, 8.0, 35.2},
		336: {12.0, 11.5, 8.0, 43.2},
	}

	base, ok := baseParams[*frameNum]
	if !ok {
		fmt.Fprintf(os.Stderr, "No base parameters for frame %d\n", *frameNum)
		os.Exit(1)
	}

	fmt.Printf("Base: cx=%.6f, cy=%.6f, size=%.6f, angle=%.2f°\n", base.cx, base.cy, base.size, base.angle)

	// Test base
	rendered := renderSquare(base.cx, base.cy, base.size, base.angle, 24, 24)
	baseMatches := compareBitmaps(bitmap, rendered)
	fmt.Printf("Base match: %.3f%% (%d/576, diff=%d)\n\n", 100.0*float64(baseMatches)/576.0, baseMatches, 576-baseMatches)

	// Build parameter space
	step := 1.0 / 256.0 // 8.8 precision

	tasks := make(chan TestParams, 10000)
	results := make(chan TestResult, 10000)
	var tested int64

	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go worker(*frameNum, bitmap, tasks, results, &tested, &wg)
	}

	// Generate tasks in goroutine
	go func() {
		count := 0

		// Size: emphasize larger (user observation: more red than blue)
		for sizeOffset := -128; sizeOffset <= 384; sizeOffset += 1 { // -0.5 to +1.5
			testSize := base.size + float64(sizeOffset)*step
			if testSize < 1.0 {
				continue
			}

			// cx: fine search
			for cxOffset := -64; cxOffset <= 64; cxOffset += 2 { // ±0.25 in steps of 2/256
				testCx := base.cx + float64(cxOffset)*step

				// cy: fine search
				for cyOffset := -64; cyOffset <= 64; cyOffset += 2 {
					testCy := base.cy + float64(cyOffset)*step

					// angle: fine search
					for angleOffset := -30; angleOffset <= 30; angleOffset++ { // ±3° in 0.1° steps
						testAngle := base.angle + float64(angleOffset)*0.1

						tasks <- TestParams{testCx, testCy, testSize, testAngle}
						count++
					}
				}
			}
		}

		close(tasks)
		fmt.Printf("Generated %d test combinations\n\n", count)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	bestResult := TestResult{params: base, matches: baseMatches}

	for result := range results {
		if result.matches > bestResult.matches {
			bestResult = result
			matchPct := 100.0 * float64(result.matches) / 576.0
			fmt.Printf("  → New best: %.3f%% (cx=%.6f, cy=%.6f, size=%.6f, angle=%.2f°)\n",
				matchPct, result.params.cx, result.params.cy, result.params.size, result.params.angle)

			if result.matches == 576 {
				fmt.Println("  ✓ PERFECT 100%!")
			}
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("Best result:")
	matchPct := 100.0 * float64(bestResult.matches) / 576.0
	diff := 576 - bestResult.matches
	fmt.Printf("  Frame %d: %.3f%% (%d/576, diff=%d)\n", *frameNum, matchPct, bestResult.matches, diff)
	fmt.Printf("  cx=%.6f, cy=%.6f, size=%.6f, angle=%.2f°\n",
		bestResult.params.cx, bestResult.params.cy, bestResult.params.size, bestResult.params.angle)

	if bestResult.matches == 576 {
		fmt.Println("  ✓ PERFECT 100% MATCH!")
	}

	// Save to file
	filename := fmt.Sprintf("best_params_go_frame%d.txt", *frameNum)
	f, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "%d %.6f %.6f %.6f %.2f %d\n",
		*frameNum, bestResult.params.cx, bestResult.params.cy,
		bestResult.params.size, bestResult.params.angle, bestResult.matches)
	w.Flush()

	fmt.Printf("\nSaved to %s\n", filename)
}
