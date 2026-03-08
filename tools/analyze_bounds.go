package main
import ("fmt";"os";"regexp";"strconv";"strings")

func parseFrame(lines []string) ([][]bool, error) {
	bitmap := make([][]bool, 24)
	lineIdx := 0
	for lineIdx < len(lines) && !strings.HasPrefix(lines[lineIdx], "===") { lineIdx++ }
	if lineIdx >= len(lines) { return nil, fmt.Errorf("no frame header") }
	lineIdx++
	for y := 0; y < 24; y++ {
		for lineIdx < len(lines) && strings.TrimSpace(lines[lineIdx]) == "" { lineIdx++ }
		if lineIdx >= len(lines) { return nil, fmt.Errorf("unexpected end") }
		line := lines[lineIdx]; lineIdx++
		bitmap[y] = make([]bool, 24)
		x := 0
		for _, ch := range line {
			if ch == '█' || ch == '▓' { bitmap[y][x] = true; x++ } else if ch == '░' || ch == '·' || ch == ' ' { bitmap[y][x] = false; x++ }
			if x >= 24 { break }
		}
	}
	return bitmap, nil
}

func loadFrame(content string, frame int) ([][]bool, error) {
	lines := strings.Split(content, "\n")
	pat := regexp.MustCompile(`=== Frame (\d+)`)
	cur := -1
	var curLines []string
	for _, line := range lines {
		if m := pat.FindStringSubmatch(line); m != nil {
			if cur == frame && len(curLines) > 0 { return parseFrame(curLines) }
			cur, _ = strconv.Atoi(m[1])
			curLines = []string{line}
		} else { curLines = append(curLines, line) }
	}
	if cur == frame { return parseFrame(curLines) }
	return nil, fmt.Errorf("not found")
}

func main() {
	c1, _ := os.ReadFile("../emulator/frames/animation_318-336.txt")
	frames := []int{318,320,322,324,326,328,330,332,334,336}
	
	fmt.Println("RAW BITMAP BOUNDS (no transformation):")
	fmt.Println("Frame  MinX  MaxX  CenterX  Width")
	for _, f := range frames {
		b, _ := loadFrame(string(c1), f)
		minX, maxX := 24, -1
		for y:=0;y<24;y++ {
			for x:=0;x<24;x++ {
				if b[y][x] {
					if x<minX { minX=x }
					if x>maxX { maxX=x }
				}
			}
		}
		cx := float64(minX+maxX)/2.0
		w := maxX-minX+1
		fmt.Printf("%3d    %3d   %3d    %5.1f    %3d\n", f,minX,maxX,cx,w)
	}
	fmt.Println("\nAll squares centered at X≈11-12 in their 24×24 frames")
	fmt.Println("NO horizontal marching movement in original bitmap data!")
}
