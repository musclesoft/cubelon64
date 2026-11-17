; Generic polygon rendering for C64
; Implements scanline algorithm for filled polygons

.include "include/shared_imports.inc"
.include "include/render_private.inc"
.include "include/math_helpers_private.inc"

; Constants
DISPLAY_WIDTH = 24     ; 3 characters × 8 pixels (output resolution)
DISPLAY_HEIGHT = 24    ; 3 characters × 8 pixels (output resolution)
SUPERSAMPLE_WIDTH = 48 ; 2× supersampling
SUPERSAMPLE_HEIGHT = 48

; Charset location (from charset.s)
SQUARE_CHARS = CHARSET_RAM   ; Rotating square uses chars 0-23

; Sprite data (3 sprites for 24×24 coverage with medium brightness)
; Each sprite: 24 pixels wide × 21 rows = 3 bytes × 21 = 63 bytes
; Located at $6800-$683E (sprite 0), $6840-$687E (sprite 1), $6880-$68BE (sprite 2)
SPRITE_DATA = $6800          ; VIC bank 1 + $2800
SPRITE0_DATA = SPRITE_DATA           ; Covers characters 64-66, rows 0-20
SPRITE1_DATA = SPRITE_DATA + $40     ; Sprite 1 (if needed)
SPRITE2_DATA = SPRITE_DATA + $80     ; Sprite 2 (if needed)

.segment "ZEROPAGE"

RENDER_ROW_PTR: .res 2

.segment "CODE"

; Import math helpers
.import div16x16: absolute
.import mul16x8: absolute
.import div24x16: absolute

; Import animation data
.import PRECOMPUTED_FRAMES: absolute

; Render a filled square using scanline algorithm with DDA edge walking
; Input: corner0-corner3 coordinates in Fixed88 format (calculated by calc_rotation)
; Output: Character data (full brightness) + Sprite data (medium brightness)
.export render_quad
.export SPRITE_DATA
.export SQUARE_CHARS
.export SPAN_BITMASK_COL0, SPAN_BITMASK_COL1, SPAN_BITMASK_COL2
.export RND_EDGE_X0, DOWNSAMPLE_Y
.export downsample_spans
.export get_scanline_span

; Render any convex quad. The opposite edges of a parallelogram share their division
; results, so it takes two divisions; any other quad gives every edge its own.
; A quad is a parallelogram when corner 0 + corner 2 = corner 1 + corner 3.
render_quad:
    ldx #2                  ; Y word, then X word of each corner
@coordinate:
    clc
    lda CORNER0_X,x
    adc CORNER2_X,x
    sta RND_EDGE_X0
    lda CORNER0_X + 1,x
    adc CORNER2_X + 1,x
    sta RND_EDGE_X1
    clc
    lda CORNER1_X,x
    adc CORNER3_X,x
    tay
    lda CORNER1_X + 1,x
    adc CORNER3_X + 1,x
    cmp RND_EDGE_X1
    bne @every_edge
    cpy RND_EDGE_X0
    bne @every_edge
    dex
    dex
    bpl @coordinate
    lda #DIV_SLOT_01
    ldx #DIV_SLOT_12
    bne render_with_slots
@every_edge:
    lda #DIV_SLOT_23
    ldx #DIV_SLOT_30

