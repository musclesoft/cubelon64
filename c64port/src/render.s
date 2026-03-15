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
.import mul16x8: absolute
.import div24x16: absolute

; Render a filled square using scanline algorithm with DDA edge walking
; Input: corner0-corner3 coordinates in Fixed88 format (calculated by calc_rotation)
; Output: Character data (full brightness) + Sprite data (medium brightness)
.export render_square
render_square:
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
    jsr compute_edge_division_01
    jsr compute_edge_division_12

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
; Edge Division Computation (for parallel edge optimization)
; ═══════════════════════════════════════════════════════════

; compute_edge_division_01: Compute division for edge corner0→corner1
; Stores result in DIV_RESULT_01_*
; Mirrors computeEdgeDivision from proto/main.go lines 320-352
compute_edge_division_01:
    ; Use corner0 as (x0Lo, x0Hi, y0Lo, y0Hi)
    ; Use corner1 as (x1Lo, x1Hi, y1Lo, y1Hi)

    ; Compare y0_full vs y1_full (16-bit) to determine if we need to swap
    ; y0Full = (CORNER0_Y+1 << 8) | CORNER0_Y
    ; y1Full = (CORNER1_Y+1 << 8) | CORNER1_Y
    lda CORNER0_Y
    cmp CORNER1_Y
    lda CORNER0_Y+1
    sbc CORNER1_Y+1
    bcc @no_swap_01         ; y0 < y1, no swap needed
    bne @do_swap_01         ; y0 > y1, swap
    ; y0Hi == y1Hi, check lo bytes
    lda CORNER0_Y
    cmp CORNER1_Y
    beq @no_swap_01         ; Equal, no swap
    bcs @do_swap_01         ; y0 > y1, swap

@no_swap_01:
    ; Use corner0 → corner1 as-is
    ; dy_full = y1_full - y0_full
    lda CORNER1_Y
    sec
    sbc CORNER0_Y
    sta DIV_RESULT_01_DY_LO
    lda CORNER1_Y+1
    sbc CORNER0_Y+1
    sta DIV_RESULT_01_DY_HI

    ; dx_full magnitude = |x1_full - x0_full|
    lda CORNER1_X
    sec
    sbc CORNER0_X
    sta MATH_DIV_LO
    lda CORNER1_X+1
    sbc CORNER0_X+1
    sta MATH_DIV_HI
    bcs @positive_dx_01
    ; Negative: negate to get magnitude
    lda #0
    sec
    sbc MATH_DIV_LO
    sta MATH_DIV_LO
    lda #0
    sbc MATH_DIV_HI
    sta MATH_DIV_HI
@positive_dx_01:
    jmp @do_division_01

@do_swap_01:
    ; Swap: use corner1 → corner0
    ; dy_full = y0_full - y1_full
    lda CORNER0_Y
    sec
    sbc CORNER1_Y
    sta DIV_RESULT_01_DY_LO
    lda CORNER0_Y+1
    sbc CORNER1_Y+1
    sta DIV_RESULT_01_DY_HI

    ; dx_full magnitude = |x0_full - x1_full|
    lda CORNER0_X
    sec
    sbc CORNER1_X
    sta MATH_DIV_LO
    lda CORNER0_X+1
    sbc CORNER1_X+1
    sta MATH_DIV_HI
    bcs @positive_dx_swap_01
    ; Negative: negate
    lda #0
    sec
    sbc MATH_DIV_LO
    sta MATH_DIV_LO
    lda #0
    sbc MATH_DIV_HI
    sta MATH_DIV_HI
@positive_dx_swap_01:

@do_division_01:
    ; Call div16x16: dx_full / dy_full → quotient + remainder
    lda DIV_RESULT_01_DY_LO
    sta MATH_DIV2_LO
    lda DIV_RESULT_01_DY_HI
    sta MATH_DIV2_HI
    jsr div16x16
    ; Result: MATH_QUOT_LO = baseStep, MATH_REM_LO/HI = remainder
    lda MATH_QUOT_LO
    sta DIV_RESULT_01_BASE
    lda MATH_REM_LO
    sta DIV_RESULT_01_REM_LO
    lda MATH_REM_HI
    sta DIV_RESULT_01_REM_HI
    rts

; compute_edge_division_12: Compute division for edge corner1→corner2
; Stores result in DIV_RESULT_12_*
compute_edge_division_12:
    ; Compare y1 vs y2
    lda CORNER1_Y
    cmp CORNER2_Y
    lda CORNER1_Y+1
    sbc CORNER2_Y+1
    bcc @no_swap_12
    bne @do_swap_12
    lda CORNER1_Y
    cmp CORNER2_Y
    beq @no_swap_12
    bcs @do_swap_12

