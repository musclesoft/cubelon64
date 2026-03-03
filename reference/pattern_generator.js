#!/usr/bin/env node
/**
 * CUBELON Pattern Generator - Reference Implementation
 *
 * Based on ZX Spectrum algorithm at $C076-$C096
 * This generates the animated patterns seen in the original demo
 */

/**
 * Initialize pattern tiles based on ZX Spectrum data
 */
function initTiles() {
    const tiles = [];

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
function xorTiles(tile1, tile2) {
    const result = [];
    for (let i = 0; i < 8; i++) {
        result[i] = tile1[i] ^ tile2[i];
    }
    return result;
}

/**
 * Rotate tile bytes left by n positions
 */
function rotateTileLeft(tile, n) {
    const result = [];
    const len = tile.length;
    n = ((n % len) + len) % len;

    for (let i = 0; i < len; i++) {
        result[i] = tile[(i + n) % len];
    }
    return result;
}

/**
 * Rotate tile bytes right by n positions
 */
function rotateTileRight(tile, n) {
    return rotateTileLeft(tile, -n);
}

/**
 * Generate pattern for current frame (3x3 grid)
 */
function generatePattern(animFrame, tiles) {
    // Select base tiles based on animation frame
    const t0 = animFrame === 0 ? tiles[0] : tiles[8];
    const t1 = animFrame === 0 ? tiles[1] : tiles[9];

    // Create combined tile (T0 XOR T1)
    const tCombined = xorTiles(t0, t1);

    // Generate 3x3 pattern grid
    const pattern = [
        [t0, t1, t0],
        [t1, tCombined, t1],
        [t0, t1, t0]
    ];

    return pattern;
}

/**
 * Generate full 8x8 pattern grid with rotation
 * This implements the more complex ZX Spectrum algorithm
 */
function generatePattern8x8(animFrame, tiles, rotationAmount) {
    const grid = [];

    // Select base tiles
    const t0 = animFrame === 0 ? tiles[0] : tiles[8];
    const t1 = animFrame === 0 ? tiles[1] : tiles[9];
    const t2 = animFrame === 0 ? tiles[2] : tiles[10];

    // Apply rotation to tiles
    const t0rot = rotateTileLeft(t0, rotationAmount);
    const t1rot = rotateTileLeft(t1, rotationAmount);
    const t2rot = rotateTileLeft(t2, rotationAmount);

    // Generate 8x8 grid with pattern
    // Pattern repeats with variations based on position
    for (let row = 0; row < 8; row++) {
        grid[row] = [];
        for (let col = 0; col < 8; col++) {
            // Determine which tile based on position
            // Create alternating pattern with XOR combinations
            let tile;

            if ((row + col) % 2 === 0) {
                tile = t0rot;
            } else if ((row % 2 === 0 && col % 2 === 1) || (row % 2 === 1 && col % 2 === 0)) {
                tile = t1rot;
            } else {
                // XOR combined tiles for interesting patterns
                tile = xorTiles(t0rot, t1rot);
            }

            // Add some variation based on row/col
            if ((row + col) % 3 === 0) {
                tile = xorTiles(tile, t2rot);
            }

            grid[row][col] = tile;
        }
    }

    return grid;
}

/**
 * Generate pattern based on ZX Spectrum algorithm at $C076-$C096
 * This is a more faithful reproduction of the original algorithm
 */
function generatePatternZXAlgorithm(frame, tiles) {
    const pattern = [];

    // The ZX algorithm uses pattern data and rotates/combines it
    // Based on disassembly notes, it appears to:
    // 1. Select base pattern based on frame
    // 2. Rotate pattern data
    // 3. XOR combine different rotations
    // 4. Create grid with symmetry

    const baseRotation = frame * 2; // Rotate more each frame

    const t0 = tiles[0];
    const t1 = tiles[1];
    const t2 = tiles[2];
    const t3 = tiles[3];

    // Create rotated variants
    const variants = [
        t0,
        rotateTileLeft(t1, baseRotation),
        rotateTileLeft(t2, baseRotation * 2),
        xorTiles(t1, t2),
        rotateTileRight(t3, baseRotation),
        xorTiles(t0, t1),
        xorTiles(t1, t3),
        xorTiles(t2, t3)
    ];

    // Build 8x8 grid using variants
    for (let row = 0; row < 8; row++) {
        pattern[row] = [];
        for (let col = 0; col < 8; col++) {
            const index = (row + col * 2) % variants.length;
            pattern[row][col] = variants[index];
        }
    }

    return pattern;
}

/**
 * Render a tile to ASCII art
 */
function renderTileASCII(tile) {
    const lines = [];
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
function renderPatternASCII(pattern) {
    const output = [];

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

    const tiles = initTiles();

    // Test frame 0
    console.log('\nFrame 0 (Static):');
    console.log('-'.repeat(80));
    const pattern0 = generatePattern(0, tiles);
    console.log(renderPatternASCII(pattern0));

    // Test frame 1
    console.log('\n\nFrame 1 (Animated):');
    console.log('-'.repeat(80));
    const pattern1 = generatePattern(1, tiles);
    console.log(renderPatternASCII(pattern1));

    // Show individual tiles
    console.log('\n\nIndividual Tile Definitions:');
    console.log('-'.repeat(80));
    for (let i = 0; i <= 10; i++) {
        console.log(`\nTile ${i}:`);
        console.log(renderTileASCII(tiles[i]).join('\n'));
    }

    // Show hex values
    console.log('\n\nTile Hex Values (for C64 assembly):');
    console.log('-'.repeat(80));
    for (let i = 0; i <= 2; i++) {
        const hex = tiles[i].map(b => '$' + b.toString(16).toUpperCase().padStart(2, '0')).join(', ');
        console.log(`Tile ${i}: ${hex}`);
    }
}

// Run only if executed directly (not when imported in browser)
if (typeof module !== 'undefined' && require.main === module) {
    main();
}

// Export for Node.js
if (typeof module !== 'undefined') {
    module.exports = {
        initTiles,
        generatePattern,
        generatePattern8x8,
        generatePatternZXAlgorithm,
        xorTiles,
        rotateTileLeft,
        rotateTileRight,
        renderPatternASCII
    };
}
