; Generic polygon rendering for C64
; Implements scanline algorithm for filled polygons

.include "include/shared_imports.inc"
.include "include/render_private.inc"

; Constants
DISPLAY_WIDTH = 24     ; 3 characters × 8 pixels (output resolution)
DISPLAY_HEIGHT = 24    ; 3 characters × 8 pixels (output resolution)
SUPERSAMPLE_WIDTH = 48 ; 2× supersampling
SUPERSAMPLE_HEIGHT = 48

; Charset location (from charset.s)
CHARSET_RAM = $2000
SQUARE_CHARS = CHARSET_RAM + (64 * 8)

; Sprite data (3 sprites for 24×24 coverage with medium brightness)
; Each sprite: 24 pixels wide × 21 rows = 3 bytes × 21 = 63 bytes
; Located at $2800-$283E (sprite 0), $2840-$287E (sprite 1), $2880-$28BE (sprite 2)
SPRITE_DATA = $2800
SPRITE0_DATA = SPRITE_DATA           ; Covers characters 64-66, rows 0-20
SPRITE1_DATA = SPRITE_DATA + $40     ; Sprite 1 (if needed)
SPRITE2_DATA = SPRITE_DATA + $80     ; Sprite 2 (if needed)

.segment "CODE"

; Render a filled square using scanline algorithm with DDA edge walking
; Input: corner0-corner3 coordinates in Fixed88 format (calculated by calc_rotation)
; Output: Character data (full brightness) + Sprite data (medium brightness)
.export render_square
render_square:
    ; Bufferless 2×2 supersampling with 0,B,B,F,F threshold
    ; Uses span intersection testing - no buffer needed
    ; Renders at 48×48, downsamples to 24×24 with antialiasing
    ; PIXEL-EXACT match with reference implementation

    ; Clear sprite data (63 bytes)
    lda #$00
    ldx #0
@clear_sprite:
    sta SPRITE0_DATA,x
    inx
    cpx #63
    bne @clear_sprite

    ; Save original corners
    ldx #0
@save_corners:
    lda CORNER0_X,x
    sta CORNER_BACKUP,x
    inx
    cpx #16
    bne @save_corners

    ; Scale corners by 2× for 48×48 rendering
    jsr scale_corners_2x

    ; No offset needed - 12x12 square at (6,6)-(18,18) after 2× scale is (12,12)-(36,36)

    ; Initialize DDA edges at 48×48 scale
    jsr init_edge_0_1
    jsr init_edge_1_2
    jsr init_edge_2_3
    jsr init_edge_3_0

    ; Process output rows 0-23
    lda #0
    sta DOWNSAMPLE_Y
@output_row_loop:
    ; Get span for supersample row 2Y (row 0)
    lda DOWNSAMPLE_Y
    asl
    sta RND_SCANLINE_Y
    jsr get_scanline_span
    lda RND_INTERSECTION_COUNT
    cmp #2
    bne @no_span0
    lda RND_SPAN_X1
    sta SPAN_ROW0_X1
    lda RND_SPAN_X2
    sta SPAN_ROW0_X2
    lda #1
    sta SPAN_ROW0_VALID
    jmp @get_span1
@no_span0:
    lda #0
    sta SPAN_ROW0_VALID

@get_span1:
    ; Get span for supersample row 2Y+1 (row 1)
    inc RND_SCANLINE_Y
    jsr get_scanline_span
    lda RND_INTERSECTION_COUNT
    cmp #2
    bne @no_span1
    lda RND_SPAN_X1
    sta SPAN_ROW1_X1
    lda RND_SPAN_X2
    sta SPAN_ROW1_X2
    lda #1
    sta SPAN_ROW1_VALID
    jmp @downsample
@no_span1:
    lda #0
    sta SPAN_ROW1_VALID

@downsample:
    ; Downsample using span intersection testing
    jsr downsample_spans

    ; Next output row
    inc DOWNSAMPLE_Y
    lda DOWNSAMPLE_Y
    cmp #24
    bne @output_row_loop

    ; Restore original corners
    ldx #0
