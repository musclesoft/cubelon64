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
SRC_FRAME: .res 1

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
    sta @push_load+1
    txa                     ; Restore shifted high byte
    adc #>PRECOMPUTED_FRAMES
    sta @push_load+2

    ; Push all 102 bytes (17 rows × 6 bytes) onto stack in reverse order
    ; This allows fast PLA access (4 cycles vs 22 cycles for JSR get_next_byte)
    ; Add 101 to address to start from last byte
    lda @push_load+1
    clc
    adc #101
    sta @push_load+1
    lda @push_load+2
    adc #0
    sta @push_load+2

    ldx #102                ; Push 102 bytes
@push_loop:
@push_load:
    lda $FFFF               ; SMC: address set above
    pha

    ; Decrement address
    lda @push_load+1
    bne @no_borrow
    dec @push_load+2
@no_borrow:
    dec @push_load+1

    dex
    bne @push_loop

    ; Initialize row counter for all phases
    ldx #1                  ; Row counter (rows 1-17)

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
    ; Load charset bytes from stack
    pla
    sta TEMP_COL0
    pla
    sta TEMP_COL1
    pla
    sta TEMP_COL2

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

    ; Load sprite bytes from stack (reuse same temps)
    pla
    sta TEMP_COL0
    pla
    sta TEMP_COL1
    pla
    sta TEMP_COL2

    ; Calculate sprite offset from row counter: sprite_byte = row * 3
    lda @restore_x+1        ; Get saved row counter (1-17)
    asl                     ; row * 2
    adc @restore_x+1        ; row * 2 + row = row * 3 (carry clear)
    tax                     ; X now points to first sprite byte for this row

    ; Rotate right 4px for sprite data - column 0
    lda TEMP_COL0
    lsr
    lsr
    lsr
    lsr
    sta @ora3+1             ; SMC
    lda TEMP_COL2
    asl
    asl
    asl
    asl
@ora3:
    ora #$FF                ; SMC
    sta SPRITE_DATA,x
    inx

    ; Column 1
    lda TEMP_COL1
    lsr
    lsr
    lsr
    lsr
    sta @ora4+1             ; SMC
    lda TEMP_COL0
    asl
    asl
    asl
    asl
@ora4:
    ora #$FF                ; SMC
    sta SPRITE_DATA,x
    inx

    ; Column 2
    lda TEMP_COL2
    lsr
    lsr
    lsr
    lsr
    sta @ora5+1             ; SMC
    lda TEMP_COL1
    asl
    asl
    asl
    asl
@ora5:
    ora #$FF                ; SMC
    sta SPRITE_DATA,x

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
@row_loop:
    ; Load charset bytes
    pla
    sta TEMP_COL0
    pla
    sta TEMP_COL1
    pla
    sta TEMP_COL2

    ; Reverse bits and reorder (rev0, rev2, rev1)
    ; Write to charset at row X (1-17)
    stx @restore_x+1        ; SMC: save X for restoration

    ; Reverse col0, write to charset[0]
    ldy TEMP_COL0
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM,x

    ; Reverse col2, write to charset[1]
    ldy TEMP_COL2
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM+64,x

    ; Reverse col1, write to charset[2]
    ldy TEMP_COL1
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM+128,x

    ; Load sprite bytes (reuse same temps)
    pla
    sta TEMP_COL0
    pla
    sta TEMP_COL1
    pla
    sta TEMP_COL2

    ; Reverse all three bytes (clobbers Y, overwrites temps)
    ldy TEMP_COL0
    lda BIT_REVERSE_TABLE,y
    sta TEMP_COL0

    ldy TEMP_COL2
    lda BIT_REVERSE_TABLE,y
    sta TEMP_COL2

    ldy TEMP_COL1
    lda BIT_REVERSE_TABLE,y
    sta TEMP_COL1

    ; Calculate sprite offset from row counter: sprite_byte = row * 3
    lda @restore_x+1        ; Get saved row counter (1-17)
    asl                     ; row * 2
    adc @restore_x+1        ; row * 2 + row = row * 3 (carry clear)
    tax                     ; X now points to first sprite byte for this row

    lda TEMP_COL0
    sta SPRITE_DATA,x
    inx

    lda TEMP_COL2
    sta SPRITE_DATA,x
    inx

    lda TEMP_COL1
    sta SPRITE_DATA,x

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

    ; Load sprite bytes and store directly into SMC operands
    pla                     ; col0
    sta @write_col0+1       ; Store for third write

    pla                     ; col1
    sta @write_col1+1       ; Store for first write

    pla                     ; col2
    sta @write_col2+1       ; Store for second write

    ; Calculate sprite offset from row counter: sprite_byte = row * 3
    lda @restore_x+1        ; Get saved row counter (1-17)
    asl                     ; row * 2
    adc @restore_x+1        ; row * 2 + row = row * 3 (carry clear)
    tax                     ; X now points to first sprite byte for this row

@write_col1:
    lda #$FF                ; SMC: col1 value
    sta SPRITE_DATA,x       ; Column 0
    inx

@write_col2:
    lda #$FF                ; SMC: col2 value
    sta SPRITE_DATA,x       ; Column 1
    inx

@write_col0:
    lda #$FF                ; SMC: col0 value
    sta SPRITE_DATA,x       ; Column 2 (rotate left 8px)

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
@row_loop:
    ; Load charset bytes from stack
    pla
    sta TEMP_COL0
    pla
    sta TEMP_COL1
    pla
    sta TEMP_COL2

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

    ; Load sprite bytes from stack (reuse same temps)
    pla
    sta TEMP_COL0
    pla
    sta TEMP_COL1
    pla
    sta TEMP_COL2

    ; Save sprite base offset for reuse
    lda @restore_x+1        ; Get saved row counter (1-17)
    asl                     ; row * 2
    adc @restore_x+1        ; row * 2 + row = row * 3 (carry clear)
    sta @sprite_base+1      ; Save for later restores

    ; Sprite[0]: reverse((col1 << 4) | (col2 >> 4))
    lda TEMP_COL1
    asl
    asl
    asl
    asl
    sta @ora3+1             ; SMC
    lda TEMP_COL2
    lsr
    lsr
    lsr
    lsr
@ora3:
    ora #$FF                ; SMC
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
@sprite_base:
    ldx #$FF                ; SMC: sprite base offset
    sta SPRITE_DATA,x
    inx

    ; Sprite[1]: reverse((col0 << 4) | (col1 >> 4))
    lda TEMP_COL0
    asl
    asl
    asl
    asl
    sta @ora4+1             ; SMC
    lda TEMP_COL1
    lsr
    lsr
    lsr
    lsr
@ora4:
    ora #$FF                ; SMC
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
    sta SPRITE_DATA,x
    inx

    ; Sprite[2]: reverse((col2 << 4) | (col0 >> 4))
    lda TEMP_COL2
    asl
    asl
    asl
    asl
    sta @ora5+1             ; SMC
    lda TEMP_COL0
    lsr
    lsr
    lsr
    lsr
@ora5:
    ora #$FF                ; SMC
    tay
    lda BIT_REVERSE_TABLE,y ; Reverse once before storing
    sta SPRITE_DATA,x

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase4_done
    jmp @row_loop
@phase4_done:
    rts
.endproc
