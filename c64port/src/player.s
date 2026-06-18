; SID tracker player
;
; Replays the tracker ROM format produced by tools/emit_asm.go.
; Call player_init once at startup, player_tick once per PAL frame.
;
; CIA1 Timer A drives waveform toggling for voice 1 (channel 0).
; CIA1 Timer B drives waveform toggling for voice 2 (channel 1).
; Voice 3 (channel 2) never uses CIA toggling.
;
; Stream reads use self-modifying absolute LDA: the 2-byte address operand
; inside each read instruction is patched from ZP_STREAM_LO/HI before use.
;
; Zero-page layout (15 bytes):
;   ZP_STREAM_LO+ch  (ch=0,1,2): stream / block-body read pointer lo
;   ZP_STREAM_HI+ch             : stream / block-body read pointer hi
;   ZP_RETURN_LO+ch             : stream resume pointer lo (0 = reading stream)
;   ZP_RETURN_HI+ch             : stream resume pointer hi
;   ZP_BLOCK_CTR+ch             : opcodes remaining in current block (0 = stream)

.include "include/shared.inc"

; ── Hardware addresses ────────────────────────────────────────────────────────

SID         = $D400
SID_FREQ_LO = 0
SID_FREQ_HI = 1
SID_PW_LO   = 2
SID_PW_HI   = 3
SID_CTRL    = 4
SID_ATT_DEC = 5
SID_SUS_REL = 6
SID_VOL     = $D418

CIA1_TA_LO  = $DC04
CIA1_TA_HI  = $DC05
CIA1_TB_LO  = $DC06
CIA1_TB_HI  = $DC07
CIA1_ICR    = $DC0D
CIA1_CRA    = $DC0E
CIA1_CRB    = $DC0F

; ── Zero-page ─────────────────────────────────────────────────────────────────

.segment "ZEROPAGE"

ZP_STREAM_LO: .res 3
              .res 3
ZP_STREAM_HI = ZP_STREAM_LO + 3
ZP_RETURN_LO: .res 3
              .res 3
ZP_RETURN_HI = ZP_RETURN_LO + 3
ZP_BLOCK_CTR: .res 3

; ── BSS ───────────────────────────────────────────────────────────────────────

.segment "BSS"

; Per-frame SID voice state: [FreqLo FreqHi PWLo PWHi CtrlA CtrlB AttDec SusRel GateTransition] × 3
PLAYER_FRAME:      .res 27

; CIA toggle periods (uint16 LE, 0 = CIA not running for this channel)
PLAYER_TOGGLE_LO:  .res 2
PLAYER_TOGGLE_HI:  .res 2

; CIA phase: 0 = hi-phase (CtrlA active), non-zero = lo-phase (CtrlB)
PLAYER_CIA_PHASE:  .res 2

; Block count (derived at init)
PLAYER_BLOCK_COUNT: .res 1

; Scratch variables shared by player routines
PL_CH:        .res 1   ; current channel (0/1/2)
PL_TMP:       .res 1   ; scratch byte
PL_BLK_IDX:  .res 1   ; block index being entered
PL_BLK_LO:   .res 1   ; current block pointer lo
PL_BLK_HI:   .res 1   ; current block pointer hi

; ── ROM-derived compile-time constants ────────────────────────────────────────
; Computed from section offsets: notePatCount = (NOTE_PATS_size - freqModCount) / 2
; With offsets 613, 963, freqModCount=58: (963-613-58)/2 = 146
; NOTE_PAT_FREQ_BASE = MUSIC_NOTE_PATS (freqIdx array)
; NOTE_PAT_PACK_BASE = MUSIC_NOTE_PATS + 146 (packed instrID|durIdx array)

.segment "CODE"

NOTE_PAT_FREQ_BASE = MUSIC_NOTE_PATS
NOTE_PAT_PACK_BASE = MUSIC_NOTE_PATS + 146

.export player_init
.export player_tick
.export player_cia1a_fire
.export player_cia1b_fire