@no_swap_12:
    ; dy = y2 - y1
    lda CORNER2_Y
    sec
    sbc CORNER1_Y
    sta DIV_RESULT_12_DY_LO
    lda CORNER2_Y+1
    sbc CORNER1_Y+1
    sta DIV_RESULT_12_DY_HI

    ; dx magnitude = |x2 - x1|
    lda CORNER2_X
    sec
    sbc CORNER1_X
    sta MATH_DIV_LO
    lda CORNER2_X+1
    sbc CORNER1_X+1
    sta MATH_DIV_HI
    bcs @positive_dx_12
    lda #0
    sec
    sbc MATH_DIV_LO
    sta MATH_DIV_LO
    lda #0
    sbc MATH_DIV_HI
    sta MATH_DIV_HI
@positive_dx_12:
    jmp @do_division_12

@do_swap_12:
    ; dy = y1 - y2
    lda CORNER1_Y
    sec
    sbc CORNER2_Y
    sta DIV_RESULT_12_DY_LO
    lda CORNER1_Y+1
    sbc CORNER2_Y+1
    sta DIV_RESULT_12_DY_HI

    ; dx magnitude = |x1 - x2|
    lda CORNER1_X
    sec
    sbc CORNER2_X
    sta MATH_DIV_LO
    lda CORNER1_X+1
    sbc CORNER2_X+1
    sta MATH_DIV_HI
    bcs @positive_dx_swap_12
    lda #0
    sec
    sbc MATH_DIV_LO
    sta MATH_DIV_LO
    lda #0
    sbc MATH_DIV_HI
    sta MATH_DIV_HI
@positive_dx_swap_12:

@do_division_12:
    lda DIV_RESULT_12_DY_LO
    sta MATH_DIV2_LO
    lda DIV_RESULT_12_DY_HI
    sta MATH_DIV2_HI
    jsr div16x16
    lda MATH_QUOT_LO
    sta DIV_RESULT_12_BASE
    lda MATH_REM_LO
    sta DIV_RESULT_12_REM_LO
    lda MATH_REM_HI
    sta DIV_RESULT_12_REM_HI
    rts

; ═══════════════════════════════════════════════════════════
; DDA Edge Initialization Routines
; ═══════════════════════════════════════════════════════════

; Initialize edge 0 (corner0 -> corner1) using DIV_RESULT_01
init_edge_0_1:
    ldx #0              ; Edge index
    lda #0              ; Use DIV_RESULT_01
    jmp init_edge_with_div

; Initialize edge 1 (corner1 -> corner2) using DIV_RESULT_12
init_edge_1_2:
    ldx #1              ; Edge index
    lda #1              ; Use DIV_RESULT_12
    jmp init_edge_with_div

; Initialize edge 2 (corner2 -> corner3) using DIV_RESULT_01
init_edge_2_3:
    ldx #2              ; Edge index
    lda #0              ; Use DIV_RESULT_01
    jmp init_edge_with_div

; Initialize edge 3 (corner3 -> corner0) using DIV_RESULT_12
init_edge_3_0:
    ldx #3              ; Edge index
    lda #1              ; Use DIV_RESULT_12
    ; Fall through to init_edge_with_div

; init_edge_with_div: Initialize edge with pre-computed division result
; Input: X = edge index (0-3)
;        A = division result selector (0 = DIV_RESULT_01, 1 = DIV_RESULT_12)
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

    ; Load pre-computed division result based on selector
    lda INIT_EDGE_DXSIGN     ; Division selector (0 or 1)
    bne @load_div12
    jmp @load_div01
@load_div12:
    ; Load DIV_RESULT_12 and save to dedicated variables
    ; (MUL8_RESULT/DIV16_DIVIDEND will be clobbered by subpixel correction math)
    lda DIV_RESULT_12_BASE
    sta SAVED_BASE_STEP
    lda DIV_RESULT_12_REM_LO
    sta SAVED_REM_LO
    lda DIV_RESULT_12_REM_HI
    sta SAVED_REM_HI
    lda DIV_RESULT_12_DY_LO
    sta SAVED_DY_LO
    lda DIV_RESULT_12_DY_HI
    sta SAVED_DY_HI
    jmp @div_loaded
@load_div01:
    lda DIV_RESULT_01_BASE
    sta SAVED_BASE_STEP
    lda DIV_RESULT_01_REM_LO
    sta SAVED_REM_LO
    lda DIV_RESULT_01_REM_HI
    sta SAVED_REM_HI
    lda DIV_RESULT_01_DY_LO
    sta SAVED_DY_LO
    lda DIV_RESULT_01_DY_HI
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
downsample_spans:
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
    ; Clamp to 24
    cmp #24
    bcc @bounds_ok
    lda #24
