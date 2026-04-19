// Optimized phrase-based encoding
package main

import (
	"fmt"
	"os"
	"sort"
)

type SIDCommand struct {
	Frame    uint32
	Register byte
	Value    byte
}

type Note struct {
	freq        uint16
	dur         uint8
	frame       int
	channel     int
	freqSeq     []uint16
	waveformSeq []byte
	patternID   int
	instrumentID int
}

type Instrument struct {
	waveform     byte
	vibratoDepth byte  // Frequency delta in Hz
	vibratoSpeed byte  // Frames per cycle
	vibratoDelay byte  // Frames before vibrato starts
}

type NotePattern struct {
	freqIdx      byte
	duration     byte
	instrumentID byte
}

type Phrase struct {
	notes []byte // Pattern IDs
}

func main() {
	commands, _ := loadSIDCommands("../emulator/sid_commands_optimized.bin")
	notes := extractNotesWithFreqSequences(commands)

	// Detect vibrato and build instruments
	instruments, instrumentMap := buildInstruments(notes)
	for i := range notes {
		waveform := byte(0x04)
		if len(notes[i].waveformSeq) > 0 {
			waveform = notes[i].waveformSeq[0]
		}
		vibratoDepth, vibratoSpeed, vibratoDelay := detectVibrato(notes[i].freqSeq)
		key := fmt.Sprintf("%d_%d_%d_%d", waveform, vibratoDepth, vibratoSpeed, vibratoDelay)
		notes[i].instrumentID = instrumentMap[key]
	}

	// Build frequency table
	freqTable := buildFrequencyTable(notes)
	freqIndex := make(map[uint16]byte)
	for i, f := range freqTable {
		freqIndex[f] = byte(i)
	}

	// Build note patterns with freq index
	notePatterns, notePatternMap := buildNotePatterns(notes, freqIndex)
	
	// Assign pattern IDs
	for i := range notes {
		baseFreq := notes[i].freqSeq[0]
		key := fmt.Sprintf("%d_%d_%d", freqIndex[baseFreq], notes[i].dur, notes[i].instrumentID)
		notes[i].patternID = notePatternMap[key]
	}
	
	// Try different phrase lengths
	bestSize := 99999
	bestLen := 8

	for phraseLen := 6; phraseLen <= 16; phraseLen++ {
		size := estimateSize(notes, phraseLen, len(freqTable), len(instruments), len(notePatterns))
		if size < bestSize {
			bestSize = size
			bestLen = phraseLen
		}
	}

	// Build with optimal length
	phrases, _, channelOrders := buildPhrases(notes, bestLen)

	// Encode
	buf := []byte{}

	// Freq table
	buf = append(buf, byte(len(freqTable)))
	for _, f := range freqTable {
		buf = append(buf, byte(f&0xFF), byte(f>>8))
	}

	// Instruments
	buf = append(buf, byte(len(instruments)))
	for _, inst := range instruments {
		buf = append(buf, inst.waveform, inst.vibratoDepth, inst.vibratoSpeed, inst.vibratoDelay)
	}

	// Note patterns
	buf = append(buf, byte(len(notePatterns)))
	for _, np := range notePatterns {
		buf = append(buf, np.freqIdx, np.duration, np.instrumentID)
	}
	
	// Phrases
	buf = append(buf, byte(len(phrases)))
	for _, phrase := range phrases {
		buf = append(buf, byte(len(phrase.notes)))
		buf = append(buf, phrase.notes...)
	}
	
	// Orders (delta-encoded)
	for ch := 0; ch < 3; ch++ {
		buf = append(buf, byte(len(channelOrders[ch])))
		prev := 0
		for _, phraseID := range channelOrders[ch] {
			delta := phraseID - prev
			buf = append(buf, byte(delta))
			prev = phraseID
		}
	}
	
	ftBytes := 1 + len(freqTable)*2
	instBytes := 1 + len(instruments)*4
	npBytes := 1 + len(notePatterns)*3
	phraseBytes := 1
	for _, p := range phrases {
		phraseBytes += 1 + len(p.notes)
	}
	orderBytes := 3
	for _, order := range channelOrders {
		orderBytes += len(order)
	}

	fmt.Printf("Total: %d bytes\n", len(buf))
	fmt.Printf("  Freq table: %d bytes (%d entries)\n", ftBytes, len(freqTable))
	fmt.Printf("  Instruments: %d bytes (%d entries)\n", instBytes, len(instruments))
	fmt.Printf("  Note patterns: %d bytes (%d entries)\n", npBytes, len(notePatterns))
	fmt.Printf("  Phrases: %d bytes (%d phrases)\n", phraseBytes, len(phrases))
	fmt.Printf("  Orders: %d bytes\n", orderBytes)

	// Show instruments with vibrato
	fmt.Printf("\nInstruments:\n")
	for i, inst := range instruments {
		if inst.vibratoDepth > 0 {
			fmt.Printf("  %d: wave=0x%02X vibrato depth=%d speed=%d delay=%d\n",
				i, inst.waveform, inst.vibratoDepth, inst.vibratoSpeed, inst.vibratoDelay)
		}
	}
	
	os.WriteFile("sid_song.dat", buf, 0644)
}

