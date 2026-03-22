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

; Charset column destinations (column-major layout)
; COL0_PTR data destinations
CHARSET_COL0 = CHARSET_RAM+0           ; Normal row
CHARSET_REVERSE_COL0 = CHARSET_RAM+152 ; Row+24 (bit-reversed to right position)

; COL1_PTR data destinations
CHARSET_COL1 = CHARSET_RAM+64          ; Normal row
CHARSET_REVERSE_COL1 = CHARSET_RAM+88  ; Row+24 (bit-reversed, stays center)

; COL2_PTR data destinations
CHARSET_COL2 = CHARSET_RAM+128         ; Normal row
CHARSET_REVERSE_COL2 = CHARSET_RAM+24  ; Row+24 (bit-reversed to left position)

.segment "ZEROPAGE"

; Three pointers for column data access (one per column)
; Each offset by 0, 1, 2 bytes into frame data
COL0_PTR: .res 2
COL1_PTR: .res 2
COL2_PTR: .res 2

.segment "BSS"

; Precomputed frame storage: 11 frames × 128 bytes
; Aligned to page boundary so each frame stays within one page
.align 256
.export PRECOMPUTED_FRAMES
PRECOMPUTED_FRAMES: .res 11 * 128

; (No temporary storage needed)

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
    ; Source = FRAME (already in A)
    jmp @setup_and_run

@phase2:
    ; Source = 20 - FRAME
    lda #20
    sec
    sbc FRAME               ; Result in A
    jmp @setup_and_run

@phase3:
    ; Source = FRAME - 20
    lda FRAME
    sec
    sbc #20                 ; Result in A
    jmp @setup_and_run

@phase4:
    ; Source = 40 - FRAME
    lda #40
    sec
    sbc FRAME               ; Result in A
    ; Fall through to setup

@setup_and_run:
    ; Calculate base address: PRECOMPUTED_FRAMES + (frame × 128)
    ; Set up three pointers offset by 0, 1, 2 bytes for column access
    ; frame × 128: put frame in high byte, shift right 1 bit (carry → MSB of low byte)
    ; (frame already in A from phase calculation)
    lsr                     ; Shift right in A, bit 0 → carry
    tax                     ; Save shifted high byte in X
    lda #0
    ror                     ; Rotate carry into A, bit 0→carry (always 0)
    adc #<PRECOMPUTED_FRAMES ; Low byte in A, carry clear
    sta COL0_PTR            ; Store low byte for COL0
    adc #1
    sta COL1_PTR            ; Store low byte for COL1
    adc #1                  ; Add 1 more (carry still clear)
    sta COL2_PTR            ; Store low byte for COL2
    txa                     ; Restore shifted high byte
    adc #>PRECOMPUTED_FRAMES ; High byte in A
    sta COL0_PTR+1          ; Store high byte for all three pointers
    sta COL1_PTR+1
    sta COL2_PTR+1

    ; Initialize Y offset and row counter for all phases
    ldy #0                  ; Y tracks row position (increments by 6 per row)
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
    ; Rotate right 4px and write to charset at row X (1-17)
    ; Row 1 → scanline 1, row 2 → scanline 2, etc. (row 0 stays blank)
    ; Use SMC scratch (faster than dest as scratch)

    ; Charset byte 0: (col0 >> 4) | (col2 << 4)
    lda (COL0_PTR),y
    lsr
    lsr
    lsr
    lsr
    sta @ora0+1             ; SMC
    lda (COL2_PTR),y
    asl
    asl
    asl
    asl
@ora0:
    ora #$FF                ; SMC
    sta CHARSET_COL0,x
    sta @reverse0+1         ; SMC
@reverse0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL0,x

    ; Charset byte 1: (col1 >> 4) | (col0 << 4)
    lda (COL1_PTR),y
    lsr
    lsr
    lsr
    lsr
    sta @ora1+1             ; SMC
    lda (COL0_PTR),y
    asl
    asl
    asl
    asl
@ora1:
    ora #$FF                ; SMC
    sta CHARSET_COL1,x
    sta @reverse1+1         ; SMC
@reverse1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL1,x

    ; Charset byte 2: (col2 >> 4) | (col1 << 4)
    lda (COL2_PTR),y
    lsr
    lsr
    lsr
    lsr
    sta @ora2+1             ; SMC
    lda (COL1_PTR),y
    asl
    asl
    asl
    asl
