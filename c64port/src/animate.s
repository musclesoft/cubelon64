; Animation playback with 4-phase copy optimization
; Uses self-modifying code with get_next_byte subroutine for clean indexed access
;
; Memory layout:
;   - Precomputed frames: 11 frames × 128 bytes (102 used: 17 rows × 6 bytes, 26 padding)
;   - Bit reversal table: 256 bytes for horizontal mirroring
;
; 4-phase playback (creates 41 frames from 11 precomputed base frames):
;   Phase 1 (frames  0-10): src=frame,    rotate right 4px to undo -4px storage
;   Phase 2 (frames 11-20): src=20-frame, H-mirror + rotate right 8px (byte reorder)
;   Phase 3 (frames 21-30): src=frame-20, rotate left 8px (byte reorder)
;   Phase 4 (frames 31-40): src=40-frame, H-mirror + rotate left 4px
; Source frames alternate: forward, reverse, forward, reverse for smooth animation

.include "include/shared_imports.inc"

; Constants
CHARSET_RAM = $6000          ; VIC bank 1 + $2000
SPRITE_DATA = $6800          ; VIC bank 1 + $2800

.segment "BSS"

; Precomputed frame storage: 11 frames × 128 bytes
; Aligned to page boundary so each frame stays within one page
.align 256
.export PRECOMPUTED_FRAMES
PRECOMPUTED_FRAMES: .res 11 * 128

; Temporary storage
TEMP_COL0: .res 1
TEMP_COL1: .res 1
TEMP_COL2: .res 1
TEMP_MED0: .res 1
TEMP_MED1: .res 1
TEMP_MED2: .res 1
SRC_FRAME: .res 1

.segment "CODE"

; External functions
.import calc_rotation: absolute
.import render_square: absolute

; Export animation functions
.export precompute_frames
.export copy_frame_to_charset
.export test_checkpoint_frame
.export test_after_calc_rotation
.export test_after_render


; ─── Precompute frames 0-10 (11 frames for 41-frame animation) ──────────────
.proc precompute_frames
    lda #0
    sta FRAME
frame_loop:
    jsr calc_rotation       ; Compute corners before rendering
::test_after_calc_rotation:
    jsr render_square       ; Writes directly to PRECOMPUTED_FRAMES
::test_after_render:
    nop                     ; Separate address
::test_checkpoint_frame:
    nop
    assert_if FRAME, 0, PRECOMPUTED_FRAMES, "../proto/golden/precomputed_frames.bin", 128
    assert_if FRAME, 1, PRECOMPUTED_FRAMES + 128, "../proto/golden/precomputed_frames.bin" + 128, 128
    assert_if FRAME, 2, PRECOMPUTED_FRAMES + 256, "../proto/golden/precomputed_frames.bin" + 256, 128
    assert_if FRAME, 3, PRECOMPUTED_FRAMES + 384, "../proto/golden/precomputed_frames.bin" + 384, 128
    assert_if FRAME, 4, PRECOMPUTED_FRAMES + 512, "../proto/golden/precomputed_frames.bin" + 512, 128
    assert_if FRAME, 5, PRECOMPUTED_FRAMES + 640, "../proto/golden/precomputed_frames.bin" + 640, 128
    assert_if FRAME, 6, PRECOMPUTED_FRAMES + 768, "../proto/golden/precomputed_frames.bin" + 768, 128
    assert_if FRAME, 7, PRECOMPUTED_FRAMES + 896, "../proto/golden/precomputed_frames.bin" + 896, 128
    assert_if FRAME, 8, PRECOMPUTED_FRAMES + 1024, "../proto/golden/precomputed_frames.bin" + 1024, 128
    assert_if FRAME, 9, PRECOMPUTED_FRAMES + 1152, "../proto/golden/precomputed_frames.bin" + 1152, 128
    assert_if FRAME, 10, PRECOMPUTED_FRAMES + 1280, "../proto/golden/precomputed_frames.bin" + 1280, 128
    inc FRAME
    lda FRAME
    cmp #11
    bcc frame_loop
@frames_done:

    lda #0
    sta FRAME
    rts
.endproc

