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

; ─── Helper: Get next byte from precomputed data ─────────────────────────────
; Returns byte in A, auto-increments internal address
; Must call set_source_addr first to initialize
; Note: PRECOMPUTED_FRAMES is page-aligned, so no page crossing within a frame
get_next_byte:
gnb_load:
    lda $FFFF               ; SMC: address set by set_source_addr
    inc gnb_load+1          ; Safe: never crosses page boundary
    rts

; ─── Helper: Set source address for reading ──────────────────────────────────
; Input: A = frame (0-10), X = row (1-17)
; Sets up get_next_byte to read from PRECOMPUTED_FRAMES[frame][row]
.proc set_source_addr
    ; Save frame and row
    sta @frame
    stx @row

    ; Calculate base: PRECOMPUTED_FRAMES + (frame × 128)
    ; frame × 128 = frame × 2^7 = (frame << 7)
    asl                     ; frame × 2
    tax                     ; Save for later
    lda #0
    rol                     ; Get high bit
    ; Now need to shift 6 more times
    stx @temp
    txa
    asl
    asl
    asl
    asl
    asl
    asl                     ; Low byte of frame × 128
    sta @addr_lo
    lda @temp
    lsr
    lsr                     ; High byte of frame × 128: (frame×2) >> 2
    clc
    adc #>PRECOMPUTED_FRAMES
    sta @addr_hi
    lda @addr_lo
    clc
    adc #<PRECOMPUTED_FRAMES
    sta @addr_lo
    bcc @no_carry1
    inc @addr_hi
@no_carry1:
    ; Add row offset: (row - 1) × 6
    lda @row
    sec
    sbc #1
    tay                     ; Y = row - 1
    ; Y × 6 = Y × 4 + Y × 2
    tya
    asl                     ; × 2
    sta @offset
    tya
    asl
    asl                     ; × 4
    clc
    adc @offset             ; × 6

    ; Add to base address
    clc
    adc @addr_lo
    sta gnb_load+1
    lda @addr_hi
    adc #0
    sta gnb_load+2

    ldx @row            ; Restore X before returning
    rts

@frame: .res 1
@row: .res 1
@addr_lo: .res 1
@addr_hi: .res 1
@offset: .res 1
@temp: .res 1
.endproc

; ─── Helper: Store next byte to precomputed data ─────────────────────────────
; Input: A = byte to store
; Must call set_dest_addr first to initialize
store_next_byte:
snb_store:
    sta $FFFF               ; SMC: address set by set_dest_addr
    inc snb_store+1
    bne @skip
    inc snb_store+2
@skip:
    rts

; ─── Helper: Set destination address for storing ─────────────────────────────
; Input: A = frame (0-10), X = row (1-17)
.proc set_dest_addr
    sta @frame
    stx @row

    ; Same calculation as set_source_addr
    asl
    tax
    lda #0
    rol
    stx @temp
    txa
    asl
    asl
    asl
    asl
    asl
    asl
    sta @addr_lo
    lda @temp
    lsr
    lsr                     ; High byte of frame × 128: (frame×2) >> 2
    clc
    adc #>PRECOMPUTED_FRAMES
    sta @addr_hi
    lda @addr_lo
    clc
    adc #<PRECOMPUTED_FRAMES
    sta @addr_lo
    bcc @no_carry2
    inc @addr_hi
@no_carry2:
    ; Add row offset
    lda @row
    sec
    sbc #1
    tay
    tya
    asl
    sta @offset
    tya
    asl
    asl
    clc
    adc @offset

    clc
    adc @addr_lo
    sta snb_store+1
    lda @addr_hi
    adc #0
    sta snb_store+2

    ldx @row            ; Restore X before returning
    rts

@frame: .res 1
@row: .res 1
@addr_lo: .res 1
@addr_hi: .res 1
@offset: .res 1
@temp: .res 1
.endproc

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
    ; Common setup: initialize source address (auto-increments through all rows)
    ; Inline: PRECOMPUTED_FRAMES + (SRC_FRAME × 128) + 0 (row=1 offset)
    lda SRC_FRAME
    asl                     ; frame × 2
    tax
    lda #0
    rol                     ; Catch high bit
    stx @temp
    txa
    asl
    asl
    asl
    asl
    asl
    asl                     ; Low byte of frame × 128
    sta @addr_lo
    lda @temp
    lsr
    lsr                     ; High byte (frame×2 >> 2)
    clc
    adc #>PRECOMPUTED_FRAMES
    sta @addr_hi
    lda @addr_lo
    clc
    adc #<PRECOMPUTED_FRAMES
    sta gnb_load+1
    lda @addr_hi
    adc #0                  ; Add carry from low byte
    sta gnb_load+2

    ; Initialize row counter for all phases
    ldx #1

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

@temp: .res 1
@addr_lo: .res 1
@addr_hi: .res 1
.endproc

