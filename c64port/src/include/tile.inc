; tile.inc - The 24x24 tile of the kite scenes
;
; A tile is 24 rows of 6 bytes, the format of proto/golden/morph_tiles.bin:
; 3 bytes of full mask (set: outside), then 3 bytes of medium mask (set: not fully inside).
; The rasterizer renders a quad into the petal, rows 1-17 shifted 4 pixels down and right.

PETAL_SLOT = 22
TILE_ROWS = 24
TILE_ROW_BYTES = 6
TILE_BYTES = TILE_ROWS * TILE_ROW_BYTES
Y_SAMPLE_BIAS = $80
IMAGE_BYTES = TILE_BYTES
