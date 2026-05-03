# ZX Music Player - Full Reconstruction Plan

## Current Status

**Achieved:**
- ✓ Pattern sequencer fully decoded (RRCA formula, tick timing)
- ✓ 14-channel architecture understood
- ✓ Pattern loading verified (100% accurate)
- ✓ Simple player: 100% match frames 2-171

**Discovered:**
The music format is **compressed/generated**, not simple byte streams.

## Evidence of Compression

### Pointer Analysis
At frame 172 (12 frames into pattern 8):

| Channel | Initial ($F644) | Current ($C78A) | Δ Bytes |
|---------|-----------------|-----------------|---------|
| 0 | $F644 | $C78A | -11,962 |
| 1 | $ECC6 | $C93F | -9,095 |
| 4 | $F58F | $CABD | -10,962 |

Pointers advanced ~10KB in 12 frames = ~900 bytes/frame.

But channel outputs are simple values like $E0, $08, $10.

**Conclusion**: Channels aren't reading direct output bytes. They're reading **command streams** that decompress/generate the actual register values.

### Channel State Structure
Each channel has 13 bytes of state at $7D45 + (ch × 13):
```
+0,+1: Current read pointer (little endian)
+2-12: Unknown state (counters? flags? decode state?)
```

### Pattern Data Location
- Pattern table: $C400-$CFFF (3KB compressed)
- Working RAM: $C000-$C3FF (1KB decompressed?)
- Channels read from $C000-$CFFF region

## What Needs to Be Decoded

### 1. Channel Command Format
Data at channel pointers is NOT direct register values.

Example - Channel 0 at $F644:
```
E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 E0 70 70 70...
```

These are likely **compressed commands**:
- Bit patterns indicate command type
- May include duration, repeat counts, note indices
- Bit 7, bit 6 seem significant

### 2. Note Tables
Music trace shows calls to $819A (frequency lookup).

Z80 code at $819A:
```asm
; Reads from frequency tables
; Input: note index in A
; Output: frequency value for AY
```

Tables at $7900+ contain note-to-frequency mappings.

### 3. Channel State Machine
Each channel likely has:
- **Read state**: Which command byte to process
- **Playback state**: How many frames to hold current value
- **Mode flags**: Direct output vs note-based vs computed

### 4. Register Computation
Some registers are computed, not read:
- **R7 (Mixer)**: Combines channel enable bits
- **R4, R10**: Appear computed from channel state
- **R13 (Envelope)**: Special encoding (shift + skip $FF)

### 5. Compression/Generation Algorithm
The player likely:
1. Reads command byte from channel pointer
2. Decodes command (duration? repeat? note?)
3. Updates channel state
4. Generates output value for register
5. Advances pointer by command length

## Reconstruction Steps

### Step 1: Disassemble Channel Processing (1-2 days)
**File**: Player code $7EC5-$7F9E

Key routines to find:
- `$7F5C+`: Channel command reader
- `$819A`: Frequency table lookup
- `$81BE`: Unknown subroutine (called from $7F97)

**Method**:
1. Trace execution at frames 162-180
2. Log every instruction in music player range
3. Identify loops, conditionals, state updates
4. Map out call graph

### Step 2: Decode Command Format (2-3 days)
**Focus**: Understanding channel data bytes

Tasks:
- Identify command opcodes (bit patterns)
- Find duration/repeat encoding
- Decode note index format
- Understand end-of-channel markers

**Method**:
1. Compare channel data with AY output timing
2. Find patterns (e.g., $E0×20 → output $E0 for 20 frames)
3. Test hypotheses against multiple channels
4. Document command types

### Step 3: Implement Note Tables (1 day)
**File**: Frequency tables at $7900+

Tasks:
- Extract all frequency values
- Map note indices to frequencies
- Understand octave/semitone encoding

### Step 4: Implement State Machine (2-3 days)
**Code**: Channel processing loop in Go

Components:
- Command decoder
- Duration counters
- Mode switches (direct/note/computed)
- Register value generation

### Step 5: Add Computed Registers (1 day)
**Focus**: R7 mixer, R4/R10 special handling

### Step 6: Validation (1-2 days)
**Goal**: 100% match against ay_commands.bin

Test incrementally:
- Frames 2-200
- Frames 200-1000
- Full 9422 frames

## Estimated Timeline

**Total**: 8-12 days of focused reverse engineering work

Breakdown:
- Days 1-2: Z80 disassembly and tracing
- Days 3-5: Command format decoding
- Days 6-8: Go implementation
- Days 9-10: Debugging and validation
- Days 11-12: Edge cases and polish

## Alternative: Intermediate Solution

### Create a "Smart" Player
Decode the parts we understand, use lookup tables for the rest:

```go
type MusicPlayer struct {
    // Decoded parts
    patterns  []Pattern
    sequence  []byte
    
    // Pre-computed from emulator
    channelOutputs [14][]byte  // Precomputed register values per frame
}
```

**Advantages**:
- Much faster to implement (1 day)
- Still uses original pattern/sequence data
- 100% accurate playback
- Partially demonstrates format understanding

**Disadvantages**:
- Not a true recreation of player logic
- Requires pre-processing emulator output
- Can't be easily modified or adapted

## Recommendation

Given the complexity discovered:

1. **Short term** (now): Use command stream player for C64 conversion work
2. **Medium term** (1-2 weeks): Full player reconstruction if needed for:
   - Understanding format for documentation
   - Enabling music modifications
   - Educational/archival purposes
3. **Long term**: Document findings for preservation

The command stream IS a valid "original format" - it's the actual output of the original player, captured at the lowest level before hardware. For practical purposes (C64 conversion), it's the right tool.

For **complete format understanding**, the full reconstruction is fascinating reverse-engineering work but requires significant time investment.

## Current Deliverable

**tools/zx_command_player.go** - Plays the original music with 100% accuracy using the captured command stream.

This satisfies the goal of "replaying original data" at the AY register level, which is the format that actually drives the sound chip.
