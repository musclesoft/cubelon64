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
CHARSET_RAM = $6000          ; VIC bank 1 + $2000
SQUARE_CHARS = CHARSET_RAM   ; Rotating square uses chars 0-23

; Sprite data (3 sprites for 24×24 coverage with medium brightness)
; Each sprite: 24 pixels wide × 21 rows = 3 bytes × 21 = 63 bytes
; Located at $6800-$683E (sprite 0), $6840-$687E (sprite 1), $6880-$68BE (sprite 2)
SPRITE_DATA = $6800          ; VIC bank 1 + $2800
SPRITE0_DATA = SPRITE_DATA           ; Covers characters 64-66, rows 0-20
SPRITE1_DATA = SPRITE_DATA + $40     ; Sprite 1 (if needed)
SPRITE2_DATA = SPRITE_DATA + $80     ; Sprite 2 (if needed)

.segment "CODE"

; Import math helpers
.import div16x16: absolute
.import job_checkpoint: absolute
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
    jsr job_checkpoint
    ldx #4
    ldy #8
    lda #DIV_SLOT_12
    jsr compute_edge_division
    jsr job_checkpoint
    lda RND_DIV_SLOT_EDGE2
    beq @divisions_done
    ldx #8
    ldy #12
    lda #DIV_SLOT_23
    jsr compute_edge_division
    jsr job_checkpoint
    ldx #12
    ldy #0
    lda #DIV_SLOT_30
    jsr compute_edge_division
    jsr job_checkpoint
@divisions_done:

    ; Initialize DDA edges at 48×48 scale using pre-computed divisions
    jsr job_checkpoint
    jsr init_edge_0_1    ; Uses DIV_RESULT_01
    jsr job_checkpoint
    jsr init_edge_1_2    ; Uses DIV_RESULT_12
    jsr job_checkpoint
    jsr init_edge_2_3    ; Uses DIV_RESULT_01 (reuse)
    jsr job_checkpoint
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
    jsr job_checkpoint
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
    jsr job_checkpoint
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
    ; Save edge index and division selector
    stx SPAN_SAVED_Y        ; Edge index (0-3)
    sta INIT_EDGE_DXSIGN    ; Division result selector temporarily

    ; Load corner coordinates based on edge index
    cpx #0
    bne @not_edge0
    jmp @load_01
@not_edge0:
    cpx #1
    bne @not_edge1
    jmp @load_12
@not_edge1:
    cpx #2
    bne @edge3
    jmp @load_23
@edge3:
    ; Edge 3: corner3→corner0
    lda CORNER3_X
    sta INIT_EDGE_X0_LO
    lda CORNER3_X+1
    sta INIT_EDGE_X0_HI
    lda CORNER3_Y
    sta INIT_EDGE_Y0_LO
    lda CORNER3_Y+1
    sta INIT_EDGE_Y0_HI
    lda CORNER0_X
    sta INIT_EDGE_X1_LO
    lda CORNER0_X+1
    sta INIT_EDGE_X1_HI
    lda CORNER0_Y
    sta INIT_EDGE_Y1_LO
    lda CORNER0_Y+1
    sta INIT_EDGE_Y1_HI
    jmp @corners_loaded
@load_01:
    lda CORNER0_X
    sta INIT_EDGE_X0_LO
    lda CORNER0_X+1
    sta INIT_EDGE_X0_HI
    lda CORNER0_Y
    sta INIT_EDGE_Y0_LO
    lda CORNER0_Y+1
    sta INIT_EDGE_Y0_HI
    lda CORNER1_X
    sta INIT_EDGE_X1_LO
    lda CORNER1_X+1
    sta INIT_EDGE_X1_HI
    lda CORNER1_Y
    sta INIT_EDGE_Y1_LO
    lda CORNER1_Y+1
    sta INIT_EDGE_Y1_HI
    jmp @corners_loaded
