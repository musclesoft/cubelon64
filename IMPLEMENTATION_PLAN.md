# Phrase-Based Format Implementation Plan

## Goal
Reduce ROM size from 5401 bytes to ~4229 bytes by separating musical structure from instrumentation.

## Current State
- **ROM size:** 5401 bytes
- **Note encoding:** 2081 bytes (patterns + streams)
- **Problem:** Bundles pitch+duration+effects in TrackerPattern, missing massive repetition

## Calculated Savings
- **Phrase separation:** 994 bytes (47.8% reduction in note encoding)
- **Bass RLE:** 178 bytes (Channel 1 two-note alternation)
- **Total projected:** 4229 bytes
- **Gap to 3KB:** 1157 bytes (achievable with further phrase/stream optimization)

## Implementation Steps

### 1. Define New Structures
```go
// Phrase: reusable (pitch, duration) sequence
type Phrase struct {
    Notes []PhraseNote
}

type PhraseNote struct {
    FreqIndex byte // Pitch
    Duration  byte // Rhythm
}

// PhraseInstance: how to render a phrase
type PhraseInstance struct {
    PhraseID             byte
    InstrumentIndex      byte
    FreqSeqIndex         byte // vibrato/portamento effect
    SustainPatternIndex  byte
    WaveformPatternIndex byte
}
```

### 2. Build Phrase Table (extract_tracker_format.go)
```go
func buildPhraseTable(notes []NoteWithMetadata) ([]Phrase, map[string]int) {
    // Find repeating (pitch, duration) sequences length 4-8
    // Prioritize by savings (count × length - overhead)
    // Build phrase dictionary
    // Return phrases and lookup map
}
```

### 3. Update Stream Encoding
Instead of:
```go
streams[ch] = []byte{patternID, patternID, patternID, ...}
```

Use:
```go
streams[ch] = []byte{
    phraseID | effectsIndex,  // Combined or separate bytes
    ...
}
```

### 4. Update Simulator (simulate_tracker.go)
```go
func expandPhrase(phrase Phrase, effects PhraseInstance, format *TrackerFormat) []SimulatedNote {
    // Expand phrase notes
    // Apply instrument (ADSR)
    // Apply frequency modulation
    // Apply sustain pattern
    // Apply waveform changes
    // Return simulated notes
}
```

### 5. Size Calculation Updates
```go
// Old
notePatterns: 162 × 6 bytes = 973 bytes
streams: 1576 bytes
total: 2549 bytes

// New  
phrases: ~50 unique × 12 bytes avg = 600 bytes
phraseInstances: ~110 unique × 5 bytes = 550 bytes
streams: ~900 bytes (phrase refs + effect refs)
total: 2050 bytes

// Savings: 2549 - 2050 = 499 bytes (conservative)
// With optimizations: 994 bytes (calculated)
```

### 6. Channel 1 Bass Optimization
Special encoding for two-note alternation:
```go
type BassPattern struct {
    Note1Freq byte
    Note2Freq byte
    Pattern   []byte // RLE: [count1, count2, count1, count2, ...]
}
// Current: 228 bytes
// Optimal: ~50 bytes  
// Saves: 178 bytes
```

## Verification
1. Run simulator on new format
2. Compare output with current format (should be identical)
3. Measure actual ROM size
4. Validate < 3KB target achievable with stream optimizations

## Next Steps After Implementation
1. **Phrase encoding:** Use delta encoding for pitch sequences
2. **Effect compression:** Deduplicate effect combinations
3. **Stream optimization:** Better bytecode for phrase references
4. **Final target:** 3072 bytes

## Files to Modify
- `tools/extract_tracker_format.go` (~300 lines)
- `tools/simulate_tracker.go` (~150 lines)
- `tools/optimize_volume.go` (~50 lines for validation)

## Estimated Impact
- Implementation: 2-3 hours
- Testing: 1 hour
- Total ROM reduction: 1172 bytes (21.7%)
- **Final size: 4229 bytes (4.1 KB)**