@restore_corners:
    lda CORNER_BACKUP,x
    sta CORNER0_X,x
    inx
    cpx #16
    bne @restore_corners

    rts

; Get span for current scanline (step edges, get intersections)
; Input: RND_SCANLINE_Y = scanline (0-47)
; Output: RND_INTERSECTION_COUNT, RND_SPAN_X1, RND_SPAN_X2
get_scanline_span:
    lda #0
    sta RND_INTERSECTION_COUNT
    jsr step_edge_0_1
    jsr step_edge_1_2
    jsr step_edge_2_3
    jsr step_edge_3_0

    ; Ensure X1 <= X2
    lda RND_INTERSECTION_COUNT
    cmp #2
    bne @done
    lda RND_SPAN_X1
    cmp RND_SPAN_X2
    bcc @done
    beq @done
    ldy RND_SPAN_X1
    lda RND_SPAN_X2
    sta RND_SPAN_X1
    sty RND_SPAN_X2
@done:
    rts

; ═══════════════════════════════════════════════════════════
; DDA Edge Initialization Routines
; ═══════════════════════════════════════════════════════════

; Initialize edge 0 (corner0 -> corner1)
init_edge_0_1:
    lda CORNER0_X+1
    sta RND_EDGE_X0
    lda CORNER1_X+1
    sta RND_EDGE_X1
    lda CORNER0_Y+1
    sta RND_EDGE_Y0
    lda CORNER1_Y+1
    sta RND_EDGE_Y1
    ldx #0              ; Edge index
    jmp init_edge_common

; Initialize edge 1 (corner1 -> corner2)
init_edge_1_2:
    lda CORNER1_X+1
    sta RND_EDGE_X0
    lda CORNER2_X+1
    sta RND_EDGE_X1
    lda CORNER1_Y+1
    sta RND_EDGE_Y0
    lda CORNER2_Y+1
    sta RND_EDGE_Y1
    ldx #1
    jmp init_edge_common

; Initialize edge 2 (corner2 -> corner3)
init_edge_2_3:
    lda CORNER2_X+1
    sta RND_EDGE_X0
    lda CORNER3_X+1
    sta RND_EDGE_X1
    lda CORNER2_Y+1
    sta RND_EDGE_Y0
    lda CORNER3_Y+1
    sta RND_EDGE_Y1
    ldx #2
    jmp init_edge_common

; Initialize edge 3 (corner3 -> corner0)
init_edge_3_0:
    lda CORNER3_X+1
    sta RND_EDGE_X0
    lda CORNER0_X+1
    sta RND_EDGE_X1
    lda CORNER3_Y+1
    sta RND_EDGE_Y0
    lda CORNER0_Y+1
    sta RND_EDGE_Y1
    ldx #3
    ; Fall through to init_edge_common

; Common edge initialization
; Input: X = edge index (0-3), RND_EDGE_X0/X1/Y0/Y1 loaded
init_edge_common:
    ; Save edge index
    stx SPAN_SAVED_Y

    ; Calculate dy_abs and determine y_start/y_end
    lda RND_EDGE_Y0
    cmp RND_EDGE_Y1
    bne @not_horizontal
    jmp @horizontal     ; Y0 == Y1, horizontal edge
@not_horizontal:
    bcs @y0_greater

    ; Y0 < Y1: upward edge
    ; dy_abs = Y1 - Y0
    lda RND_EDGE_Y1
    sec
    sbc RND_EDGE_Y0
    sta RND_DY_ABS
    ; y_start = Y0, y_end = Y1
    lda RND_EDGE_Y0
    sta RND_EDGE_Y0         ; Keep Y0 as start
    lda RND_EDGE_Y1
    sta RND_EDGE_Y1         ; Keep Y1 as end
    ; X_current = X0
    lda RND_EDGE_X0
    sta RND_EDGE_X0         ; Keep X0
    jmp @calc_dx

