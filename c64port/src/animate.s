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
CHARSET = $6000             ; VIC bank 1 + $2000
SPRITE = $6800              ; VIC bank 1 + $2800
SPRITE_REVERSE = $6880      ; VIC bank 1 + $2880 (bit-reversed)

; Charset column destinations (column-major layout)
; COL0_PTR data destinations
CHARSET_COL0 = CHARSET+0           ; Normal row
CHARSET_REVERSE_COL0 = CHARSET+152 ; Row+24 (bit-reversed to right position)

; COL1_PTR data destinations
CHARSET_COL1 = CHARSET+64          ; Normal row
CHARSET_REVERSE_COL1 = CHARSET+88  ; Row+24 (bit-reversed, stays center)

; COL2_PTR data destinations
CHARSET_COL2 = CHARSET+128         ; Normal row
CHARSET_REVERSE_COL2 = CHARSET+24  ; Row+24 (bit-reversed to left position)

; Sprite column destinations (3 bytes per row)
; Sprites use indexed addressing: SPRITE_COL0,x where x = row offset
SPRITE_COL0 = SPRITE+3             ; Normal column 0
SPRITE_COL1 = SPRITE+4             ; Normal column 1
SPRITE_COL2 = SPRITE+5             ; Normal column 2

SPRITE_REVERSE_COL0 = SPRITE_REVERSE+3  ; Reversed column 0
SPRITE_REVERSE_COL1 = SPRITE_REVERSE+4  ; Reversed column 1
SPRITE_REVERSE_COL2 = SPRITE_REVERSE+5  ; Reversed column 2

.segment "ZEROPAGE"

; Six pointers into the source frame, one per output byte of a row
; (A, B, C order depends on the source, see source_offsets)
CHARSET_A_PTR: .res 2
CHARSET_B_PTR: .res 2
CHARSET_C_PTR: .res 2
SPRITE_A_PTR: .res 2
SPRITE_B_PTR: .res 2
SPRITE_C_PTR: .res 2

FRAME_LO: .res 1
ROW_BYTE_0: .res 1
ROW_BYTE_1: .res 1
ROW_BYTE_2: .res 1

.segment "BSS"

; Precomputed frame storage: 11 frames × 128 bytes
; Aligned to 128 so each frame stays within one page
.align 128
.export PRECOMPUTED_FRAMES
PRECOMPUTED_FRAMES: .res 11 * 128

; The same frames rotated right 4px, so playback only reorders bytes
PRECOMPUTED_SHIFTED_FRAMES: .res 11 * 128

.segment "CODE"

; External functions
.import calc_rotation: absolute
.import render_square: absolute
.import BIT_REVERSE_TABLE: absolute

; Export animation functions
.export precompute_frames
.export copy_frame_to_charset
.export test_checkpoint_frame
.export test_after_calc_rotation
.export test_after_render
.export SPRITE
.export SPRITE_REVERSE


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
    jsr shift_frame
    inc FRAME
    lda FRAME
    cmp #11
    bcc frame_loop
@frames_done:

    lda #0
    sta FRAME
    rts
.endproc

; ─── Rotate the current frame right 4px into PRECOMPUTED_SHIFTED_FRAMES ──────
.proc shift_frame
    lda FRAME
    jsr frame_address
    sta CHARSET_A_PTR+1
    lda FRAME_LO
    sta CHARSET_A_PTR
    lda FRAME
    clc
    adc #11
    jsr frame_address
    sta CHARSET_B_PTR+1
    lda FRAME_LO
    sta CHARSET_B_PTR

    ldy #0
@triple:
    lda (CHARSET_A_PTR),y
    sta ROW_BYTE_0
    iny
    lda (CHARSET_A_PTR),y
    sta ROW_BYTE_1
    iny
    lda (CHARSET_A_PTR),y
    sta ROW_BYTE_2
    ldx #4