@load_12:
    lda CORNER1_X
    sta INIT_EDGE_X0_LO
    lda CORNER1_X+1
    sta INIT_EDGE_X0_HI
    lda CORNER1_Y
    sta INIT_EDGE_Y0_LO
    lda CORNER1_Y+1
    sta INIT_EDGE_Y0_HI
    lda CORNER2_X
    sta INIT_EDGE_X1_LO
    lda CORNER2_X+1
    sta INIT_EDGE_X1_HI
    lda CORNER2_Y
    sta INIT_EDGE_Y1_LO
    lda CORNER2_Y+1
    sta INIT_EDGE_Y1_HI
    jmp @corners_loaded
@load_23:
    lda CORNER2_X
    sta INIT_EDGE_X0_LO
    lda CORNER2_X+1
    sta INIT_EDGE_X0_HI
    lda CORNER2_Y
    sta INIT_EDGE_Y0_LO
    lda CORNER2_Y+1
    sta INIT_EDGE_Y0_HI
    lda CORNER3_X
    sta INIT_EDGE_X1_LO
    lda CORNER3_X+1
    sta INIT_EDGE_X1_HI
    lda CORNER3_Y
    sta INIT_EDGE_Y1_LO
    lda CORNER3_Y+1
    sta INIT_EDGE_Y1_HI

@corners_loaded:
    ; Sort top-to-bottom: if y0 > y1, swap
    lda INIT_EDGE_Y0_LO
    cmp INIT_EDGE_Y1_LO
    lda INIT_EDGE_Y0_HI
    sbc INIT_EDGE_Y1_HI
    bcs @not_less           ; y0 >= y1
    jmp @sorted             ; y0 < y1
@not_less:
    bne @do_swap            ; y0 > y1
    lda INIT_EDGE_Y0_LO
    cmp INIT_EDGE_Y1_LO
    bne @do_swap            ; y0 > y1
    jmp @sorted             ; y0 == y1
@do_swap:
    ; Swap all 4 coordinate pairs
    ldx INIT_EDGE_X0_LO
    lda INIT_EDGE_X1_LO
    sta INIT_EDGE_X0_LO
    stx INIT_EDGE_X1_LO
    ldx INIT_EDGE_X0_HI
    lda INIT_EDGE_X1_HI
    sta INIT_EDGE_X0_HI
    stx INIT_EDGE_X1_HI
    ldx INIT_EDGE_Y0_LO
    lda INIT_EDGE_Y1_LO
    sta INIT_EDGE_Y0_LO
    stx INIT_EDGE_Y1_LO
    ldx INIT_EDGE_Y0_HI
    lda INIT_EDGE_Y1_HI
    sta INIT_EDGE_Y0_HI
    stx INIT_EDGE_Y1_HI

@sorted:
    ; yStart = ceil(y0_full / 256) = y0Hi + (y0Lo != 0 ? 1 : 0)
    lda INIT_EDGE_Y0_HI
    sta INIT_EDGE_YSTART
    lda INIT_EDGE_Y0_LO
    bne @inc_ystart
    jmp @ystart_done
@inc_ystart:
    inc INIT_EDGE_YSTART
@ystart_done:

    ; yEnd = ceil(y1_full / 256) = y1Hi + (y1Lo != 0 ? 1 : 0)
    lda INIT_EDGE_Y1_HI
    sta INIT_EDGE_YEND
    lda INIT_EDGE_Y1_LO
    bne @inc_yend
    jmp @yend_done
@inc_yend:
    inc INIT_EDGE_YEND
@yend_done:

    ; Check if edge spans >= 1 scanline
    lda INIT_EDGE_YSTART
    cmp INIT_EDGE_YEND
    bcc @active_edge        ; yStart < yEnd
    jmp @inactive           ; yStart >= yEnd
@active_edge:

    ; Compute dx_full = x1 - x0 (signed 16-bit)
    lda INIT_EDGE_X1_LO
    sec
    sbc INIT_EDGE_X0_LO
    sta INIT_EDGE_DX_LO
    lda INIT_EDGE_X1_HI
    sbc INIT_EDGE_X0_HI
    sta INIT_EDGE_DX_HI
    bcs @dx_positive
    ; Negative: negate and set sign
    lda #0
    sec
    sbc INIT_EDGE_DX_LO
    sta INIT_EDGE_DX_LO
    lda #0
    sbc INIT_EDGE_DX_HI
    sta INIT_EDGE_DX_HI
    lda #1
    sta INIT_EDGE_YDELTA     ; Save dxSign
    jmp @dx_done