render_with_slots:
    sta RND_DIV_SLOT_EDGE2
    stx RND_DIV_SLOT_EDGE3

    ; Bufferless 2×2 supersampling with 0,B,B,F,F threshold
    ; Uses span intersection testing - no buffer needed
    ; Renders at 48×48, downsamples to 24×24 with antialiasing
    ; PIXEL-EXACT match with reference implementation

    ; Corners are already in 48×48 supersample space - no scaling needed

    ; Initialize SAVED_* variables to prevent garbage from affecting edge init
    lda #0
    sta SAVED_BASE_STEP
    sta SAVED_REM_LO
    sta SAVED_REM_HI
    sta SAVED_DY_LO
    sta SAVED_DY_HI

    ; Parallel edge optimization: compute division once per parallel pair
    ; Edges 0→1 and 2→3 share division result (stored in DIV_RESULT_01)
    ; Edges 1→2 and 3→0 share division result (stored in DIV_RESULT_12)
    ldx #0
    ldy #4
    lda #DIV_SLOT_01
    jsr compute_edge_division
    ldx #4
    ldy #8
    lda #DIV_SLOT_12
    jsr compute_edge_division
    lda RND_DIV_SLOT_EDGE2
    beq @divisions_done
    ldx #8
    ldy #12
    lda #DIV_SLOT_23
    jsr compute_edge_division
    ldx #12
    ldy #0
    lda #DIV_SLOT_30
    jsr compute_edge_division
@divisions_done:

    ; Initialize DDA edges at 48×48 scale using pre-computed divisions
    jsr init_edge_0_1    ; Uses DIV_RESULT_01
    jsr init_edge_1_2    ; Uses DIV_RESULT_12
    jsr init_edge_2_3    ; Uses DIV_RESULT_01 (reuse)
    jsr init_edge_3_0    ; Uses DIV_RESULT_12 (reuse)

    ; Clear row 0 to foreground (proto skips it)
    lda #$FF
    sta SQUARE_CHARS + 0
    sta SQUARE_CHARS + 64
    sta SQUARE_CHARS + 128
    sta SPRITE0_DATA + 0
    sta SPRITE0_DATA + 1
    sta SPRITE0_DATA + 2

    ; Process output rows 1-17 (proto skips 0, 18-23)
    lda #1
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
    cmp #18
    beq @done_rows
    jmp @output_row_loop
@done_rows:

    ; Clear rows 18-23 to foreground (proto lines 813-819)
    lda #$FF
    ldx #18
@clear_bottom_rows:
    sta SQUARE_CHARS,x
    sta SQUARE_CHARS+64,x
    sta SQUARE_CHARS+128,x
    inx
    cpx #24
    bne @clear_bottom_rows
    ; Write sprite rows 18-23 (6 rows × 3 bytes = 18 bytes)
    ldx #0
@clear_sprite_bottom:
    sta SPRITE0_DATA + 54,x  ; Start at row 18 * 3 = 54
    inx
    cpx #18
    bne @clear_sprite_bottom

    rts

; Get span for current scanline (step edges, get intersections)
; Input: RND_SCANLINE_Y = scanline (0-47)
; Output: RND_INTERSECTION_COUNT, RND_SPAN_X1, RND_SPAN_X2
::get_scanline_span:
    lda #0
    sta RND_INTERSECTION_COUNT
    tax
@edge:
    jsr step_render_edge
    txa
    clc
    adc #EDGE_STRIDE
    tax
    cpx #4 * EDGE_STRIDE
    bcc @edge

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
; Edge Division Computation
; ═══════════════════════════════════════════════════════════

; compute_edge_division: Compute the division of one edge, sorted top to bottom
; Input: X = CORNER0_X offset of the first corner (4 bytes per corner)
;        Y = the same for the second corner
;        A = result slot: offset of the DIV_RESULT_*_BASE from DIV_RESULT_01_BASE
; Stores baseStep, remainder and dy_full in the slot
; Mirrors computeEdgeDivision from proto/main.go
compute_edge_division:
    sta RND_DIV_SLOT

    ; Sort the corners: swap them when y0 >= y1
    lda CORNER0_Y,x
    cmp CORNER0_Y,y
    lda CORNER0_Y+1,x
    sbc CORNER0_Y+1,y
    bcc @sorted
    stx SPAN_TEMP
    tya
    tax
    ldy SPAN_TEMP