func estimateSize(notes []Note, phraseLen int, freqTableSize, instrumentCount, notePatternCount int) int {
	phrases, _, channelOrders := buildPhrases(notes, phraseLen)

	size := 1 + freqTableSize*2 // freq table
	size += 1 + instrumentCount*4 // instruments
	size += 1 + notePatternCount*3 // note patterns
	size += 1 // phrase count
	for _, p := range phrases {
		size += 1 + len(p.notes)
	}
	size += 3 // order counts
	for _, order := range channelOrders {
		size += len(order)
	}

	return size
}

func buildPhrases(notes []Note, phraseLen int) ([]Phrase, map[string]int, [][]int) {
	phrases := []Phrase{}
	phraseMap := make(map[string]int)
	channelOrders := make([][]int, 3)
	
	for ch := 0; ch < 3; ch++ {
		channelNotes := []Note{}
		for _, note := range notes {
			if note.channel == ch {
				channelNotes = append(channelNotes, note)
			}
		}
		
		order := []int{}
		
		for i := 0; i < len(channelNotes); i += phraseLen {
			end := i + phraseLen
			if end > len(channelNotes) {
				end = len(channelNotes)
			}
			
			key := ""
			phraseNotes := []byte{}
			for j := i; j < end; j++ {
				phraseNotes = append(phraseNotes, byte(channelNotes[j].patternID))
				key += fmt.Sprintf("%d,", channelNotes[j].patternID)
			}
			
			if _, exists := phraseMap[key]; !exists {
				phraseMap[key] = len(phrases)
				phrases = append(phrases, Phrase{notes: phraseNotes})
			}
			
			order = append(order, phraseMap[key])
		}
		
		channelOrders[ch] = order
	}
	
	return phrases, phraseMap, channelOrders
}

func buildNotePatterns(notes []Note, freqIndex map[uint16]byte) ([]NotePattern, map[string]int) {
	patternMap := make(map[string]int)
	patterns := []NotePattern{}

	for _, note := range notes {
		baseFreq := note.freqSeq[0]

		key := fmt.Sprintf("%d_%d_%d", freqIndex[baseFreq], note.dur, note.instrumentID)
		if _, exists := patternMap[key]; !exists {
			patternMap[key] = len(patterns)
			patterns = append(patterns, NotePattern{
				freqIdx:      freqIndex[baseFreq],
				duration:     note.dur,
				instrumentID: byte(note.instrumentID),
			})
		}
	}

	return patterns, patternMap
}