@dx_positive:
    lda #0
    sta INIT_EDGE_YDELTA     ; Save dxSign
@dx_done:

    ; Load the pre-computed division result of the slot and save it to dedicated variables
    ; (MUL8_RESULT/DIV16_DIVIDEND will be clobbered by subpixel correction math)
    ldx INIT_EDGE_DXSIGN
    lda DIV_RESULT_01_BASE,x
    sta SAVED_BASE_STEP
    lda DIV_RESULT_01_REM_LO,x
    sta SAVED_REM_LO
    lda DIV_RESULT_01_REM_HI,x
    sta SAVED_REM_HI
    lda DIV_RESULT_01_DY_LO,x
    sta SAVED_DY_LO
    lda DIV_RESULT_01_DY_HI,x
    sta SAVED_DY_HI
@div_loaded:

    ; Sub-pixel correction: yDelta = 256 - y0Lo (if y0Lo != 0)
    lda INIT_EDGE_Y0_LO
    bne @has_subpixel
    jmp @no_subpixel
@has_subpixel:
    ; yDelta = 256 - y0Lo
    sta INIT_EDGE_DXSIGN     ; Save for later
    lda #0
    sec
    sbc INIT_EDGE_DXSIGN
    sta INIT_EDGE_DXSIGN     ; yDelta

    ; mul16x8: dxFull × yDelta → 24-bit
    lda INIT_EDGE_DX_LO
    sta MATH_MUL_A_LO
    lda INIT_EDGE_DX_HI
    sta MATH_MUL_A_HI
    lda INIT_EDGE_DXSIGN
    sta MATH_MUL_B
    jsr mul16x8

    ; div24x16: (product / dyFull) → xAdvance + errInit
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
    lda INIT_EDGE_YDELTA     ; dxSign
    beq @positive_adv
    ; Negative: x0 - xAdvance + 128
    lda INIT_EDGE_X0_LO
    sec
    sbc MATH_QUOT_LO
    sta INIT_EDGE_DXSIGN
    lda INIT_EDGE_X0_HI
    sbc MATH_QUOT_HI
    tax
    lda INIT_EDGE_DXSIGN
    clc
    adc #128
    txa
    adc #0
    sta INIT_EDGE_XCUR
    jmp @store_all
@positive_adv:
    lda INIT_EDGE_X0_LO
    clc
    adc MATH_QUOT_LO
    sta INIT_EDGE_DXSIGN
    lda INIT_EDGE_X0_HI
    adc MATH_QUOT_HI
    tax
    lda INIT_EDGE_DXSIGN
    clc
    adc #128
    txa
    adc #0
    sta INIT_EDGE_XCUR
    jmp @store_all

@no_subpixel:
    ; xCurrent = round(x0), errInit = 0
    lda INIT_EDGE_X0_HI
    sta INIT_EDGE_XCUR
    lda INIT_EDGE_X0_LO
    cmp #128
    bcs @do_round
    jmp @no_round
@do_round:
    inc INIT_EDGE_XCUR
@no_round:
    lda #0
    sta MATH_REM_LO
    sta MATH_REM_HI

@store_all:
    ; Store to appropriate edge structure
    ldx SPAN_SAVED_Y
    cpx #0
    bne @not_st0
    jmp @st0
@not_st0:
    cpx #1
    bne @not_st1
    jmp @st1
@not_st1:
    cpx #2
    bne @st3
    jmp @st2
@st3:
    ; Edge 3
    lda INIT_EDGE_XCUR
    sta EDGE3_X_CURRENT
    lda MATH_REM_LO
    sta EDGE3_ERR_LO
    lda MATH_REM_HI
    sta EDGE3_ERR_HI
    lda SAVED_BASE_STEP
    sta EDGE3_BASE_STEP
    lda SAVED_REM_LO
    sta EDGE3_REM_LO
    lda SAVED_REM_HI
    sta EDGE3_REM_HI
    lda SAVED_DY_LO
    sta EDGE3_DY_LO
    lda SAVED_DY_HI
    sta EDGE3_DY_HI
    lda INIT_EDGE_YDELTA
    sta EDGE3_DX_SIGN
    lda INIT_EDGE_YSTART
    sta EDGE3_Y_START
    lda INIT_EDGE_YEND
    sta EDGE3_Y_END
    rts
