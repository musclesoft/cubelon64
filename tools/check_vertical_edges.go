package main
import ("fmt"; "math")
func main() {
	frames := []struct{num int; cx,cy,size,angle float64}{
		{318, 12.003906, 11.496094, 7.906250, 33.70},
		{320, 11.750000, 10.875000, 8.109375, 63.50},
		{322, 12.269531, 11.359375, 8.085938, 70.90},
		{324, 11.597656, 11.175781, 7.914062, 81.40},
		{326, 11.750000, 11.250000, 7.753906, -1.70},
		{328, 12.250000, 11.050781, 7.988281, 11.20},
		{330, 11.664062, 11.410156, 8.066406, 17.30},
		{332, 11.750000, 11.023438, 8.132812, 26.60},
		{334, 11.750000, 11.656250, 8.054688, -146.30},
		{336, 12.000000, 11.500000, 8.000000, 45.00},
	}
	
	fmt.Println("Leftmost and Rightmost X-coordinates for each frame:")
	fmt.Println("(checking if left/right edges stay fixed)")
	fmt.Println("Frame  Angle     LeftX   RightX  Width  Center-X")
	for _, f := range frames {
		angleRad := f.angle * math.Pi / 180.0
		minX, maxX := 999.0, -999.0
		for i := 0; i < 4; i++ {
			cornerAngle := angleRad + float64(i)*math.Pi/2.0
			x := f.cx + f.size*math.Cos(cornerAngle)
			if x < minX { minX = x }
			if x > maxX { maxX = x }
		}
		width := maxX - minX
		centerX := (minX + maxX) / 2.0
		fmt.Printf("%3d   %7.2f°  %6.2f  %6.2f  %5.2f  %6.2f\n",
			f.num, f.angle, minX, maxX, width, centerX)
	}
}
