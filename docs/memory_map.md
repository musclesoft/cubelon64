# Memory Map

## ZX Spectrum Original

### System Memory
```
$0000-$3FFF  ROM (16K)
$4000-$57FF  Screen memory (6K bitmap + attributes)
$5800-$5AFF  Attributes
$5B00-$FFFF  User RAM
```

### CUBELON Memory Layout
```
$0000-$5FFF  BASIC + Variables (CLEAR 24575)
$7800-$????  Demo code and data
             - Starts at 30720 ($7800)
             - Loader/decompressor
             - Main demo code
             - Graphics and data
```

## C64 Target

### System Memory
```
$0000-$00FF  Zero page
$0100-$01FF  Stack
$0200-$03FF  BASIC and KERNAL usage
$0400-$07FF  Default screen RAM (1K)
$0800-$9FFF  User RAM (38K)
$A000-$BFFF  BASIC ROM or RAM
$C000-$CFFF  RAM
$D000-$DFFF  I/O and color RAM
$E000-$FFFF  KERNAL ROM
```

### Planned CUBELON C64 Layout
```
$0002-$001B  Zero page variables (26 bytes)
$0800-$080C  BASIC stub (SYS 2061)
$080D-$9FFF  Demo code (~38K available)
$A000-$BFFF  Demo data (8K, banked if needed)
$C000-$CFFF  BSS/runtime variables (4K)
$D000-$DFFF  I/O access (VIC, SID, CIA)
$D800-$DBFF  Color RAM (1K)

Specific allocations TBD based on demo requirements
```

### VIC-II Banks
```
Bank 0: $0000-$3FFF (default, contains KERNAL)
Bank 1: $4000-$7FFF
Bank 2: $8000-$BFFF
Bank 3: $C000-$FFFF (screen at $0400, charset at $1000)

Will use Bank 3 for initial development
```

### Screen and Charset
```
Default screen:  $0400 (1K)
Color RAM:       $D800 (1K, fixed)
Charset:         $1000 (2K) - if custom charset used
Sprites:         $2000 (depends on VIC bank)
Bitmap (if used): $2000 (8K for hires, 8K for bitmap)
```

## Memory Budget

### ZX Spectrum
- Total available: ~40KB after BASIC
- Demo uses: ~49KB (compressed/packed)

### C64
- Total available: ~50KB
- Demo code: ~20-30KB estimated
- Graphics data: ~10-15KB estimated
- Music: ~2-5KB estimated
- Runtime BSS: ~4KB

**Note**: C64 has more RAM (64KB vs 48KB), providing comfortable space for the port.

## Data Compression

### Original (ZX Spectrum)
- Uses custom Z80 decompressor
- Packs ~49KB of data into TAP file
- Need to extract decompressed data or re-implement

### C64 Port Options
1. Use decompressed data (simpler, larger)
2. Re-pack with Exomizer or Pucrunch (C64 standard packers)
3. Use RLE or custom compression for graphics

**Decision**: TBD based on final size requirements
