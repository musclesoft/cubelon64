; Optimized render_square implementation
; Based on Go prototype optimizations:
; - Inverted rendering (clear bits for square, set for background)
; - Supersample coordinate loop
; - Inline span checks with wrapping detection
; - Direct mask lookup
; - Geometry bounds optimization (skip rows 0, 18-23)

.include "include/shared_imports.inc"
.include "include/render_private.inc"

; Constants
DISPLAY_WIDTH = 24
DISPLAY_HEIGHT = 24
SUPERSAMPLE_WIDTH = 48
SUPERSAMPLE_HEIGHT = 48

; Charset location
CHARSET_RAM = $2000
SQUARE_CHARS = CHARSET_RAM + (64 * 8)

; Sprite data
SPRITE_DATA = $2800
SPRITE0_DATA = SPRITE_DATA

.segment "CODE"

.export render_square
render_square:
    ; Clear sprite data (63 bytes)
    lda #$00
    ldx #0
@clear_sprite:
    sta SPRITE0_DATA,x
    inx
    cpx #63
    bne @clear_sprite

    ; Save and scale corners
    ldx #0
@save_corners:
    lda CORNER0_X,x
    sta CORNER_BACKUP,x
    inx
    cpx #16
    bne @save_corners

    jsr scale_corners_2x

    ; Initialize DDA edges
    jsr init_edge_0_1
    jsr init_edge_1_2
    jsr init_edge_2_3
    jsr init_edge_3_0

    ; Set row 0 to 0xFF (all lit, no square pixels)
    lda #$FF
    sta SQUARE_CHARS
    sta SQUARE_CHARS+64
    sta SQUARE_CHARS+128
    lda #$FF
    sta SPRITE0_DATA          ; Row 0 sprite data
    sta SPRITE0_DATA+1
    sta SPRITE0_DATA+2

    ; Process output rows 1-17 (skip 0, 18-23 - geometry optimization)
    lda #1
    sta DOWNSAMPLE_Y
@output_row_loop:
    ; Get span for supersample row 2Y
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
    ; Get span for supersample row 2Y+1
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
    jmp @check_empty
@no_span1:
    lda #0
    sta SPAN_ROW1_VALID

@check_empty:
    ; Early exit: if both spans invalid, write 0xFF and skip
    lda SPAN_ROW0_VALID
    ora SPAN_ROW1_VALID
    bne @downsample
    ldy DOWNSAMPLE_Y
    lda #$FF
    sta SQUARE_CHARS,y
    sta SQUARE_CHARS+64,y
    sta SQUARE_CHARS+128,y
    ; Sprite row offset = Y × 3
    tya
    asl
    clc
    adc DOWNSAMPLE_Y
    tay
    lda #$FF
    sta SPRITE0_DATA,y
    sta SPRITE0_DATA+1,y
    sta SPRITE0_DATA+2,y
    jmp @next_row

@downsample:
    jsr downsample_spans_optimized

@next_row:
    inc DOWNSAMPLE_Y
    lda DOWNSAMPLE_Y
    cmp #18              ; Process rows 1-17
    beq @done_rows
    jmp @output_row_loop

@done_rows:
    ; Set rows 18-23 to 0xFF (always empty)
    ldx #18