@sorted:

    ; dx_full magnitude = |x1_full - x0_full|
    lda CORNER0_X,y
    sec
    sbc CORNER0_X,x
    sta MATH_DIV_LO
    lda CORNER0_X+1,y
    sbc CORNER0_X+1,x
    sta MATH_DIV_HI
    bcs @positive_dx
    lda #0
    sec
    sbc MATH_DIV_LO
    sta MATH_DIV_LO
    lda #0
    sbc MATH_DIV_HI
    sta MATH_DIV_HI
@positive_dx:

    ; dy_full = y1_full - y0_full
    lda CORNER0_Y,y
    sec
    sbc CORNER0_Y,x
    sta MATH_DIV2_LO
    lda CORNER0_Y+1,y
    sbc CORNER0_Y+1,x
    sta MATH_DIV2_HI
    ldx RND_DIV_SLOT
    lda MATH_DIV2_LO
    sta DIV_RESULT_01_DY_LO,x
    lda MATH_DIV2_HI
    sta DIV_RESULT_01_DY_HI,x

    ; Call div16x16: dx_full / dy_full → quotient + remainder
    jsr div16x16
    ldx RND_DIV_SLOT
    lda MATH_QUOT_LO
    sta DIV_RESULT_01_BASE,x
    lda MATH_REM_LO
    sta DIV_RESULT_01_REM_LO,x
    lda MATH_REM_HI
    sta DIV_RESULT_01_REM_HI,x
    rts

; ═══════════════════════════════════════════════════════════
; DDA Edge Initialization Routines
; ═══════════════════════════════════════════════════════════

; Initialize edge 0 (corner0 -> corner1)
init_edge_0_1:
    ldx #0              ; Edge index
    lda #DIV_SLOT_01
    jmp init_edge_with_div

; Initialize edge 1 (corner1 -> corner2)
init_edge_1_2:
    ldx #1              ; Edge index
    lda #DIV_SLOT_12
    jmp init_edge_with_div

; Initialize edge 2 (corner2 -> corner3)
init_edge_2_3:
    ldx #2              ; Edge index
    lda RND_DIV_SLOT_EDGE2
    jmp init_edge_with_div

; Initialize edge 3 (corner3 -> corner0)
init_edge_3_0:
    ldx #3              ; Edge index
    lda RND_DIV_SLOT_EDGE3
    ; Fall through to init_edge_with_div

; init_edge_with_div: Initialize edge with pre-computed division result
; Input: X = edge index (0-3)
;        A = division result slot (DIV_SLOT_*)
; Uses full Fixed88 coordinates from CORNER registers
; Mirrors initEdge from proto/main.go lines 359-475
init_edge_with_div:
    stx SPAN_SAVED_Y
    sta INIT_EDGE_DXSIGN
    txa
    asl
    asl
    tax
    ldy #0
@first_corner:
    lda CORNER0_X,x
    sta INIT_EDGE_X0_LO,y
    inx
    iny
    cpy #4
    bne @first_corner
    txa
    and #$0F
    tax
@second_corner:
    lda CORNER0_X,x
    sta INIT_EDGE_X0_LO,y
    inx
    iny
    cpy #8
    bne @second_corner

    lda INIT_EDGE_Y1_LO
    cmp INIT_EDGE_Y0_LO
    lda INIT_EDGE_Y1_HI
    sbc INIT_EDGE_Y0_HI
    bcs @sorted
    ldx #3
@swap:
    lda INIT_EDGE_X0_LO,x
    pha
    lda INIT_EDGE_X1_LO,x
    sta INIT_EDGE_X0_LO,x
    pla
    sta INIT_EDGE_X1_LO,x
    dex
    bpl @swap

@sorted:
    lda INIT_EDGE_Y0_LO
    cmp #1
    lda INIT_EDGE_Y0_HI
    adc #0
    sta INIT_EDGE_YSTART
    lda INIT_EDGE_Y1_LO
    cmp #1
    lda INIT_EDGE_Y1_HI
    adc #0
    sta INIT_EDGE_YEND
    cmp INIT_EDGE_YSTART
    beq @empty
    bcs @active
@empty:
    jmp @inactive
