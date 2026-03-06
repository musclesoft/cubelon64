# CUBELON Part 2 - Animation Frame Reference

## Complete 52-Frame Cycle (Frames 319-370)

Extracted from ZX Spectrum screen memory (Bank 5, Y=6-17, X=4-6 bytes).
Each square is **24×22 pixels**.

### Key Frames

#### Frame 319 (Start - Emergence)
```
········█···············
········████████········
········████████████····
········████████████····
·······█████████████····
·······████████████·····
·······████████████·····
·······████████████·····
·······████████████·····
·······████████████·····
······█████████████·····
······████████████······
·········█████████······
···············███······
························
························
························
························
························
························
························
························
```

#### Frame 325
```
·······██···············
·······████·············
······██████············
·····████████···········
·····█████████··········
····██████████··········
····███████████·········
····███████████·········
····███████████·········
····███████████·········
····██████████··········
·····█████████··········
······███████···········
·······█████············
·········██·············
························
························
························
························
························
························
························
```

#### Frame 337 (Middle - Two Bars)
```
························
························
██████············██████
██████············██████
██████············██████
██████············██████
██████············██████
██████············██████
██████············██████
██████············██████
██████············██████
██████············██████
██████············██████
██████············██████
························
························
························
························
························
························
························
························
```

#### Frame 345
```
·····████···············
····██████··············
···████████·············
···█████████············
··██████████············
··███████████···········
·████████████···········
·████████████···········
·████████████···········
·████████████···········
··███████████···········
··██████████············
···████████·············
····██████··············
·····████···············
························
························
························
························
························
························
························
```

#### Frame 355
```
·······██···············
·······████·············
······██████············
·····████████···········
·····█████████··········
····██████████··········
····███████████·········
····███████████·········
····███████████·········
····███████████·········
····██████████··········
·····█████████··········
······███████···········
·······████·············
········██··············
························
························
························
························
························
························
························
```

#### Frame 370 (End - Rotation Complete)
```
····███████·············
···█████████············
·███████████············
█████████████···········
██████████████·········█
███████████████·········
·██████████████·········
·██████████████·········
··████████████··········
···█████████············
···████████·············
····█████···············
·····███················
······█·················
························
························
························
························
························
························
························
························
```

## Pattern Analysis

### Frame 337 - The "FC 00 3F" Pattern

This frame corresponds to the pattern we traced in the algorithm:
- **Pattern bytes**: `FC 00 3F 3F 00 FC` (repeating)
- **Screen output**: Two vertical bars (6 pixels each) separated by 12 pixels
- **Binary breakdown**:
  - `FC` = `11111100` = 6 pixels ON, 2 pixels OFF
  - `00` = `00000000` = 8 pixels OFF
  - `3F` = `00111111` = 2 pixels OFF, 6 pixels ON

### Animation Characteristics

**Rotation cycle:**
- Frames 319-330: Cube emerging from center (angular shape)
- Frames 331-345: Rotating through 3D space
- Frames 346-355: Continuing rotation
- Frames 356-370: Completing cycle back to emergence angle

**Pattern types:**
- **Solid blocks**: Frames where cube face is perpendicular to view
- **Diagonal patterns**: Frames showing cube edge rotation
- **Bar patterns**: Frames like 337 showing two separated edges

**Consistency:**
- All frames are 24×24 pixels (72 bytes = 24 rows × 3 bytes)
- Animation is 52 frames total (frames 319-370)
- Each frame represents 1/52 of complete rotation
- Pattern changes smoothly between frames

## Technical Notes

**Extraction method:**
- Extracted directly from ZX Spectrum memory (Bank 2)
- Pattern table at $84C0 (64 entries × 2 bytes)
- Each pattern: 72 bytes with interleaved byte order
  - Even rows: bytes in order [b0, b1, b2]
  - Odd rows: bytes reversed [b2, b1, b0]
- First square of 8×8 grid (table entry 0)
- Fully reproducible from source code

**Pattern structure:**
- 72 bytes = 24 rows × 3 bytes per row
- Row layout: 6 padding + 12 visible + 6 padding
- Total dimensions: 24 pixels wide × 24 pixels tall

**For C64 port:**
- Store all 52 frames × 24×24 pixels = 1.5KB per frame
- Total: 52 frames × 1.5KB ≈ 78KB for one square animation
- Or use pattern table approach with 72-byte patterns
- Frame rate: 25 fps (same as ZX Spectrum)

---

**Files:**
- **Sprite sheet**: [animation_sprite_sheet_source.png](../tools/animation_sprite_sheet_source.png) (52 frames, 13×4 grid)
- **Source data**: [animation_frames_319-370.json](../tools/animation_frames_319-370.json) (patterns from memory)
- **Example pattern**: [pattern_frame337_square0_72bytes.bin](../emulator/pattern_frame337_square0_72bytes.bin)

**Extraction commands:**
```bash
# Extract from memory
cd emulator
go run run_cubelon.go --no-audio --no-frames --export-patterns=319-370 --stop-at=371

# Generate sprite sheet
cd ../tools
python3 create_sprite_sheet_from_source.py
```
