# Emulator Analysis Principles

## Core Rule: Never Modify Execution

**CRITICAL**: The emulator must run cycle-accurately without modification. Debug code that changes execution flow breaks timing and invalidates results.

### What Went Wrong

A debug block was inserted at frame 800 that ran 200,000 extra CPU steps:
```go
// WRONG - This breaks cycle-accurate timing
for writesFound < maxWrites && stepsTraced < maxSteps {
    cpu.Step()  // Extra cycles not in original program!
    stepsTraced++
}
```

**Result**: Audio glitch at 16 seconds because extra cycles weren't accounted for in audio generation.

### The Right Way to Analyze

All analysis must happen by **inspecting** the virtual machine, not **modifying** it:

#### ✅ Correct Approaches

1. **Memory Inspection**
   ```go
   // Read pattern table at specific frame
   if demoFrame == 800 {
       patternTable := uint16(0x84c0)
       for i := 0; i < 8; i++ {
           addr := mem.Read(patternTable + uint16(i*2))
           // Inspect memory state WITHOUT running extra cycles
       }
   }
   ```

2. **Disassembly Analysis**
   - Generate disassembly with `--trace-all-code`
   - Read `full_disassembly.asm` offline
   - Never modify execution to generate traces

3. **External Tracing**
   - Use flags like `--trace-code` to log execution
   - Trace writes to files
   - PC address logging (doesn't affect execution)

4. **TAP File Analysis**
   - Extract data from TAP file directly
   - No emulator involvement at all

#### ❌ Never Do This

- **Running extra CPU steps** during a frame
- **Inserting loops** that call `cpu.Step()`
- **Modifying registers** or memory for testing
- **Changing banking** to inspect other banks
- **Pausing execution** mid-frame (breaks audio)

### Why This Matters

The emulator is **cycle-accurate**:
- Audio generated based on cycles per frame (69,888 cycles)
- Video synchronized to CPU cycles
- Any extra/missing cycles breaks timing
- "Wall time doesn't matter" - only cycles matter

### The Golden Rule

> **Observe, don't interfere.**

The virtual machine must run exactly as the original would. All debugging happens by observing its state, not by changing its execution.

### Correct Analysis Workflow

1. **Run emulator normally** - let it execute unmodified
2. **Log state** - PC addresses, memory snapshots at frame boundaries
3. **Analyze offline** - study logs, disassembly, memory dumps
4. **Repeat if needed** - add more logging, not more execution

### Memory Inspection Template

```go
// At frame boundary (after frame completes)
if demoFrame == targetFrame {
    // ✅ Read memory
    data := mem.Read(address)

    // ✅ Print state
    fmt.Printf("PC: %04X\n", cpu.PC)

    // ✅ Dump region
    for i := 0; i < size; i++ {
        fmt.Printf("%02X ", mem.Read(baseAddr + uint16(i)))
    }

    // ❌ DON'T run extra steps
    // for i < limit {
    //     cpu.Step()  // NO!
    // }
}
```

### Trace Flags (Safe)

These flags enable logging **without changing execution**:

- `--trace-all-code=frames` - Log all PC addresses
- `--trace-code=frame:pcrange` - Log specific routine
- Memory read tracking (passive)
- Register snapshots at frame boundaries

All of these observe execution without modifying it.

---

**Date**: 2025-03-07
**Lesson**: Debug code at frame 800 caused audio glitch by running extra cycles
**Solution**: Removed debug code, documented proper analysis methods
**Principle**: Observe, don't interfere with cycle-accurate execution
