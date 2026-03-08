package main

import (
	"fmt"
	"math"
	"strings"
)

func main() {
	// Current best parameters (center-pivot model)
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

	fmt.Println("Ground Contact Analysis - Marching Square")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println()

	fmt.Println("If square is marching/rolling, pivot should be at bottom-most corner")
	fmt.Println()
	fmt.Println("Frame  Angle    Corner Positions (Y-coords)              Lowest   Contact Y")
	fmt.Println("-----  -------  ---------------------------------------  ------   ---------")

	var minGroundY, maxGroundY float64
	first := true

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

		// Find which corner is lowest (highest Y value = bottom of screen)
		lowestCornerIdx := 0
		lowestY := corners[0][1]
		for i := 1; i < 4; i++ {
			if corners[i][1] > lowestY {
				lowestY = corners[i][1]
				lowestCornerIdx = i
			}
		}

		if first {
			minGroundY = lowestY
			maxGroundY = lowestY
			first = false
		} else {
			if lowestY < minGroundY {
				minGroundY = lowestY
			}
			if lowestY > maxGroundY {
				maxGroundY = lowestY
			}
		}

		fmt.Printf("%3d    %7.2f°  ", frameNum, s.angle)
		for i := 0; i < 4; i++ {
			fmt.Printf("%5.2f ", corners[i][1])
		}
		fmt.Printf(" C%d    %6.3f\n", lowestCornerIdx, lowestY)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 75))
	fmt.Printf("Ground contact Y range: %.3f to %.3f (delta: %.3f pixels)\n",
		minGroundY, maxGroundY, maxGroundY-minGroundY)
	fmt.Println()

	if maxGroundY-minGroundY < 1.0 {
		fmt.Println("✓ Ground contact is STABLE - square maintains constant ground level")
		fmt.Println("  This suggests a ROLLING motion with fixed ground plane")
	} else {
		fmt.Println("✗ Ground contact VARIES significantly")
		fmt.Println("  Square may be bouncing, not pure rolling")
	}
	fmt.Println()

	// Check if we should use bottom corner as pivot
	fmt.Println("Expected pivot for marching square:")
	fmt.Printf("  Y-coordinate: ~%.1f (bottom edge of visible area)\n", maxGroundY)
	fmt.Printf("  Current average pivot Y: 11.280\n")
	fmt.Printf("  Difference: %.3f pixels\n", maxGroundY-11.280)
	fmt.Println()

	if maxGroundY > 11.280 {
		fmt.Println("⚠ Ground contact is BELOW current pivot!")
		fmt.Println("  Suggests square might be marching with bottom-corner pivot")
		fmt.Println("  Should re-optimize with pivot at ground contact point")
	} else {
		fmt.Println("Current pivot is at or below ground contact")
		fmt.Println("This doesn't match typical marching/rolling behavior")
	}
}