@ora2:
    ora #$FF                ; SMC
    sta CHARSET_COL2,x
    sta @reverse2+1         ; SMC
@reverse2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL2,x

    ; Advance Y by 3 to sprite bytes
    iny
    iny
    iny

    ; Calculate sprite offset from Y: sprite_byte = Y/2
    tya                     ; Get Y (0, 6, 12, ..., 96)
    lsr                     ; Y/2 (carry clear since Y always even)
    stx @restore_x+1        ; Save X for loop restoration
    tax                     ; X = Y/2 (sprite base offset)

    iny
    iny
    iny

    ; Sprite byte 0: (col0 >> 4) | (col2 << 4)
    lda (COL0_PTR),y
    lsr
    lsr
    lsr
    lsr
    sta @ora3+1             ; SMC
    lda (COL2_PTR),y
    asl
    asl
    asl
    asl
@ora3:
    ora #$FF                ; SMC
    sta SPRITE_DATA+3,x

    ; Sprite byte 1: (col1 >> 4) | (col0 << 4)
    lda (COL1_PTR),y
    lsr
    lsr
    lsr
    lsr
    sta @ora4+1             ; SMC
    lda (COL0_PTR),y
    asl
    asl
    asl
    asl
@ora4:
    ora #$FF                ; SMC
    sta SPRITE_DATA+4,x

    ; Sprite byte 2: (col2 >> 4) | (col1 << 4)
    lda (COL2_PTR),y
    lsr
    lsr
    lsr
    lsr
    sta @ora5+1             ; SMC
    lda (COL1_PTR),y
    asl
    asl
    asl
    asl
@ora5:
    ora #$FF                ; SMC
    sta SPRITE_DATA+5,x

    ; Advance Y by 3 to next row
    iny
    iny
    iny

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
    ; Reverse bits and reorder (rev0, rev2, rev1)
    ; Write to charset at row X (1-17)

    ; Charset byte 0: reverse(col0)
    lda (COL0_PTR),y
    sta CHARSET_REVERSE_COL0,x
    sta @reverse0+1         ; SMC
@reverse0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL0,x

    ; Charset byte 2: reverse(col1)
    lda (COL1_PTR),y
    sta CHARSET_REVERSE_COL2,x
    sta @reverse2+1         ; SMC
@reverse2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL2,x

    ; Charset byte 1: reverse(col2)
    lda (COL2_PTR),y
    sta CHARSET_REVERSE_COL1,x
    sta @reverse1+1         ; SMC
@reverse1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL1,x

    ; Advance Y by 3 to sprite bytes
    iny
    iny
    iny

    ; Calculate sprite offset from Y: sprite_byte = Y/2
    tya                     ; Get Y (0, 6, 12, ..., 96)
    lsr                     ; Y/2 (carry clear since Y always even)
    stx @restore_x+1        ; Save X for loop restoration
    tax                     ; X = Y/2 (sprite base offset)

    iny
    iny
    iny

    ; Sprite byte 0: reverse(col0)
    lda (COL0_PTR),y
    sta @load_sprite0+1     ; SMC
@load_sprite0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_DATA+3,x

    ; Sprite byte 2: reverse(col1)
    lda (COL1_PTR),y
    sta @load_sprite2+1     ; SMC
@load_sprite2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_DATA+5,x

    ; Sprite byte 1: reverse(col2)
    lda (COL2_PTR),y
    sta @load_sprite1+1     ; SMC
@load_sprite1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_DATA+4,x

    iny
    iny
    iny

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
    ; Rotate left 8px: just reorder bytes (col1, col2, col0)
    ; Write to charset at row X (1-17)

    ; Charset: reorder col0→2, col1→0, col2→1
    lda (COL0_PTR),y
    sta CHARSET_COL2,x
    sta @reverse0+1         ; SMC
@reverse0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL2,x
    lda (COL1_PTR),y
    sta CHARSET_COL0,x
    sta @reverse1+1         ; SMC
@reverse1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL0,x
    lda (COL2_PTR),y
    sta CHARSET_COL1,x
    sta @reverse2+1         ; SMC
