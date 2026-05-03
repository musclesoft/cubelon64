# ZX Music Player - Complete Reverse Engineering Guide

## Architecture Discovered

### Three-Stage Music System

```
Stage 1: COMPRESSED SOURCE DATA
  Location: $C400-$FFFF (pattern table + channel data)
  Format: Unknown compression/encoding
  Example: $F644 = [E0 E0 E0 E0 E0 ... 70 70 70 ...]

Stage 2: THREADED CODE GENERATION  
  Location: $C000-$C3FF (working buffer)
  Format: [cmd_byte][addr_lo][addr_hi] triplets
  Example: $C78A = [EB 8F F5] [8F F5 B2] [ED 72 F4] ...

Stage 3: THREAD INTERPRETER
  Code: $7F5C-$7F9E
  Executes generated threaded code
  Calls subroutines to output AY registers
```

### Execution Flow

```
IRQ (50Hz)
  ↓
$7E0E: Main IRQ handler
  ↓
$980E: Pre-music setup (CALL $857A, CALL $981D)
  ↓  
$7EC5: Music player entry
  ↓
  DEC tick counter
  IF ticks == 0:
    Load next pattern
    Calculate address (RRCA×3)
    Load 14 channel pointers via stack
  ↓
$7F5C: Thread interpreter (for each channel)
  Read command byte
  Check bit patterns
  Call appropriate subroutine
  ↓
Subroutines write to AY registers
  ↓
Return from IRQ
```

## Code Locations

### Pattern System ($7EC5-$7F4C)
- **$7EC5**: Entry point
- **$7EC8**: DEC (HL) - tick counter
- **$7ECA-$7ED5**: Bank switching
- **$7ED7**: Sequence pointer ($C00A)
- **$7EF6-$7F06**: RRCA×3 address calculation
- **$7F07**: Read tick count
- **$7F0C-$7F46**: Load 14 pointers using stack trick
- **$7F4C**: Return

### Thread Interpreter ($7F5C-$7F9E)
- **$7F5C**: Load channel pointer from ($7902)
- **$7F5F**: Main command loop
- **$7F61**: BIT 5,A - check END marker
- **$7F65**: BIT 7,A - check command type
- **$7F78**: BIT 6,A - check subroutine select
- **$7F7E**: CALL $819A - frequency table lookup
- **$7F87**: CALL subroutine (indirect via loaded address)
- **$7F8E**: JP (HL) - execute subroutine
- **$7F97**: CALL $81BE - alternate subroutine

### Channel State
Location: $7D45 + (channel × 13 bytes)

```
Offset  Purpose
+0,+1   Current read pointer (little endian)
+2-12   Unknown (flags? counters? decode state?)
```

14 channels:
- Ch 0: $7D45
- Ch 1: $7D52  
- Ch 2: $7D5F
- ... (13 bytes apart)
- Ch 13: $7DED

### Pattern Table
- Sequence: $C000-$C1AF (432 bytes)
- Patterns: $C400-$CFFF (3KB)
- Pattern format:
  ```
  [tick_count:byte]
  [ch0_ptr_lo:byte][ch0_ptr_hi:byte]
  [ch1_ptr_lo:byte][ch1_ptr_hi:byte]
  ... ×14 channels
  ```

## Command Format (Threaded Code)

### Bit Pattern
```
Bit 7: Command type flag
Bit 6: Subroutine selector
Bit 5: END marker (stop channel)
Bits 2-0: Parameter (note index, register, etc.)
```

### Command Structure
```
[command_byte] [subroutine_addr_lo] [subroutine_addr_hi]
```

Each command is 3 bytes and calls a subroutine that performs the actual work.

### Observed Commands (at $C78A)
```
$EB $8F $F5  - bit7=1 bit6=1 bit5=1 param=3
$8F $F5 $B2  - Subroutine at $B2F5
$ED $72 $F4  - bit7=1 bit6=1 bit5=1 param=5
...
```

## Decompression Mystery

**Key Finding**: Channel pointers change between pattern load and execution.

Pattern 8 loads channel 0 pointer as **$F644** (source data)
But at frame 160, pointer is **$CABB** (generated code)

**Where does decompression happen?**

Candidates:
1. **$980E routine** - called before $7EC5
2. **$857A routine** - processes channels  
3. **Lazy generation** - on first channel access
4. **Pattern load continuation** - after pointer loading

