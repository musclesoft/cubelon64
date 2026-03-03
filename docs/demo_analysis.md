# CUBELON Demo Analysis

## Overview

CUBELON is a ZX Spectrum demo extracted from a 48KB TAP file.

**Status**: Phase 1 - Analysis in progress

## Demo Structure (To Be Documented)

🚧 **TODO**: Watch original demo and document:

- ~ 3 min 2 sec
- monochrome square screen area, white on black
- Number of parts/scenes
- Effect types:
  - tiled animated patterns
  - tiled 3d-rotating cubes
  - mid part: non-tiled animated patterns covering the full square area
  - end part: letters appearing on an 8x8 grid
- Single song, not looping

### Running the Original

To analyze the demo:

1. Load `original/cubelon_final.tap` in a ZX Spectrum emulator (Fuse, ZEsarUX, etc.)
2. The demo will auto-load and run
3. Document each part with screenshots and timing notes

## Technical Details

### Memory Layout (ZX Spectrum)

- BASIC loader clears memory at 24575 ($5FFF)
- Main code loads at 30720 ($7800)
- Total program data: ~49KB

### Data Blocks

| Block               | Size        | Likely Content                       |
| ------------------- | ----------- | ------------------------------------ |
| cubelon_code.bin    | 139 bytes   | Loader/decompressor                  |
| cubelon_data_01.bin | 6145 bytes  | Screen data (98% filled)             |
| cubelon_data_02.bin | 7476 bytes  | Graphics tiles or additional screens |
| cubelon_data_03.bin | 2495 bytes  | Sprite/graphic data                  |
| cubelon_data_04.bin | 457 bytes   | Lookup tables                        |
| cubelon_data_05.bin | 1863 bytes  | TBD                                  |
| cubelon_data_06.bin | 126 bytes   | Small lookup table                   |
| cubelon_data_07.bin | 1531 bytes  | TBD                                  |
| cubelon_data_08.bin | 1117 bytes  | TBD                                  |
| cubelon_data_09.bin | 2006 bytes  | Address pointers/jump tables         |
| cubelon_data_10.bin | 4595 bytes  | Main demo code (Z80)                 |
| cubelon_data_11.bin | 11751 bytes | Major code/data block                |
| cubelon_data_12.bin | 9446 bytes  | Additional code/data                 |

### Initial Findings

**Loader Code** (cubelon_code.bin):

- Starts with `DI` (disable interrupts)
- Sets stack pointer to $784D
- Contains decompression/loading routine
- Jumps to main demo code

**Graphics Data** (cubelon_data_01.bin):

- 6145 bytes = 6144 bytes bitmap + 1 byte header
- ZX Spectrum screen format
- 98% filled with pixel data
- Likely title screen or main graphics

**Code Blocks** (data_10, data_11, data_12):

- Contains Z80 instructions (CALL, JP, etc.)
- Main demo logic and effects
- Needs detailed disassembly

## Effects Analysis (To Be Completed)

🚧 **TODO**: Document each effect:

- [ ] Effect 1: [Name/Description]
- [ ] Effect 2: [Name/Description]
- [ ] ...

## Timing and Music

🚧 **TODO**: Document:

- Music format (beeper engine?)
- Synchronization points
- Frame timing
- Duration of each part

## Conversion Notes

### Challenges Identified

1. **Data Decompression**: Original uses Z80 decompressor - need to extract final data
2. **Graphics Format**: ZX 256×192 → C64 320×200 or character mode
3. **Code Translation**: Significant Z80 → 6502 translation required
4. **Music**: Beeper → SID conversion needed

### Next Steps

1. Watch and document the complete demo
2. Disassemble main code blocks
3. Extract and convert graphics data
4. Identify music player routine
5. Create detailed effect-by-effect breakdown
