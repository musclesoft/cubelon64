# CUBELON Effects List

## Demo Structure

**Total Duration**: ~3 minutes 2 seconds
**Display**: Monochrome (white on black), square screen area
**Music**: Single non-looping song

## Effects Breakdown

### Part 1: Tiled Animated Patterns
**Estimated Duration**: TBD
**Description**: Grid of animated tile patterns

**Technical Details**:
- Uses 8x8 tile system
- Animation cycles through pattern variations
- Monochrome display
- Likely character-mode compatible

**C64 Implementation Strategy**:
- Use character mode (40x25 or custom viewport)
- Store tiles as charset
- Animate by cycling character definitions or screen data
- Center playfield with border

### Part 2: 3D Rotating Cubes
**Estimated Duration**: TBD
**Description**: Tiled display of rotating cube animations

**Technical Details**:
- Multiple cubes on grid
- Rotation animation (pre-rendered frames or calculated)
- Synchronized movement

**C64 Implementation Strategy**:
- Pre-render cube animation frames as charset
- Use character mode for tile display
- Cycle through animation frames
- Possible sprite overlay for enhancement

### Part 3: Full-Screen Animated Patterns
**Estimated Duration**: TBD
**Description**: Non-tiled animated patterns covering full square area

**Technical Details**:
- Fills entire display area
- More complex animation than Part 1
- May use different technique than character tiles

**C64 Implementation Strategy**:
- Bitmap mode for full-screen effects
- Or creative character mode with rapid updates
- Pre-calculate patterns or use generation algorithm

### Part 4: Letter Grid
**Estimated Duration**: End sequence
**Description**: Letters appearing on 8x8 grid

**Technical Details**:
- 8x8 grid layout
- Letters appear in sequence
- Likely credits/greetings

**C64 Implementation Strategy**:
- Character mode
- Custom font if needed
- Timed appearance sequence

## Data Block Mapping (Preliminary)

| Block | Likely Content |
|-------|----------------|
| data_01 | Full screen bitmap (title/intro?) |
| data_02 | Full screen with attributes (color data - not used in mono demo) |
| data_03 | Tile set or sprite graphics (2495 bytes) |
| data_04 | Lookup tables / indices (457 bytes) |
| data_05 | Tile animations or patterns (1863 bytes) |
| data_06 | Small lookup table (126 bytes) |
| data_07 | More tile data (1531 bytes) |
| data_08 | Sparse data - possible tiles or masks (1117 bytes, 6% filled) |
| data_09 | Jump tables / addresses (2006 bytes) |
| data_10 | Main demo code (4595 bytes Z80) |
| data_11 | Additional code / music player (11751 bytes) |
| data_12 | More code / data (9446 bytes) |

## Implementation Priority

1. ✅ Basic framework and initialization
2. 🔄 Display system (character mode setup)
3. ⏳ Extract and convert tile graphics
4. ⏳ Implement pattern animation (Part 1)
5. ⏳ Implement cube rotation (Part 2)
6. ⏳ Full-screen patterns (Part 3)
7. ⏳ Letter grid credits (Part 4)
8. ⏳ Music/sound (SID conversion)
9. ⏳ Timing and synchronization
10. ⏳ Transitions between parts

## Technical Notes

- **Monochrome**: Simplifies conversion (no color attribute handling)
- **Square area**: Likely centered on ZX screen (256x192), will center on C64
- **Tiled effects**: Well-suited to C64 character mode
- **Character mode**: Most efficient for tiled patterns (1KB screen + 2KB charset vs 8KB bitmap)

## Next Steps

1. Analyze tile data blocks (03, 05, 07, 08) to extract actual tiles
2. Create simple pattern display test
3. Watch demo again to time each part precisely
4. Begin implementing Part 1 (tiled patterns)
