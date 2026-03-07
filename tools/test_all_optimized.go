package main

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
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
func renderSquare(cx, cy, size, angleDeg float64, width, height int) [][]bool {
	bitmap := make([][]bool, height)
	for i := range bitmap {
		bitmap[i] = make([]bool, width)
	}

	angleRad := angleDeg * math.Pi / 180.0
	halfSize := size / 2.0

	cos := math.Cos(angleRad)
	sin := math.Sin(angleRad)

	// Four corners
	corners := [4][2]float64{
		{-halfSize, -halfSize},
		{halfSize, -halfSize},
		{halfSize, halfSize},
		{-halfSize, halfSize},
	}

	vertices := [4][2]float64{}
	for i := 0; i < 4; i++ {
		x := corners[i][0]*cos - corners[i][1]*sin + cx
		y := corners[i][0]*sin + corners[i][1]*cos + cy
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
		yf := float64(y) + 0.5

		// Find active edges
		var activeEdges []*Edge
		for _, e := range edges {
			if yf >= e.yStart && yf < e.yEnd {
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

		// Fill spans
		for i := 0; i < len(xIntersects); i += 2 {
			if i+1 >= len(xIntersects) {
				break
			}

			x1 := xIntersects[i]
			x2 := xIntersects[i+1]

			if x1 < 0 {
				x1 = 0
			}
			if x2 >= width {
				x2 = width - 1
			}

			for x := x1; x <= x2; x++ {
				if x >= 0 && x < width {
					bitmap[y][x] = true
				}
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

type Params struct {
	cx, cy, size, angle float64
}

func main() {
	fmt.Println("Testing all optimized parameters (Go + Python best)")
	fmt.Println(strings.Repeat("=", 70))

	// Load animation
	content, err := os.ReadFile("../emulator/frames/animation_318-336.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading animation file: %v\n", err)
		os.Exit(1)
	}

	// Best parameters: Python perfect matches + Go optimized
	params := map[int]Params{
		320: {12.109375, 11.187500, 9.410156, 63.50}, // Go optimized
		322: {12.269531, 11.359375, 8.085938, 70.9},  // Python perfect
		324: {11.750000, 11.250000, 9.335938, 76.00}, // Go optimized
		326: {11.750000, 11.250000, 8.628906, -1.20}, // Go optimized
		328: {12.484375, 11.101562, 9.484375, 6.10},  // Go optimized
		330: {11.664062, 11.410156, 8.066406, 17.3},  // Python perfect
		332: {12.007812, 11.195312, 9.480469, 21.90}, // Go optimized
		334: {12.226562, 11.640625, 9.437500, 33.70}, // Go optimized
	}

	totalMatches := 0
	totalPixels := 0

	for frame := 320; frame <= 334; frame += 2 {
		p, ok := params[frame]
		if !ok {
			continue
		}

		bitmap, err := loadFrame(string(content), frame)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading frame %d: %v\n", frame, err)
			continue
		}

		rendered := renderSquare(p.cx, p.cy, p.size, p.angle, 24, 24)
		matches := compareBitmaps(bitmap, rendered)
		matchPct := 100.0 * float64(matches) / 576.0
		diff := 576 - matches

		totalMatches += matches
		totalPixels += 576

		fmt.Printf("Frame %d: %.3f%% (%d/576, diff=%d)\n", frame, matchPct, matches, diff)
		if matches == 576 {
			fmt.Printf("  ✓ PERFECT!\n")
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	avgMatch := 100.0 * float64(totalMatches) / float64(totalPixels)
	avgDiff := float64(totalPixels-totalMatches) / float64(len(params))
	fmt.Printf("Average: %.3f%%\n", avgMatch)
	fmt.Printf("Average diff: %.1f pixels/frame\n", avgDiff)

	perfectCount := 0
	for frame := range params {
		bitmap, _ := loadFrame(string(content), frame)
		p := params[frame]
		rendered := renderSquare(p.cx, p.cy, p.size, p.angle, 24, 24)
		if compareBitmaps(bitmap, rendered) == 576 {
			perfectCount++
		}
	}

	if perfectCount > 0 {
		fmt.Printf("\n✓ %d PERFECT matches!\n", perfectCount)
	}
}