@active:
    lda INIT_EDGE_X1_LO
    sec
    sbc INIT_EDGE_X0_LO
    sta INIT_EDGE_DX_LO
    lda INIT_EDGE_X1_HI
    sbc INIT_EDGE_X0_HI
    sta INIT_EDGE_DX_HI
    ldy #0
    bcs @dx_done
    lda #0
    sec
    sbc INIT_EDGE_DX_LO
    sta INIT_EDGE_DX_LO
    lda #0
    sbc INIT_EDGE_DX_HI
    sta INIT_EDGE_DX_HI
    iny
@dx_done:
    sty INIT_EDGE_YDELTA

    ; The division result's five bytes are saved: the multiplication clobbers its slot's registers
    ldx INIT_EDGE_DXSIGN
    ldy #0
@save_division:
    lda DIV_RESULT_01_BASE,x
    sta SAVED_BASE_STEP,y
    inx
    iny
    cpy #5
    bne @save_division

    lda INIT_EDGE_Y0_LO
    beq @no_subpixel
    eor #$FF
    clc
    adc #1
    sta MATH_MUL_B
    lda INIT_EDGE_DX_LO
    sta MATH_MUL_A_LO
    lda INIT_EDGE_DX_HI
    sta MATH_MUL_A_HI
    jsr mul16x8

    lda MATH_PROD_B0
    sta MATH_DIV_LO
    lda MATH_PROD_B1
    sta MATH_DIV_HI
    lda MATH_PROD_B2
    sta MATH_DIV_B2
    lda SAVED_DY_LO
    sta MATH_DIV2_LO
    lda SAVED_DY_HI
    sta MATH_DIV2_HI
    jsr div24x16

    ; xCurrent = (x0 ± xAdvance + 128) >> 8
    lda INIT_EDGE_YDELTA
    bne @subtract
    lda INIT_EDGE_X0_LO
    clc
    adc MATH_QUOT_LO
    sta INIT_EDGE_DXSIGN
    lda INIT_EDGE_X0_HI
    adc MATH_QUOT_HI
    jmp @rounded
@subtract:
    lda INIT_EDGE_X0_LO
    sec
    sbc MATH_QUOT_LO
    sta INIT_EDGE_DXSIGN
    lda INIT_EDGE_X0_HI
    sbc MATH_QUOT_HI
@rounded:
    tax
    lda INIT_EDGE_DXSIGN
    clc
    adc #128
    txa
    adc #0
    sta INIT_EDGE_XCUR
    jmp @store_all

@no_subpixel:
    lda INIT_EDGE_X0_LO
    cmp #128
    lda INIT_EDGE_X0_HI
    adc #0
    sta INIT_EDGE_XCUR
    lda #0
    sta MATH_REM_LO
    sta MATH_REM_HI

@store_all:
    ldx SPAN_SAVED_Y
    lda edge_offsets,x
    tax
    lda INIT_EDGE_XCUR
    sta EDGE0_X_CURRENT,x
    lda MATH_REM_LO
    sta EDGE0_ERR_LO,x
    lda MATH_REM_HI
    sta EDGE0_ERR_HI,x
    lda SAVED_BASE_STEP
    sta EDGE0_BASE_STEP,x
    lda SAVED_REM_LO
    sta EDGE0_REM_LO,x
    lda SAVED_REM_HI
    sta EDGE0_REM_HI,x
    lda SAVED_DY_LO
    sta EDGE0_DY_LO,x
    lda SAVED_DY_HI
    sta EDGE0_DY_HI,x
    lda INIT_EDGE_YDELTA
    sta EDGE0_DX_SIGN,x
    lda INIT_EDGE_YSTART
    sta EDGE0_Y_START,x
    lda INIT_EDGE_YEND
    sta EDGE0_Y_END,x
    rts

