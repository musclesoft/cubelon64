package main

import (
	"fmt"
)

// 8.8 fixed-point type
type Fixed88 int16

// Point in 8.8 fixed-point
type Point struct {
	X, Y Fixed88
}

// Test scanline rendering with exact positions
func main() {
	fmt.Println("Testing scanline fill algorithm")
	fmt.Println()

	// Test case: horizontal line at y=0.5 from x=0.5 to x=12.5
	// Should fill pixels 1 through 12 (12 pixels total)
	intersections := []int{
		128,  // 0.5 in 8.8 fixed-point
		3200, // 12.5 in 8.8 fixed-point
	}

	fmt.Println("Test 1: Horizontal span from 0.5 to 12.5")
	fmt.Printf("  Intersections: %d (%.2f), %d (%.2f)\n",
		intersections[0], float64(intersections[0])/256,
		intersections[1], float64(intersections[1])/256)

	// Method 1: Truncation
	x1_trunc := intersections[0] >> 8
	x2_trunc := intersections[1] >> 8
	fmt.Printf("  Truncation: x1=%d, x2=%d → fills %d to %d-1 = %d pixels\n",
		x1_trunc, x2_trunc, x1_trunc, x2_trunc, x2_trunc-x1_trunc)

	// Method 2: Rounding
	x1_round := (intersections[0] + 128) >> 8
	x2_round := (intersections[1] + 128) >> 8
	fmt.Printf("  Rounding: x1=%d, x2=%d → fills %d to %d-1 = %d pixels\n",
		x1_round, x2_round, x1_round, x2_round, x2_round-x1_round)

	fmt.Println()
	fmt.Println("Expected: 12 pixels (from pixel 0 or 1 through pixel 11 or 12)")
	fmt.Println()

	// Test case 2: from x=6.0 to x=18.0 (exactly 12 pixels)
	intersections2 := []int{
		1536, // 6.0 in 8.8
		4608, // 18.0 in 8.8
	}

	fmt.Println("Test 2: Horizontal span from 6.0 to 18.0")
	fmt.Printf("  Intersections: %d (%.2f), %d (%.2f)\n",
		intersections2[0], float64(intersections2[0])/256,
		intersections2[1], float64(intersections2[1])/256)

	x1_trunc = intersections2[0] >> 8
	x2_trunc = intersections2[1] >> 8
	fmt.Printf("  Truncation: x1=%d, x2=%d → fills %d to %d-1 = %d pixels\n",
		x1_trunc, x2_trunc, x1_trunc, x2_trunc, x2_trunc-x1_trunc)

	x1_round = (intersections2[0] + 128) >> 8
	x2_round = (intersections2[1] + 128) >> 8
	fmt.Printf("  Rounding: x1=%d, x2=%d → fills %d to %d-1 = %d pixels\n",
		x1_round, x2_round, x1_round, x2_round, x2_round-x1_round)

	fmt.Println()
	fmt.Println("Expected: 12 pixels (from pixel 6 through pixel 17)")
}
