package main

import (
	"fmt"
)

// LoopBasedStream represents a channel encoded as loop + layers
type LoopBasedStream struct {
	IntroPatterns []byte           // Pattern IDs before loop starts
	MasterLoop    []LoopPosition   // Master loop structure (32 positions)
	Layers        []Layer          // Layer definitions
	LoopSequence  []byte           // Per-iteration: which layers are active (bitmask or list)
}

type LoopPosition struct {
	PatternSlots []PatternSlot // All possible patterns at this position
}

type PatternSlot struct {
	PatternID byte
	LayerID   byte   // Which layer this slot belongs to
}

type Layer struct {
	ID        byte
	LoopsMask uint16 // Which loop iterations have this layer active
}

// BuildLoopBasedStreams generates loop-based encoding for all channels
func BuildLoopBasedStreams(allNotes []NoteWithMetadata, notePatterns *[]TrackerPattern) [3]LoopBasedStream {
	fmt.Printf("\n=== Building Loop-Based Streams ===\n\n")

	var streams [3]LoopBasedStream

	for ch := 0; ch < 3; ch++ {
		streams[ch] = buildChannelLoopStream(allNotes, notePatterns, ch)
	}

	// Calculate total size
	totalBytes := 0
	for ch := 0; ch < 3; ch++ {
		chBytes := calculateLoopStreamSize(streams[ch])
		totalBytes += chBytes
		fmt.Printf("Channel %d loop stream: %d bytes\n", ch, chBytes)
	}
	fmt.Printf("Total loop streams: %d bytes\n", totalBytes)

	return streams
}

func buildChannelLoopStream(allNotes []NoteWithMetadata, notePatterns *[]TrackerPattern, targetChannel int) LoopBasedStream {
	// Filter to target channel
	var chNotes []NoteWithMetadata
	for _, note := range allNotes {
		if note.Channel == targetChannel {
			chNotes = append(chNotes, note)
		}
	}

	// Timing parameters
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

	// Build pattern grid
	patternGrid := make([]int, totalPatterns)
	for _, note := range chNotes {
		patternPos := note.StartFrame / framesPerPattern
		if patternPos < totalPatterns {
			if patternGrid[patternPos] == 0 {
				patternGrid[patternPos] = note.patternID
			}
		}
	}

	// Extract intro
	intro := make([]byte, loopStartPattern)
	for i := 0; i < loopStartPattern && i < len(patternGrid); i++ {
		intro[i] = byte(patternGrid[i])
	}

	// Extract loops
	loops := make([][]int, numLoops)
	for loopIdx := 0; loopIdx < numLoops; loopIdx++ {
		startPat := loopStartPattern + loopIdx*patternsPerLoop
		endPat := startPat + patternsPerLoop
		if endPat > totalPatterns {
			endPat = totalPatterns
		}
		loops[loopIdx] = patternGrid[startPat:endPat]
	}

	// Build master loop with all variants
	type MasterSlot struct {
		Position   int
		PatternIDs map[int]uint16 // pattern_id -> loop mask
		MuteMask   uint16
	}

	masterSlots := make([]MasterSlot, patternsPerLoop)
	for pos := 0; pos < patternsPerLoop; pos++ {
		masterSlots[pos] = MasterSlot{
			Position:   pos,
			PatternIDs: make(map[int]uint16),
			MuteMask:   0,
		}
	}

	// Populate from loops (skip pattern 0 = mute)
	for loopIdx := 0; loopIdx < numLoops && loopIdx < 16; loopIdx++ {
		if loopIdx >= len(loops) {
			break
		}
		loop := loops[loopIdx]
		for pos := 0; pos < patternsPerLoop && pos < len(loop); pos++ {
			patternID := loop[pos]
			if patternID == 0 {
				masterSlots[pos].MuteMask |= (1 << loopIdx)
			} else {
				masterSlots[pos].PatternIDs[patternID] |= (1 << loopIdx)
			}
		}
	}

	// Build layers: group (position, pattern) pairs by identical loop mask
	type SlotKey struct {
		Position  int
		PatternID int
	}

	maskToSlots := make(map[uint16][]SlotKey)

	for pos := 0; pos < patternsPerLoop; pos++ {
		for patID, mask := range masterSlots[pos].PatternIDs {
			key := SlotKey{pos, patID}
			maskToSlots[mask] = append(maskToSlots[mask], key)
		}
	}

	// Create layer list
	var layers []Layer
	layerMap := make(map[uint16]byte) // mask -> layer_id

	for mask := range maskToSlots {
		layerID := byte(len(layers))
		layerMap[mask] = layerID
		layers = append(layers, Layer{
			ID:        layerID,
			LoopsMask: mask,
		})
	}

	// Build master loop with layer assignments
	masterLoop := make([]LoopPosition, patternsPerLoop)
	for pos := 0; pos < patternsPerLoop; pos++ {
		var slots []PatternSlot
		for patID, mask := range masterSlots[pos].PatternIDs {
			layerID := layerMap[mask]
			slots = append(slots, PatternSlot{
				PatternID: byte(patID),
				LayerID:   layerID,
			})
		}
		masterLoop[pos] = LoopPosition{
			PatternSlots: slots,
		}
	}

	// Build loop sequence (which layers active per iteration)
	// For now, store as bitmask in the layer definition itself
	// The player will iterate through layers and check if current loop is in the mask

	return LoopBasedStream{
		IntroPatterns: intro,
		MasterLoop:    masterLoop,
		Layers:        layers,
		LoopSequence:  nil, // Not needed - mask is in layer definition
	}
}

func calculateLoopStreamSize(stream LoopBasedStream) int {
	// Intro: 1 byte per pattern
	introBytes := len(stream.IntroPatterns)

	// Master loop: position(1) + pattern_id(1) + layer_id(1) per slot
	masterBytes := 0
	for _, loopPos := range stream.MasterLoop {
		masterBytes += len(loopPos.PatternSlots) * 3 // pos + pattern + layer
	}

	// Layers: layer_id(implicit) + loop_mask(2 bytes)
	layerBytes := len(stream.Layers) * 2

	// Header overhead: intro_count(1) + master_slot_count(1) + layer_count(1)
	headerBytes := 3

	return headerBytes + introBytes + masterBytes + layerBytes
}

// EncodeLoopBasedStream converts LoopBasedStream to bytecode
func EncodeLoopBasedStream(stream LoopBasedStream) []byte {
	var bytecode []byte

	// Header
	bytecode = append(bytecode, byte(len(stream.IntroPatterns)))

	// Count total master slots
	totalSlots := 0
	for _, pos := range stream.MasterLoop {
		totalSlots += len(pos.PatternSlots)
	}
	bytecode = append(bytecode, byte(totalSlots))
	bytecode = append(bytecode, byte(len(stream.Layers)))

	// Intro patterns
	bytecode = append(bytecode, stream.IntroPatterns...)

	// Master loop: store position + pattern + layer for each slot
	// Simpler encoding: position(1 byte, 0-31) + pattern_id(1) + layer_id(1)
	for pos, loopPos := range stream.MasterLoop {
		for _, slotData := range loopPos.PatternSlots {
			bytecode = append(bytecode, byte(pos)) // position 0-31
			bytecode = append(bytecode, slotData.PatternID)
			bytecode = append(bytecode, slotData.LayerID)
		}
	}

	// Layer table (loop masks)
	for _, layer := range stream.Layers {
		bytecode = append(bytecode, byte(layer.LoopsMask&0xFF))
		bytecode = append(bytecode, byte(layer.LoopsMask>>8))
	}

	return bytecode
}
