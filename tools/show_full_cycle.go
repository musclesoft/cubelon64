package main
import ("fmt";"image";"image/color";"image/png";"os";"regexp";"strconv";"strings")

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

func loadFrame(content, file string, frame int) ([][]bool, error) {
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
	c2, _ := os.ReadFile("../emulator/frames/animation_336-354.txt")
	
	frames := []int{326, 328, 330, 332, 334, 336, 338}
	var bitmaps [][][]bool
	
	for _, f := range frames {
		var b [][]bool
		var err error
		if f <= 336 { b, err = loadFrame(string(c1), "318-336", f)
		} else { b, err = loadFrame(string(c2), "336-354", f) }
		if err == nil { bitmaps = append(bitmaps, b) }
	}
	
	scale := 10
	w := len(bitmaps)*24*scale
	h := 24*scale
	img := image.NewRGBA(image.Rect(0,0,w,h))
	for y:=0;y<h;y++ { for x:=0;x<w;x++ { img.Set(x,y,color.RGBA{240,240,240,255}) }}
	
	for i, bm := range bitmaps {
		ox := i*24*scale
		for y:=0;y<24;y++ {
			for x:=0;x<24;x++ {
				col := color.RGBA{255,255,255,255}
				if bm[y][x] { col = color.RGBA{0,0,0,255} }
				for dy:=0;dy<scale;dy++ {
					for dx:=0;dx<scale;dx++ {
						img.Set(ox+x*scale+dx, y*scale+dy, col)
					}
				}
			}
		}
		for y:=0;y<h;y++ { img.Set(ox+24*scale-1,y,color.RGBA{200,0,0,255}) }
	}
	
	f,_:=os.Create("full_cycle.png")
	defer f.Close()
	png.Encode(f,img)
	fmt.Println("Created full_cycle.png - frames 326,328,330,332,334,336,338")
}