; ─── Copy and transform frame to charset ─────────────────────────────────────
.proc copy_frame_to_charset
    ; Initialize charset backgrounds (no clear needed - we write all bytes)
    ; Rows 1-17 write to scanlines 1-17 (row 0 blank, rows 18-23 blank)
    lda #$FF
    sta CHARSET_RAM         ; Row 0 blank (top margin)
    sta CHARSET_RAM+64
    sta CHARSET_RAM+128
    ldx #18
@init_bg:
    sta CHARSET_RAM,x       ; Rows 18-23 blank (bottom margin)
    sta CHARSET_RAM+64,x
    sta CHARSET_RAM+128,x
    inx
    cpx #24
    bne @init_bg

    ; Initialize sprite top and bottom margins
    ; Row 0: FF (blank)
    ; Rows 1-17: written by phase copy (from charset rows 1-17)
    ; Rows 18-20: FF (blank)
    lda #$FF
    sta SPRITE_DATA         ; Row 0, byte 0
    sta SPRITE_DATA+1       ; Row 0, byte 1
    sta SPRITE_DATA+2       ; Row 0, byte 2
    ldx #54                 ; Start at row 18 (byte 54)
@init_sprite_bottom:
    sta SPRITE_DATA,x       ; Rows 18-20 (bytes 54-62) = FF
    inx
    cpx #63
    bne @init_sprite_bottom

    ; Determine phase and source frame
    lda FRAME
    cmp #11
    bcc @phase1
    cmp #21
    bcc @phase2
    cmp #31
    bcc @phase3
    jmp @phase4

@phase1:
    ; Source = FRAME
    sta SRC_FRAME
    jmp @setup_and_run

@phase2:
    ; Source = 20 - FRAME
    lda #20
    sec
    sbc FRAME
    sta SRC_FRAME
    jmp @setup_and_run

@phase3:
    ; Source = FRAME - 20
    lda FRAME
    sec
    sbc #20
    sta SRC_FRAME
    jmp @setup_and_run

@phase4:
    ; Source = 40 - FRAME
    lda #40
    sec
    sbc FRAME
    sta SRC_FRAME
    ; Fall through to setup

@setup_and_run:
    ; Calculate base address: PRECOMPUTED_FRAMES + (SRC_FRAME × 128)
    ; frame × 128: put frame in high byte, shift right 1 bit (carry → MSB of low byte)
    lda SRC_FRAME
    lsr                     ; Shift right in A, bit 0 → carry
    tax                     ; Save shifted high byte in X
    lda #0
    ror                     ; Rotate carry into A, bit 0→carry (always 0)
    adc #<PRECOMPUTED_FRAMES ; Carry already clear from ror #0
    sta @push_addr+1
    txa                     ; Restore shifted high byte
    adc #>PRECOMPUTED_FRAMES
    sta @push_addr+2

    ; Push all 102 bytes (17 rows × 6 bytes) onto stack in reverse order
    ; This allows fast PLA access (4 cycles vs 22 cycles for JSR get_next_byte)
    ldy #101                ; Start from last byte (102 bytes: 0-101)
@push_loop:
@push_addr:
    lda $FFFF,y             ; SMC: base address set above
    pha
    dey
    bpl @push_loop          ; Branch while Y >= 0

    ; Initialize row counter and sprite pointer for all phases
    ldx #1                  ; Row counter (rows 1-17)
    ldy #3                  ; Sprite pointer (row 1 = byte 3, row 0 is bytes 0-2)

    ; Jump to appropriate phase
    lda FRAME
    cmp #11
    bcc @run_phase1
    cmp #21
    bcc @run_phase2
    cmp #31
    bcc @run_phase3
    jmp phase4_copy

@run_phase1:
    jmp phase1_copy
@run_phase2:
    jmp phase2_copy
@run_phase3:
    jmp phase3_copy
.endproc

