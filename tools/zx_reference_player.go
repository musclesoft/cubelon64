// Reference player - just replays the AY command log
// This proves we can generate correct output, separate from format decoding
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func main() {
	fmt.Println("=== ZX Music Reference Player ===")
	fmt.Println("Replaying AY commands from emulator log\n")

	// Load original AY commands
	data, err := os.ReadFile("../emulator/ay_commands.bin")
	if err != nil {
		fmt.Printf("Error loading AY log: %v\n", err)
		return
	}

	count := binary.LittleEndian.Uint32(data[0:4])
	fmt.Printf("Commands in log: %d\n", count)

	// Copy to output
	output, err := os.Create("zx_reference_output.bin")
	if err != nil {
		fmt.Printf("Error creating output: %v\n", err)
		return
	}
	defer output.Close()

	// Write count
	binary.Write(output, binary.LittleEndian, count)

	// Copy all commands
	output.Write(data[4:])

	fmt.Printf("✓ Copied %d commands to zx_reference_output.bin\n", count)

	// Now compare
	fmt.Println("\nComparing with original...")

	orig, _ := os.ReadFile("../emulator/ay_commands.bin")
	ref, _ := os.ReadFile("zx_reference_output.bin")

	if len(orig) == len(ref) {
		matches := true
		for i := range orig {
			if orig[i] != ref[i] {
				matches = false
				break
			}
		}

		if matches {
			fmt.Println("✓ Perfect match!")
		} else {
			fmt.Println("✗ Files differ")
		}
	} else {
		fmt.Printf("✗ Size mismatch: %d vs %d\n", len(orig), len(ref))
	}

	fmt.Println("\nThis proves we can generate the exact output.")
	fmt.Println("Now the task is to generate it from the pattern data...")
}
