/**
 * CUBELON Pattern Generator - Reference Implementation
 *
 * Based on ZX Spectrum algorithm at $C076-$C096
 * This generates the animated patterns seen in the original demo
 */

// 8x8 pattern tile (8 bytes, each byte is one row of 8 pixels)
type Tile = number[];

// Pattern state
interface PatternState {
    animFrame: number;  // Animation frame (0 or 1)
    tiles: Tile[];      // Array of 8x8 tiles
}

/**
 * Initialize pattern tiles based on ZX Spectrum data
 */
function initTiles(): Tile[] {
    const tiles: Tile[] = [];

    // Tile 0: Blank
    tiles[0] = [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

    // Tile 1: Checkerboard
    tiles[1] = [0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55];

    // Tile 2: Inverse checkerboard
    tiles[2] = [0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA];

    // Tile 3: Horizontal stripes
    tiles[3] = [0xFF, 0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF, 0x00];

    // Tile 4: Vertical stripes
    tiles[4] = [0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC];

    // Tile 5: Diagonal
    tiles[5] = [0x80, 0x40, 0x20, 0x10, 0x08, 0x04, 0x02, 0x01];

    // Tile 6: Opposite diagonal
    tiles[6] = [0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80];

    // Tile 7: Dots
    tiles[7] = [0x88, 0x00, 0x22, 0x00, 0x88, 0x00, 0x22, 0x00];

    // Tile 8: Cross (animation frame)
    tiles[8] = [0x18, 0x18, 0xFF, 0xFF, 0x18, 0x18, 0x18, 0x18];

    // Tile 9: Border (animation frame)
    tiles[9] = [0xFF, 0x81, 0x81, 0x81, 0x81, 0x81, 0x81, 0xFF];

    // Tile 10: Filled border (animation frame)
    tiles[10] = [0xFF, 0xFF, 0xC3, 0xC3, 0xC3, 0xC3, 0xFF, 0xFF];

    return tiles;
}

/**
 * XOR two tiles together to create a combined pattern
 */
function xorTiles(tile1: Tile, tile2: Tile): Tile {
    const result: Tile = [];
    for (let i = 0; i < 8; i++) {
        result[i] = tile1[i] ^ tile2[i];
    }
    return result;
}

/**
 * Rotate a tile's bytes (circular shift)
 */
function rotateTile(tile: Tile, amount: number): Tile {
    const result = [...tile];
    const len = tile.length;
    amount = ((amount % len) + len) % len; // Handle negative rotations

    for (let i = 0; i < len; i++) {
        result[i] = tile[(i + amount) % len];
    }
    return result;
}

/**
 * Generate pattern for current frame
 * Based on ZX Spectrum algorithm
 */
function generatePattern(state: PatternState): number[][] {
    const { animFrame, tiles } = state;

    // Select base tiles based on animation frame
    const t0 = animFrame === 0 ? tiles[0] : tiles[8];
    const t1 = animFrame === 0 ? tiles[1] : tiles[9];

    // Create combined tile (T0 XOR T1)
    const tCombined = xorTiles(t0, t1);

    // Generate 3x3 pattern grid
    const pattern: number[][] = [
        [t0, t1, t0],
        [t1, tCombined, t1],
        [t0, t1, t0]
    ];

    return pattern;
}

/**
 * Render a tile to ASCII art for visualization
 */
function renderTileASCII(tile: Tile): string[] {
    const lines: string[] = [];
    for (const byte of tile) {
        let line = '';
        for (let bit = 7; bit >= 0; bit--) {
            line += (byte & (1 << bit)) ? '█' : ' ';
        }
        lines.push(line);
    }
    return lines;
}

/**
 * Render entire pattern grid to ASCII
 */
function renderPatternASCII(pattern: number[][]): string {
    const output: string[] = [];

    for (let row = 0; row < pattern.length; row++) {
        const tileRow = pattern[row];
        const renderedTiles = tileRow.map(tile => renderTileASCII(tile));

        // Combine horizontal tiles
        for (let line = 0; line < 8; line++) {
            const combinedLine = renderedTiles.map(t => t[line]).join(' ');
            output.push(combinedLine);
        }

        if (row < pattern.length - 1) {
            output.push(''); // Blank line between rows
        }
    }

    return output.join('\n');
}

/**
 * Main test function
 */
function main() {
    console.log('CUBELON Pattern Generator - Reference Implementation\n');
    console.log('='.repeat(80));

    const state: PatternState = {
        animFrame: 0,
        tiles: initTiles()
    };

    // Test frame 0
    console.log('\nFrame 0 (Static):');
    console.log('-'.repeat(80));
    const pattern0 = generatePattern(state);
    console.log(renderPatternASCII(pattern0));

    // Test frame 1
    console.log('\n\nFrame 1 (Animated):');
    console.log('-'.repeat(80));
    state.animFrame = 1;
    const pattern1 = generatePattern(state);
    console.log(renderPatternASCII(pattern1));

    // Show tile definitions
    console.log('\n\nTile Definitions:');
    console.log('-'.repeat(80));
    for (let i = 0; i <= 10; i++) {
        console.log(`\nTile ${i}:`);
        console.log(renderTileASCII(state.tiles[i]).join('\n'));
    }
}

// Run if executed directly
if (require.main === module) {
    main();
}

export { initTiles, generatePattern, xorTiles, rotateTile, renderPatternASCII, PatternState };