@st0:
    lda INIT_EDGE_XCUR
    sta EDGE0_X_CURRENT
    lda MATH_REM_LO
    sta EDGE0_ERR_LO
    lda MATH_REM_HI
    sta EDGE0_ERR_HI
    lda SAVED_BASE_STEP
    sta EDGE0_BASE_STEP
    lda SAVED_REM_LO
    sta EDGE0_REM_LO
    lda SAVED_REM_HI
    sta EDGE0_REM_HI
    lda SAVED_DY_LO
    sta EDGE0_DY_LO
    lda SAVED_DY_HI
    sta EDGE0_DY_HI
    lda INIT_EDGE_YDELTA
    sta EDGE0_DX_SIGN
    lda INIT_EDGE_YSTART
    sta EDGE0_Y_START
    lda INIT_EDGE_YEND
    sta EDGE0_Y_END
    rts
@st1:
    lda INIT_EDGE_XCUR
    sta EDGE1_X_CURRENT
    lda MATH_REM_LO
    sta EDGE1_ERR_LO
    lda MATH_REM_HI
    sta EDGE1_ERR_HI
    lda SAVED_BASE_STEP
    sta EDGE1_BASE_STEP
    lda SAVED_REM_LO
    sta EDGE1_REM_LO
    lda SAVED_REM_HI
    sta EDGE1_REM_HI
    lda SAVED_DY_LO
    sta EDGE1_DY_LO
    lda SAVED_DY_HI
    sta EDGE1_DY_HI
    lda INIT_EDGE_YDELTA
    sta EDGE1_DX_SIGN
    lda INIT_EDGE_YSTART
    sta EDGE1_Y_START
    lda INIT_EDGE_YEND
    sta EDGE1_Y_END
    rts
@st2:
    lda INIT_EDGE_XCUR
    sta EDGE2_X_CURRENT
    lda MATH_REM_LO
    sta EDGE2_ERR_LO
    lda MATH_REM_HI
    sta EDGE2_ERR_HI
    lda SAVED_BASE_STEP
    sta EDGE2_BASE_STEP
    lda SAVED_REM_LO
    sta EDGE2_REM_LO
    lda SAVED_REM_HI
    sta EDGE2_REM_HI
    lda SAVED_DY_LO
    sta EDGE2_DY_LO
    lda SAVED_DY_HI
    sta EDGE2_DY_HI
    lda INIT_EDGE_YDELTA
    sta EDGE2_DX_SIGN
    lda INIT_EDGE_YSTART
    sta EDGE2_Y_START
    lda INIT_EDGE_YEND
    sta EDGE2_Y_END
    rts

@inactive:
    ; Inactive edge: set dy = 0
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

; ═══════════════════════════════════════════════════════════
; DDA Edge Stepping Routines
; ═══════════════════════════════════════════════════════════

; Step edge 0 and check for intersection
step_edge_0_1:
    ; Check if scanline is in range [y_start, y_end)
    lda RND_SCANLINE_Y
    cmp EDGE0_Y_START
    bcs @in_range0          ; scanline >= y_start
    rts                     ; Early exit: scanline < y_start
@in_range0:
    cmp EDGE0_Y_END
    bcc @process0            ; scanline < y_end
    rts                     ; Early exit: scanline >= y_end
@process0:

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
    ; 16-bit Bresenham step (mirrors proto stepEdge lines 499-518)
    ; 1. Apply base step unconditionally
    lda EDGE0_DX_SIGN
    beq @positive_step0
    ; Negative: X -= baseStep
    lda EDGE0_X_CURRENT
    sec
    sbc EDGE0_BASE_STEP
    sta EDGE0_X_CURRENT
    jmp @accumulate0
