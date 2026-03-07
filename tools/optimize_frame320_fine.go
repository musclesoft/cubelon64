package main

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	fpShift = 8
	fpScale = 1 << fpShift
)

type fp88 int32

func floatToFP(f float64) fp88 { return fp88(f * fpScale) }
func fpMul(a, b fp88) fp88      { return fp88((int64(a) * int64(b)) >> fpShift) }
func fpToInt(f fp88) int        { return int(f >> fpShift) }

type Edge struct {
	yStart, yEnd, xStart float64
	dxDy                 fp88
}

func newEdge(x1, y1, x2, y2 float64) *Edge {
	if y1 > y2 {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}
	e := &Edge{yStart: y1, yEnd: y2, xStart: x1}
	if dy := y2 - y1; dy > 0.001 {
		e.dxDy = floatToFP((x2 - x1) / dy)
	}
	return e
}

func (e *Edge) getXAtY(y float64) int {
	dy := y - e.yStart
	dx := fpMul(floatToFP(dy), e.dxDy)
	return fpToInt(floatToFP(e.xStart) + dx)
}

func renderSquare(cx, cy, size, angleDeg float64) [][]bool {
	bitmap := make([][]bool, 24)
	for i := range bitmap {
		bitmap[i] = make([]bool, 24)
	}

	angleRad := angleDeg * math.Pi / 180.0
	var vertices [4][2]float64
	for i := 0; i < 4; i++ {
		cornerAngle := angleRad + float64(i)*math.Pi/2.0
		vertices[i] = [2]float64{
			cx + size*math.Cos(cornerAngle),
			cy + size*math.Sin(cornerAngle),
		}
	}

	edges := make([]*Edge, 4)
	for i := 0; i < 4; i++ {
		next := (i + 1) % 4
		edges[i] = newEdge(vertices[i][0], vertices[i][1], vertices[next][0], vertices[next][1])
	}

	for y := 0; y < 24; y++ {
		yf := float64(y)
		var activeEdges []*Edge
		for _, e := range edges {
			if e.yStart <= yf && yf < e.yEnd {
				activeEdges = append(activeEdges, e)
			}
		}
		if len(activeEdges) < 2 {
			continue
		}

		xIntersects := make([]int, len(activeEdges))
		for i, e := range activeEdges {
			xIntersects[i] = e.getXAtY(yf)
		}
		for i := 0; i < len(xIntersects)-1; i++ {
			for j := i + 1; j < len(xIntersects); j++ {
				if xIntersects[i] > xIntersects[j] {
					xIntersects[i], xIntersects[j] = xIntersects[j], xIntersects[i]
				}
			}
		}

		if len(xIntersects) >= 2 {
			x1, x2 := xIntersects[0], xIntersects[len(xIntersects)-1]
			if x1 < 0 {
				x1 = 0
			}
			if x2 >= 24 {
				x2 = 23
			}
			for x := x1; x <= x2; x++ {
				bitmap[y][x] = true
			}
		}
	}
	return bitmap
}

func compareBitmaps(target, rendered [][]bool) int {
	matches := 0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if target[y][x] == rendered[y][x] {
				matches++
			}
		}
	}
	return matches
}

