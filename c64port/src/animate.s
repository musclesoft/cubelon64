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
;   Phase 4 (frames 32-40): src=41-frame, H-mirror + rotate left 4px
; Source frames alternate: forward, reverse, forward, reverse for smooth animation

.include "include/shared_imports.inc"

; Constants
CHARSET_RAM = $6000          ; VIC bank 1 + $2000
SPRITE_DATA = $6800          ; VIC bank 1 + $2800

.segment "BSS"

; Precomputed frame storage: 11 frames × 128 bytes
PRECOMPUTED_FRAMES: .res 11 * 128

; Temporary storage
TEMP_COL0: .res 1
TEMP_COL1: .res 1
TEMP_COL2: .res 1
TEMP_MED0: .res 1
TEMP_MED1: .res 1
TEMP_MED2: .res 1
TEMP_SCRATCH: .res 1
SRC_FRAME: .res 1

.segment "CODE"

; External functions
.import calc_rotation: absolute
.import render_square: absolute

; Export animation functions
.export precompute_frames
.export copy_frame_to_charset

; ─── Helper: Get next byte from precomputed data ─────────────────────────────
; Returns byte in A, auto-increments internal address
; Must call set_source_addr first to initialize
get_next_byte:
gnb_load:
    lda $FFFF               ; SMC: address set by set_source_addr
    inc gnb_load+1
    bne @skip
    inc gnb_load+2
@skip:
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
    lsr
    lsr
    lsr
    lsr
    lsr
    lsr                     ; High byte of frame × 128
    clc
    adc #>PRECOMPUTED_FRAMES
    sta @addr_hi
    lda @addr_lo
    clc
    adc #<PRECOMPUTED_FRAMES
    sta @addr_lo
    bcc :+
    inc @addr_hi
:
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
    lsr
    lsr
    lsr
    lsr
    lsr
    lsr
    clc
    adc #>PRECOMPUTED_FRAMES
    sta @addr_hi
    lda @addr_lo
    clc
    adc #<PRECOMPUTED_FRAMES
    sta @addr_lo
    bcc :+
    inc @addr_hi
:
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

    rts

@frame: .res 1
@row: .res 1
@addr_lo: .res 1
@addr_hi: .res 1
@offset: .res 1
@temp: .res 1
.endproc

; ─── Precompute frames 0-10 ──────────────────────────────────────────────────
.proc precompute_frames
    lda #0
    sta FRAME
@frame_loop:
    jsr calc_rotation       ; Compute corners before rendering
    jsr render_square

    ; Copy rows 1-17 to precomputed storage
    ldx #1
@row_loop:
    lda FRAME
    jsr set_dest_addr

    ; Store 6 bytes
    lda CHARSET_RAM,x
    jsr store_next_byte
    lda CHARSET_RAM+64,x
    jsr store_next_byte
    lda CHARSET_RAM+128,x
    jsr store_next_byte
    lda SPRITE_DATA,x
    jsr store_next_byte
    lda SPRITE_DATA+64,x
    jsr store_next_byte
    lda SPRITE_DATA+128,x
    jsr store_next_byte

    inx
    cpx #18
    bne @row_loop

    inc FRAME
    lda FRAME
    cmp #11
    bcc @frame_loop

    lda #0
    sta FRAME
    rts
.endproc

; ─── Copy and transform frame to charset ─────────────────────────────────────
.proc copy_frame_to_charset
    ; Initialize background
    lda #$FF
    sta CHARSET_RAM+0
    sta CHARSET_RAM+64
    sta CHARSET_RAM+128
    sta SPRITE_DATA+0
    sta SPRITE_DATA+1
    sta SPRITE_DATA+2
    ldx #18
@init_bg:
    sta CHARSET_RAM,x
    sta CHARSET_RAM+64,x
    sta CHARSET_RAM+128,x
    sta SPRITE_DATA,x
    sta SPRITE_DATA+64,x
    sta SPRITE_DATA+128,x
    inx
    cpx #24
    bne @init_bg

    ; Determine phase and source frame
    lda FRAME
    cmp #11
    bcc @phase1
    cmp #21
    bcc @phase2
    cmp #32
    bcc @phase3
    jmp @phase4

