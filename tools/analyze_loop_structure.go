package main

import (
	"fmt"
)

// AnalyzeLoopStructure analyzes loop-based structure at pattern granularity for all channels.
// It identifies the master loop with variants and calculates potential compression savings.
func AnalyzeLoopStructure(allNotes []NoteWithMetadata, notePatterns *[]TrackerPattern) {
	fmt.Printf("\n=== Loop Structure Analysis (All Channels) ===\n\n")

	// Analyze each channel
	for targetChannel := 0; targetChannel < 3; targetChannel++ {
		analyzeChannelLoop(allNotes, notePatterns, targetChannel)
	}
}

func analyzeChannelLoop(allNotes []NoteWithMetadata, notePatterns *[]TrackerPattern, targetChannel int) {
	fmt.Printf("\n--- Channel %d ---\n", targetChannel)

	// Filter to target channel
	var chNotes []NoteWithMetadata
	for _, note := range allNotes {
		if note.Channel == targetChannel {
			chNotes = append(chNotes, note)
		}
	}

	// Tracker timing
	framesPerPattern := 20
	patternsPerLoop := 32
	loopStartPattern := 8

	// Find last frame
	lastFrame := 0
	for _, note := range chNotes {
		endFrame := note.StartFrame + int(note.dur)
		if endFrame > lastFrame {
			lastFrame = endFrame
		}
	}

	totalPatterns := lastFrame / framesPerPattern
	numLoops := (totalPatterns - loopStartPattern) / patternsPerLoop

	fmt.Printf("Notes: %d total\n", len(chNotes))
	fmt.Printf("Loop structure: %d patterns × %d iterations\n", patternsPerLoop, numLoops)

	// Build pattern grid: what pattern plays at each absolute pattern position
	patternGrid := make([]int, totalPatterns)

	// We need to map notes to pattern positions
	// The current code has notes with StartFrame, we convert to pattern index
	// But notes can span multiple patterns, so we assign each note to the pattern where it starts

	// Build a map of pattern positions to pattern IDs
	for _, note := range chNotes {
		patternPos := note.StartFrame / framesPerPattern
		if patternPos < totalPatterns {
			// For simplicity, just use the first note's pattern ID at each position
			// (In reality, a pattern position might have multiple notes, but we'll handle that)
			if patternGrid[patternPos] == 0 {
				patternGrid[patternPos] = note.patternID
			}
		}
	}

	// Extract loops as sequences of pattern IDs
	loops := make([][]int, numLoops)
	for loopIdx := 0; loopIdx < numLoops; loopIdx++ {
		startPat := loopStartPattern + loopIdx*patternsPerLoop
		endPat := startPat + patternsPerLoop
		if endPat > totalPatterns {
			endPat = totalPatterns
		}
		loops[loopIdx] = patternGrid[startPat:endPat]
	}

	fmt.Printf("\nLoop pattern sequences extracted\n")

	// For each of the 32 pattern positions in the loop, collect all possible pattern IDs
	// IMPORTANT: Treat pattern ID 0 as "mute" not as a variant
	type PatternSlot struct {
		Position    int               // 0-31
		PatternIDs  map[int]uint16    // pattern_id -> which loops have it (bitmask)
		MuteMask    uint16            // Which loops have this position muted (pattern 0)
	}

	masterLoop := make([]PatternSlot, patternsPerLoop)
	for pos := 0; pos < patternsPerLoop; pos++ {
		masterLoop[pos] = PatternSlot{
			Position:   pos,
			PatternIDs: make(map[int]uint16),
			MuteMask:   0,
		}
	}

	// Populate master loop with all pattern variants at each position
	// Skip pattern ID 0 (silence/rest) - track it separately as mute
	for loopIdx := 0; loopIdx < numLoops && loopIdx < 16; loopIdx++ {
		if loopIdx >= len(loops) {
			break
		}
		loop := loops[loopIdx]
		for pos := 0; pos < patternsPerLoop && pos < len(loop); pos++ {
			patternID := loop[pos]
			if patternID == 0 {
				// This position is muted in this loop
				masterLoop[pos].MuteMask |= (1 << loopIdx)
			} else {
				// This position has a pattern in this loop
				masterLoop[pos].PatternIDs[patternID] |= (1 << loopIdx)
			}
		}
	}

	// Analyze master loop
	totalSlots := 0
	maxVariantsAtPosition := 0
	for pos := 0; pos < patternsPerLoop; pos++ {
		variants := len(masterLoop[pos].PatternIDs)
		totalSlots += variants
		if variants > maxVariantsAtPosition {
			maxVariantsAtPosition = variants
		}
	}

	fmt.Printf("Master loop analysis:\n")
	fmt.Printf("  Positions: %d\n", patternsPerLoop)
	fmt.Printf("  Total pattern slots: %d\n", totalSlots)
	fmt.Printf("  Avg variants per position: %.1f\n", float64(totalSlots)/float64(patternsPerLoop))
	fmt.Printf("  Max variants at one position: %d\n", maxVariantsAtPosition)

	// Show some examples
	fmt.Printf("\nExample positions (first 5):\n")
	for pos := 0; pos < min(5, patternsPerLoop); pos++ {
		slot := masterLoop[pos]
		fmt.Printf("Position %2d: %d variants\n", pos, len(slot.PatternIDs))
		variantIdx := 0
		for patID, mask := range slot.PatternIDs {
			if variantIdx >= 3 {
				fmt.Printf("    ... (%d more)\n", len(slot.PatternIDs)-3)
				break
			}
			loopList := ""
			for l := 0; l < numLoops; l++ {
				if mask&(1<<l) != 0 {
					if loopList != "" {
						loopList += ","
					}
					loopList += fmt.Sprintf("%d", l)
				}
			}
			fmt.Printf("    Pattern %3d in loops [%s]\n", patID, loopList)
			variantIdx++
		}
	}

	// Group pattern slots by identical loop masks (layers)
	type Layer struct {
		Mask  uint16
		Slots []struct {
			Position  int
			PatternID int
		}
	}

	maskToLayer := make(map[uint16]*Layer)

	for pos := 0; pos < patternsPerLoop; pos++ {
		for patID, mask := range masterLoop[pos].PatternIDs {
			if layer, exists := maskToLayer[mask]; exists {
				layer.Slots = append(layer.Slots, struct {
					Position  int
					PatternID int
				}{pos, patID})
			} else {
				maskToLayer[mask] = &Layer{
					Mask: mask,
					Slots: []struct {
						Position  int
						PatternID int
					}{{pos, patID}},
				}
			}
		}
	}

	var layers []*Layer
	for _, layer := range maskToLayer {
		layers = append(layers, layer)
	}

	// Sort by size
	for i := 0; i < len(layers); i++ {
		for j := i+1; j < len(layers); j++ {
			if len(layers[j].Slots) > len(layers[i].Slots) {
				layers[i], layers[j] = layers[j], layers[i]
			}
		}
	}

	fmt.Printf("\n=== Layer Analysis ===\n")
	fmt.Printf("Found %d unique layers\n", len(layers))
	fmt.Printf("Top 10 layers:\n")
	for i := 0; i < min(10, len(layers)); i++ {
		layer := layers[i]
		loopList := ""
		loopCount := 0
		for l := 0; l < numLoops; l++ {
			if layer.Mask&(1<<l) != 0 {
				if loopList != "" {
					loopList += ","
				}
				loopList += fmt.Sprintf("%d", l)
				loopCount++
			}
		}
		fmt.Printf("  Layer %2d: %3d slots, active in %2d/%2d loops [%s]\n",
			i, len(layer.Slots), loopCount, numLoops, loopList)
	}

	// Calculate encoding size
	fmt.Printf("\n=== Size Calculation ===\n")

	// Encoding: Master + Layer table
	// Master: for each slot: position(1 byte if <32) + pattern_id(1 byte) + layer_id(1 byte)
	// Since position is 0-31, we can encode as: position(5 bits) + layer_id(depends on # layers)

	// Simple encoding: position(1) + pattern_id(1) per slot
	masterBytes := totalSlots * 2

	// Layer table: layer_id(implicit) + loop_mask(2 bytes)
	layerBytes := len(layers) * 2

	// Per-loop overhead: 1 byte per loop for "which layers are active" (not needed, mask has it)
	loopSeqBytes := 0

	totalBytes := masterBytes + layerBytes + loopSeqBytes

	fmt.Printf("Master slots: %d × 2 bytes = %d bytes\n", totalSlots, masterBytes)
	fmt.Printf("Layer table: %d × 2 bytes = %d bytes\n", len(layers), layerBytes)
	fmt.Printf("Total sequence: %d bytes\n", totalBytes)

	// Pattern table - channel only patterns
	chPatternIDs := make(map[int]bool)
	for pos := 0; pos < patternsPerLoop; pos++ {
		for patID := range masterLoop[pos].PatternIDs {
			chPatternIDs[patID] = true
		}
	}
	patternCount := len(chPatternIDs)
	patternBytes := patternCount * 8

	fmt.Printf("\nPattern table: %d unique patterns × 8 bytes = %d bytes\n", patternCount, patternBytes)
	fmt.Printf("\n*** TOTAL: %d bytes ***\n", totalBytes + patternBytes)

	// Get current stream size for this channel
	currentStreamBytes := 0
	if targetChannel == 0 {
		currentStreamBytes = 488
	} else if targetChannel == 1 {
		currentStreamBytes = 268 // from output
	} else if targetChannel == 2 {
		currentStreamBytes = 387 // from output
	}

	fmt.Printf("\nComparison (STREAM ONLY - patterns are shared):\n")
	fmt.Printf("  Current stream: %d bytes\n", currentStreamBytes)
	fmt.Printf("  Loop stream: %d bytes\n", totalBytes)

	if totalBytes < currentStreamBytes {
		savings := currentStreamBytes - totalBytes
		pct := 100.0 * float64(savings) / float64(currentStreamBytes)
		fmt.Printf("  ✓ STREAM SAVINGS: %d bytes (%.1f%% reduction)\n", savings, pct)
	} else {
		overhead := totalBytes - currentStreamBytes
		pct := 100.0 * float64(overhead) / float64(currentStreamBytes)
		fmt.Printf("  ✗ STREAM OVERHEAD: +%d bytes (+%.1f%%)\n", overhead, pct)
	}

	fmt.Printf("\nNote: Pattern table (%d bytes) is SHARED across all channels,\n", patternBytes)
	fmt.Printf("      so savings should be calculated on stream data only.\n")

	// Show pattern usage distribution
	fmt.Printf("\n=== Pattern Usage Distribution ===\n")
	usageCounts := make(map[int]int) // how many loops -> how many pattern slots
	for pos := 0; pos < patternsPerLoop; pos++ {
		for _, mask := range masterLoop[pos].PatternIDs {
			count := 0
			for l := 0; l < numLoops; l++ {
				if mask&(1<<l) != 0 {
					count++
				}
			}
			usageCounts[count]++
		}
	}

	fmt.Printf("Pattern slots appearing in X loops:\n")
	for loops := 1; loops <= numLoops; loops++ {
		if count := usageCounts[loops]; count > 0 {
			fmt.Printf("  %2d loops: %3d pattern slots\n", loops, count)
		}
	}
}