; ─────────────────────────────────────────────────────────────────────────────
; player_init
; ─────────────────────────────────────────────────────────────────────────────
.proc player_init
    ldx #24
    lda #0
@clr:
    sta SID,x
    dex
    bpl @clr

    lda #$0F
    sta SID_VOL

    lda #0
    sta CIA1_CRA
    sta CIA1_CRB
    lda #$7F
    sta CIA1_ICR

    lda #0
    sta PLAYER_CIA_PHASE
    sta PLAYER_CIA_PHASE+1
    sta PLAYER_TOGGLE_LO
    sta PLAYER_TOGGLE_LO+1
    sta PLAYER_TOGGLE_HI
    sta PLAYER_TOGGLE_HI+1

    ; Count blocks by walking BLOCKS section
    lda #<MUSIC_BLOCKS
    sta ZP_STREAM_LO
    lda #>MUSIC_BLOCKS
    sta ZP_STREAM_HI
    lda #0
    sta PLAYER_BLOCK_COUNT
@count:
    lda ZP_STREAM_HI
    cmp #>MUSIC_STREAM_A
    bcc @count_entry
    bne @count_done
    lda ZP_STREAM_LO
    cmp #<MUSIC_STREAM_A
    bcs @count_done
@count_entry:
    inc PLAYER_BLOCK_COUNT
    ldy #0
    lda (ZP_STREAM_LO),y
    clc
    adc #1
    adc ZP_STREAM_LO
    sta ZP_STREAM_LO
    bcc @count
    inc ZP_STREAM_HI
    jmp @count
@count_done:

    lda #<MUSIC_STREAM_A
    sta ZP_STREAM_LO+0
    lda #>MUSIC_STREAM_A
    sta ZP_STREAM_HI+0
    lda #<MUSIC_STREAM_B
    sta ZP_STREAM_LO+1
    lda #>MUSIC_STREAM_B
    sta ZP_STREAM_HI+1
    lda #<MUSIC_STREAM_C
    sta ZP_STREAM_LO+2
    lda #>MUSIC_STREAM_C
    sta ZP_STREAM_HI+2

    lda #0
    sta ZP_RETURN_LO+0
    sta ZP_RETURN_LO+1
    sta ZP_RETURN_LO+2
    sta ZP_RETURN_HI+0
    sta ZP_RETURN_HI+1
    sta ZP_RETURN_HI+2
    sta ZP_BLOCK_CTR+0
    sta ZP_BLOCK_CTR+1
    sta ZP_BLOCK_CTR+2
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; player_tick  — call once per PAL frame
; ─────────────────────────────────────────────────────────────────────────────
.proc player_tick
    ldx #0
@loop:
    jsr tick_channel
    inx
    cpx #3
    bne @loop
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; tick_channel  (X = channel)
; Fetches next stream entry; on note writes freq+ctrl+ADSR to SID voice.
; On rest: gate off, clear toggle period.
; ─────────────────────────────────────────────────────────────────────────────
.proc tick_channel
    stx PL_CH
    jsr fetch_next          ; A = byte, C=0 note / C=1 rest
    bcc @note
    jmp @rest
