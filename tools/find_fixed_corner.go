package main

import (
	"fmt"
	"math"
	"strings"
)

func main() {
	// Current best parameters (center-based model)
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

	fmt.Println("Fixed Corner Analysis - Marching Square")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println("For a marching square, one corner should stay fixed at ground level")
	fmt.Println()

	// For each frame, find the bottom-most corner
	type CornerInfo struct {
		frameNum   int
		cornerIdx  int
		x, y       float64
		allCorners [4][2]float64
	}

	var bottomCorners []CornerInfo

	for _, frameNum := range frames {
		s := squares[frameNum]
		angleRad := s.angle * math.Pi / 180.0

		// Calculate all 4 corner positions
		var corners [4][2]float64
		for i := 0; i < 4; i++ {
			cornerAngle := angleRad + float64(i)*math.Pi/2.0
			corners[i] = [2]float64{
				s.cx + s.size*math.Cos(cornerAngle),
				s.cy + s.size*math.Sin(cornerAngle),
			}
		}

		// Find bottom-most corner (highest Y = bottom of screen)
		lowestIdx := 0
		lowestY := corners[0][1]
		for i := 1; i < 4; i++ {
			if corners[i][1] > lowestY {
				lowestY = corners[i][1]
				lowestIdx = i
			}
		}

		bottomCorners = append(bottomCorners, CornerInfo{
			frameNum:   frameNum,
			cornerIdx:  lowestIdx,
			x:          corners[lowestIdx][0],
			y:          corners[lowestIdx][1],
			allCorners: corners,
		})
	}

	fmt.Println("Bottom-most corner for each frame:")
	fmt.Println("Frame  Corner#  Position (X, Y)")
	fmt.Println("-----  -------  -----------------")
	for _, info := range bottomCorners {
		fmt.Printf("%3d    C%d       (%7.3f, %7.3f)\n",
			info.frameNum, info.cornerIdx, info.x, info.y)
	}
	fmt.Println()

	// Calculate statistics for bottom corners
	var sumX, sumY float64
	var minX, maxX, minY, maxY float64
	first := true
	for _, info := range bottomCorners {
		sumX += info.x
		sumY += info.y
		if first {
			minX, maxX = info.x, info.x
			minY, maxY = info.y, info.y
			first = false
		} else {
			if info.x < minX {
				minX = info.x
			}
			if info.x > maxX {
				maxX = info.x
			}
			if info.y < minY {
				minY = info.y
			}
			if info.y > maxY {
				maxY = info.y
			}
		}
	}
	avgX := sumX / float64(len(bottomCorners))
	avgY := sumY / float64(len(bottomCorners))

	fmt.Println(strings.Repeat("=", 75))
	fmt.Printf("Average bottom corner: (%.3f, %.3f)\n", avgX, avgY)
	fmt.Printf("X range: %.3f to %.3f (delta: %.3f)\n", minX, maxX, maxX-minX)
	fmt.Printf("Y range: %.3f to %.3f (delta: %.3f)\n", minY, maxY, maxY-minY)
	fmt.Println()

	// Calculate max deviation from average
	var maxDev float64
	for _, info := range bottomCorners {
		dx := info.x - avgX
		dy := info.y - avgY
		dev := math.Sqrt(dx*dx + dy*dy)
		if dev > maxDev {
			maxDev = dev
		}
	}

	fmt.Printf("Maximum deviation from average: %.3f pixels\n", maxDev)
	fmt.Println()

	if maxDev < 0.5 {
		fmt.Println("✓✓✓ FIXED CORNER DETECTED!")
		fmt.Printf("    Bottom corners cluster at (%.3f, %.3f)\n", avgX, avgY)
		fmt.Println("    This IS a marching square with fixed ground-contact pivot!")
	} else if maxDev < 1.5 {
		fmt.Println("✓ NEARLY FIXED CORNER")
		fmt.Printf("    Bottom corners cluster loosely around (%.3f, %.3f)\n", avgX, avgY)
		fmt.Println("    Marching square with slight ground variations")
	} else {
		fmt.Println("✗ NO FIXED CORNER")
		fmt.Println("    Bottom corners move significantly between frames")
		fmt.Println("    NOT a marching square - likely floating/orbiting motion")
	}
	fmt.Println()

	// Also check other corners to see if any corner stays more fixed
	fmt.Println("Checking all 4 corners to find most stable one:")
	for cornerNum := 0; cornerNum < 4; cornerNum++ {
		var positions [][2]float64
		for _, info := range bottomCorners {
			positions = append(positions, info.allCorners[cornerNum])
		}

		// Calculate variance
		var sumX, sumY float64
		for _, pos := range positions {
			sumX += pos[0]
			sumY += pos[1]
		}
		meanX := sumX / float64(len(positions))
		meanY := sumY / float64(len(positions))

		var maxDist float64
		for _, pos := range positions {
			dx := pos[0] - meanX
			dy := pos[1] - meanY
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > maxDist {
				maxDist = dist
			}
		}

		fmt.Printf("  Corner %d: avg (%.3f, %.3f), max deviation %.3f\n",
			cornerNum, meanX, meanY, maxDist)
	}
}
