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
	return fpToInt(floatToFP(e.xStart) + fpMul(floatToFP(y-e.yStart), e.dxDy))
}

func renderSquareGroundPivot(pivotX, pivotY, sideLength, angleDeg float64) [][]bool {
	bitmap := make([][]bool, 24)
	for i := range bitmap {
		bitmap[i] = make([]bool, 24)
	}

	angleRad := angleDeg * math.Pi / 180.0

	// For a square with bottom-left corner at pivot, calculate other corners
	// Assuming pivot is at corner 0, and we rotate around it
	halfDiag := sideLength / math.Sqrt(2.0)

	// Corners relative to pivot, rotating counter-clockwise
	var corners [4][2]float64
	for i := 0; i < 4; i++ {
		cornerAngle := angleRad + float64(i)*math.Pi/2.0
		corners[i] = [2]float64{
			pivotX + halfDiag*math.Cos(cornerAngle+math.Pi/4.0),
			pivotY + halfDiag*math.Sin(cornerAngle+math.Pi/4.0),
		}
	}

	edges := []*Edge{
		newEdge(corners[0][0], corners[0][1], corners[1][0], corners[1][1]),
		newEdge(corners[1][0], corners[1][1], corners[2][0], corners[2][1]),
		newEdge(corners[2][0], corners[2][1], corners[3][0], corners[3][1]),
		newEdge(corners[3][0], corners[3][1], corners[0][0], corners[0][1]),
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
			} else if ch == '░' || ch == '·' || ch == ' ' {
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
	pivotX, pivotY, sideLength, angle float64
}

type TestResult struct {
	params  TestParams
	matches int
}

func worker(bitmap [][]bool, tasks <-chan TestParams, results chan<- TestResult, tested *int64, wg *sync.WaitGroup) {
	defer wg.Done()
	for params := range tasks {
		rendered := renderSquareGroundPivot(params.pivotX, params.pivotY, params.sideLength, params.angle)
		matches := compareBitmaps(bitmap, rendered)
		results <- TestResult{params: params, matches: matches}
		atomic.AddInt64(tested, 1)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run optimize_ground_pivot.go <frame_number>")
		os.Exit(1)
	}

	frameNum, _ := strconv.Atoi(os.Args[1])
	content, _ := os.ReadFile("../emulator/frames/animation_318-336.txt")
	bitmap, _ := loadFrame(string(content), frameNum)

	fmt.Printf("Ground-Pivot Optimization for Frame %d\n", frameNum)
	fmt.Println("Assuming pivot at bottom edge (marching/rolling model)")
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

	// Search space for ground pivot model
	go func() {
		// Pivot Y should be near bottom (17-20)
		// Pivot X can vary (left to right edge)
		// Side length around 10-12 pixels
		// Angle 0-180°
		for pivotY := 16.0; pivotY <= 20.0; pivotY += 0.25 {
			for pivotX := 3.0; pivotX <= 21.0; pivotX += 0.25 {
				for sideLength := 9.0; sideLength <= 13.0; sideLength += 0.25 {
					for angle := 0.0; angle < 180.0; angle += 2.0 {
						tasks <- TestParams{pivotX, pivotY, sideLength, angle}
					}
				}
			}
		}
		close(tasks)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	bestResult := TestResult{matches: 0}

	for result := range results {
		if result.matches > bestResult.matches {
			bestResult = result
			matchPct := 100.0 * float64(result.matches) / 576.0
			if matchPct >= 99.0 {
				fmt.Printf("→ %.3f%% (pivot=%.2f,%.2f, side=%.2f, angle=%.1f°)\n",
					matchPct, result.params.pivotX, result.params.pivotY,
					result.params.sideLength, result.params.angle)
			}
			if result.matches == 576 {
				fmt.Println("✓✓✓ PERFECT 100% FOUND! ✓✓✓")
				break
			}
		}
	}

	fmt.Println(strings.Repeat("=", 70))
	matchPct := 100.0 * float64(bestResult.matches) / 576.0
	fmt.Printf("BEST: %.3f%% (%d/576, diff=%d)\n", matchPct, bestResult.matches, 576-bestResult.matches)
	fmt.Printf("Ground pivot: (%.3f, %.3f)\n", bestResult.params.pivotX, bestResult.params.pivotY)
	fmt.Printf("Side length: %.3f\n", bestResult.params.sideLength)
	fmt.Printf("Angle: %.2f°\n", bestResult.params.angle)

	if bestResult.matches == 576 {
		fmt.Println("\n✓ Ground-pivot model WORKS!")
	} else {
		fmt.Println("\nGround-pivot model did not improve results")
	}
}
