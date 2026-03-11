package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// Fixed88 represents a fixed-point number with 8 integer bits and 8 fractional bits
type Fixed88 int16

func NewFixed88(i int) Fixed88 {
	return Fixed88(i << 8)
}

func (f Fixed88) ToInt() int {
	return int(f >> 8)
}

func (f Fixed88) Mul(other Fixed88) Fixed88 {
	return Fixed88((int32(f) * int32(other)) >> 8)
}

// Coordinate represents a 2D point in fixed-point format
type Coordinate struct {
	X, Y Fixed88
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run generate_reference_24x24.go <angle_degrees> <output.png>")
		os.Exit(1)
	}

	angleDeg := 0
	fmt.Sscanf(os.Args[1], "%d", &angleDeg)
	outputPath := os.Args[2]

	// Convert angle to radians
	angleRad := float64(angleDeg) * math.Pi / 180.0

	// Calculate cos256 and sin256 (scaled by 256)
	cos256 := int(math.Cos(angleRad) * 256.0)
	sin256 := int(math.Sin(angleRad) * 256.0)

	fmt.Printf("Generating 24×24 reference frame for %d° rotation\n", angleDeg)
	fmt.Printf("cos256 = %d (0x%04X), sin256 = %d (0x%04X)\n", cos256, cos256, sin256, sin256)

	// Calculate rotated corners
	corners := calculateCorners(Fixed88(cos256), Fixed88(sin256))

	// Render using scanline algorithm (24×24 grid, no wrapping)
	grid := renderSquare(corners)

	// Save as PNG
	img := createImage(grid)
	saveImage(img, outputPath)

	fmt.Printf("Reference frame saved to: %s\n", outputPath)
}

func calculateCorners(cos256, sin256 Fixed88) [4]Coordinate {
	// Square corners before rotation (relative to pivot at 18,18)
	// Top-left: (6,6) → relative (-12,-12)
	// Top-right: (30,6) → relative (12,-12)
	// Bottom-right: (30,30) → relative (12,12)
	// Bottom-left: (6,30) → relative (-12,12)

	pivot := Coordinate{NewFixed88(18), NewFixed88(18)}

	relativeCorners := [4]Coordinate{
		{NewFixed88(-12), NewFixed88(-12)}, // Top-left
		{NewFixed88(12), NewFixed88(-12)},  // Top-right
		{NewFixed88(12), NewFixed88(12)},   // Bottom-right
		{NewFixed88(-12), NewFixed88(12)},  // Bottom-left
	}

	var corners [4]Coordinate
	for i, rel := range relativeCorners {
		// Rotate: x' = x*cos - y*sin, y' = x*sin + y*cos
		rotX := rel.X.Mul(cos256) - rel.Y.Mul(sin256)
		rotY := rel.X.Mul(sin256) + rel.Y.Mul(cos256)

		// Add pivot
		corners[i] = Coordinate{
			X: pivot.X + rotX,
			Y: pivot.Y + rotY,
		}

		fmt.Printf("Corner %d: (%d,%d) → (%.2f,%.2f)\n",
			i,
			rel.X.ToInt(), rel.Y.ToInt(),
			float64(corners[i].X)/256.0, float64(corners[i].Y)/256.0)
	}

	return corners
}

func renderSquare(corners [4]Coordinate) [24][24]bool {
	var grid [24][24]bool

	// Process each scanline
	for y := 0; y < 24; y++ {
		intersections := findIntersections(y, corners)

		if len(intersections) == 2 {
			x1, x2 := intersections[0], intersections[1]
			if x1 > x2 {
				x1, x2 = x2, x1
			}

			// Draw span (exclusive of x2)
			// Clamp to 0-23 range (no wrapping for 24×24 grid)
			if x1 < 0 {
				x1 = 0
			}
			if x2 > 24 {
				x2 = 24
			}

			for x := x1; x < x2; x++ {
				if x >= 0 && x < 24 {
					grid[y][x] = true
				}
			}
		}
	}

	return grid
}

func findIntersections(y int, corners [4]Coordinate) []int {
	var intersections []int

	edges := [][2]int{
		{0, 1}, {1, 2}, {2, 3}, {3, 0},
	}

	for _, edge := range edges {
		c0, c1 := corners[edge[0]], corners[edge[1]]
		y0, y1 := c0.Y.ToInt(), c1.Y.ToInt()

		// Check if scanline intersects this edge
		hasIntersection := false
		if y0 < y1 {
			// Upward edge: y0 <= y < y1
			hasIntersection = (y >= y0 && y < y1)
		} else if y0 > y1 {
			// Downward edge: y1 <= y < y0
			hasIntersection = (y >= y1 && y < y0)
		}

		if hasIntersection {
			// Linear interpolation to find X
			x0, x1 := c0.X.ToInt(), c1.X.ToInt()
			dY := y1 - y0
			if dY == 0 {
				intersections = append(intersections, x0)
			} else {
				dX := x1 - x0
				x := x0 + (dX*(y-y0))/dY
				intersections = append(intersections, x)
			}
		}
	}

	return intersections
}

func createImage(grid [24][24]bool) *image.RGBA {
	// Create 24x24 image scaled 8x for visibility
	scale := 8
	img := image.NewRGBA(image.Rect(0, 0, 24*scale, 24*scale))

	// Fill with yellow background
	yellow := color.RGBA{0xFF, 0xFF, 0x00, 0xFF}
	blue := color.RGBA{0x00, 0x00, 0xFF, 0xFF}

	for y := 0; y < 24*scale; y++ {
		for x := 0; x < 24*scale; x++ {
			gridY := y / scale
			gridX := x / scale
			if grid[gridY][gridX] {
				img.Set(x, y, blue)
			} else {
				img.Set(x, y, yellow)
			}
		}
	}

	return img
}

func saveImage(img *image.RGBA, path string) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		fmt.Printf("Error encoding PNG: %v\n", err)
		os.Exit(1)
	}
}
