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

const (fpShift = 8; fpScale = 1 << fpShift)
type fp88 int32
func floatToFP(f float64) fp88 { return fp88(f * fpScale) }
func fpMul(a, b fp88) fp88 { return fp88((int64(a) * int64(b)) >> fpShift) }
func fpToInt(f fp88) int { return int(f >> fpShift) }

type Edge struct {yStart, yEnd, xStart float64; dxDy fp88}
func newEdge(x1, y1, x2, y2 float64) *Edge {
	if y1 > y2 { x1, y1, x2, y2 = x2, y2, x1, y1 }
	e := &Edge{yStart: y1, yEnd: y2, xStart: x1}
	if dy := y2 - y1; dy > 0.001 { e.dxDy = floatToFP((x2 - x1) / dy) }
	return e
}
func (e *Edge) getXAtY(y float64) int {
	return fpToInt(floatToFP(e.xStart) + fpMul(floatToFP(y-e.yStart), e.dxDy))
}

func renderSquare(cx, cy, size, angleDeg float64) [][]bool {
	bitmap := make([][]bool, 24)
	for i := range bitmap { bitmap[i] = make([]bool, 24) }
	angleRad := angleDeg * math.Pi / 180.0
	var vertices [4][2]float64
	for i := 0; i < 4; i++ {
		cornerAngle := angleRad + float64(i)*math.Pi/2.0
		vertices[i] = [2]float64{cx + size*math.Cos(cornerAngle), cy + size*math.Sin(cornerAngle)}
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
			if e.yStart <= yf && yf < e.yEnd { activeEdges = append(activeEdges, e) }
		}
		if len(activeEdges) < 2 { continue }
		xIntersects := make([]int, len(activeEdges))
		for i, e := range activeEdges { xIntersects[i] = e.getXAtY(yf) }
		for i := 0; i < len(xIntersects)-1; i++ {
			for j := i + 1; j < len(xIntersects); j++ {
				if xIntersects[i] > xIntersects[j] {
					xIntersects[i], xIntersects[j] = xIntersects[j], xIntersects[i]
				}
			}
		}
		if len(xIntersects) >= 2 {
			x1, x2 := xIntersects[0], xIntersects[len(xIntersects)-1]
			if x1 < 0 { x1 = 0 }
			if x2 >= 24 { x2 = 23 }
			for x := x1; x <= x2; x++ { bitmap[y][x] = true }
		}
	}
	return bitmap
}

func compareBitmaps(target, rendered [][]bool) int {
	matches := 0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if target[y][x] == rendered[y][x] { matches++ }
		}
	}
	return matches
}