@positive_step0:
    ; Positive: X += baseStep
    lda EDGE0_X_CURRENT
    clc
    adc EDGE0_BASE_STEP
    sta EDGE0_X_CURRENT

@accumulate0:
    ; 2. Accumulate 16-bit remainder into 16-bit error
    lda EDGE0_ERR_LO
    clc
    adc EDGE0_REM_LO
    sta EDGE0_ERR_LO
    lda EDGE0_ERR_HI
    adc EDGE0_REM_HI
    sta EDGE0_ERR_HI

    ; 3. Compare 16-bit error with 16-bit dy_full
    lda EDGE0_ERR_LO
    cmp EDGE0_DY_LO
    lda EDGE0_ERR_HI
    sbc EDGE0_DY_HI
    bcs @extra_step0        ; error >= dy, apply extra step
    rts                     ; Early exit: error < dy

@extra_step0:
    ; error >= dy: subtract dy and apply extra ±1
    lda EDGE0_ERR_LO
    sec
    sbc EDGE0_DY_LO
    sta EDGE0_ERR_LO
    lda EDGE0_ERR_HI
    sbc EDGE0_DY_HI
    sta EDGE0_ERR_HI

    lda EDGE0_DX_SIGN
    beq @inc_x0
    dec EDGE0_X_CURRENT
    rts
@inc_x0:
    inc EDGE0_X_CURRENT
    rts

; Step edge 1 and check for intersection
step_edge_1_2:
    lda RND_SCANLINE_Y
    cmp EDGE1_Y_START
    bcs @in_range1
    rts
@in_range1:
    cmp EDGE1_Y_END
    bcc @process1
    rts
@process1:

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
    lda EDGE1_DX_SIGN
    beq @positive_step1
    lda EDGE1_X_CURRENT
    sec
    sbc EDGE1_BASE_STEP
    sta EDGE1_X_CURRENT
    jmp @accumulate1
@positive_step1:
    lda EDGE1_X_CURRENT
    clc
    adc EDGE1_BASE_STEP
    sta EDGE1_X_CURRENT

@accumulate1:
    lda EDGE1_ERR_LO
    clc
    adc EDGE1_REM_LO
    sta EDGE1_ERR_LO
    lda EDGE1_ERR_HI
    adc EDGE1_REM_HI
    sta EDGE1_ERR_HI

    lda EDGE1_ERR_LO
    cmp EDGE1_DY_LO
    lda EDGE1_ERR_HI
    sbc EDGE1_DY_HI
    bcs @extra_step1
    rts

@extra_step1:
    lda EDGE1_ERR_LO
    sec
    sbc EDGE1_DY_LO
    sta EDGE1_ERR_LO
    lda EDGE1_ERR_HI
    sbc EDGE1_DY_HI
    sta EDGE1_ERR_HI

    lda EDGE1_DX_SIGN
    beq @inc_x1
    dec EDGE1_X_CURRENT
    rts
@inc_x1:
    inc EDGE1_X_CURRENT
    rts

; Step edge 2 and check for intersection
step_edge_2_3:
    lda RND_SCANLINE_Y
    cmp EDGE2_Y_START
    bcs @in_range2
    rts
@in_range2:
    cmp EDGE2_Y_END
    bcc @process2
    rts
@process2:

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
    lda EDGE2_DX_SIGN
    beq @positive_step2
    lda EDGE2_X_CURRENT
    sec
    sbc EDGE2_BASE_STEP
    sta EDGE2_X_CURRENT
    jmp @accumulate2
@positive_step2:
    lda EDGE2_X_CURRENT
    clc
    adc EDGE2_BASE_STEP
    sta EDGE2_X_CURRENT

@accumulate2:
    lda EDGE2_ERR_LO
    clc
    adc EDGE2_REM_LO
    sta EDGE2_ERR_LO
    lda EDGE2_ERR_HI
    adc EDGE2_REM_HI
    sta EDGE2_ERR_HI

    lda EDGE2_ERR_LO
    cmp EDGE2_DY_LO
    lda EDGE2_ERR_HI
    sbc EDGE2_DY_HI
    bcs @extra_step2
    rts