@reverse2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_REVERSE_COL1,x

    ; Advance Y by 3 to sprite bytes
    iny
    iny
    iny

    ; Calculate sprite offset from Y: sprite_byte = Y/2
    tya                     ; Get Y (0, 6, 12, ..., 96)
    lsr                     ; Y/2 (carry clear since Y always even)
    stx @restore_x+1        ; Save X for loop restoration
    tax                     ; X = Y/2 (sprite base offset)

    iny
    iny
    iny

    ; Sprite: reorder col0→2, col1→0, col2→1
    lda (COL0_PTR),y
    sta SPRITE_DATA+5,x
    lda (COL1_PTR),y
    sta SPRITE_DATA+3,x
    lda (COL2_PTR),y
    sta SPRITE_DATA+4,x

    iny
    iny
    iny

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    bne @row_loop
    rts
.endproc

; ─── Phase 4: H-mirror + rotate left 4px ─────────────────────────────────────
.proc phase4_copy
@row_loop:
    ; Rotate left 4px: reverse((B << 4) | (A >> 4))
    ; Shift unreversed bytes, combine, then reverse once before storing

    ; Charset[0]: reverse((col1 << 4) | (col2 >> 4))
    lda (COL1_PTR),y
    asl
    asl
    asl
    asl
    sta @ora0+1             ; SMC
    lda (COL2_PTR),y
    lsr
    lsr
    lsr
    lsr
@ora0:
    ora #$FF                ; SMC
    sta CHARSET_REVERSE_COL0,x
    sta @reverse0+1         ; SMC
@reverse0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL0,x

    ; Charset[1]: reverse((col0 << 4) | (col1 >> 4))
    lda (COL0_PTR),y
    asl
    asl
    asl
    asl
    sta @ora1+1             ; SMC
    lda (COL1_PTR),y
    lsr
    lsr
    lsr
    lsr
@ora1:
    ora #$FF                ; SMC
    sta CHARSET_REVERSE_COL1,x
    sta @reverse1+1         ; SMC
@reverse1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL1,x

    ; Charset[2]: reverse((col2 << 4) | (col0 >> 4))
    lda (COL2_PTR),y
    asl
    asl
    asl
    asl
    sta @ora2+1             ; SMC
    lda (COL0_PTR),y
    lsr
    lsr
    lsr
    lsr
@ora2:
    ora #$FF                ; SMC
    sta CHARSET_REVERSE_COL2,x
    sta @reverse2+1         ; SMC
@reverse2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta CHARSET_COL2,x

    ; Advance Y by 3 to sprite bytes
    iny
    iny
    iny

    ; Calculate sprite offset from Y: sprite_byte = Y/2
    tya                     ; Get Y (0, 6, 12, ..., 96)
    lsr                     ; Y/2 (carry clear since Y always even)
    stx @restore_x+1        ; Save X for loop restoration
    tax                     ; X = Y/2 (sprite base offset)

    iny
    iny
    iny

    ; Sprite[0]: reverse((col1 << 4) | (col2 >> 4))
    lda (COL1_PTR),y
    asl
    asl
    asl
    asl
    sta @ora3+1             ; SMC
    lda (COL2_PTR),y
    lsr
    lsr
    lsr
    lsr
@ora3:
    ora #$FF                ; SMC
    sta @load0+1            ; SMC
@load0:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_DATA+3,x

    ; Sprite[1]: reverse((col0 << 4) | (col1 >> 4))
    lda (COL0_PTR),y
    asl
    asl
    asl
    asl
    sta @ora4+1             ; SMC
    lda (COL1_PTR),y
    lsr
    lsr
    lsr
    lsr
@ora4:
    ora #$FF                ; SMC
    sta @load1+1            ; SMC
@load1:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_DATA+4,x

    ; Sprite[2]: reverse((col2 << 4) | (col0 >> 4))
    lda (COL2_PTR),y
    asl
    asl
    asl
    asl
    sta @ora5+1             ; SMC
    lda (COL0_PTR),y
    lsr
    lsr
    lsr
    lsr
@ora5:
    ora #$FF                ; SMC
    sta @load2+1            ; SMC
@load2:
    lda BIT_REVERSE_TABLE   ; SMC
    sta SPRITE_DATA+5,x

    ; Advance Y by 3 to next row
    iny
    iny
    iny

@restore_x:
    ldx #$FF                ; SMC: restored row counter
    inx
    cpx #18
    beq @phase4_done
    jmp @row_loop
@phase4_done:
    rts
.endproc