@rotate_bit:
    lda ROW_BYTE_2
    lsr
    ror ROW_BYTE_0
    ror ROW_BYTE_1
    ror ROW_BYTE_2
    dex
    bne @rotate_bit
    lda ROW_BYTE_2
    sta (CHARSET_B_PTR),y
    dey
    lda ROW_BYTE_1
    sta (CHARSET_B_PTR),y
    dey
    lda ROW_BYTE_0
    sta (CHARSET_B_PTR),y
    iny
    iny
    iny
    cpy #102
    bcc @triple
    rts
.endproc

; ─── Copy and transform frame to charset ─────────────────────────────────────
; Playback is a byte reorder of a stored frame (source slots 0-10 are the
; precomputed frames, 11-21 the same frames rotated right 4px):
;   Phase 1 (frames  0-10): shifted frame,    reorder 2,0,1
;   Phase 2 (frames 11-20): frame 20-frame,   H-mirror
;   Phase 3 (frames 21-30): frame frame-20,   reorder 0,1,2 (rotate left 8px)
;   Phase 4 (frames 31-40): shifted 40-frame, H-mirror + reorder 2,0,1
.proc copy_frame_to_charset
    lda FRAME
    cmp #11
    bcc @phase1
    cmp #21
    bcc @phase2
    cmp #31
    bcc @phase3

@phase4:
    lda #40+11
    sec
    sbc FRAME
    ldy #SHIFTED_OFFSETS
    jsr set_source
    jmp mirror_copy

@phase1:
    adc #11
    ldy #SHIFTED_OFFSETS
    jsr set_source
    jmp reorder_copy

@phase2:
    lda #20
    sec
    sbc FRAME
    ldy #0
    jsr set_source
    jmp mirror_copy

@phase3:
    sec
    sbc #20
    ldy #0
    jsr set_source
    jmp reorder_copy
.endproc

; ─── Point the six column pointers at a stored frame ─────────────────────────
; A = source slot, Y = offset into source_offsets
; Returns with Y and X set up as the row loop start
.proc set_source
    jsr frame_address
    sta CHARSET_A_PTR+1
    sta CHARSET_B_PTR+1
    sta CHARSET_C_PTR+1
    sta SPRITE_A_PTR+1
    sta SPRITE_B_PTR+1
    sta SPRITE_C_PTR+1
    ldx #0
@pointer:
    lda FRAME_LO
    clc
    adc source_offsets,y
    sta CHARSET_A_PTR,x
    adc #3
    sta SPRITE_A_PTR,x
    iny
    inx
    inx
    cpx #6
    bne @pointer
    ldy #96                 ; Y tracks row position (decrements by 6 per row)
    ldx #17                 ; Row counter (rows 17 down to 1)
    rts
.endproc

; ─── Address of the stored frame in slot A: low byte in FRAME_LO, high in A ──
.proc frame_address
    lsr                     ; slot × 128: slot in the high byte, shifted right 1
    tax
    lda #0
    ror
    adc #<PRECOMPUTED_FRAMES
    sta FRAME_LO
    txa
    adc #>PRECOMPUTED_FRAMES
    rts
.endproc

; Column offsets of output bytes A, B, C: raw frames, then shifted frames
source_offsets:
    .byte 0, 1, 2
    .byte 2, 0, 1
SHIFTED_OFFSETS = 3

; ─── Reorder: columns A,B,C go to charset/sprite columns 2,0,1 ───────────────
.proc reorder_copy
@row_loop:
    ; Rotate left 8px: just reorder bytes (col1, col2, col0)
    ; Write to charset at row X (1-17)

    ; Charset: reorder col0→2, col1→0, col2→1
    lda (CHARSET_A_PTR),y
    sta CHARSET_COL2,x
    sta @reverse0+1         ; SMC
@reverse0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL2,x
    lda (CHARSET_B_PTR),y
    sta CHARSET_COL0,x
    sta @reverse1+1         ; SMC
@reverse1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL0,x
    lda (CHARSET_C_PTR),y
    sta CHARSET_COL1,x
    sta @reverse2+1         ; SMC
