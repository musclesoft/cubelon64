; Animation playback with 4-phase copy optimization
; Precomputes frames 0-10 at -4px offset, then plays back 41 frames using transformations
;
; Memory layout:
;   - Precomputed frames: 11 frames × 128 bytes (102 used: 17 rows × 6 bytes, 26 padding)
;   - Bit reversal table: 256 bytes for horizontal mirroring
;
; 4-phase playback:
;   Phase 1 (frames 0-10):   Rotate right 4px to undo -4px storage
;   Phase 2 (frames 11-20):  Horizontal mirror + rotate right 8px (byte reorder)
;   Phase 3 (frames 21-31):  Rotate left 8px (byte reorder)
;   Phase 4 (frames 32-40):  Horizontal mirror + rotate left 4px

.include "include/shared_imports.inc"

; Constants
CHARSET_RAM = $6000          ; VIC bank 1 + $2000
SPRITE_DATA = $6800          ; VIC bank 1 + $2800

.segment "CODE"

; External functions
.import render_square: absolute

; Export animation functions
.export precompute_frames
.export copy_frame_to_charset

; ─── Precompute frames 0-10 at -4px offset ──────────────────────────────────
; Called once at startup to fill PRECOMPUTED_FRAMES
; Simpler approach: just call render_square 11 times and manually copy each
.proc precompute_frames
    lda #0
    sta FRAME
@frame_loop:
    ; Render current frame (already at -4px due to pivot in rotation.s)
    jsr render_square

    ; Copy rows 1-17 from charset/sprite to precomputed storage
    ; Manual unroll for simplicity - each frame copies 17 rows × 6 bytes
    ; Destination offset = FRAME × 128

    ; For frames 0-10, just manually copy all the data
    ; This is tedious but straightforward

    ldy #1
@copy_rows:
    ; Get destination base for this frame (frame × 128)
    ; We'll compute the destination address for each byte
    ; dest = PRECOMPUTED_FRAMES + (FRAME × 128) + ((row-1) × 6) + byte_offset

    ; For now, skip the complex indexing and just store sequentially
    ; We'll fix the copy logic to be simpler

    iny
    cpy #18
    bne @copy_rows

    ; Next frame
    inc FRAME
    lda FRAME
    cmp #11
    bcc @frame_loop

    lda #0
    sta FRAME
    rts
.endproc

; ─── Copy and transform frame to charset ─────────────────────────────────────
; Input: FRAME (0-40)
; Output: CHARSET_RAM and SPRITE_DATA updated
;
; For now, just fall back to calling render_square directly
; TODO: Implement full 4-phase copy optimization
.proc copy_frame_to_charset
    ; Temporary implementation: just call render_square
    ; This doesn't do the optimization yet but keeps the code working
    jsr render_square
    rts
.endproc

; ─── Data segments ───────────────────────────────────────────────────────────

.segment "BSS"

; Precomputed frame storage: 11 frames × 128 bytes
; Each frame stores rows 1-17, 6 bytes per row (102 bytes used, 26 padding)
; 128-byte stride allows efficient frame selection via shift
PRECOMPUTED_FRAMES: .res 11 * 128

.segment "RODATA"

; Bit reversal lookup table for horizontal mirroring (256 bytes)
; Maps each byte to its bit-reversed value
; On 6502: LDA table,X is 4 cycles
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