; ─── Phase 1: Rotate right 4px ───────────────────────────────────────────────
.proc phase1_copy
@row_loop:
    ; Load 6 bytes (address auto-increments)
    jsr get_next_byte
    sta TEMP_COL0
    jsr get_next_byte
    sta TEMP_COL1
    jsr get_next_byte
    sta TEMP_COL2
    jsr get_next_byte
    sta TEMP_MED0
    jsr get_next_byte
    sta TEMP_MED1
    jsr get_next_byte
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

    ; Write sprite data in row-major format (3 consecutive bytes per row)
    ; VIC sprite format: 24 pixels wide (3 bytes) × 21 rows = 63 bytes
    ; Sprite row offset: X × 3 (sprite rows 1-17, matching charset rows 1-17)
    stx @row_temp
    txa
    asl                     ; X × 2
    clc
    adc @row_temp           ; X × 3
    tax                     ; X = sprite offset

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
    sta SPRITE_DATA,x

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
    inx
    sta SPRITE_DATA,x

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
    inx
    sta SPRITE_DATA,x

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase1_done
    jmp @row_loop
@row_temp: .res 1
@phase1_done:
    rts
.endproc

; ─── Phase 2: H-mirror + rotate right 8px ────────────────────────────────────
.proc phase2_copy
@row_loop:
    ; Reverse bits and reorder (rev0, rev2, rev1)
    ; Write to charset at row X (1-17)
    stx @restore_x+1        ; SMC: save X for restoration

    ; Load col0, reverse, write to charset[0]
    jsr get_next_byte
    tay
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM,x

    ; Load col1, save for charset[2] and sprite later
    jsr get_next_byte
    sta TEMP_COL1

    ; Load col2, reverse, write to charset[1]
    jsr get_next_byte
    tay
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM+64,x

    ; Now reverse col1 and write to charset[2]
    ldy TEMP_COL1
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM+128,x

    ; Load medium bytes and save all 3 for sprite
    jsr get_next_byte
    sta TEMP_MED0
    jsr get_next_byte
    sta TEMP_MED1
    jsr get_next_byte
    sta TEMP_MED2

    ; Write sprite data in row-major format
    ; Sprite row offset: X × 3 (sprite rows 1-17, matching charset rows 1-17)
    stx @row_temp
    txa
    asl
    clc
    adc @row_temp
    tax                     ; X = sprite offset

    ; Reverse and write column 0
    ldy TEMP_MED0
    lda BIT_REVERSE_TABLE,y
    sta SPRITE_DATA,x

    ; Reverse and write column 1
    ldy TEMP_MED2
    lda BIT_REVERSE_TABLE,y
    inx
    sta SPRITE_DATA,x

    ; Reverse and write column 2
    ldy TEMP_MED1
    lda BIT_REVERSE_TABLE,y
    inx
    sta SPRITE_DATA,x

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase2_done
    jmp @row_loop
@row_temp: .res 1
@phase2_done:
    rts
.endproc

; ─── Phase 3: Rotate left 8px ────────────────────────────────────────────────
.proc phase3_copy
@row_loop:
    ; Load 6 bytes and directly store with reorder (address auto-increments)
    ; Write to charset at row X (1-17)
    stx @restore_x+1        ; SMC: save X for restoration

    jsr get_next_byte       ; col0
    sta CHARSET_RAM+128,x   ; → col 2 (rotate left)
    jsr get_next_byte       ; col1
    sta CHARSET_RAM,x       ; → col 0
    jsr get_next_byte       ; col2
    sta CHARSET_RAM+64,x    ; → col 1

    ; Load medium data
    jsr get_next_byte       ; med0
    sta @med0
    jsr get_next_byte       ; med1
    sta @med1
    jsr get_next_byte       ; med2
    sta @med2

    ; Write sprite data in row-major format (reordered: med1, med0, med2)
    ; Sprite row offset: X × 3 (sprite rows 1-17, matching charset rows 1-17)
    stx @row_temp
    txa
    asl
    clc
    adc @row_temp
    tay                     ; Y = sprite offset

    lda @med0
    sta SPRITE_DATA+2,y     ; Column 2 (rotate left 8px)
    lda @med1
    sta SPRITE_DATA,y       ; Column 0
    lda @med2
    sta SPRITE_DATA+1,y     ; Column 1

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase3_done
    jmp @row_loop
@row_temp: .res 1
@med0: .res 1
@med1: .res 1
@med2: .res 1
@phase3_done:
    rts
.endproc

; ─── Phase 4: H-mirror + rotate left 4px ─────────────────────────────────────
.proc phase4_copy
@row_loop:
    ; Load 6 bytes (address auto-increments)
    jsr get_next_byte
    sta TEMP_COL0
    jsr get_next_byte
    sta TEMP_COL1
    jsr get_next_byte
    sta TEMP_COL2
    jsr get_next_byte
    sta TEMP_MED0
    jsr get_next_byte
    sta TEMP_MED1
    jsr get_next_byte
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

    ; Sprite row offset: X × 3 (sprite rows 1-17, matching charset rows 1-17)
    stx @row_temp
    txa
    asl                     ; X × 2
    clc
    adc @row_temp           ; X × 3
    tax                     ; X = sprite offset

    ; Rotate left 4px and write row-major
    ; Shift unreversed bytes, combine, then reverse once before storing

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
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
    sta SPRITE_DATA,x

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
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
    inx
    sta SPRITE_DATA,x

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
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
    inx
    sta SPRITE_DATA,x

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase4_done
    jmp @row_loop
@row_temp: .res 1
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