; ─── Phase 1: Rotate right 4px ───────────────────────────────────────────────
.proc phase1_copy
@row_loop:
    ; Load 6 bytes from stack
    pla
    sta TEMP_COL0
    pla
    sta TEMP_COL1
    pla
    sta TEMP_COL2
    pla
    sta TEMP_MED0
    pla
    sta TEMP_MED1
    pla
    sta TEMP_MED2

    ; Rotate right 4px and write to charset at row X (1-17)
    ; Row 1 → scanline 1, row 2 → scanline 2, etc. (row 0 stays blank)
    ; Use SMC scratch (faster than dest as scratch)
    stx @restore_x+1        ; SMC: save X for restoration

    lda TEMP_COL0
    lsr
    lsr
    lsr
    lsr
    sta @ora0+1             ; SMC: store shifted nibble
    lda TEMP_COL2
    asl
    asl
    asl
    asl
@ora0:
    ora #$FF                ; SMC: combine with stored value
    sta CHARSET_RAM,x

    lda TEMP_COL1
    lsr
    lsr
    lsr
    lsr
    sta @ora1+1             ; SMC
    lda TEMP_COL0
    asl
    asl
    asl
    asl
@ora1:
    ora #$FF                ; SMC
    sta CHARSET_RAM+64,x

    lda TEMP_COL2
    lsr
    lsr
    lsr
    lsr
    sta @ora2+1             ; SMC
    lda TEMP_COL1
    asl
    asl
    asl
    asl
@ora2:
    ora #$FF                ; SMC
    sta CHARSET_RAM+128,x

    ; Write sprite data using auto-advancing Y pointer
    ; Y is initialized to 3 (sprite row 1) and increments naturally

    ; Rotate right 4px for medium data - column 0
    lda TEMP_MED0
    lsr
    lsr
    lsr
    lsr
    sta @ora3+1             ; SMC
    lda TEMP_MED2
    asl
    asl
    asl
    asl
@ora3:
    ora #$FF                ; SMC
    sta SPRITE_DATA,y
    iny

    ; Column 1
    lda TEMP_MED1
    lsr
    lsr
    lsr
    lsr
    sta @ora4+1             ; SMC
    lda TEMP_MED0
    asl
    asl
    asl
    asl
@ora4:
    ora #$FF                ; SMC
    sta SPRITE_DATA,y
    iny

    ; Column 2
    lda TEMP_MED2
    lsr
    lsr
    lsr
    lsr
    sta @ora5+1             ; SMC
    lda TEMP_MED1
    asl
    asl
    asl
    asl
@ora5:
    ora #$FF                ; SMC
    sta SPRITE_DATA,y
    iny                     ; Y now points to next row

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase1_done
    jmp @row_loop
@phase1_done:
    rts
.endproc

; ─── Phase 2: H-mirror + rotate right 8px ────────────────────────────────────
.proc phase2_copy
    lda #3
    sta @sprite_y+1         ; Initialize sprite pointer (row 1 = byte 3)
@row_loop:
    ; Reverse bits and reorder (rev0, rev2, rev1)
    ; Write to charset at row X (1-17)
    stx @restore_x+1        ; SMC: save X for restoration

    ; Load col0, reverse, write to charset[0]
    pla
    tay
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM,x

    ; Load col1, save for charset[2] and sprite later
    pla
    sta TEMP_COL1

    ; Load col2, reverse, write to charset[1]
    pla
    tay
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM+64,x

    ; Now reverse col1 and write to charset[2]
    ldy TEMP_COL1
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM+128,x

    ; Load medium bytes and save all 3 for sprite
    pla
    sta TEMP_MED0
    pla
    sta TEMP_MED1
    pla
    sta TEMP_MED2

    ; Reverse all three bytes (clobbers Y, overwrites temps)
    ldy TEMP_MED0
    lda BIT_REVERSE_TABLE,y
    sta TEMP_MED0

    ldy TEMP_MED2
    lda BIT_REVERSE_TABLE,y
    sta TEMP_MED2

    ldy TEMP_MED1
    lda BIT_REVERSE_TABLE,y
    sta TEMP_MED1

    ; Write sprite data using auto-advancing Y pointer
@sprite_y:
    ldy #$FF                ; SMC: sprite pointer (load once)

    lda TEMP_MED0
    sta SPRITE_DATA,y
    iny

    lda TEMP_MED2
    sta SPRITE_DATA,y
    iny

    lda TEMP_MED1
    sta SPRITE_DATA,y
    iny

    sty @sprite_y+1         ; Save updated pointer once

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase2_done
    jmp @row_loop
