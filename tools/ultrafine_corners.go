package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
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

func renderQuad(x1, y1, x2, y2, x3, y3, x4, y4 float64) [][]bool {
	bitmap := make([][]bool, 24)
	for i := range bitmap {
		bitmap[i] = make([]bool, 24)
	}

	edges := []*Edge{
		newEdge(x1, y1, x2, y2),
		newEdge(x2, y2, x3, y3),
		newEdge(x3, y3, x4, y4),
		newEdge(x4, y4, x1, y1),
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

func optimizeCornerUltraFine(bitmap [][]bool, corners *[4][2]float64, cornerIdx int, step float64) (bool, int) {
	bestMatches := compareBitmaps(bitmap, renderQuad(
		corners[0][0], corners[0][1],
		corners[1][0], corners[1][1],
		corners[2][0], corners[2][1],
		corners[3][0], corners[3][1]))

	improved := false

	// Try all  16 directions in a fine grid
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			testCorners := *corners
			testCorners[cornerIdx][0] += float64(dx) * step
			testCorners[cornerIdx][1] += float64(dy) * step

			rendered := renderQuad(
				testCorners[0][0], testCorners[0][1],
				testCorners[1][0], testCorners[1][1],
				testCorners[2][0], testCorners[2][1],
				testCorners[3][0], testCorners[3][1])

			matches := compareBitmaps(bitmap, rendered)
			if matches > bestMatches {
				bestMatches = matches
				*corners = testCorners
				improved = true
			}
		}
	}

	return improved, bestMatches
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ultrafine_corners.go <frame_number>")
		os.Exit(1)
	}

	frameNum, _ := strconv.Atoi(os.Args[1])
	content, _ := os.ReadFile("../emulator/frames/animation_318-336.txt")
	bitmap, _ := loadFrame(string(content), frameNum)

	// Load existing best corners
	data, err := os.ReadFile(fmt.Sprintf("corner_opt_frame%d.txt", frameNum))
	if err != nil {
		fmt.Printf("No existing corner data for frame %d\n", frameNum)
		return
	}

	parts := strings.Fields(string(data))
	var corners [4][2]float64
	for i := 0; i < 4; i++ {
		corners[i][0], _ = strconv.ParseFloat(parts[i*2+1], 64)
		corners[i][1], _ = strconv.ParseFloat(parts[i*2+2], 64)
	}

	rendered := renderQuad(corners[0][0], corners[0][1], corners[1][0], corners[1][1],
		corners[2][0], corners[2][1], corners[3][0], corners[3][1])
	startMatches := compareBitmaps(bitmap, rendered)

	fmt.Printf("Ultra-Fine Corner Optimization for Frame %d\n", frameNum)
	fmt.Println("Pushing to absolute limits with 1/512 and 1/1024 pixel steps")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Start: %.3f%% (%d/576, diff=%d)\n\n", 100.0*float64(startMatches)/576.0, startMatches, 576-startMatches)

	// Ultra-fine steps: down to 1/1024 pixel precision
	steps := []float64{1.0 / 512.0, 1.0 / 1024.0, 1.0 / 2048.0}

	for _, step := range steps {
		fmt.Printf("Optimizing with step size 1/%.0f pixels...\n", 1.0/step)
		iteration := 0
		maxIterations := 100
		for iteration < maxIterations {
			iteration++
			anyImprovement := false

			for cornerIdx := 0; cornerIdx < 4; cornerIdx++ {
				improved, matches := optimizeCornerUltraFine(bitmap, &corners, cornerIdx, step)
				if improved {
					anyImprovement = true
					matchPct := 100.0 * float64(matches) / 576.0
					fmt.Printf("  Iter %d, corner %d: %.3f%% (%d/576)\n", iteration, cornerIdx, matchPct, matches)
					if matches == 576 {
						fmt.Println("  ✓✓✓ PERFECT 100%! ✓✓✓")
						goto done
					}
				}
			}

			if !anyImprovement {
				break
			}
		}
		fmt.Println()
	}

done:
	rendered = renderQuad(corners[0][0], corners[0][1], corners[1][0], corners[1][1],
		corners[2][0], corners[2][1], corners[3][0], corners[3][1])
	finalMatches := compareBitmaps(bitmap, rendered)

	fmt.Println(strings.Repeat("=", 70))
	matchPct := 100.0 * float64(finalMatches) / 576.0
	fmt.Printf("FINAL: %.3f%% (%d/576, diff=%d)\n", matchPct, finalMatches, 576-finalMatches)

	if finalMatches == 576 {
		fmt.Println("✓✓✓ PERFECT 100% ACHIEVED! ✓✓✓")
		os.WriteFile(fmt.Sprintf("ultrafine_frame%d.txt", frameNum),
			[]byte(fmt.Sprintf("%d %.9f %.9f %.9f %.9f %.9f %.9f %.9f %.9f %d\n",
				frameNum, corners[0][0], corners[0][1], corners[1][0], corners[1][1],
				corners[2][0], corners[2][1], corners[3][0], corners[3][1], finalMatches)), 0644)
	} else if finalMatches > startMatches {
		fmt.Printf("\nImproved by %d pixels\n", finalMatches-startMatches)
		os.WriteFile(fmt.Sprintf("ultrafine_frame%d.txt", frameNum),
			[]byte(fmt.Sprintf("%d %.9f %.9f %.9f %.9f %.9f %.9f %.9f %.9f %d\n",
				frameNum, corners[0][0], corners[0][1], corners[1][0], corners[1][1],
				corners[2][0], corners[2][1], corners[3][0], corners[3][1], finalMatches)), 0644)
	} else {
		fmt.Println("\nNo improvement - already at maximum for this algorithm")
	}
}
