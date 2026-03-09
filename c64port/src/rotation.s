; Rotation and rendering for C64
; Implements fixed-point 8.8 rotation with lookup tables

.include "zeropage.inc"

; Constants
SQUARE_SIZE = 6        ; Half-size of square in pixels (12x12 total)

.segment "RODATA"

; Sine/Cosine lookup tables (256 entries, signed -128 to +127)
; sin(angle) where angle = 0..255 maps to 0..2π
sine_table:
    .byte $00, $03, $06, $09, $0C, $0F, $12, $15
    .byte $18, $1B, $1E, $21, $24, $27, $2A, $2D
    .byte $30, $33, $36, $39, $3C, $3F, $42, $45
    .byte $48, $4B, $4E, $51, $54, $57, $5A, $5D
    .byte $60, $63, $65, $68, $6B, $6E, $70, $73
    .byte $76, $78, $7B, $7D, $7F, $82, $84, $86
    .byte $88, $8A, $8C, $8E, $90, $91, $93, $94
    .byte $96, $97, $98, $99, $9A, $9B, $9C, $9D
    .byte $9D, $9E, $9E, $9E, $9F, $9F, $9F, $9F
    .byte $9F, $9F, $9F, $9F, $9E, $9E, $9E, $9D
    .byte $9D, $9C, $9B, $9A, $99, $98, $97, $96
    .byte $94, $93, $91, $90, $8E, $8C, $8A, $88
    .byte $86, $84, $82, $7F, $7D, $7B, $78, $76
    .byte $73, $70, $6E, $6B, $68, $65, $63, $60
    .byte $5D, $5A, $57, $54, $51, $4E, $4B, $48
    .byte $45, $42, $3F, $3C, $39, $36, $33, $30
    .byte $2D, $2A, $27, $24, $21, $1E, $1B, $18
    .byte $15, $12, $0F, $0C, $09, $06, $03, $00
    .byte $FD, $FA, $F7, $F4, $F1, $EE, $EB, $E8
    .byte $E5, $E2, $DF, $DC, $D9, $D6, $D3, $D0
    .byte $CD, $CA, $C7, $C4, $C1, $BE, $BB, $B8
    .byte $B5, $B2, $AF, $AC, $A9, $A6, $A3, $A0
    .byte $9D, $9A, $98, $95, $92, $8F, $8D, $8A
    .byte $87, $85, $82, $80, $7E, $7B, $79, $77
    .byte $75, $73, $71, $6F, $6D, $6C, $6A, $69
    .byte $67, $66, $65, $64, $63, $62, $61, $60
    .byte $60, $5F, $5F, $5F, $5E, $5E, $5E, $5E
    .byte $5E, $5E, $5E, $5E, $5F, $5F, $5F, $60
    .byte $60, $61, $62, $63, $64, $65, $66, $67
    .byte $69, $6A, $6C, $6D, $6F, $71, $73, $75
    .byte $77, $79, $7B, $7E, $80, $82, $85, $87
    .byte $8A, $8D, $8F, $92, $95, $98, $9A, $9D

; Cosine is just sine shifted by 64 (90 degrees)
; cos(x) = sin(x + 64)

.segment "CODE"

; Calculate rotated square corners
; Uses the angle stored in zero page
; Outputs corner coordinates in Fixed88 format
.export calc_rotation
calc_rotation:
    ; Get sin/cos values for current angle
    ldx angle
    lda sine_table,x
    sta sin_val

    ; cos = sin(angle + 64)
    txa
    clc
    adc #64
    tax
    lda sine_table,x
    sta cos_val

    ; Calculate the 4 corners of the square
    ; Square centered at (12, 12) in pixels = (12*256, 12*256) in Fixed88
    ; Corners relative to center: (-6,-6), (+6,-6), (+6,+6), (-6,+6)

    ; Corner 0: (-SQUARE_SIZE, -SQUARE_SIZE)
    lda #<(-SQUARE_SIZE)
    sta temp1
    lda #>(-SQUARE_SIZE)
    sta temp1+1
    lda #<(-SQUARE_SIZE)
    sta temp2
    lda #>(-SQUARE_SIZE)
    sta temp2+1
    jsr rotate_point
    ; Add center offset (12*256 = $0C00)
    lda corner0_x
    clc
    adc #$00
    sta corner0_x
    lda corner0_x+1
    adc #$0C
    sta corner0_x+1
    lda corner0_y
    clc
    adc #$00
    sta corner0_y
    lda corner0_y+1
    adc #$0C
    sta corner0_y+1

    ; Corner 1: (+SQUARE_SIZE, -SQUARE_SIZE)
    lda #<SQUARE_SIZE
    sta temp1
    lda #>SQUARE_SIZE
    sta temp1+1
    lda #<(-SQUARE_SIZE)
    sta temp2
    lda #>(-SQUARE_SIZE)
    sta temp2+1
    jsr rotate_point
    lda corner1_x
    clc
    adc #$00
    sta corner1_x
    lda corner1_x+1
    adc #$0C
    sta corner1_x+1
    lda corner1_y
    clc
    adc #$00
    sta corner1_y
    lda corner1_y+1
    adc #$0C
    sta corner1_y+1

    ; Corner 2: (+SQUARE_SIZE, +SQUARE_SIZE)
    lda #<SQUARE_SIZE
    sta temp1
    lda #>SQUARE_SIZE
    sta temp1+1
    lda #<SQUARE_SIZE
    sta temp2
    lda #>SQUARE_SIZE
    sta temp2+1
    jsr rotate_point
    lda corner2_x
    clc
    adc #$00
    sta corner2_x
    lda corner2_x+1
    adc #$0C
    sta corner2_x+1
    lda corner2_y
    clc
    adc #$00
    sta corner2_y
    lda corner2_y+1
    adc #$0C
    sta corner2_y+1

    ; Corner 3: (-SQUARE_SIZE, +SQUARE_SIZE)
    lda #<(-SQUARE_SIZE)
    sta temp1
    lda #>(-SQUARE_SIZE)
    sta temp1+1
    lda #<SQUARE_SIZE
    sta temp2
    lda #>SQUARE_SIZE
    sta temp2+1
    jsr rotate_point
    lda corner3_x
    clc
    adc #$00
    sta corner3_x
    lda corner3_x+1
    adc #$0C
    sta corner3_x+1
    lda corner3_y
    clc
    adc #$00
    sta corner3_y
    lda corner3_y+1
    adc #$0C
    sta corner3_y+1

    rts

; Rotate a point (temp1, temp2) by angle
; Result stored in corner0 temporarily (will be moved to proper corner)
rotate_point:
    ; x' = x * cos - y * sin
    ; y' = x * sin + y * cos

    ; For now, simplified: just copy the input (TODO: implement full rotation)
    ; This is a placeholder - full fixed-point multiplication needed
    lda temp1
    sta corner0_x
    lda temp1+1
    sta corner0_x+1
    lda temp2
    sta corner0_y
    lda temp2+1
    sta corner0_y+1

    rts

; Render the rotated square to character data
; Uses a 3x3 character area (24x24 pixels) starting at character position (0,0)
.export render_square
render_square:
    ; For now, just draw a simple test pattern
    ; TODO: Implement scanline rendering algorithm

    ; Just return for now
    rts
