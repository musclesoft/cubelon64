package main

import (
	"fmt"
	"math"
	"strings"
)

func main() {
	// Current best parameters
	squares := map[int]struct{ cx, cy, size, angle float64 }{
		318: {12.003906, 11.496094, 7.906250, 33.70},
		320: {11.750000, 10.875000, 8.109375, 63.50},
		322: {12.269531, 11.359375, 8.085938, 70.90},
		324: {11.597656, 11.175781, 7.914062, 81.40},
		326: {11.750000, 11.250000, 7.753906, -1.70},
		328: {12.250000, 11.050781, 7.988281, 11.20},
		330: {11.664062, 11.410156, 8.066406, 17.30},
		332: {11.750000, 11.023438, 8.132812, 26.60},
		334: {11.750000, 11.656250, 8.054688, -146.30},
		336: {12.000000, 11.500000, 8.000000, 45.00},
	}

	frames := []int{318, 320, 322, 324, 326, 328, 330, 332, 334, 336}

	fmt.Println("Fixed Edge Analysis - Marching Square")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println("For marching: square pivots on bottom EDGE (2 corners), not single corner")
	fmt.Println()

	type EdgeInfo struct {
		frameNum int
		angle    float64
		corners  [4][2]float64
		edge1Idx int
		edge2Idx int
		edge1    [2]float64
		edge2    [2]float64
		edgeMidX float64
		edgeMidY float64
	}

	var edges []EdgeInfo

	for _, frameNum := range frames {
		s := squares[frameNum]
		angleRad := s.angle * math.Pi / 180.0

		// Calculate all 4 corners
		var corners [4][2]float64
		for i := 0; i < 4; i++ {
			cornerAngle := angleRad + float64(i)*math.Pi/2.0
			corners[i] = [2]float64{
				s.cx + s.size*math.Cos(cornerAngle),
				s.cy + s.size*math.Sin(cornerAngle),
			}
		}

		// Find the bottom edge (the edge with highest average Y of its 2 corners)
		// Check all 4 edges
		var bestEdge1, bestEdge2 int
		var bestAvgY float64 = -999.0

		for i := 0; i < 4; i++ {
			j := (i + 1) % 4
			avgY := (corners[i][1] + corners[j][1]) / 2.0
			if avgY > bestAvgY {
				bestAvgY = avgY
				bestEdge1 = i
				bestEdge2 = j
			}
		}

		edgeMidX := (corners[bestEdge1][0] + corners[bestEdge2][0]) / 2.0
		edgeMidY := (corners[bestEdge1][1] + corners[bestEdge2][1]) / 2.0

		edges = append(edges, EdgeInfo{
			frameNum: frameNum,
			angle:    s.angle,
			corners:  corners,
			edge1Idx: bestEdge1,
			edge2Idx: bestEdge2,
			edge1:    corners[bestEdge1],
			edge2:    corners[bestEdge2],
			edgeMidX: edgeMidX,
			edgeMidY: edgeMidY,
		})
	}

	fmt.Println("Bottom edge midpoint for each frame:")
	fmt.Println("Frame  Angle      Edge(corners)  Midpoint(X, Y)        Corner1         Corner2")
	fmt.Println("-----  --------   ------------   -----------------    --------------  --------------")
	for _, info := range edges {
		fmt.Printf("%3d    %7.2f°   C%d-C%d          (%6.2f, %6.2f)   (%6.2f,%6.2f)  (%6.2f,%6.2f)\n",
			info.frameNum, info.angle,
			info.edge1Idx, info.edge2Idx,
			info.edgeMidX, info.edgeMidY,
			info.edge1[0], info.edge1[1],
			info.edge2[0], info.edge2[1])
	}
	fmt.Println()

	// Calculate statistics for edge midpoints
	var sumX, sumY float64
	var minX, maxX, minY, maxY float64
	first := true
	for _, info := range edges {
		sumX += info.edgeMidX
		sumY += info.edgeMidY
		if first {
			minX, maxX = info.edgeMidX, info.edgeMidX
			minY, maxY = info.edgeMidY, info.edgeMidY
			first = false
		} else {
			if info.edgeMidX < minX {
				minX = info.edgeMidX
			}
			if info.edgeMidX > maxX {
				maxX = info.edgeMidX
			}
			if info.edgeMidY < minY {
				minY = info.edgeMidY
			}
			if info.edgeMidY > maxY {
				maxY = info.edgeMidY
			}
		}
	}
	avgX := sumX / float64(len(edges))
	avgY := sumY / float64(len(edges))

	fmt.Println(strings.Repeat("=", 75))
	fmt.Printf("Average edge midpoint: (%.3f, %.3f)\n", avgX, avgY)
	fmt.Printf("X range: %.3f to %.3f (delta: %.3f)\n", minX, maxX, maxX-minX)
	fmt.Printf("Y range: %.3f to %.3f (delta: %.3f)\n", minY, maxY, maxY-minY)
	fmt.Println()

	var maxDev float64
	for _, info := range edges {
		dx := info.edgeMidX - avgX
		dy := info.edgeMidY - avgY
		dev := math.Sqrt(dx*dx + dy*dy)
		if dev > maxDev {
			maxDev = dev
		}
	}

	fmt.Printf("Maximum deviation from average: %.3f pixels\n", maxDev)
	fmt.Println()

	if maxDev < 0.5 {
		fmt.Println("✓✓✓ FIXED EDGE PIVOT!")
		fmt.Println("    Bottom edge midpoints cluster tightly")
		fmt.Println("    This IS a marching square rotating around fixed bottom edge!")
	} else if maxX-minX < 2.0 && maxY-minY < 1.0 {
		fmt.Println("✓ NEARLY FIXED EDGE")
		fmt.Println("    Bottom edge shows good stability")
		fmt.Println("    Marching square with edge pivot (slight variations)")
	} else {
		fmt.Println("✗ Edge still moves significantly")
		fmt.Printf("    X movement: %.1f pixels (too much for fixed edge)\n", maxX-minX)
	}
	fmt.Println()

	// Look for sequences where edge stays fixed
	fmt.Println("Looking for fixed-edge sequences:")
	fmt.Println("(checking if same 2 corners stay at bottom for multiple consecutive frames)")
	fmt.Println()

	for i := 0; i < len(edges)-1; i++ {
		curr := edges[i]
		next := edges[i+1]

		// Check if same edge (same 2 corner indices)
		samePair := (curr.edge1Idx == next.edge1Idx && curr.edge2Idx == next.edge2Idx) ||
			(curr.edge1Idx == next.edge2Idx && curr.edge2Idx == next.edge1Idx)

		// Also check actual corner positions
		dist1 := math.Sqrt(math.Pow(curr.edge1[0]-next.edge1[0], 2) + math.Pow(curr.edge1[1]-next.edge1[1], 2))
		dist2 := math.Sqrt(math.Pow(curr.edge2[0]-next.edge2[0], 2) + math.Pow(curr.edge2[1]-next.edge2[1], 2))
		avgDist := (dist1 + dist2) / 2.0

		fmt.Printf("  %d→%d: ", curr.frameNum, next.frameNum)
		if samePair {
			fmt.Printf("Same edge indices (C%d-C%d) ", curr.edge1Idx, curr.edge2Idx)
		} else {
			fmt.Printf("Different edges (C%d-C%d → C%d-C%d) ",
				curr.edge1Idx, curr.edge2Idx, next.edge1Idx, next.edge2Idx)
		}
		fmt.Printf("Corner movement: %.3f px\n", avgDist)
	}
}
