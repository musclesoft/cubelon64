package main

import (
	"fmt"
)

// FindChannel2LoopStart tries different loop start positions for channel 2
func FindChannel2LoopStart(allNotes []NoteWithMetadata, notePatterns *[]TrackerPattern) {
	fmt.Printf("\n=== Finding Channel 2 Loop Start ===\n\n")

	// Filter to channel 2
	var ch2Notes []NoteWithMetadata
	for _, note := range allNotes {
		if note.Channel == 2 {
			ch2Notes = append(ch2Notes, note)
		}
	}

	framesPerPattern := 20
	patternsPerLoop := 32

	// Find last frame
	lastFrame := 0
	for _, note := range ch2Notes {
		endFrame := note.StartFrame + int(note.dur)
		if endFrame > lastFrame {
			lastFrame = endFrame
		}
	}

	totalPatterns := lastFrame / framesPerPattern
	fmt.Printf("Channel 2: %d notes, %d total patterns\n", len(ch2Notes), totalPatterns)

	// Build pattern grid
	patternGrid := make([]int, totalPatterns)
	for _, note := range ch2Notes {
		patternPos := note.StartFrame / framesPerPattern
		if patternPos < totalPatterns {
			if patternGrid[patternPos] == 0 {
				patternGrid[patternPos] = note.patternID
			}
		}
	}

	// Try different loop start positions
	fmt.Printf("\nTrying different loop start positions:\n")

	for tryStart := 0; tryStart < 16; tryStart++ {
		if tryStart + patternsPerLoop*2 > totalPatterns {
			break
		}

		numLoops := (totalPatterns - tryStart) / patternsPerLoop
		if numLoops < 2 {
			continue
		}

		// Extract loops
		loops := make([][]int, numLoops)
		for loopIdx := 0; loopIdx < numLoops; loopIdx++ {
			startPat := tryStart + loopIdx*patternsPerLoop
			endPat := startPat + patternsPerLoop
			if endPat > totalPatterns {
				endPat = totalPatterns
			}
			loops[loopIdx] = patternGrid[startPat:endPat]
		}

		// Count matches between consecutive loops
		if len(loops) >= 2 {
			totalPos := 0
			matchPos := 0

			for pos := 0; pos < patternsPerLoop; pos++ {
				allMatch := true
				firstPat := loops[0][pos]

				for loopIdx := 1; loopIdx < min(numLoops, 5); loopIdx++ {
					if pos >= len(loops[loopIdx]) {
						allMatch = false
						break
					}
					if loops[loopIdx][pos] != firstPat {
						allMatch = false
						break
					}
				}

				totalPos++
				if allMatch {
					matchPos++
				}
			}

			matchPct := 100.0 * float64(matchPos) / float64(totalPos)
			fmt.Printf("  Start=%2d: %2d/%2d positions match across first 5 loops (%.1f%%)\n",
				tryStart, matchPos, totalPos, matchPct)
		}
	}

	// Now analyze with the best start position
	// Let's also check if ch0 and ch2 have the same loop structure
	fmt.Printf("\n=== Comparing Ch0 and Ch2 loop timing ===\n")

	var ch0Notes []NoteWithMetadata
	for _, note := range allNotes {
		if note.Channel == 0 {
			ch0Notes = append(ch0Notes, note)
		}
	}

	if len(ch0Notes) > 0 && len(ch2Notes) > 0 {
		ch0FirstNote := ch0Notes[0].StartFrame
		ch2FirstNote := ch2Notes[0].StartFrame

		fmt.Printf("Ch0 first note: frame %d\n", ch0FirstNote)
		fmt.Printf("Ch2 first note: frame %d\n", ch2FirstNote)
		fmt.Printf("Offset: %d frames\n", ch2FirstNote - ch0FirstNote)

		// Check if they align to the same loop boundaries
		loopStartFrame := 8 * framesPerPattern
		ch0LoopOffset := ch0FirstNote - loopStartFrame
		ch2LoopOffset := ch2FirstNote - loopStartFrame

		fmt.Printf("\nRelative to loop start (pattern 8 = frame %d):\n", loopStartFrame)
		fmt.Printf("Ch0 offset: %d frames\n", ch0LoopOffset)
		fmt.Printf("Ch2 offset: %d frames\n", ch2LoopOffset)
	}

	// Show actual pattern density for ch2
	fmt.Printf("\n=== Channel 2 Pattern Density ===\n")
	nonZeroPatterns := 0
	for _, pat := range patternGrid {
		if pat != 0 {
			nonZeroPatterns++
		}
	}
	fmt.Printf("Patterns with notes: %d/%d (%.1f%%)\n",
		nonZeroPatterns, len(patternGrid), 100.0*float64(nonZeroPatterns)/float64(len(patternGrid)))

	// Show first few loops visually
	fmt.Printf("\n=== First 5 loops (pattern view) ===\n")
	loopStart := 8
	for loopIdx := 0; loopIdx < 5; loopIdx++ {
		fmt.Printf("Loop %d: ", loopIdx)
		for pos := 0; pos < 32; pos++ {
			patPos := loopStart + loopIdx*32 + pos
			if patPos >= len(patternGrid) {
				break
			}
			pat := patternGrid[patPos]
			if pat == 0 {
				fmt.Printf("-- ")
			} else {
				fmt.Printf("%2d ", pat)
			}
		}
		fmt.Printf("\n")
	}

	// Compare loops 0 and 2 (skipping sparse loop 1)
	fmt.Printf("\n=== Comparing Loop 0 vs Loop 2 (ignoring zeros) ===\n")
	matches := 0
	nonZero := 0
	for pos := 0; pos < 32; pos++ {
		pat0 := patternGrid[loopStart + 0*32 + pos]
		pat2 := patternGrid[loopStart + 2*32 + pos]

		if pat0 != 0 || pat2 != 0 {
			nonZero++
			if pat0 == pat2 {
				matches++
			} else {
				if pos < 10 {
					fmt.Printf("  Pos %2d: Loop0=%2d, Loop2=%2d\n", pos, pat0, pat2)
				}
			}
		}
	}
	fmt.Printf("Match rate (non-zero positions): %d/%d (%.1f%%)\n", matches, nonZero, 100.0*float64(matches)/float64(nonZero))
}


