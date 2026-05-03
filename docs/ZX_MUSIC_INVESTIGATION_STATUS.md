# ZX Spectrum Music Format - Investigation Status

## Summary

**Achievement: 23.1% exact match (2311/10000 commands)**
**Init Phase: 100% match (frames 2-161)**
**First Music Frames: 100% match (frames 162-171)**

## What We Successfully Decoded

### 1. Pattern Sequencer ✓
- 432-byte pattern sequence at $C000
- RRCA x3 formula for pattern address calculation
- Tick-based timing (counter decrements each frame)
- Pattern structure: [tick_count][14 × 2-byte pointers]

### 2. Channel Architecture ✓
- 14 logical channels mapping to AY registers
- Channels 0-12 → AY registers 0-12
- Channel 13 → AY register 13 (envelope shape)
- Each channel has independent basePtr and offset

### 3. Register Writing Logic ✓
- Registers 0-12: written every frame (13 writes/frame)
- Register 13: written only when value changes
- Init phase (frames 2-161): all $00 except R7=$40

### 4. Special Register Handling ✓
- **R13 (Envelope Shape)**: Channel data shifted right by 1
  - Data $14 → Output $0A
  - Data $18 → Output $0C
  - Data $FF → Skip (no output)

### 5. Timing ✓
- Music player called at $7EC5 once per frame (50Hz)
- Pattern 8 loads at frame 160
- First "music" output at frame 162
- Tick counter at frame 172 = 8 (confirms 12 frames elapsed: 160→172)

## What Remains Unknown

### 1. Channel Data Format ⚠️
**Evidence of Multiple Modes:**

At frame 172, pattern 8:
- **Channel 0**: byte=$E0 → output R0=$E0 ✓ (direct mode)
- **Channel 1**: byte=$08 → output R1=$08 ✓ (direct mode)
- **Channel 4**: byte=$00 → output R4=$8E ✗ (computed/note mode)
- **Channel 7**: byte=$48 → output R7=$68 ✗ (computed, adds $20/bit5)
- **Channel 10**: byte=$00 → output R10=$0D ✗ (computed/note mode)

**Observations:**
- Channels pointing to $F58F appear to be special (null/inactive marker?)
- Music trace shows calls to $819A reading frequency tables at $793B, $793D
- Suggests **note-based playback** for some channels (tone generators?)
- Direct value playback for others (volumes, mixer, envelope?)

### 2. Computed Registers ⚠️

**R7 (Mixer)**:
- Channel 7 data: $58, $48
- Original output: $58, $68
- Pattern: sometimes adds $20 (bit 5), sometimes doesn't
- Likely computed based on active tone/noise channels

**R4, R10 (Tone C Period, Volume C)**:
- Both channels point to $F58F (all zeros)
- Output non-zero values ($8E, $0D)
- Definitely computed, not direct reads

### 3. Note-to-Frequency Mapping ⚠️
- Frequency tables exist at $7900+ region (~256 bytes)
- Routine at $819A performs table lookups
- Format of note data in channels unknown
- Mapping between note indices and frequency values unknown

### 4. Channel Activation Logic ⚠️
Different channels activate at different frames:
- R0, R1, R8, R11, R13: frame 162 (pattern 8 start)
- R4, R7, R10: frame 172 (10 frames into pattern 8)
- R2, R3, R9: frame 182 (pattern 9 start)

**Unclear**: What determines when a channel becomes active?
- Not based on pointer value (all have valid $C000+ addresses)
- Not based on first byte value (no obvious marker)
- Possibly based on data format flags or player state

## Data Extraction Complete ✓

All music data successfully extracted:

### File: tools/zx_music_complete_memory.bin
- **Size**: 16,384 bytes ($C000-$FFFF region)
- **Contents**:
  - Pattern sequence (432 bytes at $0000-$01AF)
  - Pattern table (3,072 bytes at $0400-$0FFF)
  - All channel data streams
  - Complete and ready for C64 conversion

### Music Player Code
- **Address**: $7EC5-$7F9E (218 bytes)
- **Location**: Below $C000 (not in zx_music_complete_memory.bin)
- Available in full memory dumps (memory_frame300.bin)

## Next Steps for 100% Accuracy

To achieve complete reproduction:

1. **Disassemble Music Player** ($7EC5-$7F9E)
   - Trace execution flow at frames 172-180
   - Identify conditionals for direct vs note mode
   - Find channel activation logic

2. **Decode Frequency Tables** ($7900+)
   - Extract note-to-frequency mappings
   - Understand table indexing method

3. **Understand Computed Registers**
   - How R7 (mixer) bits are calculated
   - How volume registers relate to channel state
   - Special handling for channels pointing to $F58F

4. **Channel Data Format**
   - Identify format byte or mode indicator
   - Decode note data structure
   - Understand effect commands (if any)

## Conversion Strategy for C64

Despite incomplete understanding, conversion is possible:

### Option 1: Use Emulator Output
- We have exact AY command stream (123,027 commands)
- Can convert AY commands → SID commands
- Guaranteed 100% accurate timing and values
- File size: ~738 KB (123027 × 6 bytes)

### Option 2: Convert Known Data
- Pattern sequencer + channel architecture proven correct
- Frames 2-171 reproduce perfectly
- Could implement known portions, accept differences for complex channels
- Much smaller data size: ~4 KB

### Option 3: Complete Reverse Engineering
- Finish decoding music player
- Implement full format support
- Would enable understanding and modifying music
- Significant time investment

## Technical Achievements

1. ✓ Identified music player entry point ($7EC5) via CALL tracing
2. ✓ Decoded RRCA×3 pattern address formula
3. ✓ Discovered 14-channel architecture (not standard 3-voice)
4. ✓ Proved R13 shift-right encoding
5. ✓ Confirmed tick-based timing and pattern loading
6. ✓ Achieved 100% accuracy for init and early music frames
7. ✓ Extracted complete music data (3,978 bytes)

## Conclusion

**Status**: Format partially decoded, data fully extracted.

The core sequencer and timing logic are 100% understood and verified. The channel data format has multiple modes (direct values vs note data vs computed values) that require deeper Z80 code analysis to fully decode.

For C64 conversion, using the emulator's AY command output is the most practical approach, guaranteeing perfect reproduction while we continue investigating the underlying format.
