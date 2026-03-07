package main

import (
	"fmt"
	"math"
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

func floatToFP(f float64) fp88        { return fp88(f * fpScale) }
func fpMul(a, b fp88) fp88              { return fp88((int64(a) * int64(b)) >> fpShift) }
func fpToInt(f fp88) int                { return int(f >> fpShift) }

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
	e := &Edge{yStart: y1, yEnd: y2, xStart: x1}
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

func renderSquare(cx, cy, size, angleDeg float64, width, height int) [][]bool {
	bitmap := make([][]bool, height)
	for i := range bitmap {
		bitmap[i] = make([]bool, width)
	}

	angleRad := angleDeg * math.Pi / 180.0

	vertices := [4][2]float64{}
	for i := 0; i < 4; i++ {
		cornerAngle := angleRad + float64(i)*math.Pi/2.0
		x := cx + size*math.Cos(cornerAngle)
		y := cy + size*math.Sin(cornerAngle)
		vertices[i] = [2]float64{x, y}
	}

	edges := make([]*Edge, 4)
	for i := 0; i < 4; i++ {
		next := (i + 1) % 4
		edges[i] = newEdge(vertices[i][0], vertices[i][1], vertices[next][0], vertices[next][1])
	}

	for y := 0; y < height; y++ {
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

func parseFrame(lines []string) ([][]bool, error) {
	bitmap := make([][]bool, 24)
	lineIdx := 0
	for lineIdx < len(lines) && !strings.HasPrefix(lines[lineIdx], "===") {
		lineIdx++
	}
	if lineIdx >= len(lines) {
		return nil, fmt.Errorf("no frame header found")
	}
	lineIdx++

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

type Params struct {
	cx, cy, size, angle float64
}

func main() {
	content, err := os.ReadFile("../emulator/frames/animation_318-336.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	params := map[int]Params{
		320: {11.859375, 11.070312, 7.902344, 63.50},
		322: {12.269531, 11.359375, 8.085938, 70.90},
		324: {11.625000, 11.078125, 7.863281, 78.80},
		326: {11.750000, 11.250000, 7.753906, -1.70},
		328: {12.382812, 11.078125, 7.859375, 11.20},
		330: {11.664062, 11.410156, 8.066406, 17.30},
		332: {12.007812, 10.992188, 7.789062, 24.50},
		334: {11.851562, 11.531250, 7.910156, 33.70},
	}

	fmt.Println("Final Verification: All Optimized Parameters")
	fmt.Println(strings.Repeat("=", 70))

	totalMatches := 0
	totalPixels := 0
	perfectCount := 0

	for frame := 320; frame <= 334; frame += 2 {
		p := params[frame]
		bitmap, err := loadFrame(string(content), frame)
		if err != nil {
			continue
		}

		rendered := renderSquare(p.cx, p.cy, p.size, p.angle, 24, 24)
		matches := compareBitmaps(bitmap, rendered)
		matchPct := 100.0 * float64(matches) / 576.0
		diff := 576 - matches

		totalMatches += matches
		totalPixels += 576

		perfect := ""
		if matches == 576 {
			perfect = "  ✓ PERFECT"
			perfectCount++
		}

		fmt.Printf("Frame %d: %5.3f%% (%3d/576, diff=%2d)%s\n",
			frame, matchPct, matches, diff, perfect)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	avgMatch := 100.0 * float64(totalMatches) / float64(totalPixels)
	avgDiff := float64(totalPixels-totalMatches) / float64(len(params))
	fmt.Printf("Total:   %d/%d (%.3f%%)\n", totalMatches, totalPixels, avgMatch)
	fmt.Printf("Average: %.3f%% (%.1f pixels/frame)\n", avgMatch, avgDiff)
	fmt.Printf("Perfect: %d/%d frames (%.1f%%)\n",
		perfectCount, len(params), 100.0*float64(perfectCount)/float64(len(params)))
}