@extra_step2:
    lda EDGE2_ERR_LO
    sec
    sbc EDGE2_DY_LO
    sta EDGE2_ERR_LO
    lda EDGE2_ERR_HI
    sbc EDGE2_DY_HI
    sta EDGE2_ERR_HI

    lda EDGE2_DX_SIGN
    beq @inc_x2
    dec EDGE2_X_CURRENT
    rts
@inc_x2:
    inc EDGE2_X_CURRENT
    rts

; Step edge 3 and check for intersection
step_edge_3_0:
    lda RND_SCANLINE_Y
    cmp EDGE3_Y_START
    bcs @in_range3
    rts
@in_range3:
    cmp EDGE3_Y_END
    bcc @process3
    rts
@process3:

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
    lda EDGE3_DX_SIGN
    beq @positive_step3
    lda EDGE3_X_CURRENT
    sec
    sbc EDGE3_BASE_STEP
    sta EDGE3_X_CURRENT
    jmp @accumulate3
@positive_step3:
    lda EDGE3_X_CURRENT
    clc
    adc EDGE3_BASE_STEP
    sta EDGE3_X_CURRENT

@accumulate3:
    lda EDGE3_ERR_LO
    clc
    adc EDGE3_REM_LO
    sta EDGE3_ERR_LO
    lda EDGE3_ERR_HI
    adc EDGE3_REM_HI
    sta EDGE3_ERR_HI

    lda EDGE3_ERR_LO
    cmp EDGE3_DY_LO
    lda EDGE3_ERR_HI
    sbc EDGE3_DY_HI
    bcs @extra_step3
    rts

@extra_step3:
    lda EDGE3_ERR_LO
    sec
    sbc EDGE3_DY_LO
    sta EDGE3_ERR_LO
    lda EDGE3_ERR_HI
    sbc EDGE3_DY_HI
    sta EDGE3_ERR_HI

    lda EDGE3_DX_SIGN
    beq @inc_x3
    dec EDGE3_X_CURRENT
    rts
@inc_x3:
    inc EDGE3_X_CURRENT
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

    ; Direct mask lookup from supersample coordinate.
    ; Replaces: outX = ssX>>1 (LSR, 2cyc), bitPos = outX&7 (AND, 2cyc), mask = bitMask[bitPos] (LDA, 4cyc)
    ; With: maskIdx = ssX&15 (AND, 2cyc), mask = maskBySSX[maskIdx] (LDA, 4cyc)
    ; Saves: 8 - 6 = 2 cycles per pixel with coverage.
    lda SPAN_PIXEL_X
    and #$0F
    tax
    lda mask_by_ssx,x       ; A = mask

    ; Column selection using supersample coordinate (thresholds doubled).
    cpy #3
    bcs @clear_both         ; count >= 3: clear both → (0,0) background/full

    ; count = 1-2: clear charset only → (0,1) sprite/medium
    ldx SPAN_PIXEL_X
    cpx #32
    bcs @col2_charset
    cpx #16
    bcs @col1_charset
    ; Column 0
    eor #$FF
    and RND_EDGE_X0
    sta RND_EDGE_X0
    jmp @next_x
@col1_charset:
    eor #$FF
    and RND_EDGE_X1
    sta RND_EDGE_X1
    jmp @next_x
@col2_charset:
    eor #$FF
    and RND_EDGE_Y0
    sta RND_EDGE_Y0
    jmp @next_x

@clear_both:
    ; count >= 3: clear both charset and sprite → (0,0)
    ldx SPAN_PIXEL_X
    cpx #32
    bcs @col2_both
    cpx #16
    bcs @col1_both
    ; Column 0 - clear both
    eor #$FF
    sta DOWNSAMPLE_X_MAX    ; Save inverted mask temporarily
    and RND_EDGE_X0
    sta RND_EDGE_X0
    lda DOWNSAMPLE_X_MAX
    and SPAN_BITMASK_COL0
    sta SPAN_BITMASK_COL0
    jmp @next_x
@col1_both:
    eor #$FF
    sta DOWNSAMPLE_X_MAX
    and RND_EDGE_X1
    sta RND_EDGE_X1
    lda DOWNSAMPLE_X_MAX
    and SPAN_BITMASK_COL1
    sta SPAN_BITMASK_COL1
    jmp @next_x
