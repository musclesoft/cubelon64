package main

import (
	"fmt"
	"strings"
)

const deltaGroupBytes = 8

// The states of a tile table as the C64 unpacks them: per group of 8 bytes one flag byte (bit n set: byte n
// differs from the same byte of the previous state, which is zero before the first), then the differing bytes
func writeDeltaStates(out *strings.Builder, states [][]byte) (total int) {
	previous := make([]byte, len(states[0]))
	for state, bytes := range states {
		fmt.Fprintf(out, "    ; state %d\n", state)
		for start := 0; start < len(bytes); start += deltaGroupBytes {
			flags := byte(0)
			var changed []byte
			for bit := 0; bit < deltaGroupBytes && start+bit < len(bytes); bit++ {
				if bytes[start+bit] != previous[start+bit] {
					flags |= 1 << bit
					changed = append(changed, bytes[start+bit])
				}
			}
			line := fmt.Sprintf("    .byte $%02X", flags)
			for _, value := range changed {
				line += fmt.Sprintf(", $%02X", value)
			}
			out.WriteString(line + "\n")
			total += 1 + len(changed)
		}
		previous = bytes
	}
	return total
}