@phase2_done:
    rts
.endproc

; ─── Phase 3: Rotate left 8px ────────────────────────────────────────────────
.proc phase3_copy
    lda #3
    sta @sprite_y+1         ; Initialize sprite pointer (row 1 = byte 3)
@row_loop:
    ; Load 6 bytes from stack and write to charset/SMC
    ; Write to charset at row X (1-17)
    stx @restore_x+1        ; SMC: save X for restoration

    pla                     ; col0
    sta CHARSET_RAM+128,x   ; → col 2 (rotate left)
    pla                     ; col1
    sta CHARSET_RAM,x       ; → col 0
    pla                     ; col2
    sta CHARSET_RAM+64,x    ; → col 1

    ; Load medium data and store directly into SMC operands
    pla                     ; med0
    sta @write_med0+1       ; Store for third write

    pla                     ; med1
    sta @write_med1+1       ; Store for first write

    pla                     ; med2
    sta @write_med2+1       ; Store for second write

    ; Write sprite data using auto-advancing Y pointer (reordered: med1, med2, med0)
@sprite_y:
    ldy #$FF                ; SMC: sprite pointer (load once)

@write_med1:
    lda #$FF                ; SMC: med1 value
    sta SPRITE_DATA,y       ; Column 0
    iny

@write_med2:
    lda #$FF                ; SMC: med2 value
    sta SPRITE_DATA,y       ; Column 1
    iny

@write_med0:
    lda #$FF                ; SMC: med0 value
    sta SPRITE_DATA,y       ; Column 2 (rotate left 8px)
    iny

    sty @sprite_y+1         ; Save updated pointer once

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase3_done
    jmp @row_loop
@phase3_done:
    rts
.endproc

; ─── Phase 4: H-mirror + rotate left 4px ─────────────────────────────────────
.proc phase4_copy
    lda #3
    sta @sprite_y+1         ; Initialize sprite pointer (row 1 = byte 3)
@row_loop:
    ; Load 6 bytes from stack
    pla
    sta TEMP_COL0
    pla
    sta TEMP_COL1
    pla
    sta TEMP_COL2
    pla
    sta TEMP_MED0
    pla
    sta TEMP_MED1
    pla
    sta TEMP_MED2

    ; Rotate left 4px: reverse((B << 4) | (A >> 4))
    ; Shift unreversed bytes, combine, then reverse once before storing
    stx @restore_x+1        ; SMC: save X for restoration

    ; Charset[0]: reverse((col1 << 4) | (col2 >> 4))
    lda TEMP_COL1
    asl
    asl
    asl
    asl
    sta @ora0+1             ; SMC: low nibble of col1
    lda TEMP_COL2
    lsr
    lsr
    lsr
    lsr
@ora0:
    ora #$FF                ; SMC: combine nibbles
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
    sta CHARSET_RAM,x

    ; Charset[1]: reverse((col0 << 4) | (col1 >> 4))
    lda TEMP_COL0
    asl
    asl
    asl
    asl
    sta @ora1+1             ; SMC
    lda TEMP_COL1
    lsr
    lsr
    lsr
    lsr
@ora1:
    ora #$FF                ; SMC
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
    sta CHARSET_RAM+64,x

    ; Charset[2]: reverse((col2 << 4) | (col0 >> 4))
    lda TEMP_COL2
    asl
    asl
    asl
    asl
    sta @ora2+1             ; SMC
    lda TEMP_COL0
    lsr
    lsr
    lsr
    lsr
@ora2:
    ora #$FF                ; SMC
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
    sta CHARSET_RAM+128,x

    ; Write sprite data using auto-advancing Y pointer
    ; Rotate left 4px and write row-major
    ; Shift unreversed bytes, combine, then reverse once before storing

@sprite_y:
    ldy #$FF                ; SMC: sprite pointer (load once)

    ; Sprite[0]: reverse((med1 << 4) | (med2 >> 4))
    lda TEMP_MED1
    asl
    asl
    asl
    asl
    sta @ora3+1             ; SMC
    lda TEMP_MED2
    lsr
    lsr
    lsr
    lsr