func buildInstruments(notes []Note) ([]Instrument, map[string]int) {
	instrumentMap := make(map[string]int)
	instruments := []Instrument{}

	for _, note := range notes {
		waveform := byte(0x04)
		if len(note.waveformSeq) > 0 {
			waveform = note.waveformSeq[0]
		}

		vibratoDepth, vibratoSpeed, vibratoDelay := detectVibrato(note.freqSeq)

		key := fmt.Sprintf("%d_%d_%d_%d", waveform, vibratoDepth, vibratoSpeed, vibratoDelay)
		if _, exists := instrumentMap[key]; !exists {
			instrumentMap[key] = len(instruments)
			instruments = append(instruments, Instrument{
				waveform:     waveform,
				vibratoDepth: vibratoDepth,
				vibratoSpeed: vibratoSpeed,
				vibratoDelay: vibratoDelay,
			})
		}
	}

	return instruments, instrumentMap
}

func detectVibrato(freqSeq []uint16) (depth byte, speed byte, delay byte) {
	if len(freqSeq) < 6 {
		return 0, 0, 0 // Need at least one full cycle
	}

	// Find where vibrato starts (frequency changes)
	startIdx := 0
	baseFreq := freqSeq[0]
	for i := 1; i < len(freqSeq); i++ {
		if freqSeq[i] != baseFreq {
			startIdx = i
			break
		}
	}

	if startIdx == 0 || startIdx >= len(freqSeq)-4 {
		return 0, 0, 0 // No vibrato detected
	}

	// Measure vibrato depth (max deviation from base)
	maxDelta := 0
	for i := startIdx; i < len(freqSeq); i++ {
		delta := int(freqSeq[i]) - int(baseFreq)
		if delta < 0 {
			delta = -delta
		}
		if delta > maxDelta {
			maxDelta = delta
		}
	}

	// Measure vibrato speed (frames per cycle)
	// Look for frequency crossing base frequency at least twice
	cycleFrames := 0
	crossings := 0
	lastAbove := freqSeq[startIdx] > baseFreq
	firstCrossingIdx := startIdx
	for i := startIdx + 1; i < len(freqSeq) && crossings < 3; i++ {
		currentAbove := freqSeq[i] > baseFreq
		if currentAbove != lastAbove {
			crossings++
			if crossings == 1 {
				firstCrossingIdx = i
			} else if crossings == 3 {
				// Full cycle detected
				cycleFrames = i - firstCrossingIdx
				break
			}
		}
		lastAbove = currentAbove
	}

	// Only consider it vibrato if we detected at least one full cycle
	if crossings < 3 || cycleFrames < 2 {
		return 0, 0, 0
	}

	if maxDelta > 255 {
		maxDelta = 255
	}
	if cycleFrames > 255 {
		cycleFrames = 255
	}
	if startIdx > 255 {
		startIdx = 255
	}

	return byte(maxDelta), byte(cycleFrames), byte(startIdx)
}

func buildFrequencyTable(notes []Note) []uint16 {
	freqMap := make(map[uint16]bool)
	for _, n := range notes {
		if len(n.freqSeq) > 0 && n.freqSeq[0] > 0 {
			freqMap[n.freqSeq[0]] = true
		}
	}
	
	freqs := []uint16{}
	for f := range freqMap {
		freqs = append(freqs, f)
	}
	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i] < freqs[j]
	})
	
	return freqs
}