@y0_greater:
    ; Y0 > Y1: downward edge
    ; dy_abs = Y0 - Y1
    lda RND_EDGE_Y0
    sec
    sbc RND_EDGE_Y1
    sta RND_DY_ABS
    ; y_start = Y1, y_end = Y0 (swap so we always go up)
    lda RND_EDGE_Y1
    pha
    lda RND_EDGE_Y0
    sta RND_EDGE_Y1         ; Y1 becomes end
    pla
    sta RND_EDGE_Y0         ; Y0 becomes start
    ; X_current = X at y_start (was X1)
    lda RND_EDGE_X1
    pha
    lda RND_EDGE_X0
    sta RND_EDGE_X1         ; Swap X values too
    pla
    sta RND_EDGE_X0

@calc_dx:
    ; Calculate dx_abs and dx_sign
    lda RND_EDGE_X1
    cmp RND_EDGE_X0
    bcs @x1_ge_x0

    ; X1 < X0: negative dx
    lda RND_EDGE_X0
    sec
    sbc RND_EDGE_X1
    sta RND_DX_ABS
    lda #1
    sta RND_DX_SIGN
    jmp @store_edge

@x1_ge_x0:
    ; X1 >= X0: positive dx
    lda RND_EDGE_X1
    sec
    sbc RND_EDGE_X0
    sta RND_DX_ABS
    lda #0
    sta RND_DX_SIGN

@store_edge:
    ; Store edge state based on edge index
    ldx SPAN_SAVED_Y
    cpx #0
    beq @store_edge0
    cpx #1
    beq @store_edge1
    cpx #2
    beq @store_edge2
    ; Must be edge 3
    lda RND_EDGE_X0
    sta EDGE3_X_CURRENT
    lda #0
    sta EDGE3_ERROR
    lda RND_DX_ABS
    sta EDGE3_DX_ABS
    lda RND_DY_ABS
    sta EDGE3_DY_ABS
    lda RND_DX_SIGN
    sta EDGE3_DX_SIGN
    lda RND_EDGE_Y0
    sta EDGE3_Y_START
    lda RND_EDGE_Y1
    sta EDGE3_Y_END
    rts

@store_edge0:
    lda RND_EDGE_X0
    sta EDGE0_X_CURRENT
    lda #0
    sta EDGE0_ERROR
    lda RND_DX_ABS
    sta EDGE0_DX_ABS
    lda RND_DY_ABS
    sta EDGE0_DY_ABS
    lda RND_DX_SIGN
    sta EDGE0_DX_SIGN
    lda RND_EDGE_Y0
    sta EDGE0_Y_START
    lda RND_EDGE_Y1
    sta EDGE0_Y_END
    rts

@store_edge1:
    lda RND_EDGE_X0
    sta EDGE1_X_CURRENT
    lda #0
    sta EDGE1_ERROR
    lda RND_DX_ABS
    sta EDGE1_DX_ABS
    lda RND_DY_ABS
    sta EDGE1_DY_ABS
    lda RND_DX_SIGN
    sta EDGE1_DX_SIGN
    lda RND_EDGE_Y0
    sta EDGE1_Y_START
    lda RND_EDGE_Y1
    sta EDGE1_Y_END
    rts

@store_edge2:
    lda RND_EDGE_X0
    sta EDGE2_X_CURRENT
    lda #0
    sta EDGE2_ERROR
    lda RND_DX_ABS
    sta EDGE2_DX_ABS
    lda RND_DY_ABS
    sta EDGE2_DY_ABS
    lda RND_DX_SIGN
    sta EDGE2_DX_SIGN
    lda RND_EDGE_Y0
    sta EDGE2_Y_START
    lda RND_EDGE_Y1
    sta EDGE2_Y_END
    rts

@horizontal:
    ; Horizontal edge: set dy_abs = 0, but still need dx_abs for intersection calc
    lda #0
    sta RND_DY_ABS
    ; Y values are equal, keep them as-is
    ; X_current = X0 (no need to swap)
    ; Calculate dx_abs
    lda RND_EDGE_X1
    cmp RND_EDGE_X0
    bcs @horizontal_x1_ge_x0
    ; X1 < X0: negative dx
    lda RND_EDGE_X0
    sec
    sbc RND_EDGE_X1
    sta RND_DX_ABS
    lda #1
    sta RND_DX_SIGN
    jmp @store_edge