@reverse2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL1,x

    ; Calculate sprite offset from Y: sprite_byte = Y/2
    tya                     ; Get Y (0, 6, 12, ..., 96)
    lsr                     ; Y/2 (carry clear since Y always even)
    stx @restore_x+1        ; Save X for loop restoration
    tax                     ; X = Y/2 (sprite base offset)

    ; Sprite: reorder col0→2, col1→0, col2→1
    ; SPRITE_REVERSE gets bit-reversed with columns 0↔2 swapped:
    ; reverse(col0)→byte 0, reverse(col1)→byte 2, reverse(col2)→byte 1
    lda (SPRITE_A_PTR),y
    sta SPRITE_COL2,x
    sta @reverse_sprite0+1  ; SMC: reverse(col0)→byte 0 for SPRITE_REVERSE (swapped)
@reverse_sprite0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_REVERSE_COL0,x
    lda (SPRITE_B_PTR),y
    sta SPRITE_COL0,x
    sta @reverse_sprite2+1  ; SMC: reverse(col1)→byte 2 for SPRITE_REVERSE (swapped)
@reverse_sprite2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_REVERSE_COL2,x
    lda (SPRITE_C_PTR),y
    sta SPRITE_COL1,x
    sta @reverse_sprite1+1  ; SMC: reverse(col2)→byte 1 for SPRITE_REVERSE (stays center)
@reverse_sprite1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_REVERSE_COL1,x

@restore_x:
    ldx #$FF                ; SMC: restored row counter

    ; Advance Y to previous row (6 bytes per row, counting down)
    tya
    sec
    sbc #6
    tay

    dex
    bne @row_loop
    rts
.endproc

; ─── Mirror: columns A,B,C are bit-reversed into columns 0,2,1 ────────────────────────────────────
.proc mirror_copy
@row_loop:
    ; Reverse bits and reorder (rev0, rev2, rev1)
    ; Write to charset at row X (1-17)

    ; Charset byte 0: reverse(col0)
    lda (CHARSET_A_PTR),y
    sta CHARSET_REVERSE_COL0,x
    sta @reverse0+1         ; SMC
@reverse0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL0,x

    ; Charset byte 2: reverse(col1)
    lda (CHARSET_B_PTR),y
    sta CHARSET_REVERSE_COL2,x
    sta @reverse2+1         ; SMC
@reverse2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL2,x

    ; Charset byte 1: reverse(col2)
    lda (CHARSET_C_PTR),y
    sta CHARSET_REVERSE_COL1,x
    sta @reverse1+1         ; SMC
@reverse1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL1,x

    ; Calculate sprite offset from Y: sprite_byte = Y/2
    tya                     ; Get Y (0, 6, 12, ..., 96)
    lsr                     ; Y/2 (carry clear since Y always even)
    stx @restore_x+1        ; Save X for loop restoration
    tax                     ; X = Y/2 (sprite base offset)

    ; Sprite byte 0: reverse(col0)
    lda (SPRITE_A_PTR),y
    sta SPRITE_REVERSE_COL2,x        ; Store original col0 to SPRITE_REVERSE byte 2 (swapped)
    sta @reverse_sprite0+1  ; SMC
@reverse_sprite0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_COL0,x

    ; Sprite byte 2: reverse(col1)
    lda (SPRITE_B_PTR),y
    sta SPRITE_REVERSE_COL0,x        ; Store original col1 to SPRITE_REVERSE byte 0 (swapped with col2)
    sta @reverse_sprite2+1  ; SMC
@reverse_sprite2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_COL2,x

    ; Sprite byte 1: reverse(col2)
    lda (SPRITE_C_PTR),y
    sta SPRITE_REVERSE_COL1,x        ; Store original col2 to SPRITE_REVERSE byte 1 (swapped with col1)
    sta @reverse_sprite1+1  ; SMC
@reverse_sprite1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_COL1,x

@restore_x:
    ldx #$FF                ; SMC: restored row counter

    ; Advance Y to previous row (6 bytes per row, counting down)
    tya
    sec
    sbc #6
    tay

    dex
    bne @row_loop
    rts
.endproc