func extractNotesWithFreqSequences(commands []SIDCommand) []Note {
	if len(commands) == 0 {
		return nil
	}

	maxFrame := int(commands[len(commands)-1].Frame)
	freqPerFrame := make([][3]uint16, maxFrame+1)
	gatePerFrame := make([][3]bool, maxFrame+1)
	waveformPerFrame := make([][3]byte, maxFrame+1)

	for _, cmd := range commands {
		frame := int(cmd.Frame)
		if frame > maxFrame {
			continue
		}

		voice := -1
		regOffset := -1
		if cmd.Register < 7 {
			voice = 0
			regOffset = int(cmd.Register)
		} else if cmd.Register >= 7 && cmd.Register < 14 {
			voice = 1
			regOffset = int(cmd.Register - 7)
		} else if cmd.Register >= 14 && cmd.Register < 21 {
			voice = 2
			regOffset = int(cmd.Register - 14)
		}

		if voice == -1 {
			continue
		}

		if regOffset == 0 {
			freqPerFrame[frame][voice] = (freqPerFrame[frame][voice] & 0xFF00) | uint16(cmd.Value)
		} else if regOffset == 1 {
			freqPerFrame[frame][voice] = (freqPerFrame[frame][voice] & 0x00FF) | (uint16(cmd.Value) << 8)
		} else if regOffset == 4 {
			gatePerFrame[frame][voice] = (cmd.Value & 0x01) != 0
			waveformPerFrame[frame][voice] = (cmd.Value >> 4) & 0x0F
		}
	}

	gateUpdated := make([][3]bool, maxFrame+1)
	for _, cmd := range commands {
		frame := int(cmd.Frame)
		if frame > maxFrame {
			continue
		}
		voice := -1
		regOffset := -1
		if cmd.Register < 7 {
			voice = 0
			regOffset = int(cmd.Register)
		} else if cmd.Register >= 7 && cmd.Register < 14 {
			voice = 1
			regOffset = int(cmd.Register - 7)
		} else if cmd.Register >= 14 && cmd.Register < 21 {
			voice = 2
			regOffset = int(cmd.Register - 14)
		}
		if voice != -1 && regOffset == 4 {
			gateUpdated[frame][voice] = true
		}
	}

	for i := 1; i <= maxFrame; i++ {
		for ch := 0; ch < 3; ch++ {
			if freqPerFrame[i][ch] == 0 {
				freqPerFrame[i][ch] = freqPerFrame[i-1][ch]
			}
			if !gateUpdated[i][ch] {
				gatePerFrame[i][ch] = gatePerFrame[i-1][ch]
			}
			if waveformPerFrame[i][ch] == 0 {
				waveformPerFrame[i][ch] = waveformPerFrame[i-1][ch]
			}
		}
	}

	notes := []Note{}
	for ch := 0; ch < 3; ch++ {
		noteStart := -1

		for frame := 0; frame <= maxFrame; frame++ {
			prevGate := false
			if frame > 0 {
				prevGate = gatePerFrame[frame-1][ch]
			}
			currGate := gatePerFrame[frame][ch]

			if !prevGate && currGate {
				noteStart = frame
			}

			if prevGate && !currGate {
				if noteStart >= 0 {
					dur := frame - noteStart
					if dur > 255 {
						dur = 255
					}
					if dur > 0 {
						freqSeq := make([]uint16, dur)
						waveformSeq := make([]byte, dur)
						for i := 0; i < dur; i++ {
							if noteStart+i <= maxFrame {
								freqSeq[i] = freqPerFrame[noteStart+i][ch]
								waveformSeq[i] = waveformPerFrame[noteStart+i][ch]
							}
						}

						notes = append(notes, Note{
							freq:        freqPerFrame[noteStart][ch],
							dur:         uint8(dur),
							frame:       noteStart,
							channel:     ch,
							freqSeq:     freqSeq,
							waveformSeq: waveformSeq,
						})
					}
					noteStart = -1
				}
			}
		}
	}

	sort.Slice(notes, func(i, j int) bool {
		if notes[i].frame == notes[j].frame {
			return notes[i].channel < notes[j].channel
		}
		return notes[i].frame < notes[j].frame
	})

	return notes
}

func loadSIDCommands(filename string) ([]SIDCommand, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	count := uint32(data[0]) | (uint32(data[1]) << 8) | (uint32(data[2]) << 16) | (uint32(data[3]) << 24)
	commands := make([]SIDCommand, count)

	pos := 4
	for i := uint32(0); i < count; i++ {
		commands[i].Frame = uint32(data[pos]) | (uint32(data[pos+1]) << 8) | (uint32(data[pos+2]) << 16) | (uint32(data[pos+3]) << 24)
		commands[i].Register = data[pos+4]
		commands[i].Value = data[pos+5]
		pos += 6
	}

	return commands, nil
}
