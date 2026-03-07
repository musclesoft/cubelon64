# CUBELON Part 3 - Extraction Complete

## Summary

Successfully extracted all 61 frames (900-960) of Part 3's morphing animation using runtime pattern capture from correct memory banks.

## Key Achievement

**Problem Solved**: Pattern data at addresses $C000-$FFFF exists in pageable memory banks that change during execution. Reading patterns at end-of-frame gave wrong data because banking had changed.

**Solution**: Capture pattern data during CPU execution (at PC=85AB, right before drawing) when correct bank is paged, store in map, then export using captured data on subsequent frame.

## Animation Sequence

### Static Phase (Frames 900-919)
- **Pattern**: $BE06 (Bank 2)
- **Duration**: 20 frames (400ms @ 50Hz)
- **Visual**: Complex 3D diamond/cube structure
- **First 16 bytes**: `FF F8 03 03 F8 FF 3F F0 07 07 F0 0F 03 E0 0F 0F`

### Morphing Phase (Frames 920-939)
Each morph step displays for 2 frames (40ms):

```
Frame 920-921: $C946 - FF F8 01 03 F8 FF 7F F8
Frame 922-923: $C8FE - FF F0 01 01 F8 FF 7F F8
Frame 924-925: $C8B6 - FF F0 01 01 F0 FF FF F0
Frame 926-927: $C86E - FF F0 01 00 E0 FF FF E0
Frame 928-929: $C826 - FF E0 00 00 C0 FF FF C0
Frame 930-931: $C7DE - 01 C0 00 00 00 00 00 00
Frame 932-933: $C796 - 00 00 00 00 00 00 00 00
Frame 934-935: $C74E - 00 00 00 00 00 00 00 00
Frame 936-937: $C706 - C0 1F FF FE 0F E0 E0 0F
```

**Pattern Address Sequence**: Descending by $48 (72 bytes) per step
- $C946 → $C8FE → $C8B6 → $C86E → $C826 → $C7DE → $C796 → $C74E → $C706

### Final Phase (Frames 938-959)
- **Pattern**: $C046 (Bank 2)
- **Duration**: 22 frames (440ms)
- **Visual**: Final 3D structure state
- **First 16 bytes**: `C0 1F FF FF 1F C0 E0 0F FC 07 07 F8 00 C0 07 07`

### Transition Frame (Frame 960)
- **Pattern**: $C98E
- **Duration**: 1 frame
- **First 16 bytes**: `80 1F FF FF 1F C0 C0 1F FF FF 1F C0 C0 1F FF FE`

## Technical Details

### Pattern Storage
- **Location**: Bank 2 (fixed at $8000-$BFFF, pageable at $C000-$FFFF)
- **Size**: 288 bytes per pattern (48×48 pixels, 6 bytes/row × 48 rows)
- **Format**: Linear byte order (sequential rows)
- **Total patterns**: 12 unique patterns for entire animation

### Memory Banking
- Pattern addresses $C000+ require correct bank to be paged
- During drawing (PC=85AB): Bank 0 or specific bank paged at $C000
- After frame ends: Banking changes to Bank 4
- **Critical**: Must capture during execution with correct banking

### Drawing Routine
- **Location**: $859D-$866D
- **Technique**: Stack-based drawing using `LD SP,HL` + `POP BC` (144 times)
- **Speed**: ~60K Z80 cycles per tile @ 3.5MHz
- **Pattern Table**: $84C0 in Bank 2 (2-byte little-endian addresses)

## Files Generated

### Data Files
- **part3_tiles_900-960.json** - 61 frames with pattern addresses and pixel data
  - Each entry: frame number, pattern address, 288-byte pattern, 48×48 pixel array
  - Total size: ~26KB (base64 encoded patterns)

### Visualizations
- **part3_tiles_900-960_CAPTURED.png** - Sprite sheet showing all 61 frames
  - 8×8 grid layout
  - 1536×1536 pixels total
  - 48×48 per tile at 4× scale
  - Shows complete morphing sequence

## Verification

All 61 frames successfully captured:
- ✅ Frames 900-919: Static pattern ($BE06) - consistent data
- ✅ Frames 920-939: Morphing patterns - smooth transitions
- ✅ Frames 938-959: Final pattern ($C046) - consistent data
- ✅ Frame 960: Transition pattern ($C98E)

Sprite sheet visualization confirms:
- Real 3D cube/diamond structures (not noise)
- Smooth morphing animation between states
- Coherent patterns matching expected visual output

## Extraction Method

```bash
# Export all Part 3 frames
cd emulator
go run run_cubelon.go --no-audio --no-frames \
    --export-part3-tiles=900-960 --stop-at=961

# Generate sprite sheet visualization
cd ../tools
python3 visualize_part3_tiles.py
```

## Code Implementation

Pattern capture hook in `run_cubelon.go`:
- **Hook location**: PC=85AB (right before first `POP BC`)
- **Capture method**: `mem.Read()` respecting current banking
- **Storage**: Map[uint16][]byte persisting across frames
- **Export timing**: Frame N exports patterns captured during frame N-1

## Next Steps

1. ✅ Part 3 algorithm documented (precomputed patterns, not generated)
2. ✅ All pattern data extracted (61 frames)
3. ✅ Sprite sheet visualization created
4. Next: Analyze Part 4/5 or complete documentation

---

**Status**: Part 3 extraction complete and verified
**Date**: March 7, 2026
**Total frames**: 61 (frames 900-960)
**Pattern addresses**: 12 unique addresses across Bank 2