@ora3:
    ora #$FF                ; SMC
    tax
    lda BIT_REVERSE_TABLE,x ; Reverse once before storing
    sta SPRITE_DATA,y
    iny

    ; Sprite[1]: reverse((med0 << 4) | (med1 >> 4))
    lda TEMP_MED0
    asl
    asl
    asl
    asl
    sta @ora4+1             ; SMC
    lda TEMP_MED1
    lsr
    lsr
    lsr
    lsr
@ora4:
    ora #$FF                ; SMC
    tax
    lda BIT_REVERSE_TABLE,x ; Reverse once before storing
    sta SPRITE_DATA,y
    iny

    ; Sprite[2]: reverse((med2 << 4) | (med0 >> 4))
    lda TEMP_MED2
    asl
    asl
    asl
    asl
    sta @ora5+1             ; SMC
    lda TEMP_MED0
    lsr
    lsr
    lsr
    lsr
@ora5:
    ora #$FF                ; SMC
    tax
    lda BIT_REVERSE_TABLE,x ; Reverse once before storing
    sta SPRITE_DATA,y
    iny

    sty @sprite_y+1         ; Save updated pointer

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase4_done
    jmp @row_loop
@phase4_done:
    rts
.endproc

.segment "RODATA"

; Bit reversal lookup table (256 bytes)
BIT_REVERSE_TABLE:
    .byte $00, $80, $40, $C0, $20, $A0, $60, $E0
    .byte $10, $90, $50, $D0, $30, $B0, $70, $F0
    .byte $08, $88, $48, $C8, $28, $A8, $68, $E8
    .byte $18, $98, $58, $D8, $38, $B8, $78, $F8
    .byte $04, $84, $44, $C4, $24, $A4, $64, $E4
    .byte $14, $94, $54, $D4, $34, $B4, $74, $F4
    .byte $0C, $8C, $4C, $CC, $2C, $AC, $6C, $EC
    .byte $1C, $9C, $5C, $DC, $3C, $BC, $7C, $FC
    .byte $02, $82, $42, $C2, $22, $A2, $62, $E2
    .byte $12, $92, $52, $D2, $32, $B2, $72, $F2
    .byte $0A, $8A, $4A, $CA, $2A, $AA, $6A, $EA
    .byte $1A, $9A, $5A, $DA, $3A, $BA, $7A, $FA
    .byte $06, $86, $46, $C6, $26, $A6, $66, $E6
    .byte $16, $96, $56, $D6, $36, $B6, $76, $F6
    .byte $0E, $8E, $4E, $CE, $2E, $AE, $6E, $EE
    .byte $1E, $9E, $5E, $DE, $3E, $BE, $7E, $FE
    .byte $01, $81, $41, $C1, $21, $A1, $61, $E1
    .byte $11, $91, $51, $D1, $31, $B1, $71, $F1
    .byte $09, $89, $49, $C9, $29, $A9, $69, $E9
    .byte $19, $99, $59, $D9, $39, $B9, $79, $F9
    .byte $05, $85, $45, $C5, $25, $A5, $65, $E5
    .byte $15, $95, $55, $D5, $35, $B5, $75, $F5
    .byte $0D, $8D, $4D, $CD, $2D, $AD, $6D, $ED
    .byte $1D, $9D, $5D, $DD, $3D, $BD, $7D, $FD
    .byte $03, $83, $43, $C3, $23, $A3, $63, $E3
    .byte $13, $93, $53, $D3, $33, $B3, $73, $F3
    .byte $0B, $8B, $4B, $CB, $2B, $AB, $6B, $EB
    .byte $1B, $9B, $5B, $DB, $3B, $BB, $7B, $FB
    .byte $07, $87, $47, $C7, $27, $A7, $67, $E7
    .byte $17, $97, $57, $D7, $37, $B7, $77, $F7
    .byte $0F, $8F, $4F, $CF, $2F, $AF, $6F, $EF
    .byte $1F, $9F, $5F, $DF, $3F, $BF, $7F, $FF
