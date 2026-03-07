package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Read the JSON and check pattern structure
type PatternFrame struct {
	Frame        int       `json:"frame"`
	PatternAddr  int       `json:"pattern_addr"`
	PatternBytes string    `json:"pattern_bytes"`
	Pixels       [][]int   `json:"pixels"`
}

func main() {
	data, err := os.ReadFile("../emulator/part3_tiles_900-960.json")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var frames []PatternFrame
	if err := json.Unmarshal(data, &frames); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Total frames: %d\n\n", len(frames))

	// Group by pattern address
	addrFrames := make(map[int][]int)
	for _, f := range frames {
		addrFrames[f.PatternAddr] = append(addrFrames[f.PatternAddr], f.Frame)
	}

	fmt.Printf("Unique pattern addresses: %d\n", len(addrFrames))

	// Check spacing
	var addrs []int
	for addr := range addrFrames {
		addrs = append(addrs, addr)
	}

	// Sort
	for i := 0; i < len(addrs)-1; i++ {
		for j := i+1; j < len(addrs); j++ {
			if addrs[i] > addrs[j] {
				addrs[i], addrs[j] = addrs[j], addrs[i]
			}
		}
	}

	fmt.Printf("\nPattern addresses and spacing:\n")
	for i, addr := range addrs {
		fmt.Printf("$%04X", addr)
		if i > 0 {
			diff := addr - addrs[i-1]
			fmt.Printf(" (+ $%04X = %d bytes)", diff, diff)
		}
		fmt.Printf(" - frames %v\n", addrFrames[addr])
	}
}