@horizontal_x1_ge_x0:
    ; X1 >= X0: positive dx
    lda RND_EDGE_X1
    sec
    sbc RND_EDGE_X0
    sta RND_DX_ABS
    lda #0
    sta RND_DX_SIGN
    jmp @store_edge

; ═══════════════════════════════════════════════════════════
; DDA Edge Stepping Routines
; ═══════════════════════════════════════════════════════════

; Step edge 0 and check for intersection
step_edge_0_1:
    ; Check if scanline is in range [y_start, y_end)
    lda RND_SCANLINE_Y
    cmp EDGE0_Y_START
    bcc @no_int             ; scanline < y_start
    cmp EDGE0_Y_END
    bcs @no_int             ; scanline >= y_end

    ; Check if edge is horizontal (dy_abs == 0)
    lda EDGE0_DY_ABS
    beq @no_int

    ; Store X_current as intersection
    lda EDGE0_X_CURRENT
    ldx RND_INTERSECTION_COUNT
    beq @store_x1
    sta RND_SPAN_X2
    inc RND_INTERSECTION_COUNT
    jmp @update

@store_x1:
    sta RND_SPAN_X1
    inc RND_INTERSECTION_COUNT

@update:
    ; Update X using Bresenham: error += dx_abs
    lda EDGE0_ERROR
    clc
    adc EDGE0_DX_ABS
    sta EDGE0_ERROR
    ; If error >= dy_abs: error -= dy_abs, X += sign(dx)
    cmp EDGE0_DY_ABS
    bcc @no_int
    sec
    sbc EDGE0_DY_ABS
    sta EDGE0_ERROR
    ; Adjust X based on sign
    lda EDGE0_DX_SIGN
    beq @inc_x0
    dec EDGE0_X_CURRENT
    rts
@inc_x0:
    inc EDGE0_X_CURRENT
@no_int:
    rts

; Step edge 1 and check for intersection
step_edge_1_2:
    lda RND_SCANLINE_Y
    cmp EDGE1_Y_START
    bcc @no_int
    cmp EDGE1_Y_END
    bcs @no_int
    lda EDGE1_DY_ABS
    beq @no_int

    lda EDGE1_X_CURRENT
    ldx RND_INTERSECTION_COUNT
    beq @store_x1
    sta RND_SPAN_X2
    inc RND_INTERSECTION_COUNT
    jmp @update

@store_x1:
    sta RND_SPAN_X1
    inc RND_INTERSECTION_COUNT

@update:
    lda EDGE1_ERROR
    clc
    adc EDGE1_DX_ABS
    sta EDGE1_ERROR
    cmp EDGE1_DY_ABS
    bcc @no_int
    sec
    sbc EDGE1_DY_ABS
    sta EDGE1_ERROR
    lda EDGE1_DX_SIGN
    beq @inc_x1
    dec EDGE1_X_CURRENT
    rts
@inc_x1:
    inc EDGE1_X_CURRENT
@no_int:
    rts

; Step edge 2 and check for intersection
step_edge_2_3:
    lda RND_SCANLINE_Y
    cmp EDGE2_Y_START
    bcc @no_int
    cmp EDGE2_Y_END
    bcs @no_int
    lda EDGE2_DY_ABS
    beq @no_int

    lda EDGE2_X_CURRENT
    ldx RND_INTERSECTION_COUNT
    beq @store_x1
    sta RND_SPAN_X2
    inc RND_INTERSECTION_COUNT
    jmp @update

@store_x1:
    sta RND_SPAN_X1
    inc RND_INTERSECTION_COUNT

@update:
    lda EDGE2_ERROR
    clc
    adc EDGE2_DX_ABS
    sta EDGE2_ERROR
    cmp EDGE2_DY_ABS
    bcc @no_int
    sec
    sbc EDGE2_DY_ABS
    sta EDGE2_ERROR
    lda EDGE2_DX_SIGN
    beq @inc_x2
    dec EDGE2_X_CURRENT
    rts
