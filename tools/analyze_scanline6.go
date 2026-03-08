package main

import (
	"fmt"
	"math"
)

// 8.8 fixed-point type
type Fixed88 int16

func toFixed88(f float64) Fixed88 {
	return Fixed88(f * 256.0)
}

func (f Fixed88) toFloat() float64 {
	return float64(f) / 256.0
}

// Point in 8.8 fixed-point
type Point struct {
	X, Y Fixed88
}

// Rotate with truncation
func rotateTruncated(p Point, pivot Point, cos256, sin256 int32) Point {
	dx := int32(p.X - pivot.X)
	dy := int32(p.Y - pivot.Y)

	rxTemp := dx*cos256 - dy*sin256
	ryTemp := dx*sin256 + dy*cos256

	// Simple truncation - no rounding
	rx := rxTemp >> 8
	ry := ryTemp >> 8

	return Point{
		X: pivot.X + Fixed88(rx),
		Y: pivot.Y + Fixed88(ry),
	}
}

func main() {
	fmt.Println("🔍 Detailed Analysis of Scanline y=6")
	fmt.Println()

	// Truncation-optimized parameters
	size := Fixed88(3072)
	centerX := Fixed88(3072)
	centerY := Fixed88(2944)
	pivotX := Fixed88(4608)
	pivotY := Fixed88(4480)

	// 45° angle
	angle := math.Pi / 4.0
	cos := math.Cos(angle)
	sin := math.Sin(angle)
	cos256 := int32(cos * 256)
	sin256 := int32(sin * 256)

	fmt.Printf("cos256 = %d (exact: %.10f)\n", cos256, float64(cos256)/256.0)
	fmt.Printf("sin256 = %d (exact: %.10f)\n", sin256, float64(sin256)/256.0)
	fmt.Println()

	// Compute rotated corners
	half := size / 2
	initial := [4]Point{
		{centerX - half, centerY - half},
		{centerX + half, centerY - half},
		{centerX + half, centerY + half},
		{centerX - half, centerY + half},
	}
	pivot := Point{pivotX, pivotY}

	var rotated [4]Point
	for i := 0; i < 4; i++ {
		rotated[i] = rotateTruncated(initial[i], pivot, cos256, sin256)
	}

	fmt.Println("Rotated corners:")
	for i := 0; i < 4; i++ {
		fmt.Printf("  Corner %d: (%d, %d) = (%.6f, %.6f)\n",
			i, rotated[i].X, rotated[i].Y, rotated[i].X.toFloat(), rotated[i].Y.toFloat())
	}
	fmt.Println()

	// Analyze scanline at y=6
	y := 6
	yFixed := Fixed88(y << 8)
	fmt.Printf("Analyzing scanline y=%d (yFixed=%d = %.6f)\n\n", y, yFixed, yFixed.toFloat())

	// Check each edge
	for i := 0; i < 4; i++ {
		p1 := rotated[i]
		p2 := rotated[(i+1)%4]

		fmt.Printf("Edge %d-%d:\n", i, (i+1)%4)
		fmt.Printf("  p1: (%d, %d) = (%.6f, %.6f)\n", p1.X, p1.Y, p1.X.toFloat(), p1.Y.toFloat())
		fmt.Printf("  p2: (%d, %d) = (%.6f, %.6f)\n", p2.X, p2.Y, p2.X.toFloat(), p2.Y.toFloat())

		crossesY6 := (p1.Y <= yFixed && p2.Y > yFixed) || (p2.Y <= yFixed && p1.Y > yFixed)
		fmt.Printf("  Crosses y=6? %v\n", crossesY6)

		if crossesY6 {
			dy := int32(p2.Y - p1.Y)
			if dy != 0 {
				t := int32(yFixed-p1.Y) * 256 / dy
				x := int32(p1.X) + (int32(p2.X-p1.X)*t)/256

				fmt.Printf("  Intersection calculation:\n")
				fmt.Printf("    dy = %d\n", dy)
				fmt.Printf("    yFixed - p1.Y = %d\n", int32(yFixed-p1.Y))
				fmt.Printf("    t = (yFixed - p1.Y) * 256 / dy = %d * 256 / %d = %d\n",
					int32(yFixed-p1.Y), dy, t)
				fmt.Printf("    p2.X - p1.X = %d\n", int32(p2.X-p1.X))
				fmt.Printf("    (p2.X - p1.X) * t = %d * %d = %d\n",
					int32(p2.X-p1.X), t, int32(p2.X-p1.X)*t)
				fmt.Printf("    ((p2.X - p1.X) * t) / 256 = %d / 256 = %d\n",
					int32(p2.X-p1.X)*t, (int32(p2.X-p1.X)*t)/256)
				fmt.Printf("    x = p1.X + offset = %d + %d = %d\n",
					int32(p1.X), (int32(p2.X-p1.X)*t)/256, x)
				fmt.Printf("    x in float: %.6f\n", float64(x)/256.0)
				fmt.Printf("    x rounded: (%d + 128) >> 8 = %d >> 8 = %d\n",
					x, x+128, (x+128)>>8)
			}
		}
		fmt.Println()
	}

	// Now compute final intersections
	fmt.Println("Final intersection processing:")
	var intersections []int
	for i := 0; i < 4; i++ {
		p1 := rotated[i]
		p2 := rotated[(i+1)%4]

		if (p1.Y <= yFixed && p2.Y > yFixed) || (p2.Y <= yFixed && p1.Y > yFixed) {
			dy := int32(p2.Y - p1.Y)
			if dy != 0 {
				t := int32(yFixed-p1.Y) * 256 / dy
				x := int32(p1.X) + (int32(p2.X-p1.X)*t)/256
				intersections = append(intersections, int(x))
				fmt.Printf("  Edge %d-%d: x = %d (%.6f)\n", i, (i+1)%4, x, float64(x)/256.0)
			}
		}
	}

	// Sort intersections
	for i := 0; i < len(intersections)-1; i++ {
		for j := i + 1; j < len(intersections); j++ {
			if intersections[i] > intersections[j] {
				intersections[i], intersections[j] = intersections[j], intersections[i]
			}
		}
	}

	fmt.Printf("\nSorted intersections: %v\n", intersections)
	for i, x := range intersections {
		fmt.Printf("  [%d] = %d (%.6f)\n", i, x, float64(x)/256.0)
	}
	fmt.Println()

	// Fill scanline
	fmt.Println("Filling scanline:")
	for i := 0; i+1 < len(intersections); i += 2 {
		x1 := (intersections[i] + 128) >> 8
		x2 := (intersections[i+1] + 128) >> 8

		fmt.Printf("  Span %d:\n", i/2)
		fmt.Printf("    x1_raw = %d (%.6f) → x1_rounded = (%d + 128) >> 8 = %d >> 8 = %d\n",
			intersections[i], float64(intersections[i])/256.0,
			intersections[i], intersections[i]+128, x1)
		fmt.Printf("    x2_raw = %d (%.6f) → x2_rounded = (%d + 128) >> 8 = %d >> 8 = %d\n",
			intersections[i+1], float64(intersections[i+1])/256.0,
			intersections[i+1], intersections[i+1]+128, x2)
		fmt.Printf("    Fill range: x in [%d, %d) → pixels:", x1, x2)
		for x := x1; x < x2; x++ {
			if x >= 0 && x < 24 {
				fmt.Printf(" %d", x)
			} else if x >= 24 {
				fmt.Printf(" %d(=%d)", x, x%24)
			}
		}
		fmt.Println()
	}

	fmt.Println()
	fmt.Println("Analysis of pixel at x=12:")
	fmt.Printf("  For x=12 to be filled, it must be in range [x1, x2) where:\n")
	fmt.Printf("  x1 <= 12 < x2\n")
	fmt.Println()
	fmt.Println("  Let's check if this is a rounding edge case:")
	for i := 0; i+1 < len(intersections); i += 2 {
		x1Raw := intersections[i]
		x2Raw := intersections[i+1]
		x1 := (x1Raw + 128) >> 8
		x2 := (x2Raw + 128) >> 8

		if x1 <= 12 && 12 < x2 {
			fmt.Printf("  Pixel 12 IS filled by span %d:\n", i/2)
			fmt.Printf("    x1_raw = %d → %.6f pixels\n", x1Raw, float64(x1Raw)/256.0)
			fmt.Printf("    x2_raw = %d → %.6f pixels\n", x2Raw, float64(x2Raw)/256.0)
			fmt.Printf("    With rounding: [%d, %d) includes 12\n", x1, x2)

			// What would truncation do?
			x1Trunc := x1Raw >> 8
			x2Trunc := x2Raw >> 8
			fmt.Printf("\n  If we used truncation instead of rounding:\n")
			fmt.Printf("    x1_trunc = %d >> 8 = %d\n", x1Raw, x1Trunc)
			fmt.Printf("    x2_trunc = %d >> 8 = %d\n", x2Raw, x2Trunc)
			fmt.Printf("    Truncated range: [%d, %d)\n", x1Trunc, x2Trunc)
			if x1Trunc <= 12 && 12 < x2Trunc {
				fmt.Printf("    Would STILL include pixel 12\n")
			} else {
				fmt.Printf("    Would NOT include pixel 12\n")
			}

			// Boundary analysis
			fmt.Printf("\n  Boundary analysis:\n")
			fmt.Printf("    x1_raw = %d = %d * 256 + %d\n", x1Raw, x1Raw>>8, x1Raw&0xFF)
			fmt.Printf("           = %d + %d/256 = %.6f\n", x1Raw>>8, x1Raw&0xFF, float64(x1Raw)/256.0)
			fmt.Printf("    x2_raw = %d = %d * 256 + %d\n", x2Raw, x2Raw>>8, x2Raw&0xFF)
			fmt.Printf("           = %d + %d/256 = %.6f\n", x2Raw>>8, x2Raw&0xFF, float64(x2Raw)/256.0)

			fracX1 := x1Raw & 0xFF
			fracX2 := x2Raw & 0xFF
			fmt.Printf("\n  Fractional parts:\n")
			fmt.Printf("    x1 fraction: %d/256 = %.6f\n", fracX1, float64(fracX1)/256.0)
			fmt.Printf("    x2 fraction: %d/256 = %.6f\n", fracX2, float64(fracX2)/256.0)

			if fracX1 >= 128 {
				fmt.Printf("    x1 rounded UP from %d to %d\n", x1Raw>>8, x1)
			} else {
				fmt.Printf("    x1 rounded DOWN (stayed at %d)\n", x1)
			}
			if fracX2 >= 128 {
				fmt.Printf("    x2 rounded UP from %d to %d\n", x2Raw>>8, x2)
			} else {
				fmt.Printf("    x2 rounded DOWN (stayed at %d)\n", x2)
			}
		}
	}
}