@col2_both:
    eor #$FF
    sta DOWNSAMPLE_X_MAX
    and RND_EDGE_Y0
    sta RND_EDGE_Y0
    lda DOWNSAMPLE_X_MAX
    and SPAN_BITMASK_COL2
    sta SPAN_BITMASK_COL2
    jmp @next_x

@next_x:
    ; Increment by 2 in supersample space.
    ; 6502: INX, INX (4 cycles) vs previous ASL + ADC (6 cycles) = 2 cycles saved.
    lda SPAN_PIXEL_X
    clc
    adc #2
    sta SPAN_PIXEL_X
    jmp @x_loop

@done_x_charset_only:
    ; Write scanline to PRECOMPUTED_FRAMES (empty rows: all $FF)
    ; Calculate address same as @done_x
    lda DOWNSAMPLE_Y
    sec
    sbc #1
    sta TEMP_ROW
    asl
    sta TEMP_OFFSET_LO
    lda TEMP_ROW
    asl
    asl
    clc
    adc TEMP_OFFSET_LO
    sta TEMP_OFFSET_LO

    ; Calculate frame offset: BUILD_SLOT * 128 (BUILD_SLOT << 7)
    ; Result: high byte = BUILD_SLOT >> 1, low byte = bit0 ? $80 : $00
    lda BUILD_SLOT
    lsr                      ; High byte = BUILD_SLOT >> 1, C = bit 0
    sta TEMP_OFFSET_HI
    lda #0
    ror                      ; Low byte = $00 or $80 based on carry
    sta TEMP_FRAME_LO

    lda TEMP_OFFSET_LO
    clc
    adc TEMP_FRAME_LO
    sta TEMP_OFFSET_LO
    lda #0
    adc TEMP_OFFSET_HI
    sta TEMP_OFFSET_HI

    lda TEMP_OFFSET_LO
    clc
    adc #<PRECOMPUTED_FRAMES
    sta TEMP_ADDR_LO
    lda TEMP_OFFSET_HI
    adc #>PRECOMPUTED_FRAMES
    sta TEMP_ADDR_HI

    ; Write 6 bytes of $FF using self-modifying code
    ; Set address for each store instruction (save original high byte)
    ldx TEMP_ADDR_HI         ; Save original high byte

    lda TEMP_ADDR_LO
    sta @store0+1
    clc
    adc #1
    sta @store1+1
    clc
    adc #1
    sta @store2+1
    clc
    adc #1
    sta @store3+1
    clc
    adc #1
    sta @store4+1
    clc
    adc #1
    sta @store5+1

    ; Now set high bytes with proper carry handling
    ; store0 high byte
    lda TEMP_ADDR_LO
    clc
    adc #0
    txa
    adc #0
    sta @store0+2

    ; store1 high byte
    lda TEMP_ADDR_LO
    clc
    adc #1
    txa
    adc #0
    sta @store1+2

    ; store2 high byte
    lda TEMP_ADDR_LO
    clc
    adc #2
    txa
    adc #0
    sta @store2+2

    ; store3 high byte
    lda TEMP_ADDR_LO
    clc
    adc #3
    txa
    adc #0
    sta @store3+2

    ; store4 high byte
    lda TEMP_ADDR_LO
    clc
    adc #4
    txa
    adc #0
    sta @store4+2

    ; store5 high byte
    lda TEMP_ADDR_LO
    clc
    adc #5
    txa
    adc #0
    sta @store5+2

    lda #$FF
@store0:
    sta $FFFF               ; SMC: byte 0
@store1:
    sta $FFFF               ; SMC: byte 1
@store2:
    sta $FFFF               ; SMC: byte 2
@store3:
    sta $FFFF               ; SMC: byte 3
@store4:
    sta $FFFF               ; SMC: byte 4
@store5:
    sta $FFFF               ; SMC: byte 5
    rts

