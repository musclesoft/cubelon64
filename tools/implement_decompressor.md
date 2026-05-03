# Implementing the ZX LZ Decompressor

## Goal
Extract compressed music data from $F000+ range and decompress it to match what the Z80 decompressor produces.

## Current Blocker
- We have memory dumps but they're AFTER channels have already advanced
- Pattern pointers point to compressed source ($0A6E, $CF18, etc - all < $C000)
- Need either:
  1. Full LZ decompressor implementation
  2. Memory dump RIGHT when pattern loads (before any OUTI execution)

## Next Steps

### Option 1: Implement Decompressor (3-5 days)
1. Extract compressed source for each channel from TAP blocks
2. Reverse-engineer complete $820A-$82AE algorithm
3. Implement bit-stream reader and LZ decoder in Go
4. Test against known output

### Option 2: Emulator Hook (1 hour)
1. Add PC breakpoint at $7F4C (end of pattern load)
2. Dump full memory at frame 42 when PC=$7F4C
3. Get fresh decompressed buffer before any OUTI advances pointers
4. Use that buffer in Go player

## Recommendation
Try Option 2 first - it's much faster and gives us the actual decompressed data to work with.

If that doesn't work (pattern load might happen earlier), then implement Option 1.

## Status
Blocked on getting initial decompressed buffer state.