@inactive:
    lda #0
    sta SAVED_BASE_STEP
    sta SAVED_REM_LO
    sta SAVED_REM_HI
    sta SAVED_DY_LO
    sta SAVED_DY_HI
    sta INIT_EDGE_YDELTA
    sta INIT_EDGE_XCUR
    sta MATH_REM_LO
    sta MATH_REM_HI
    sta INIT_EDGE_YSTART
    sta INIT_EDGE_YEND
    jmp @store_all

edge_offsets:
    .byte 0, EDGE_STRIDE, 2 * EDGE_STRIDE, 3 * EDGE_STRIDE

; ═══════════════════════════════════════════════════════════
; DDA Edge Stepping
; ═══════════════════════════════════════════════════════════

; Step the edge at offset X (EDGE_STRIDE * edge) and check for intersection
step_render_edge:
    lda RND_SCANLINE_Y
    cmp EDGE0_Y_START,x
    bcc @done
    cmp EDGE0_Y_END,x
    bcs @done

    lda EDGE0_X_CURRENT,x
    ldy RND_INTERSECTION_COUNT
    bne @second
    sta RND_SPAN_X1
    beq @counted
@second:
    sta RND_SPAN_X2
@counted:
    inc RND_INTERSECTION_COUNT

    ; 16-bit Bresenham step (mirrors proto stepEdge lines 499-518)
    ldy EDGE0_DX_SIGN,x
    bne @backwards
    clc
    adc EDGE0_BASE_STEP,x
    jmp @stepped
@backwards:
    sec
    sbc EDGE0_BASE_STEP,x
@stepped:
    sta EDGE0_X_CURRENT,x
    lda EDGE0_ERR_LO,x
    clc
    adc EDGE0_REM_LO,x
    sta EDGE0_ERR_LO,x
    lda EDGE0_ERR_HI,x
    adc EDGE0_REM_HI,x
    sta EDGE0_ERR_HI,x

    lda EDGE0_ERR_LO,x
    sec
    sbc EDGE0_DY_LO,x
    tay
    lda EDGE0_ERR_HI,x
    sbc EDGE0_DY_HI,x
    bcc @done
    sta EDGE0_ERR_HI,x
    tya
    sta EDGE0_ERR_LO,x
    lda EDGE0_DX_SIGN,x
    beq @forwards
    dec EDGE0_X_CURRENT,x
    rts
@forwards:
    inc EDGE0_X_CURRENT,x
@done:
    rts

; Draw a horizontal span of pixels on a scanline
; Input:
;   Y = scanline (0-23)
;   RND_SPAN_X1 = start X (pixel coordinate, 0-23)
;   RND_SPAN_X2 = end X (pixel coordinate, 0-23)
; Modifies: A, X, Y, RND_EDGE_X0, RND_EDGE_X1, RND_EDGE_Y0
; ═══════════════════════════════════════════════════════════
; Supersampling Functions
; ═══════════════════════════════════════════════════════════