@bounds_ok:
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

    ; Check ssX in row0
    lda SPAN_PIXEL_X
    cmp SPAN_ROW0_X1
    bcc @check_r0_ssX1
    cmp SPAN_ROW0_X2
    bcs @check_r0_ssX1
    iny                     ; count++
@check_r0_ssX1:

    ; Check ssX+1 in row0
    lda SPAN_PIXEL_X
    clc
    adc #1
    cmp SPAN_ROW0_X1
    bcc @check_row1
    cmp SPAN_ROW0_X2
    bcs @check_row1
    iny                     ; count++

@check_row1:
    ; Check subpixels in row1 if valid (one validity check, with early exit at count>=3)
    lda SPAN_ROW1_VALID
    beq @apply_threshold    ; Skip row1 if invalid

    ; Check ssX in row1
    lda SPAN_PIXEL_X
    cmp SPAN_ROW1_X1
    bcc @check_r1_ssX1
    cmp SPAN_ROW1_X2
    bcs @check_r1_ssX1
    iny                     ; count++
@check_r1_ssX1:

    ; Early exit: if count >= 3, skip 4th subpixel check
    cpy #3
    bcs @apply_threshold

    ; Check ssX+1 in row1
    lda SPAN_PIXEL_X
    clc
    adc #1
    cmp SPAN_ROW1_X1
    bcc @apply_threshold
    cmp SPAN_ROW1_X2
    bcs @apply_threshold
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
    bcs @clear_both         ; count >= 3: clear both → $f background

    ; count = 1-2: clear charset only → $b sprite
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
    ; count >= 3: clear both charset and sprite → $f background
    ldx SPAN_PIXEL_X
    cpx #32
    bcs @col2_both
    cpx #16
    bcs @col1_both
    ; Column 0
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
    ; Write scanline to character data only (for empty rows)
    ; Saves 12 cycles (3 STA) by skipping sprite data writes
    ; For frames >= 8, rotate positions: col0→pos1, col1→pos2, col2→pos0
    ldy DOWNSAMPLE_Y
    lda USE_OFFSET
    beq @write_normal_charset_only
    ; Rotated write
    lda RND_EDGE_X0
    sta SQUARE_CHARS+64,y    ; col0 → pos 1
    lda RND_EDGE_X1
    sta SQUARE_CHARS+128,y   ; col1 → pos 2
    lda RND_EDGE_Y0
    sta SQUARE_CHARS,y       ; col2 → pos 0 (wrap)
    rts
@write_normal_charset_only:
    lda RND_EDGE_X0
    sta SQUARE_CHARS,y
    lda RND_EDGE_X1
    sta SQUARE_CHARS+64,y
    lda RND_EDGE_Y0
    sta SQUARE_CHARS+128,y
    rts

@done_x:
    ; Write scanline to character data (full brightness)
    ; For frames >= 8, rotate positions: col0→pos1, col1→pos2, col2→pos0
    ldy DOWNSAMPLE_Y
    lda USE_OFFSET
    beq @write_normal
    ; Rotated write
    lda RND_EDGE_X0
    sta SQUARE_CHARS+64,y    ; col0 → pos 1
    lda RND_EDGE_X1
    sta SQUARE_CHARS+128,y   ; col1 → pos 2
    lda RND_EDGE_Y0
    sta SQUARE_CHARS,y       ; col2 → pos 0 (wrap)

    ; Write scanline to sprite data (medium brightness)
    lda DOWNSAMPLE_Y
    tay
    asl                      ; Y × 2
    clc
    adc DOWNSAMPLE_Y         ; Y × 2 + Y = Y × 3
    tay
    lda SPAN_BITMASK_COL0
    sta SPRITE0_DATA+1,y     ; col0 → pos 1
    lda SPAN_BITMASK_COL1
    sta SPRITE0_DATA+2,y     ; col1 → pos 2
    lda SPAN_BITMASK_COL2
    sta SPRITE0_DATA,y       ; col2 → pos 0 (wrap)
    rts

@write_normal:
    lda RND_EDGE_X0
    sta SQUARE_CHARS,y
    lda RND_EDGE_X1
    sta SQUARE_CHARS+64,y
    lda RND_EDGE_Y0
    sta SQUARE_CHARS+128,y

    ; Write scanline to sprite data (medium brightness)
    ; Sprite row offset = Y × 3
    lda DOWNSAMPLE_Y
    tay
    asl                      ; Y × 2
    clc
    adc DOWNSAMPLE_Y         ; Y × 2 + Y = Y × 3
    tay
    lda SPAN_BITMASK_COL0
    sta SPRITE0_DATA,y
    lda SPAN_BITMASK_COL1
    sta SPRITE0_DATA+1,y
    lda SPAN_BITMASK_COL2
    sta SPRITE0_DATA+2,y

    rts

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