@phase1:
    ; Source = FRAME
    sta SRC_FRAME
    jmp phase1_copy

@phase2:
    ; Source = 20 - FRAME
    lda #20
    sec
    sbc FRAME
    sta SRC_FRAME
    jmp phase2_copy

@phase3:
    ; Source = FRAME - 20 (or 40 - FRAME for frame 31)
    lda FRAME
    sec
    sbc #20
    cmp #11
    bcc @p3_normal
    lda #40
    sec
    sbc FRAME
@p3_normal:
    sta SRC_FRAME
    jmp phase3_copy

@phase4:
    ; Source = 41 - FRAME
    lda #41
    sec
    sbc FRAME
    sta SRC_FRAME
    jmp phase4_copy
.endproc

; ─── Phase 1: Rotate right 4px ───────────────────────────────────────────────
.proc phase1_copy
    ldx #1
@row_loop:
    ; Set up source address
    lda SRC_FRAME
    jsr set_source_addr

    ; Load 6 bytes
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

    ; Rotate right 4px: (col0 >> 4) | (col2 << 4)
    lda TEMP_COL0
    lsr
    lsr
    lsr
    lsr
    sta TEMP_SCRATCH
    lda TEMP_COL2
    asl
    asl
    asl
    asl
    ora TEMP_SCRATCH
    sta CHARSET_RAM,x

    lda TEMP_COL1
    lsr
    lsr
    lsr
    lsr
    sta TEMP_SCRATCH
    lda TEMP_COL0
    asl
    asl
    asl
    asl
    ora TEMP_SCRATCH
    sta CHARSET_RAM+64,x

    lda TEMP_COL2
    lsr
    lsr
    lsr
    lsr
    sta TEMP_SCRATCH
    lda TEMP_COL1
    asl
    asl
    asl
    asl
    ora TEMP_SCRATCH
    sta CHARSET_RAM+128,x

    ; Same for medium
    lda TEMP_MED0
    lsr
    lsr
    lsr
    lsr
    sta TEMP_SCRATCH
    lda TEMP_MED2
    asl
    asl
    asl
    asl
    ora TEMP_SCRATCH
    sta SPRITE_DATA,x

    lda TEMP_MED1
    lsr
    lsr
    lsr
    lsr
    sta TEMP_SCRATCH
    lda TEMP_MED0
    asl
    asl
    asl
    asl
    ora TEMP_SCRATCH
    sta SPRITE_DATA+64,x

    lda TEMP_MED2
    lsr
    lsr
    lsr
    lsr
    sta TEMP_SCRATCH
    lda TEMP_MED1
    asl
    asl
    asl
    asl
    ora TEMP_SCRATCH
    sta SPRITE_DATA+128,x

    inx
    cpx #18
    beq @done
    jmp @row_loop
@done:
    rts
.endproc

; ─── Phase 2: H-mirror + rotate right 8px ────────────────────────────────────
.proc phase2_copy
    ldx #1
@row_loop:
    lda SRC_FRAME
    jsr set_source_addr

    ; Load 6 bytes
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

    ; Reverse bits and reorder (rev2, rev0, rev1)
    ldy TEMP_COL2
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM,x

    ldy TEMP_COL0
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM+64,x

    ldy TEMP_COL1
    lda BIT_REVERSE_TABLE,y
    sta CHARSET_RAM+128,x

    ldy TEMP_MED2
    lda BIT_REVERSE_TABLE,y
    sta SPRITE_DATA,x

    ldy TEMP_MED0
    lda BIT_REVERSE_TABLE,y
    sta SPRITE_DATA+64,x

    ldy TEMP_MED1
    lda BIT_REVERSE_TABLE,y
    sta SPRITE_DATA+128,x

    inx
    cpx #18
    beq @done
    jmp @row_loop