; Downsample using span intersection testing
; Input: DOWNSAMPLE_Y = output row (0-23)
;        SPAN_ROW0_X1/X2, SPAN_ROW0_VALID
;        SPAN_ROW1_X1/X2, SPAN_ROW1_VALID
::downsample_spans:
    ; Initialize to $FF (all pixels lit, we'll clear bits where square is)
    lda #$FF
    sta RND_EDGE_X0         ; Column 0 char (full brightness)
    sta RND_EDGE_X1         ; Column 1 char
    sta RND_EDGE_Y0         ; Column 2 char
    sta SPAN_BITMASK_COL0   ; Column 0 sprite (medium brightness)
    sta SPAN_BITMASK_COL1   ; Column 1 sprite
    sta SPAN_BITMASK_COL2   ; Column 2 sprite

@check_valid_spans:
    ; Early exit: if both supersample rows are empty, keep $FF (all lit) and skip to write
    ; No need to write sprite data - sprite mask irrelevant when all pixels are background
    lda SPAN_ROW0_VALID
    ora SPAN_ROW1_VALID
    bne @compute_bounds
    jmp @done_x_charset_only   ; Both invalid: skip pixel loop, write $FF to charset only

@compute_bounds:
    ; Compute output X bounds (proto lines 647-677)
    ; Find min/max supersample X across both rows
    ldx #48                    ; ssXMin = 48
    ldy #0                     ; ssXMax = 0

    ; if row0.valid: update ssXMin/ssXMax from row0
    lda SPAN_ROW0_VALID
    beq @check_row1_bounds
    ; if (row0.x1 < ssXMin) ssXMin = row0.x1
    cpx SPAN_ROW0_X1
    bcc @skip_row0_x1
    ldx SPAN_ROW0_X1
@skip_row0_x1:
    ; if (row0.x2 > ssXMax) ssXMax = row0.x2
    cpy SPAN_ROW0_X2
    bcs @check_row1_bounds
    ldy SPAN_ROW0_X2

@check_row1_bounds:
    ; if row1.valid: update ssXMin/ssXMax from row1
    lda SPAN_ROW1_VALID
    beq @convert_bounds
    ; if (row1.x1 < ssXMin) ssXMin = row1.x1
    cpx SPAN_ROW1_X1
    bcc @skip_row1_x1
    ldx SPAN_ROW1_X1
@skip_row1_x1:
    ; if (row1.x2 > ssXMax) ssXMax = row1.x2
    cpy SPAN_ROW1_X2
    bcs @convert_bounds
    ldy SPAN_ROW1_X2

@convert_bounds:
    ; outXMin = ssXMin >> 1
    txa
    lsr
    sta DOWNSAMPLE_X
    ; outXMax = (ssXMax + 1) >> 1
    tya
    clc
    adc #1
    lsr
    sta DOWNSAMPLE_X_MAX

@start_loop:
    ; Iterate in supersample space (2× coordinates) to avoid conversion overhead.
    ; Use ssX as loop variable (0, 2, 4, ..., 46), derive outX when needed.
    ; ssXStart = outXMin * 2 (already in DOWNSAMPLE_X from @convert_bounds)
    lda DOWNSAMPLE_X
    asl
    sta SPAN_PIXEL_X        ; ssXStart
    ; ssXEnd = outXMax * 2
    lda DOWNSAMPLE_X_MAX
    asl
    sta SUBPIXEL_COUNT      ; Reuse SUBPIXEL_COUNT to store ssXEnd

@x_loop:
    ; Check loop condition: ssX < ssXEnd
    lda SPAN_PIXEL_X
    cmp SUBPIXEL_COUNT
    bcc @continue_loop      ; Continue if ssX < ssXEnd
    jmp @done_x             ; Exit if ssX >= ssXEnd
@continue_loop:

    ; Count how many of the 4 subpixels are inside spans.
    ; Hoist validity checks: check row0.valid once (not twice) and row1.valid once (not twice).
    ; On 6502: LDA + BEQ is 4 cycles. Hoisting saves 4-8 cycles per pixel.
    ldy #0                  ; count = 0

    ; Check both subpixels in row0 if valid (one validity check instead of two)
    lda SPAN_ROW0_VALID
    beq @check_row1         ; Skip row0 if invalid

    ; Check ssX in row0 - [X1, X2) exclusive end
    lda SPAN_PIXEL_X
    cmp SPAN_ROW0_X1
    bcc @check_r0_ssX1      ; ssX < X1
    cmp SPAN_ROW0_X2
    bcs @check_r0_ssX1      ; ssX >= X2
    iny                     ; count++
@check_r0_ssX1:

    ; Check ssX+1 in row0 - [X1, X2) exclusive end
    lda SPAN_PIXEL_X
    clc
    adc #1
    cmp SPAN_ROW0_X1
    bcc @check_row1         ; ssX+1 < X1
    cmp SPAN_ROW0_X2
    bcs @check_row1         ; ssX+1 >= X2
    iny                     ; count++

@check_row1:
    ; Check subpixels in row1 if valid (one validity check, with early exit at count>=3)
    lda SPAN_ROW1_VALID
    beq @apply_threshold    ; Skip row1 if invalid

    ; Check ssX in row1 - [X1, X2) exclusive end
    lda SPAN_PIXEL_X
    cmp SPAN_ROW1_X1
    bcc @check_r1_ssX1      ; ssX < X1
    cmp SPAN_ROW1_X2
    bcs @check_r1_ssX1      ; ssX >= X2
    iny                     ; count++
@check_r1_ssX1:

    ; Early exit: if count >= 3, skip 4th subpixel check
    cpy #3
    bcs @apply_threshold

    ; Check ssX+1 in row1 - [X1, X2) exclusive end
    lda SPAN_PIXEL_X
    clc
    adc #1
    cmp SPAN_ROW1_X1
    bcc @apply_threshold    ; ssX+1 < X1
    cmp SPAN_ROW1_X2
    bcs @apply_threshold    ; ssX+1 >= X2
    iny                     ; count++

@apply_threshold:
    ; Only clear bits if count > 0 (pixel inside square)
    cpy #0
    bne @apply_mask
    jmp @next_x
@apply_mask:

    lda SPAN_PIXEL_X
    and #$0F
    tax
    lda mask_by_ssx,x
    eor #$FF
    sta DOWNSAMPLE_X_MAX
    lda SPAN_PIXEL_X
    lsr
    lsr
    lsr
    lsr
    tax
    lda DOWNSAMPLE_X_MAX
    and RND_EDGE_X0,x
    sta RND_EDGE_X0,x
    cpy #3
    bcc @next_x
    lda DOWNSAMPLE_X_MAX
    and SPAN_BITMASK_COL0,x
    sta SPAN_BITMASK_COL0,x

@next_x:
    ; Increment by 2 in supersample space.
    ; 6502: INX, INX (4 cycles) vs previous ASL + ADC (6 cycles) = 2 cycles saved.
    lda SPAN_PIXEL_X
    clc
    adc #2
    sta SPAN_PIXEL_X
    jmp @x_loop

@done_x_charset_only:
@done_x:
    lda BUILD_SLOT
    lsr
    sta RENDER_ROW_PTR + 1
    lda #0
    ror
    sta RENDER_ROW_PTR
    lda DOWNSAMPLE_Y
    sec
    sbc #1
    sta SPAN_TEMP
    asl
    adc SPAN_TEMP
    asl
    ora RENDER_ROW_PTR
    clc
    adc #<PRECOMPUTED_FRAMES
    sta RENDER_ROW_PTR
    lda RENDER_ROW_PTR + 1
    adc #>PRECOMPUTED_FRAMES
    sta RENDER_ROW_PTR + 1

    ldy #5
@sprite:
    lda SPAN_BITMASK_COL0 - 3,y
    sta (RENDER_ROW_PTR),y
    dey
    cpy #2
    bne @sprite
@charset:
    lda RND_EDGE_X0,y
    sta (RENDER_ROW_PTR),y
    dey
    bpl @charset
    rts

.segment "RODATA"

; Mask lookup indexed by supersample coordinate (ssX & 15).
; Since ssX is always even (0, 2, 4, ..., 46), this directly maps ssX to mask
; without needing to divide by 2 first. Odd entries duplicate even for completeness.
mask_by_ssx:
    .byte $80, $80  ; ssX=0,1 → mask for bit 0
    .byte $40, $40  ; ssX=2,3 → mask for bit 1
    .byte $20, $20  ; ssX=4,5 → mask for bit 2
    .byte $10, $10  ; ssX=6,7 → mask for bit 3
    .byte $08, $08  ; ssX=8,9 → mask for bit 4
    .byte $04, $04  ; ssX=10,11 → mask for bit 5
    .byte $02, $02  ; ssX=12,13 → mask for bit 6
    .byte $01, $01  ; ssX=14,15 → mask for bit 7