@note:
    ; ── note: A = pid ────────────────────────────────────────────────────────
    ; pid in A; use X for pid lookups, then switch X to voice_base for SID stores
    tax                     ; X = pid

    lda NOTE_PAT_FREQ_BASE,x
    asl
    tay                     ; Y = freqIdx * 2
    lda NOTE_PAT_PACK_BASE,x
    pha                     ; push packed byte (instrID<<4|durIdx)

    ldx PL_CH
    lda pl_voice_base,x     ; 0, 7, or 14
    sta PL_BLK_LO           ; save voice_base (reuse scratch, fetch_next is done)
    tax                     ; X = voice_base for SID stores

    lda MUSIC_FREQ_TABLE,y
    sta SID+SID_FREQ_LO,x
    lda MUSIC_FREQ_TABLE+1,y
    sta SID+SID_FREQ_HI,x
    lda #$00
    sta SID+SID_PW_LO,x
    lda #$08
    sta SID+SID_PW_HI,x

    ; instrID*4 → Y (instrument record index)
    pla
    lsr
    lsr
    lsr
    lsr                     ; A = instrID
    asl
    asl                     ; A = instrID*4
    tay

    ; wave ctrl: bits 7-5 of INSTRUMENTS[y] = waveCompact (0-3)
    lda MUSIC_INSTRUMENTS,y
    lsr
    lsr
    lsr
    lsr
    lsr                     ; waveCompact in bits 1-0
    and #3
    tax
    lda pl_wave_ctrl,x      ; $11/$21/$41/$81
    pha                     ; save ctrl byte
    ldx PL_BLK_LO           ; X = voice_base
    pla
    sta SID+SID_CTRL,x

    ; AttDec: DS byte at INSTRUMENTS+1,y
    ; bit 0 = attack (0→A=0, 1→A=$10), bit 3 = decay (0→D=0, 1→D=9)
    lda MUSIC_INSTRUMENTS+1,y
    pha                     ; save DS byte
    and #1
    beq @no_atk
    lda #$10
    jmp @have_ad
@no_atk:
    lda #0
@have_ad:
    sta PL_TMP
    pla                     ; restore DS byte
    lsr
    lsr
    lsr
    and #1
    beq @no_dec
    lda #9
    jmp @have_d
@no_dec:
    lda #0
@have_d:
    ora PL_TMP
    pha
    ldx PL_BLK_LO
    pla
    sta SID+SID_ATT_DEC,x

    ; SusRel: bits 2-1 of DS byte = sustain index; R=0
    lda MUSIC_INSTRUMENTS+1,y
    lsr
    and #3
    tax
    lda pl_sustain_vals,x
    asl
    asl
    asl
    asl
    pha
    ldx PL_BLK_LO
    pla
    sta SID+SID_SUS_REL,x
    rts

@rest:
    ldx PL_CH
    lda pl_voice_base,x
    tax                     ; X = voice_base
    lda SID+SID_CTRL,x
    and #$FE
    sta SID+SID_CTRL,x
    ldx PL_CH               ; X = channel for TOGGLE arrays
    lda #0
    sta PLAYER_TOGGLE_LO,x
    sta PLAYER_TOGGLE_HI,x
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; fetch_next  (X = channel)
; Returns: A = pid (C=0) or restDurIdx (C=1).
; ─────────────────────────────────────────────────────────────────────────────
.proc fetch_next
    stx PL_CH
    lda ZP_BLOCK_CTR,x
    beq @stream
    jmp @from_block

@stream:
    ; ── stream read ──────────────────────────────────────────────────────────
    lda ZP_STREAM_LO,x
    sta @srd+1
    lda ZP_STREAM_HI,x
    sta @srd+2
@srd:
    lda $0000               ; self-modified

    ; advance stream pointer
    inc ZP_STREAM_LO,x
    bne @sp_ok
    inc ZP_STREAM_HI,x
@sp_ok:

    cmp #$EE
    bcs @s_rest             ; b >= 0xEE → rest
    cmp PLAYER_BLOCK_COUNT
    bcs @s_note             ; b >= blockCount → direct note (pid = b - blockCount)

    ; ── enter block ──────────────────────────────────────────────────────────
    sta PL_BLK_IDX          ; save block index
    ; Save return address (already past the block-ref byte)
    lda ZP_STREAM_LO,x
    sta ZP_RETURN_LO,x
    lda ZP_STREAM_HI,x
    sta ZP_RETURN_HI,x

    ; Seek to block: sum BLOCK_LENGTHS[0..idx-1] from MUSIC_BLOCKS base
    lda #<MUSIC_BLOCKS
    sta PL_BLK_LO
    lda #>MUSIC_BLOCKS
    sta PL_BLK_HI
    ldy PL_BLK_IDX
    beq @blk_found
    ldx #0
@blk_seek:
    clc
    lda MUSIC_BLOCK_LENGTHS,x
    adc PL_BLK_LO
    sta PL_BLK_LO
    bcc @sk_hi_ok
    inc PL_BLK_HI