@inc_x2:
    inc EDGE2_X_CURRENT
@no_int:
    rts

; Step edge 3 and check for intersection
step_edge_3_0:
    lda RND_SCANLINE_Y
    cmp EDGE3_Y_START
    bcc @no_int
    cmp EDGE3_Y_END
    bcs @no_int
    lda EDGE3_DY_ABS
    beq @no_int

    lda EDGE3_X_CURRENT
    ldx RND_INTERSECTION_COUNT
    beq @store_x1
    sta RND_SPAN_X2
    inc RND_INTERSECTION_COUNT
    jmp @update

@store_x1:
    sta RND_SPAN_X1
    inc RND_INTERSECTION_COUNT

@update:
    lda EDGE3_ERROR
    clc
    adc EDGE3_DX_ABS
    sta EDGE3_ERROR
    cmp EDGE3_DY_ABS
    bcc @no_int
    sec
    sbc EDGE3_DY_ABS
    sta EDGE3_ERROR
    lda EDGE3_DX_SIGN
    beq @inc_x3
    dec EDGE3_X_CURRENT
    rts
@inc_x3:
    inc EDGE3_X_CURRENT
@no_int:
    rts

; Draw a horizontal span of pixels on a scanline
; Input:
;   Y = scanline (0-23)
;   RND_SPAN_X1 = start X (pixel coordinate, 0-23)
;   RND_SPAN_X2 = end X (pixel coordinate, 0-23)
; Modifies: A, X, Y, RND_EDGE_X0, RND_EDGE_X1, RND_EDGE_Y0
draw_span:
    ; Save Y (scanline number) on stack for later
    tya
    pha

    ; Initialize bitmasks to zero (we'll set bits for the square)
    lda #$00
    sta RND_EDGE_X0              ; Column 0 bitmask
    sta RND_EDGE_X1              ; Column 1 bitmask
    sta RND_EDGE_Y0              ; Column 2 bitmask

    ; Loop through pixels from RND_SPAN_X1 to RND_SPAN_X2-1 (exclusive of RND_SPAN_X2)
    ldx RND_SPAN_X1
@pixel_loop:
    ; Check if we've reached RND_SPAN_X2 (exclusive)
    cpx RND_SPAN_X2
    beq @pixels_done

    ; Save current pixel X position
    stx SPAN_PIXEL_X

    ; Get bit position within character (X & 7) and look up mask
    txa
    and #$07
    tax
    lda bit_mask_table,x    ; Get bit mask using X as index

    ; Restore pixel position
    ldx SPAN_PIXEL_X

    ; Determine which character and OR mask to set bits
    cpx #8
    bcc @in_char0
    cpx #16
    bcc @in_char1
    ; Must be char2
    ora RND_EDGE_Y0             ; Set bits
    sta RND_EDGE_Y0
    jmp @next_pix
@in_char1:
    ora RND_EDGE_X1             ; Set bits
    sta RND_EDGE_X1
    jmp @next_pix
@in_char0:
    ora RND_EDGE_X0             ; Set bits
    sta RND_EDGE_X0

@next_pix:
    inx
    jmp @pixel_loop

@pixels_done:
    ; Now write the bitmasks to the character data
    ; With column-major + power-of-2: offset = (column << 6) + y
    ; Column 0: y
    ; Column 1: 64 + y
    ; Column 2: 128 + y
    pla                    ; Get scanline Y
    tay                    ; Save in Y for later
    tax                    ; Also in X for indexing

    ; Write to column 0 (offset = y) - direct write (overdraw)
    lda RND_EDGE_X0
    sta SQUARE_CHARS,x

    ; Write to column 1 (offset = 64 + y) - direct write (overdraw)
    lda RND_EDGE_X1
    sta SQUARE_CHARS+64,x

    ; Write to column 2 (offset = 128 + y) - direct write (overdraw)
    lda RND_EDGE_Y0
    sta SQUARE_CHARS+128,x

    ; Y register already contains scanline Y

    rts

; ═══════════════════════════════════════════════════════════
; Supersampling Functions
; ═══════════════════════════════════════════════════════════

; Scale all corner coordinates by 2× for supersampling
; Corners are Fixed 8.8 format, so shift left by 1 (multiply by 2)
scale_corners_2x:
    ; Corner 0 X
    lda CORNER0_X
    asl
    sta CORNER0_X
    lda CORNER0_X+1
    rol
    sta CORNER0_X+1
    ; Corner 0 Y
    lda CORNER0_Y
    asl
    sta CORNER0_Y
    lda CORNER0_Y+1
    rol
    sta CORNER0_Y+1
    ; Corner 1 X
    lda CORNER1_X
    asl
    sta CORNER1_X
    lda CORNER1_X+1
    rol
    sta CORNER1_X+1
    ; Corner 1 Y
    lda CORNER1_Y
    asl
    sta CORNER1_Y
    lda CORNER1_Y+1
    rol
    sta CORNER1_Y+1
    ; Corner 2 X
    lda CORNER2_X
    asl
    sta CORNER2_X
    lda CORNER2_X+1
    rol
    sta CORNER2_X+1
    ; Corner 2 Y
    lda CORNER2_Y
    asl
    sta CORNER2_Y
    lda CORNER2_Y+1
    rol
    sta CORNER2_Y+1
    ; Corner 3 X
    lda CORNER3_X
    asl
    sta CORNER3_X
    lda CORNER3_X+1
    rol
    sta CORNER3_X+1
    ; Corner 3 Y
    lda CORNER3_Y
    asl
    sta CORNER3_Y
    lda CORNER3_Y+1
    rol
    sta CORNER3_Y+1
    rts

; Downsample using span intersection testing
; Input: DOWNSAMPLE_Y = output row (0-23)
;        SPAN_ROW0_X1/X2, SPAN_ROW0_VALID
;        SPAN_ROW1_X1/X2, SPAN_ROW1_VALID
downsample_spans:
    ; Initialize bitmasks for this scanline
    lda #$00
    sta RND_EDGE_X0         ; Column 0 char (full brightness)
    sta RND_EDGE_X1         ; Column 1 char (full brightness)
    sta RND_EDGE_Y0         ; Column 2 char (full brightness)
    sta SPAN_BITMASK_COL0   ; Column 0 sprite (medium brightness)
    sta SPAN_BITMASK_COL1   ; Column 1 sprite (medium brightness)
    sta SPAN_BITMASK_COL2   ; Column 2 sprite (medium brightness)

    ; Process each output X (0-23)
    lda #0
    sta DOWNSAMPLE_X
@x_loop:
    ; Count subpixels using span intersection testing
    lda #0
    sta SUBPIXEL_COUNT

    ; Calculate 2X and 2X+1
    lda DOWNSAMPLE_X
    asl
    sta SPAN_PIXEL_X        ; 2X

    ; Test (2X, row0): 2X >= span0_x1 && 2X < span0_x2
    lda SPAN_ROW0_VALID
    beq @skip_row0_a
    lda SPAN_PIXEL_X
    cmp SPAN_ROW0_X1
    bcc @skip_row0_a
    cmp SPAN_ROW0_X2
    bcs @skip_row0_a
    inc SUBPIXEL_COUNT
@skip_row0_a:

    ; Test (2X+1, row0): 2X+1 >= span0_x1 && 2X+1 < span0_x2
    lda SPAN_ROW0_VALID
    beq @skip_row0_b
    lda SPAN_PIXEL_X
    clc
    adc #1
    cmp SPAN_ROW0_X1
    bcc @skip_row0_b
    cmp SPAN_ROW0_X2
    bcs @skip_row0_b
    inc SUBPIXEL_COUNT
@skip_row0_b:

    ; Test (2X, row1): 2X >= span1_x1 && 2X < span1_x2
    lda SPAN_ROW1_VALID
    beq @skip_row1_a
    lda SPAN_PIXEL_X
    cmp SPAN_ROW1_X1
    bcc @skip_row1_a
    cmp SPAN_ROW1_X2
    bcs @skip_row1_a
    inc SUBPIXEL_COUNT
@skip_row1_a:

    ; Test (2X+1, row1): 2X+1 >= span1_x1 && 2X+1 < span1_x2
    lda SPAN_ROW1_VALID
    beq @skip_row1_b
    lda SPAN_PIXEL_X
    clc
    adc #1
    cmp SPAN_ROW1_X1
    bcc @skip_row1_b
    cmp SPAN_ROW1_X2
    bcs @skip_row1_b
    inc SUBPIXEL_COUNT
@skip_row1_b:

    ; Apply threshold: 0→off, 1-2→medium, 3-4→full
    lda SUBPIXEL_COUNT
    beq @next_x
    cmp #3
    bcs @full_bright
    jsr set_medium_pixel
    jmp @next_x
@full_bright:
    jsr set_full_pixel

@next_x:
    inc DOWNSAMPLE_X
    lda DOWNSAMPLE_X
    cmp #24
    beq @done_x
    jmp @x_loop

@done_x:
    ; Write scanline to character data (full brightness)
    ldy DOWNSAMPLE_Y
    lda RND_EDGE_X0
    sta SQUARE_CHARS,y
    lda RND_EDGE_X1
    sta SQUARE_CHARS+64,y
    lda RND_EDGE_Y0
    sta SQUARE_CHARS+128,y

    ; Write scanline to sprite data (medium brightness)
    ; Sprite covers 24 pixels wide (3 bytes) × 21 rows
    ; Only write first 21 rows (ignore rows 21-23 for now)
    lda DOWNSAMPLE_Y
    cmp #21
    bcs @skip_sprite
    ; Sprite row offset = Y × 3
    tay
    asl                     ; Y × 2
    clc
    adc DOWNSAMPLE_Y        ; Y × 2 + Y = Y × 3
    tay
    lda SPAN_BITMASK_COL0
    sta SPRITE0_DATA,y
    lda SPAN_BITMASK_COL1
    sta SPRITE0_DATA+1,y
    lda SPAN_BITMASK_COL2
    sta SPRITE0_DATA+2,y

@skip_sprite:
    rts

; Set full brightness pixel at current DOWNSAMPLE_X position
; Modifies RND_EDGE_X0/X1/Y0 (character bitmasks)
set_full_pixel:
    ; Get bit mask for X & 7
    lda DOWNSAMPLE_X
    and #$07
    tax
    lda bit_mask_table,x    ; A = bit mask, X = local bit index

    ; Determine which column (0-2) using X register (saved above as local index)
    ; We need DOWNSAMPLE_X for column check but must preserve A (bit mask)
    ; Use X as scratch: load DOWNSAMPLE_X into X for comparison
    ldx DOWNSAMPLE_X
    cpx #8
    bcc @col0
    cpx #16
    bcc @col1
    ; Column 2
    ora RND_EDGE_Y0
    sta RND_EDGE_Y0
    rts
@col1:
    ora RND_EDGE_X1
    sta RND_EDGE_X1
    rts
@col0:
    ora RND_EDGE_X0
    sta RND_EDGE_X0
    rts

; Set medium brightness pixel at current DOWNSAMPLE_X position
; Modifies SPAN_BITMASK_COL0/1/2 (sprite bitmasks)
set_medium_pixel:
    ; Get bit mask for X & 7
    lda DOWNSAMPLE_X
    and #$07
    tax
    lda bit_mask_table,x    ; A = bit mask, preserve it

    ; Determine which column (0-2) using X to avoid overwriting A
    ldx DOWNSAMPLE_X
    cpx #8
    bcc @col0
    cpx #16
    bcc @col1
    ; Column 2
    ora SPAN_BITMASK_COL2
    sta SPAN_BITMASK_COL2
    rts
@col1:
    ora SPAN_BITMASK_COL1
    sta SPAN_BITMASK_COL1
    rts
@col0:
    ora SPAN_BITMASK_COL0
    sta SPAN_BITMASK_COL0
    rts

.segment "RODATA"
; Bit mask table for pixel positions 0-7 (MSB first for C64)
bit_mask_table:
    .byte $80, $40, $20, $10, $08, $04, $02, $01
