// Debug specific pattern
package main

import (
	"fmt"
)

func DebugPattern(notes []NoteWithMetadata, targetNote int) {
	if targetNote >= len(notes) || targetNote < 0 {
		fmt.Printf("Note %d out of range (have %d notes)\n", targetNote, len(notes))
		return
	}

	note := notes[targetNote]
	fmt.Printf("\n=== Debug Note %d (channel %d) ===\n", targetNote, note.Channel)
	fmt.Printf("Length: %d frames\n", len(note.freqSeq))
	fmt.Printf("Frequencies: ")
	for i := 0; i < len(note.freqSeq); i++ {
		fmt.Printf("%d ", note.freqSeq[i])
	}
	fmt.Printf("\n")

	// Check if all same
	allSame := true
	if len(note.freqSeq) > 0 {
		first := note.freqSeq[0]
		for i := 1; i < len(note.freqSeq); i++ {
			if note.freqSeq[i] != first {
				allSame = false
				fmt.Printf("→ Differs at frame %d: %d → %d\n", i, first, note.freqSeq[i])
				break
			}
		}
	}

	if allSame {
		fmt.Printf("→ All frames have same frequency: %d\n", note.freqSeq[0])
	}
}
