# ZX Music Player Implementation Status

## What We Have

### ✅ Complete Extraction
- **Music player code**: 218 bytes ($7EC5-$7F9E) - disassembled and documented
- **Pattern sequence**: 432 bytes - the song structure
- **Pattern table**: 3,072 bytes - note/effect data
- **Complete memory**: 16KB ($C000-$FFFF) including all channel data
- **Original AY log**: 123,027 commands from emulator for comparison

### ✅ Format Understanding
- Pattern-based sequencer with 14 logical channels
- Tick-based timing (frame counter)
- Bank-switched memory access
- Three command types identified: notes, timing, effects
- Pattern indexing formula decoded

### ❌ Go Player Implementation
**Status**: Partially implemented but not yet producing correct output

**Current Issues**:
1. Channel command processing logic incomplete
2. Note-to-frequency conversion not implemented
3. Effect commands (bit 6) not handled
4. Timing commands (bit 7) not properly processed
5. Channel-to-AY-register mapping unclear

## The Challenge

The Z80 player is more complex than initially understood:

1. **Pattern Structure**: Each pattern contains 14 channel pointers that point to separate data streams scattered across memory ($E000-$F000 range)

2. **Channel Data Format**: Still not fully decoded
   - Bytes like $E0, $F5, $EF are note values
   - Need to understand how these map to AY registers
   - Commands can be 1, 2, or 3 bytes depending on type

3. **Multi-Frame Processing**: The player processes multiple commands per frame for each channel, not just one

4. **State Tracking**: Need to track per-channel state (position, active/inactive, current note)

## What's Needed

### Critical Missing Pieces

1. **Exact Command Format**
   - How do note bytes ($E0-$FF) work?
   - What follows a note byte (value, register)?
   - How are the 14 channels mapped to 3 AY tone generators?

2. **Subroutine Logic**
   - `$819A` - AY register write routine (type 1)
   - `$81BE` - AY register write routine (type 2)
   - These contain the actual register write logic

3. **Frequency Tables**
   - The $7900+ region contains frequency data
   - Need to extract and understand the mapping

### Approaches to Complete It

**Option A: Deep Disassembly**
- Disassemble and understand subroutines $819A and $81BE
- Trace execution flow for first few frames
- Build exact state machine

**Option B: Runtime Tracing**
- Instrument emulator to log:
  - Which channel reads which byte
  - What values are written to which AY registers
  - Correlation between pattern data and AY output
- Build player from observed behavior

**Option C: Reference Implementation**
- The existing emulator already plays it correctly
- Could extract just the player logic from emulator
- Port Z80 execution to Go (interpret the player code)

## Current Player Code

`tools/zx_music_player_v2.go` - Skeleton implementation
- ✅ Loads complete memory region
- ✅ Parses pattern sequence
- ✅ Calculates pattern addresses correctly
- ✅ Loads channel pointers
- ❌ Channel command processing incomplete
- ❌ No AY output yet

## Test Results

```
Input:  123,027 commands from emulator
Output: 0 commands from Go player
Match:  0%
```

## Next Steps

1. Add detailed tracing to emulator showing:
   - Memory reads during channel processing
   - AY register writes with source channel
   - Pattern data interpretation

2. Study subroutines $819A and $81BE in detail

3. Extract frequency table from $7900+

4. Implement proper command decoder based on traces

5. Test incrementally (first 10 frames, then 100, then full song)

## Files

- `tools/zx_music_complete_memory.bin` - Full 16KB memory region
- `tools/zx_music_player_v2.go` - Current player implementation  
- `emulator/ay_commands.bin` - Reference output from emulator
- `docs/ZX_MUSIC_FORMAT_EXTRACTED.md` - Format documentation