func parseFrame(lines []string) ([][]bool, error) {
	bitmap := make([][]bool, 24)
	lineIdx := 0
	for lineIdx < len(lines) && !strings.HasPrefix(lines[lineIdx], "===") { lineIdx++ }
	if lineIdx >= len(lines) { return nil, fmt.Errorf("no frame header") }
	lineIdx++
	for y := 0; y < 24; y++ {
		for lineIdx < len(lines) && strings.TrimSpace(lines[lineIdx]) == "" { lineIdx++ }
		if lineIdx >= len(lines) { return nil, fmt.Errorf("unexpected end") }
		line := lines[lineIdx]; lineIdx++
		bitmap[y] = make([]bool, 24)
		x := 0
		for _, ch := range line {
			if ch == '█' || ch == '▓' { bitmap[y][x] = true; x++ } else if ch == ' ' || ch == '░' || ch == '·' { bitmap[y][x] = false; x++ }
			if x >= 24 { break }
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
	if currentFrameNum == targetFrame { return parseFrame(currentFrameLines) }
	return nil, fmt.Errorf("frame not found")
}

type TestParams struct {cx, cy, size, angle float64}
type TestResult struct {params TestParams; matches int}

func worker(bitmap [][]bool, tasks <-chan TestParams, results chan<- TestResult, tested *int64, wg *sync.WaitGroup) {
	defer wg.Done()
	for params := range tasks {
		rendered := renderSquare(params.cx, params.cy, params.size, params.angle)
		matches := compareBitmaps(bitmap, rendered)
		results <- TestResult{params: params, matches: matches}
		atomic.AddInt64(tested, 1)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run fast_global_search.go <frame_number>")
		os.Exit(1)
	}

	frameNum, _ := strconv.Atoi(os.Args[1])
	content, _ := os.ReadFile("../emulator/frames/animation_318-336.txt")
	bitmap, _ := loadFrame(string(content), frameNum)

	fmt.Printf("Fast Global Search for Frame %d\n", frameNum)
	fmt.Println("Coarse grid: 5° angle steps, 0.5 position/size steps")
	fmt.Println(strings.Repeat("=", 70))

	tasks := make(chan TestParams, 10000)
	results := make(chan TestResult, 10000)
	var tested int64
	var wg sync.WaitGroup

	workers := 16
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(bitmap, tasks, results, &tested, &wg)
	}

	// Coarse grid search: 5° steps, 0.5 position/size steps
	go func() {
		for cx := 9.0; cx <= 15.0; cx += 0.5 {
			for cy := 9.0; cy <= 15.0; cy += 0.5 {
				for size := 6.0; size <= 10.0; size += 0.5 {
					for angle := 0.0; angle < 180.0; angle += 5.0 {
						tasks <- TestParams{cx, cy, size, angle}
					}
				}
			}
		}
		close(tasks)
	}()

	go func() {wg.Wait(); close(results)}()

	bestResult := TestResult{matches: 0}

	for result := range results {
		if result.matches > bestResult.matches {
			bestResult = result
			matchPct := 100.0 * float64(result.matches) / 576.0
			if matchPct >= 99.0 {
				fmt.Printf("→ %.3f%% (cx=%.1f, cy=%.1f, size=%.1f, angle=%.0f°)\n",
					matchPct, result.params.cx, result.params.cy, result.params.size, result.params.angle)
			}
			if result.matches == 576 {
				fmt.Println("✓✓✓ PERFECT 100% FOUND! ✓✓✓")
				break
			}
		}
	}

	fmt.Println(strings.Repeat("=", 70))
	matchPct := 100.0 * float64(bestResult.matches) / 576.0
	fmt.Printf("Coarse best: %.3f%% (%d/576, diff=%d)\n", matchPct, bestResult.matches, 576-bestResult.matches)
	fmt.Printf("Params: cx=%.1f, cy=%.1f, size=%.1f, angle=%.0f°\n",
		bestResult.params.cx, bestResult.params.cy, bestResult.params.size, bestResult.params.angle)

	if bestResult.matches < 576 {
		fmt.Println("\nFine tuning around best result...")

		tasks2 := make(chan TestParams, 10000)
		results2 := make(chan TestResult, 10000)
		tested = 0

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go worker(bitmap, tasks2, results2, &tested, &wg)
		}

		go func() {
			step := 1.0 / 256.0
			baseCx := bestResult.params.cx
			baseCy := bestResult.params.cy
			baseSize := bestResult.params.size
			baseAngle := bestResult.params.angle

			for cxOffset := -64; cxOffset <= 64; cxOffset++ {
				for cyOffset := -64; cyOffset <= 64; cyOffset++ {
					for sizeOffset := -128; sizeOffset <= 128; sizeOffset++ {
						testSize := baseSize + float64(sizeOffset)*step
						if testSize < 1.0 { continue }
						for angleOffset := -30; angleOffset <= 30; angleOffset++ {
							testAngle := baseAngle + float64(angleOffset)*0.1
							tasks2 <- TestParams{
								baseCx + float64(cxOffset)*step,
								baseCy + float64(cyOffset)*step,
								testSize,
								testAngle,
							}
						}
					}
				}
			}
			close(tasks2)
		}()

		go func() {wg.Wait(); close(results2)}()

		finalBest := bestResult
		for result := range results2 {
			if result.matches > finalBest.matches {
				finalBest = result
				matchPct := 100.0 * float64(result.matches) / 576.0
				fmt.Printf("→ %.3f%% (cx=%.4f, cy=%.4f, size=%.4f, angle=%.2f°)\n",
					matchPct, result.params.cx, result.params.cy, result.params.size, result.params.angle)
				if result.matches == 576 {
					fmt.Println("✓✓✓ PERFECT 100% FOUND! ✓✓✓")
					break
				}
			}
		}

		fmt.Println(strings.Repeat("=", 70))
		matchPct = 100.0 * float64(finalBest.matches) / 576.0
		fmt.Printf("FINAL: %.3f%% (%d/576, diff=%d)\n", matchPct, finalBest.matches, 576-finalBest.matches)
		fmt.Printf("cx=%.6f, cy=%.6f, size=%.6f, angle=%.2f°\n",
			finalBest.params.cx, finalBest.params.cy, finalBest.params.size, finalBest.params.angle)

		if finalBest.matches == 576 {
			fmt.Println("\n✓✓✓ PATTERN IS REACHABLE! ✓✓✓")
		} else {
			fmt.Printf("\nMaximum achievable: %d/576 (%.3f%%)\n", finalBest.matches, matchPct)
			fmt.Println("Pattern NOT reachable at 100% with this algorithm")
		}

		os.WriteFile(fmt.Sprintf("fast_global_frame%d.txt", frameNum),
			[]byte(fmt.Sprintf("%d %.6f %.6f %.6f %.2f %d\n",
				frameNum, finalBest.params.cx, finalBest.params.cy,
				finalBest.params.size, finalBest.params.angle, finalBest.matches)), 0644)
	} else {
		fmt.Println("\n✓✓✓ PATTERN IS REACHABLE! ✓✓✓")
	}
}
