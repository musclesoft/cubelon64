# Pattern Generation Analysis

## Key Findings

### Data Structure
- **873 tiles extracted** from blocks 03, 05, 07, 08
  - These are 8×8 pixel building blocks
  - NOT the final displayed patterns
  - Used as seed data for procedural generation

### Block 01 Analysis
- Contains a **complete 256×192 ZX Spectrum screen**
- 8×8 grid of 3×3 character patterns (24×24 pixels each)
- **All 64 patterns are unique** - no repetition
- Likely a title screen or single demo frame
- Patterns do NOT match extracted tiles directly

### Z80 Code Analysis
**Block 10** (4595 bytes - main demo code):
- 17 CALL instructions
- 12 JP instructions
- **127 XOR operations** - key for pattern combination
- **101 rotate/shift operations** - pattern transformation
- **26 DJNZ loops** - iterative processing
- 191 ADD operations - accumulation

### Pattern Generation Algorithm (Hypothesis)
Based on code analysis:

1. **Tile Combination**: Multiple 8×8 tiles XORed/ORed together
2. **Transformation**: Rotates and shifts create variations
3. **Animation**: Loops cycle through transformations
4. **Procedural**: Patterns generated in real-time, not pre-stored

```
Pseudo-algorithm:
for each pattern position:
    load base tile(s) from seed data
    apply XOR/OR combinations
    apply rotations/shifts
    output to screen
    repeat with variations for animation
```

## Technical Challenges

### Decompression Problem
- Blocks 11, 12 appear to be compressed/packed
- Loader code at $7800 includes decompression routine
- Can't easily extract final code without running the decompressor

### Algorithm Complexity
- Pattern generation is deeply embedded in Z80 code
- Requires full disassembly and understanding
- Complex interdependencies between code blocks

## Practical Approaches

### Option A: Full Reverse Engineering (Authentic)
**Pros:**
- Most faithful to original
- Understanding the algorithm is valuable
- True demo coding experience

**Cons:**
- Time intensive (10-20+ hours)
- Requires expert Z80 assembly knowledge
- May hit roadblocks with compression

**Steps:**
1. Properly disassemble all code blocks
2. Trace execution flow
3. Document algorithm completely
4. Port to 6502

### Option B: Frame Capture Method (Pragmatic)
**Pros:**
- Faster to implement
- Guaranteed visual accuracy
- Can iterate quickly

**Cons:**
- Less authentic (data-driven vs algorithmic)
- Larger memory footprint
- Not "true" port

**Steps:**
1. Run ZX demo in emulator
2. Capture screen at key frames
3. Extract patterns/tiles
4. Store as animation data
5. Implement playback on C64

### Option C: Hybrid Approach (Recommended)
**Pros:**
- Get something working quickly
- Can refine later
- Learn incrementally

**Cons:**
- Initial version not authentic
- Need to rework later

**Steps:**
1. Create simple test pattern generator (hard-coded)
2. Display on C64 with border
3. Add basic animation (rotation/XOR)
4. Validate display system works
5. Then either:
   - Continue with reverse engineering
   - Or capture real patterns

## Current Status

We have:
- ✅ 873 extracted tiles (seed data)
- ✅ C64 framework working
- ✅ Build system functional
- ✅ Display routines ready
- ⏳ Pattern algorithm (partially understood)

Next decision: Which approach to take forward?

## Recommendation

**Start with Option C (Hybrid)**:
1. Create a simple hard-coded pattern test
2. Implement basic XOR-based animation
3. Get visual feedback on C64
4. Prove the display system works
5. Then decide: continue reverse engineering or capture frames

This validates our work so far and provides a foundation for either direction.