@done_x:
    ; Write scanline to PRECOMPUTED_FRAMES in sequential 6-bytes-per-row format
    ; Address = PRECOMPUTED_FRAMES + (BUILD_SLOT * 128) + ((DOWNSAMPLE_Y - 1) * 6)

    ; Calculate row offset: (DOWNSAMPLE_Y - 1) * 6
    lda DOWNSAMPLE_Y
    sec
    sbc #1
    ; A = row - 1, now multiply by 6 = 4 + 2
    sta TEMP_ROW
    asl                      ; × 2
    sta TEMP_OFFSET_LO
    lda TEMP_ROW
    asl
    asl                      ; × 4
    clc
    adc TEMP_OFFSET_LO       ; × 4 + × 2 = × 6
    sta TEMP_OFFSET_LO

    ; Calculate frame offset: BUILD_SLOT * 128 (BUILD_SLOT << 7)
    ; Result: high byte = BUILD_SLOT >> 1, low byte = bit0 ? $80 : $00
    lda BUILD_SLOT
    lsr                      ; High byte = BUILD_SLOT >> 1, C = bit 0
    sta TEMP_OFFSET_HI
    lda #0
    ror                      ; Low byte = $00 or $80 based on carry
    sta TEMP_FRAME_LO

    ; Add base + frame_offset + row_offset
    lda TEMP_OFFSET_LO
    clc
    adc TEMP_FRAME_LO
    sta TEMP_OFFSET_LO
    lda #0
    adc TEMP_OFFSET_HI
    sta TEMP_OFFSET_HI

    lda TEMP_OFFSET_LO
    clc
    adc #<PRECOMPUTED_FRAMES
    sta TEMP_ADDR_LO
    lda TEMP_OFFSET_HI
    adc #>PRECOMPUTED_FRAMES
    sta TEMP_ADDR_HI

    ; Write 6 consecutive bytes using self-modifying code
    ; Set address for each write instruction (save original high byte)
    ldx TEMP_ADDR_HI         ; Save original high byte

    lda TEMP_ADDR_LO
    sta @write0+1
    clc
    adc #1
    sta @write1+1
    clc
    adc #1
    sta @write2+1
    clc
    adc #1
    sta @write3+1
    clc
    adc #1
    sta @write4+1
    clc
    adc #1
    sta @write5+1

    ; Now set high bytes with proper carry handling
    ; write0 high byte
    lda TEMP_ADDR_LO
    clc
    adc #0
    txa
    adc #0
    sta @write0+2

    ; write1 high byte
    lda TEMP_ADDR_LO
    clc
    adc #1
    txa
    adc #0
    sta @write1+2

    ; write2 high byte
    lda TEMP_ADDR_LO
    clc
    adc #2
    txa
    adc #0
    sta @write2+2

    ; write3 high byte
    lda TEMP_ADDR_LO
    clc
    adc #3
    txa
    adc #0
    sta @write3+2

    ; write4 high byte
    lda TEMP_ADDR_LO
    clc
    adc #4
    txa
    adc #0
    sta @write4+2

    ; write5 high byte
    lda TEMP_ADDR_LO
    clc
    adc #5
    txa
    adc #0
    sta @write5+2

    lda RND_EDGE_X0
@write0:
    sta $FFFF               ; SMC: col0 charset
    lda RND_EDGE_X1
@write1:
    sta $FFFF               ; SMC: col1 charset
    lda RND_EDGE_Y0
@write2:
    sta $FFFF               ; SMC: col2 charset
    lda SPAN_BITMASK_COL0
@write3:
    sta $FFFF               ; SMC: col0 sprite
    lda SPAN_BITMASK_COL1
@write4:
    sta $FFFF               ; SMC: col1 sprite
    lda SPAN_BITMASK_COL2
@write5:
    sta $FFFF               ; SMC: col2 sprite

    rts

TEMP_ROW: .res 1
TEMP_OFFSET_LO: .res 1
TEMP_OFFSET_HI: .res 1
TEMP_FRAME_LO: .res 1
TEMP_ADDR_LO: .res 1
TEMP_ADDR_HI: .res 1

; Clear charset pixel at current DOWNSAMPLE_X position (pixel inside square)
; Modifies RND_EDGE_X0/X1/Y0 (character bitmasks)
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