func parseFrame(lines []string) ([][]bool, error) {
	bitmap := make([][]bool, 24)
	lineIdx := 0
	for lineIdx < len(lines) && !strings.HasPrefix(lines[lineIdx], "===") {
		lineIdx++
	}
	if lineIdx >= len(lines) {
		return nil, fmt.Errorf("no frame header")
	}
	lineIdx++

	for y := 0; y < 24; y++ {
		for lineIdx < len(lines) && strings.TrimSpace(lines[lineIdx]) == "" {
			lineIdx++
		}
		if lineIdx >= len(lines) {
			return nil, fmt.Errorf("unexpected end")
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

func loadFrame(content string, targetFrame int) ([][]bool, error) {
	lines := strings.Split(content, "\n")
	framePattern := regexp.MustCompile(`=== Frame (\d+)`)
	currentFrameNum := -1
	var currentFrameLines []string

	for _, line := range lines {
		if match := framePattern.FindStringSubmatch(line); match != nil {
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
	return nil, fmt.Errorf("frame not found")
}

type TestParams struct {
	cx, cy, size, angle float64
}

type TestResult struct {
	params  TestParams
	matches int
}

func worker(bitmap [][]bool, tasks <-chan TestParams, results chan<- TestResult, tested *int64, wg *sync.WaitGroup) {
	defer wg.Done()
	for params := range tasks {
		rendered := renderSquare(params.cx, params.cy, params.size, params.angle)
		matches := compareBitmaps(bitmap, rendered)
		results <- TestResult{params: params, matches: matches}
		if count := atomic.AddInt64(tested, 1); count%10000 == 0 {
			fmt.Printf("  Tested: %d\n", count)
		}
	}
}

func main() {
	content, _ := os.ReadFile("../emulator/frames/animation_318-336.txt")
	bitmap, _ := loadFrame(string(content), 320)

	// Current best
	baseCx, baseCy, baseSize, baseAngle := 11.859375, 11.070312, 7.902344, 63.50

	fmt.Println("Ultra-fine optimization for frame 320")
	fmt.Println("Testing finer angle steps (0.01°) and wider range (±10°)")
	fmt.Println(strings.Repeat("=", 70))

	rendered := renderSquare(baseCx, baseCy, baseSize, baseAngle)
	baseMatches := compareBitmaps(bitmap, rendered)
	fmt.Printf("Current: %.3f%% (%d/576, diff=%d)\n", 100.0*float64(baseMatches)/576.0, baseMatches, 576-baseMatches)
	fmt.Printf("Params: cx=%.6f, cy=%.6f, size=%.6f, angle=%.2f°\n\n", baseCx, baseCy, baseSize, baseAngle)

	tasks := make(chan TestParams, 10000)
	results := make(chan TestResult, 10000)
	var tested int64
	var wg sync.WaitGroup

	workers := 16
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(bitmap, tasks, results, &tested, &wg)
	}

	go func() {
		step := 1.0 / 256.0
		count := 0

		// Ultra-fine angle: ±10° in 0.01° steps
		for angleOffset := -1000; angleOffset <= 1000; angleOffset++ {
			testAngle := baseAngle + float64(angleOffset)*0.01

			// Fine cx
			for cxOffset := -32; cxOffset <= 32; cxOffset += 2 {
				testCx := baseCx + float64(cxOffset)*step

				// Fine cy
				for cyOffset := -32; cyOffset <= 32; cyOffset += 2 {
					testCy := baseCy + float64(cyOffset)*step

					// Size
					for sizeOffset := -64; sizeOffset <= 128; sizeOffset += 2 {
						testSize := baseSize + float64(sizeOffset)*step
						if testSize < 1.0 {
							continue
						}

						tasks <- TestParams{testCx, testCy, testSize, testAngle}
						count++
					}
				}
			}
		}
		close(tasks)
		fmt.Printf("Generated %d combinations\n\n", count)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	bestResult := TestResult{params: TestParams{baseCx, baseCy, baseSize, baseAngle}, matches: baseMatches}

	for result := range results {
		if result.matches > bestResult.matches {
			bestResult = result
			matchPct := 100.0 * float64(result.matches) / 576.0
			fmt.Printf("  → %.3f%% (cx=%.6f, cy=%.6f, size=%.6f, angle=%.2f°)\n",
				matchPct, result.params.cx, result.params.cy, result.params.size, result.params.angle)
			if result.matches == 576 {
				fmt.Println("  ✓ PERFECT 100%!")
			}
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	matchPct := 100.0 * float64(bestResult.matches) / 576.0
	diff := 576 - bestResult.matches
	fmt.Printf("Best: %.3f%% (%d/576, diff=%d)\n", matchPct, bestResult.matches, diff)
	fmt.Printf("Params: cx=%.6f, cy=%.6f, size=%.6f, angle=%.2f°\n", 
		bestResult.params.cx, bestResult.params.cy, bestResult.params.size, bestResult.params.angle)

	if bestResult.matches == 576 {
		fmt.Println("\n✓ PERFECT 100% MATCH!")
	} else {
		improvement := bestResult.matches - baseMatches
		if improvement > 0 {
			fmt.Printf("\nImproved by %d pixels\n", improvement)
		} else {
			fmt.Println("\nNo improvement - already at local maximum")
		}
	}

	// Save result
	os.WriteFile("best_params_frame320_ultrafine.txt", 
		[]byte(fmt.Sprintf("320 %.6f %.6f %.6f %.2f %d\n", 
			bestResult.params.cx, bestResult.params.cy, bestResult.params.size, 
			bestResult.params.angle, bestResult.matches)), 0644)
}