@clear_bottom:
    lda #$FF
    sta SQUARE_CHARS,x
    sta SQUARE_CHARS+64,x
    sta SQUARE_CHARS+128,x
    ; Sprite: row 18-20 only (rows 21-23 don't exist in sprite)
    cpx #21
    bcs @skip_sprite_bottom
    txa
    asl
    clc
    adc DOWNSAMPLE_X      ; Reuse temp var
    tay
    lda #$FF
    sta SPRITE0_DATA,y
    sta SPRITE0_DATA+1,y
    sta SPRITE0_DATA+2,y
@skip_sprite_bottom:
    inx
    cpx #24
    bne @clear_bottom

    ; Restore corners
    ldx #0
@restore_corners:
    lda CORNER_BACKUP,x
    sta CORNER0_X,x
    inx
    cpx #16
    bne @restore_corners

    rts

; Optimized downsample with inline span checks
downsample_spans_optimized:
    ; Initialize bitmasks to 0xFF (inverted: background lit)
    lda #$FF
    sta RND_EDGE_X0         ; Column 0 full
    sta RND_EDGE_X1         ; Column 1 full
    sta RND_EDGE_Y0         ; Column 2 full
    sta SPAN_BITMASK_COL0   ; Column 0 sprite
    sta SPAN_BITMASK_COL1   ; Column 1 sprite
    sta SPAN_BITMASK_COL2   ; Column 2 sprite

    ; Precompute span wrapping flags
    ; row0 wraps if x2 >= 48
    lda SPAN_ROW0_X2
    cmp #48
    bcc @no_wrap0
    lda #1
    sta SPAN_WRAP0
    lda SPAN_ROW0_X2
    sec
    sbc #48
    sta SPAN_ROW0_X2_WRAP
    jmp @check_wrap1
@no_wrap0:
    lda #0
    sta SPAN_WRAP0

@check_wrap1:
    lda SPAN_ROW1_X2
    cmp #48
    bcc @no_wrap1
    lda #1
    sta SPAN_WRAP1
    lda SPAN_ROW1_X2
    sec
    sbc #48
    sta SPAN_ROW1_X2_WRAP
    jmp @loop_setup
@no_wrap1:
    lda #0
    sta SPAN_WRAP1

@loop_setup:
    ; Loop in supersample space: ssX = 0, 2, 4, ..., 46
    lda #0
    sta SPAN_PIXEL_X        ; ssX

@x_loop:
    ; ssX1 = ssX + 1
    lda SPAN_PIXEL_X
    clc
    adc #1
    sta SPAN_PIXEL_X1

    ; Count subpixels using inline range checks
    lda #0
    sta SUBPIXEL_COUNT

    ; Check ssX in row0
    lda SPAN_ROW0_VALID
    beq @skip_ssX_row0
    lda SPAN_WRAP0
    bne @wrap_check_ssX_row0
    ; Normal check: ssX >= x1 && ssX < x2
    lda SPAN_PIXEL_X
    cmp SPAN_ROW0_X1
    bcc @skip_ssX_row0
    cmp SPAN_ROW0_X2
    bcs @skip_ssX_row0
    inc SUBPIXEL_COUNT
    jmp @check_ssX1_row0
@wrap_check_ssX_row0:
    ; Wrapped check: ssX >= x1 || ssX < x2-48
    lda SPAN_PIXEL_X
    cmp SPAN_ROW0_X1
    bcs @inside_ssX_row0
    cmp SPAN_ROW0_X2_WRAP
    bcs @skip_ssX_row0
@inside_ssX_row0:
    inc SUBPIXEL_COUNT
@skip_ssX_row0:

@check_ssX1_row0:
    ; Check ssX+1 in row0
    lda SPAN_ROW0_VALID
    beq @skip_ssX1_row0
    lda SPAN_WRAP0
    bne @wrap_check_ssX1_row0
    lda SPAN_PIXEL_X1
    cmp SPAN_ROW0_X1
    bcc @skip_ssX1_row0
    cmp SPAN_ROW0_X2
    bcs @skip_ssX1_row0
    inc SUBPIXEL_COUNT
    jmp @early_exit_check
@wrap_check_ssX1_row0:
    lda SPAN_PIXEL_X1
    cmp SPAN_ROW0_X1
    bcs @inside_ssX1_row0
    cmp SPAN_ROW0_X2_WRAP
    bcs @skip_ssX1_row0
@inside_ssX1_row0:
    inc SUBPIXEL_COUNT
@skip_ssX1_row0:

@early_exit_check:
    ; Early exit: if count >= 3, skip remaining checks
    lda SUBPIXEL_COUNT
    cmp #3
    bcs @apply_threshold

    ; Check ssX in row1
    lda SPAN_ROW1_VALID
    beq @skip_ssX_row1
    lda SPAN_WRAP1
    bne @wrap_check_ssX_row1
    lda SPAN_PIXEL_X
    cmp SPAN_ROW1_X1
    bcc @skip_ssX_row1
    cmp SPAN_ROW1_X2
    bcs @skip_ssX_row1
    inc SUBPIXEL_COUNT
    jmp @check_ssX1_row1
@wrap_check_ssX_row1:
    lda SPAN_PIXEL_X
    cmp SPAN_ROW1_X1
    bcs @inside_ssX_row1
    cmp SPAN_ROW1_X2_WRAP
    bcs @skip_ssX_row1
@inside_ssX_row1:
    inc SUBPIXEL_COUNT
@skip_ssX_row1:

@check_ssX1_row1:
    ; Early exit before last check
    lda SUBPIXEL_COUNT
    cmp #3
    bcs @apply_threshold

    ; Check ssX+1 in row1
    lda SPAN_ROW1_VALID
    beq @skip_ssX1_row1
    lda SPAN_WRAP1
    bne @wrap_check_ssX1_row1
    lda SPAN_PIXEL_X1
    cmp SPAN_ROW1_X1
    bcc @skip_ssX1_row1
    cmp SPAN_ROW1_X2
    bcs @skip_ssX1_row1
    inc SUBPIXEL_COUNT
    jmp @apply_threshold
@wrap_check_ssX1_row1:
    lda SPAN_PIXEL_X1
    cmp SPAN_ROW1_X1
    bcs @inside_ssX1_row1
    cmp SPAN_ROW1_X2_WRAP
    bcs @skip_ssX1_row1
@inside_ssX1_row1:
    inc SUBPIXEL_COUNT
@skip_ssX1_row1:

@apply_threshold:
    ; If count == 0, leave bits set (background)
    lda SUBPIXEL_COUNT
    beq @next_ssX

    ; Get mask directly from ssX (AND #15)
    lda SPAN_PIXEL_X
    and #$0F
    tax
    lda mask_by_ssx,x

    ; count >= 3: full brightness (clear full bit)
    ldx SUBPIXEL_COUNT
    cpx #3
    bcc @medium_bright

@full_bright:
    ; Invert mask for clearing
    eor #$FF
    sta SPAN_TEMP          ; Save inverted mask

    ; Column selection using ssX
    lda SPAN_PIXEL_X
    cmp #16
    bcc @check_col0_full
    cmp #32
    bcc @col1_full
    ; Column 2
    lda RND_EDGE_Y0
    and SPAN_TEMP
    sta RND_EDGE_Y0
    jmp @next_ssX
@col1_full:
    lda RND_EDGE_X1
    and SPAN_TEMP
    sta RND_EDGE_X1
    jmp @next_ssX
@check_col0_full:
    lda RND_EDGE_X0
    and SPAN_TEMP
    sta RND_EDGE_X0
    jmp @next_ssX

@medium_bright:
    ; count 1-2: medium brightness (clear sprite bit)
    eor #$FF
    sta SPAN_TEMP

    lda SPAN_PIXEL_X
    cmp #16
    bcc @check_col0_med
    cmp #32
    bcc @col1_med
    ; Column 2
    lda SPAN_BITMASK_COL2
    and SPAN_TEMP
    sta SPAN_BITMASK_COL2
    jmp @next_ssX
@col1_med:
    lda SPAN_BITMASK_COL1
    and SPAN_TEMP
    sta SPAN_BITMASK_COL1
    jmp @next_ssX
@check_col0_med:
    lda SPAN_BITMASK_COL0
    and SPAN_TEMP
    sta SPAN_BITMASK_COL0

@next_ssX:
    ; ssX += 2
    lda SPAN_PIXEL_X
    clc
    adc #2
    sta SPAN_PIXEL_X
    cmp #48
    beq @done_x
    jmp @x_loop

@done_x:
    ; Write scanline
    ldy DOWNSAMPLE_Y
    lda RND_EDGE_X0
    sta SQUARE_CHARS,y
    lda RND_EDGE_X1
    sta SQUARE_CHARS+64,y
    lda RND_EDGE_Y0
    sta SQUARE_CHARS+128,y

    ; Write sprite (rows 0-20 only)
    lda DOWNSAMPLE_Y
    cmp #21
    bcs @skip_sprite_write
    tay
    asl
    clc
    adc DOWNSAMPLE_Y
    tay
    lda SPAN_BITMASK_COL0
    sta SPRITE0_DATA,y
    lda SPAN_BITMASK_COL1
    sta SPRITE0_DATA+1,y
    lda SPAN_BITMASK_COL2
    sta SPRITE0_DATA+2,y

@skip_sprite_write:
    rts

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
.segment "RODATA"
; Direct mask lookup from supersample coordinate (ssX & 15)
mask_by_ssx:
    .byte $80, $80  ; ssX=0,1 → bit 0
    .byte $40, $40  ; ssX=2,3 → bit 1
    .byte $20, $20  ; ssX=4,5 → bit 2
    .byte $10, $10  ; ssX=6,7 → bit 3
    .byte $08, $08  ; ssX=8,9 → bit 4
    .byte $04, $04  ; ssX=10,11 → bit 5
    .byte $02, $02  ; ssX=12,13 → bit 6
    .byte $01, $01  ; ssX=14,15 → bit 7

; Original bit mask table (for compatibility)
bit_mask_table:
    .byte $80, $40, $20, $10, $08, $04, $02, $01