**Evidence**:
- No CALL between $7F0C-$7F4C (pointer loading)
- Routine at $980E calls $857A and $981D
- Source data at $F644 is simple bytes (E0 E0...)
- Generated code at $C78A is threaded commands

## Missing Pieces

### 1. Decompression Algorithm
**Input**: Source bytes at $F644 (E0 E0 E0 ... 70 70 70 ...)
**Output**: Threaded code at $C78A (EB 8F F5 8F F5 B2 ...)

Need to find and disassemble the routine that performs this transformation.

**Method**:
- Trace execution from $980E through $857A
- Monitor memory writes to $C000-$C3FF range
- Document transformation rules

### 2. Threaded Command Set
Need complete documentation of all command types:

```
Commands with bit 6=0:
  - AND $07 to get note index
  - CALL $819A (frequency lookup)
  - Read 2-byte subroutine address
  - Execute subroutine

Commands with bit 6=1:
  - AND $07 to get parameter
  - Read 2-byte subroutine address  
  - CALL $81BE
  - Execute subroutine

Commands with bit 5=1:
  - END marker
  - Stop channel processing
```

### 3. Subroutine Catalog
Document all subroutines and their functions:

- **$819A**: Frequency table lookup
- **$81BE**: Unknown (register write?)
- **$7F8E**: Indirect jump dispatcher
- Plus many others at addresses loaded from threaded code

### 4. Register Output Logic
How do subroutines actually write to AY registers?

Expected pattern:
```
LD BC,$FFFD
LD A,register_number
OUT (C),A
LD BC,$BFFD
LD A,register_value
OUT (C),A
```

Need to find where this happens in subroutines.

### 5. Channel State Machine
What do the 13 bytes of channel state contain?

```
+0,+1: Pointer ✓ (confirmed)
+2:    ?
+3:    ?
+4:    ?
...
+12:   ?
```

Possibilities:
- Duration counters
- Note hold times
- Volume envelopes
- Effect state
- Decode flags

## Tools Needed

### 1. Full Execution Tracer
```bash
# Trace ALL instructions during frame 162
emulator --trace-all --frame=162 --output=trace162.txt
```

Log every:
- PC value
- Instruction executed
- Register states
- Memory writes to $C000-$C3FF

### 2. Memory Diff Tool
```python
# Compare memory before/after decompression
mem_before_pattern_load = dump(frame=159)
mem_after_pattern_load = dump(frame=160)
diff = compare(mem_before, mem_after, range=0xC000, 0xC3FF)
```

### 3. Disassembler with Call Graph
```python
# Build complete call graph from $980E
analyze_calls(start=0x980E, depth=5)
```

## Reconstruction Steps

### Step 1: Trace Decompression (3-4 days)
1. Add full instruction tracing to emulator
2. Capture frame 160 execution
3. Filter for writes to $C000-$C3FF
4. Identify decompression routine
5. Document algorithm

### Step 2: Decode Threaded Format (2-3 days)  
1. Analyze generated code at multiple frames
2. Catalog all command types
3. Document bit field meanings
4. Map commands to subroutines

### Step 3: Disassemble Subroutines (3-4 days)
1. Find all referenced subroutine addresses
2. Disassemble each subroutine
3. Identify register write sequences
4. Document parameters and effects

### Step 4: Implement in Go (4-5 days)
1. Decompressor: source → threaded code
2. Thread interpreter: execute commands
3. Subroutine handlers: generate register writes
4. Channel state management

### Step 5: Debug and Validate (2-3 days)
1. Test against known frames
2. Compare with ay_commands.bin
3. Fix discrepancies
4. Achieve 100% match

**Total Estimated Time: 14-19 days**

## Current Workaround

The command stream player (zx_command_player.go) provides 100% accurate playback using the captured AY register writes. This is a valid solution for practical purposes (C64 conversion) but doesn't reconstruct the original player logic.

## Recommendation

**For Complete Reconstruction**: Follow the 5-step plan above with proper tracing tools.

**For Practical Use**: The command stream is the right tool - it's the actual output of the original player, captured at the lowest level before the sound chip.

The choice depends on goals:
- **Academic/Archival**: Full reconstruction worth the investment
- **Practical/C64 Conversion**: Command stream is sufficient and guaranteed accurate