@sk_hi_ok:
    inx
    dey
    bne @blk_seek

@blk_found:
    ; PL_BLK_LO/HI → block length byte; body starts 1 past it
    lda PL_BLK_LO
    sta @brd+1
    lda PL_BLK_HI
    sta @brd+2
@brd:
    lda $0000               ; read block length

    ldx PL_CH
    sta ZP_BLOCK_CTR,x

    clc
    lda PL_BLK_LO
    adc #1
    sta ZP_STREAM_LO,x
    lda PL_BLK_HI
    adc #0
    sta ZP_STREAM_HI,x

    jmp fetch_next          ; tail call: read first byte from block body

@s_rest:
    ; restDurIdx = b - 0xEE
    sec
    sbc #$EE
    sec                     ; C=1 rest
    rts

@s_note:
    ; pid = b - blockCount
    sec
    sbc PLAYER_BLOCK_COUNT
    clc                     ; C=0 note
    rts

    ; ── block body read ───────────────────────────────────────────────────────
@from_block:
    lda ZP_STREAM_LO,x
    sta @bbd+1
    lda ZP_STREAM_HI,x
    sta @bbd+2
@bbd:
    lda $0000               ; self-modified

    ; advance block pointer
    inc ZP_STREAM_LO,x
    bne @bb_ok
    inc ZP_STREAM_HI,x
@bb_ok:

    dec ZP_BLOCK_CTR,x
    bne @bb_not_done
    ; restore stream pointer
    lda ZP_RETURN_LO,x
    sta ZP_STREAM_LO,x
    lda ZP_RETURN_HI,x
    sta ZP_STREAM_HI,x
    lda #0
    sta ZP_RETURN_LO,x
    sta ZP_RETURN_HI,x
@bb_not_done:

    ; decode block byte: b >= 0xEE → rest (durIdx = 0xFF - b); else pid
    pha                     ; save b
    cmp #$EE
    pla
    bcs @blk_rest
    clc                     ; C=0 note, A=pid
    rts
@blk_rest:
    ; durIdx = 0xFF - b = ~b (bitwise complement)
    eor #$FF
    sec                     ; C=1 rest
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; player_cia1a_fire
; ─────────────────────────────────────────────────────────────────────────────
.proc player_cia1a_fire
    lda PLAYER_CIA_PHASE
    bne @lo

@hi:
    lda PLAYER_FRAME + 4
    sta SID+SID_CTRL
    lda #1
    sta PLAYER_CIA_PHASE
    jmp @reload

@lo:
    lda PLAYER_FRAME + 5
    sta SID+SID_CTRL
    lda #0
    sta PLAYER_CIA_PHASE

@reload:
    lda PLAYER_TOGGLE_LO
    sta CIA1_TA_LO
    lda PLAYER_TOGGLE_HI
    sta CIA1_TA_HI
    lda #$11
    sta CIA1_CRA
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; player_cia1b_fire
; ─────────────────────────────────────────────────────────────────────────────
.proc player_cia1b_fire
    lda PLAYER_CIA_PHASE+1
    bne @lo

@hi:
    lda PLAYER_FRAME + 9 + 4
    sta SID+7+SID_CTRL
    lda #1
    sta PLAYER_CIA_PHASE+1
    jmp @reload

@lo:
    lda PLAYER_FRAME + 9 + 5
    sta SID+7+SID_CTRL
    lda #0
    sta PLAYER_CIA_PHASE+1

@reload:
    lda PLAYER_TOGGLE_LO+1
    sta CIA1_TB_LO
    lda PLAYER_TOGGLE_HI+1
    sta CIA1_TB_HI
    lda #$11
    sta CIA1_CRB
    rts
.endproc

; ── Tables ────────────────────────────────────────────────────────────────────

pl_voice_base:
    .byte 0, 7, 14

pl_sustain_vals:
    .byte 10, 12, 14, 15

pl_wave_ctrl:
    .byte $11, $21, $41, $81