@done:
    rts
.endproc

; ─── Phase 3: Rotate left 8px ────────────────────────────────────────────────
.proc phase3_copy
    ldx #1
@row_loop:
    lda SRC_FRAME
    jsr set_source_addr

    ; Load 6 bytes and directly store with reorder
    jsr get_next_byte       ; col0
    sta CHARSET_RAM+128,x   ; → col 2 (rotate left)
    jsr get_next_byte       ; col1
    sta CHARSET_RAM,x       ; → col 0
    jsr get_next_byte       ; col2
    sta CHARSET_RAM+64,x    ; → col 1
    jsr get_next_byte       ; med0
    sta SPRITE_DATA+128,x
    jsr get_next_byte       ; med1
    sta SPRITE_DATA,x
    jsr get_next_byte       ; med2
    sta SPRITE_DATA+64,x

    inx
    cpx #18
    beq @done
    jmp @row_loop
@done:
    rts
.endproc

; ─── Phase 4: H-mirror + rotate left 4px ─────────────────────────────────────
.proc phase4_copy
    ldx #1
@row_loop:
    lda SRC_FRAME
    jsr set_source_addr

    ; Load 6 bytes
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

    ; Reverse bits: rev(col2), rev(col1), rev(col0)
    ldy TEMP_COL2
    lda BIT_REVERSE_TABLE,y
    sta TEMP_COL2           ; Now holds rev(col2) = rev0
    ldy TEMP_COL1
    lda BIT_REVERSE_TABLE,y
    sta TEMP_COL1           ; Now holds rev(col1) = rev1
    ldy TEMP_COL0
    lda BIT_REVERSE_TABLE,y
    sta TEMP_COL0           ; Now holds rev(col0) = rev2

    ; Rotate left 4px: (rev0 << 4) | (rev1 >> 4)
    lda TEMP_COL2           ; rev0
    asl
    asl
    asl
    asl
    sta TEMP_SCRATCH
    lda TEMP_COL1           ; rev1
    lsr
    lsr
    lsr
    lsr
    ora TEMP_SCRATCH
    sta CHARSET_RAM,x

    lda TEMP_COL1           ; rev1
    asl
    asl
    asl
    asl
    sta TEMP_SCRATCH
    lda TEMP_COL0           ; rev2
    lsr
    lsr
    lsr
    lsr
    ora TEMP_SCRATCH
    sta CHARSET_RAM+64,x

    lda TEMP_COL0           ; rev2
    asl
    asl
    asl
    asl
    sta TEMP_SCRATCH
    lda TEMP_COL2           ; rev0
    lsr
    lsr
    lsr
    lsr
    ora TEMP_SCRATCH
    sta CHARSET_RAM+128,x

    ; Same for medium
    ldy TEMP_MED2
    lda BIT_REVERSE_TABLE,y
    sta TEMP_MED2
    ldy TEMP_MED1
    lda BIT_REVERSE_TABLE,y
    sta TEMP_MED1
    ldy TEMP_MED0
    lda BIT_REVERSE_TABLE,y
    sta TEMP_MED0

    lda TEMP_MED2
    asl
    asl
    asl
    asl
    sta TEMP_SCRATCH
    lda TEMP_MED1
    lsr
    lsr
    lsr
    lsr
    ora TEMP_SCRATCH
    sta SPRITE_DATA,x

    lda TEMP_MED1
    asl
    asl
    asl
    asl
    sta TEMP_SCRATCH
    lda TEMP_MED0
    lsr
    lsr
    lsr
    lsr
    ora TEMP_SCRATCH
    sta SPRITE_DATA+64,x

    lda TEMP_MED0
    asl
    asl
    asl
    asl
    sta TEMP_SCRATCH
    lda TEMP_MED2
    lsr
    lsr
    lsr
    lsr
    ora TEMP_SCRATCH
    sta SPRITE_DATA+128,x

    inx
    cpx #18
    beq @done
    jmp @row_loop
@done:
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
