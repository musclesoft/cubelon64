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

func renderSquare(cx, cy, size, angleDeg float64) [][]bool {
	bitmap := make([][]bool, 24)
	for i := range bitmap {
		bitmap[i] = make([]bool, 24)
	}

	angleRad := angleDeg * math.Pi / 180.0
	halfSize := size / 2.0
	cos, sin := math.Cos(angleRad), math.Sin(angleRad)

	corners := [4][2]float64{
		{-halfSize, -halfSize}, {halfSize, -halfSize},
		{halfSize, halfSize}, {-halfSize, halfSize},
	}

	var vertices [4][2]float64
	for i := 0; i < 4; i++ {
		x := corners[i][0]*cos - corners[i][1]*sin + cx
		y := corners[i][0]*sin + corners[i][1]*cos + cy
		vertices[i] = [2]float64{x, y}
	}

	edges := make([]*Edge, 4)
	for i := 0; i < 4; i++ {
		next := (i + 1) % 4
		edges[i] = newEdge(vertices[i][0], vertices[i][1], vertices[next][0], vertices[next][1])
	}

	for y := 0; y < 24; y++ {
		yf := float64(y)  // Match Python: no +0.5
		var activeEdges []*Edge
		for _, e := range edges {
			if e.yStart <= yf && yf < e.yEnd {  // Match Python order
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

		// Match Python: fill from first to last
		if len(xIntersects) >= 2 {
			x1 := xIntersects[0]
			x2 := xIntersects[len(xIntersects)-1]
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

func main() {
	content, err := os.ReadFile("../emulator/frames/animation_318-336.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Python perfect match parameters for frame 322
	cx, cy, size, angle := 12.269531, 11.359375, 8.085938, 70.9

	target, err := loadFrame(string(content), 322)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	rendered := renderSquare(cx, cy, size, angle)

	fmt.Println("Frame 322 Debug (Python-perfect params in Go)")
	fmt.Printf("cx=%.6f, cy=%.6f, size=%.6f, angle=%.2f°\n\n", cx, cy, size, angle)

	matches := 0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if target[y][x] == rendered[y][x] {
				fmt.Print("·")
				matches++
			} else if target[y][x] && !rendered[y][x] {
				fmt.Print("R") // Red: missing pixel
			} else {
				fmt.Print("B") // Blue: extra pixel
			}
		}
		fmt.Println()
	}

	fmt.Printf("\nMatch: %.3f%% (%d/576)\n", 100.0*float64(matches)/576.0, matches)
}
