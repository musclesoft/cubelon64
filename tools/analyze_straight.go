package main
import ("fmt"; "math")
func main() {
	s326 := struct{cx,cy,size,angle float64}{11.750000, 11.250000, 7.753906, -1.70}
	s336 := struct{cx,cy,size,angle float64}{12.000000, 11.500000, 8.000000, 45.00}
	
	fmt.Println("Straight Positions: Frame 326 (-1.7°) and Frame 336 (45.0°)")
	fmt.Println("============================================================")
	
	// Frame 326
	fmt.Printf("\nFrame 326 (nearly horizontal, -1.7°):\n")
	fmt.Printf("  Pivot: (%.3f, %.3f)\n", s326.cx, s326.cy)
	angleRad := s326.angle * math.Pi / 180.0
	for i := 0; i < 4; i++ {
		cornerAngle := angleRad + float64(i)*math.Pi/2.0
		x := s326.cx + s326.size*math.Cos(cornerAngle)
		y := s326.cy + s326.size*math.Sin(cornerAngle)
		fmt.Printf("  Corner %d: (%.2f, %.2f)\n", i, x, y)
	}
	
	// Frame 336  
	fmt.Printf("\nFrame 336 (diamond, 45.0°):\n")
	fmt.Printf("  Pivot: (%.3f, %.3f)\n", s336.cx, s336.cy)
	angleRad = s336.angle * math.Pi / 180.0
	for i := 0; i < 4; i++ {
		cornerAngle := angleRad + float64(i)*math.Pi/2.0
		x := s336.cx + s336.size*math.Cos(cornerAngle)
		y := s336.cy + s336.size*math.Sin(cornerAngle)
		fmt.Printf("  Corner %d: (%.2f, %.2f)\n", i, x, y)
	}
	
	// Movement
	dx := s336.cx - s326.cx
	dy := s336.cy - s326.cy
	fmt.Printf("\nPivot shift: (%.3f, %.3f) → distance %.3f px\n", dx, dy, math.Sqrt(dx*dx+dy*dy))
	fmt.Printf("Rotation: %.1f° → %.1f° (Δ%.1f°)\n", s326.angle, s336.angle, s336.angle-s326.angle)
}
