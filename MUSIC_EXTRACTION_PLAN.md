# Music Player Extraction Plan

## Current Status

We have:
- ✓ Working ZX Spectrum emulator (`emulator/run_cubelon.go`)
- ✓ 123,027 AY commands logged from full playback
- ✓ Analysis showing 589 notes, 43 frequencies, 185 seconds
- ✗ Music player code location
- ✗ Music data location
- ✗ Music data format
- ✗ Actual data size

## What We Need

1. **Music Player Address** - Which routine generates the AY commands
2. **Music Data Location** - Where in memory the note/pattern data is stored  
3. **Data Format** - How the data is structured
4. **Actual Size** - Size of the data in memory (compressed and uncompressed)

## Extraction Method

### Step 1: Find Music Player Routine

Add to `emulator/run_cubelon.go` around line 453:

```go
dumpMemory := flag.Int("dump-memory", 0, "Dump full 64K memory at specified frame")
traceMusic := flag.Bool("trace-music", false, "Trace music player calls (frames 200-300)")
```

Then around line 1000, in the main loop:

```go
// Dump memory at specified frame
if *dumpMemory > 0 && frameCount == *dumpMemory {
    f, _ := os.Create(fmt.Sprintf("memory_frame%d.bin", frameCount))
    for addr := uint16(0); addr < 0x10000; addr++ {
        f.Write([]byte{mem.Read(addr)})
    }
    f.Close()
    fmt.Printf("Dumped memory at frame %d\n", frameCount)
}
```

Run:
```bash
go run emulator/run_cubelon.go emulator/motion_analyzer.go emulator/assertions.go \
  --dump-memory=300 --no-frames --no-audio --stop-at=310
```

### Step 2: Analyze Memory Dump

Once we have `memory_frame300.bin`:

1. Look for patterns at known addresses from disassembly:
   - `$7D26-$7E9D` - Interrupt handler
   - `$980E` area - Called from interrupt
   
2. Search for frequency tables (sequences of 16-bit values 100-2000)

3. Search for repeating byte patterns (note sequences)

### Step 3: Disassemble Player

Use a Z80 disassembler on the identified player address:

```bash
# Assuming player is at $980E
dd if=memory_frame300.bin bs=1 skip=$((0x980E)) count=2048 > player_code.bin
# Then disassemble player_code.bin
```

### Step 4: Extract Data Tables

From the disassembled player code, identify:
- LD HL, $XXXX instructions (loading data pointers)
- LD A, (HL) / LD (HL), A (reading/writing data)
- Loop structures (note playback)

### Step 5: Determine Format

Analyze the player logic to understand:
- How it reads note data
- How it interprets durations
- How it handles effects
- Pattern structure

### Step 6: Extract Raw Data

Once format is known, extract the actual tables from memory dump and calculate exact size.

## Alternative: Use Existing Disassembly

The `emulator/full_disassembly.asm` only captured 900 frames. We need to generate a new one:

```bash
go run emulator/run_cubelon.go emulator/motion_analyzer.go emulator/assertions.go \
  --trace-all-code=200-300 --no-frames --no-audio --stop-at=310
```

This will show all code executed during music playback.

## Expected Results

After extraction:
- Player code: ~500-2000 bytes
- Music data: ~2000-5000 bytes (TBD)
- Format specification: Complete documentation
- Exact sizes: No more estimates

## Why This Approach

1. **Uses working emulator** - No need to fight with tape loading
2. **Captures live memory** - See actual decompressed data
3. **Traces execution** - Find player automatically
4. **Complete analysis** - Can see everything the player does

## Next Action

Modify `emulator/run_cubelon.go` to add memory dump flag, then run extraction.
