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
 * Generate pattern for current frame
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

// Run
main();

module.exports = { initTiles, generatePattern, xorTiles, renderPatternASCII };
