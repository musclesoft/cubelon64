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
; Zero-page layout (18 bytes, SoA per-channel arrays):
;   ZP_STREAM_LO+ch  (ch=0,1,2): stream / block-body read pointer lo
;   ZP_STREAM_HI+ch             : stream / block-body read pointer hi
;   ZP_RETURN_LO+ch             : stream resume pointer lo (0 = reading stream)
;   ZP_RETURN_HI+ch             : stream resume pointer hi
;   ZP_BLOCK_CTR+ch             : opcodes remaining in current block (0 = stream)
;   ZP_DUR_CTR+ch               : frames remaining in current note/rest (0 = fetch next)

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
ZP_DUR_CTR:   .res 3   ; frames remaining (0 = fetch next stream byte)

; ── BSS ───────────────────────────────────────────────────────────────────────

.segment "BSS"

; Per-frame SID voice state: [FreqLo FreqHi PWLo PWHi CtrlA CtrlB AttDec SusRel GateTransition] × 3
PLAYER_FRAME:      .res 27

; CIA toggle periods (uint16 LE, 0 = CIA not running for this channel)
PLAYER_TOGGLE_LO:  .res 2
PLAYER_TOGGLE_HI:  .res 2

; CIA phase: 0 = hi-phase (CtrlA active), non-zero = lo-phase (CtrlB)
PLAYER_CIA_PHASE:  .res 2

; Block count (set at init from MUSIC_BLOCK_COUNT constant)
PLAYER_BLOCK_COUNT: .res 1

; Per-channel rest flag (0=note, 1=rest) for sustain logic
PLAYER_IS_REST:    .res 3

; Scratch variables shared by player routines
PL_CH:        .res 1   ; current channel (0/1/2)
PL_VBASE:     .res 1   ; voice base for current channel (0, 7, or 14)
PL_FBASE:     .res 1   ; frame base for current channel (0, 9, or 18)
PL_TMP:       .res 1   ; scratch byte
PL_BLK_IDX:  .res 1   ; block index being entered
PL_BLK_LO:   .res 1   ; current block pointer lo
PL_BLK_HI:   .res 1   ; current block pointer hi

; ── ROM-derived compile-time constants ────────────────────────────────────────
; NOTE_PAT_FREQ_BASE and NOTE_PAT_PACK_BASE are emitted by tools/emit_asm.go
; into music_data.inc, which must be included before this file.

.segment "CODE"

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

    lda #MUSIC_BLOCK_COUNT
    sta PLAYER_BLOCK_COUNT

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
    sta ZP_DUR_CTR+0
    sta ZP_DUR_CTR+1
    sta ZP_DUR_CTR+2
    sta PLAYER_IS_REST+0
    sta PLAYER_IS_REST+1
    sta PLAYER_IS_REST+2
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
; On every frame: decrement duration counter.
;   If counter was > 0: sustain (re-write freq+ctrl or keep gate off).
;   If counter was 0: fetch next stream entry, set new counter, write SID.
; ─────────────────────────────────────────────────────────────────────────────
.proc tick_channel
    stx PL_CH

    ; Set up voice_base and frame_base for this channel
    lda pl_voice_base,x
    sta PL_VBASE
    lda pl_frame_base,x
    sta PL_FBASE

    ; Check duration counter — if > 0, sustain current note/rest
    lda ZP_DUR_CTR,x
    beq @fetch              ; 0 → need next opcode
    dec ZP_DUR_CTR,x

    ; Sustain: re-write SID registers to maintain state visible to emulator snapshot
    lda PLAYER_IS_REST,x
    bne @sustain_rest

@sustain_note:
    ; Re-write freq and ctrl from PLAYER_FRAME (stride-9 indexed by PL_FBASE)
    ldx PL_FBASE
    lda PLAYER_FRAME+0,x    ; saved FreqLo
    ldx PL_VBASE
    sta SID+SID_FREQ_LO,x
    ldx PL_FBASE
    lda PLAYER_FRAME+1,x    ; saved FreqHi
    ldx PL_VBASE
    sta SID+SID_FREQ_HI,x
    ldx PL_FBASE
    lda PLAYER_FRAME+4,x    ; saved CtrlA
    ldx PL_VBASE
    sta SID+SID_CTRL,x
    ldx PL_CH
    rts

@sustain_rest:
    ; Keep gate off
    ldx PL_VBASE
    lda SID+SID_CTRL,x
    and #$FE
    sta SID+SID_CTRL,x
    ldx PL_CH
    rts

@fetch:
    jsr fetch_next          ; A = pid (C=0) or restDurIdx (C=1)
    bcc @note
    jmp @rest

@note:
    ; ── note start: A = pid ──────────────────────────────────────────────────
    tax                     ; X = pid

    lda NOTE_PAT_FREQ_BASE,x
    sta PL_BLK_IDX          ; save freqIdx for CIA toggle lookup
    asl
    tay                     ; Y = freqIdx * 2
    lda NOTE_PAT_PACK_BASE,x
    pha                     ; push packed byte (instrID<<4|durIdx)

    ldx PL_VBASE            ; X = voice_base for SID stores

    ; Write freq to SID and save to PLAYER_FRAME
    lda MUSIC_FREQ_TABLE,y
    sta SID+SID_FREQ_LO,x
    sta PL_TMP              ; save FreqLo
    lda MUSIC_FREQ_TABLE+1,y
    sta SID+SID_FREQ_HI,x
    pha                     ; save FreqHi on stack
    lda #$00
    sta SID+SID_PW_LO,x
    lda #$08
    sta SID+SID_PW_HI,x

    ; instrID*4 → Y (instrument record index)
    ; packed byte is on stack; pull it but push again to keep durIdx available
    pla                     ; FreqHi off stack
    sta PL_BLK_LO           ; save FreqHi to scratch
    pla                     ; packed byte
    pha                     ; push packed back (for durIdx later)
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
    ldx PL_VBASE
    pla
    sta SID+SID_CTRL,x

    ; Save freq+ctrl to PLAYER_FRAME for sustain (re-read from SID after writes)
    ldx PL_VBASE
    lda SID+SID_FREQ_LO,x
    sta PL_TMP              ; FreqLo
    lda SID+SID_FREQ_HI,x
    sta PL_BLK_LO           ; FreqHi
    lda SID+SID_CTRL,x      ; CtrlA
    ldx PL_FBASE
    pha                     ; CtrlA on stack
    lda PL_TMP
    sta PLAYER_FRAME+0,x    ; FreqLo
    lda PL_BLK_LO
    sta PLAYER_FRAME+1,x    ; FreqHi
    pla                     ; CtrlA
    sta PLAYER_FRAME+4,x    ; CtrlA

    ; CtrlB (lo-phase): $51 if envelope mode (instrument flag bit 0), else 0
    lda MUSIC_INSTRUMENTS,y
    and #1                  ; envelope mode flag
    beq @ctrlb_none
    lda #$51                ; pulse+triangle+gate (lo-phase for envelope toggle)
    bne @ctrlb_store
@ctrlb_none:
    lda #0
@ctrlb_store:
    sta PLAYER_FRAME+5,x    ; CtrlB

    ; AttDec: DS byte at INSTRUMENTS+1,y
    lda MUSIC_INSTRUMENTS+1,y
    pha                     ; save DS byte
    and #1
    beq @no_atk
    lda #$10
    jmp @have_ad
@no_atk:
    lda #0
@have_ad:
    sta PL_TMP              ; attack nibble
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
    ldx PL_VBASE
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
    ldx PL_VBASE
    sta SID+SID_SUS_REL,x

    ; Set duration counter from DUR_TABLE[durIdx]
    pla                     ; pop packed byte
    and #$0F                ; durIdx
    tax
    lda MUSIC_DUR_TABLE,x
    ldx PL_CH
    sta ZP_DUR_CTR,x
    dec ZP_DUR_CTR,x        ; first frame consumed now
    lda #0
    sta PLAYER_IS_REST,x

    ; Arm CIA toggle timer (ch0→CIA1A, ch1→CIA1B; ch2 has no toggle)
    cpx #2
    beq @no_cia

    ; Toggle period lookup: TOGGLE_IDX[freqIdx/2] gives a byte of two nibbles;
    ; even freqIdx uses low nibble, odd uses high nibble. Nibble indexes TOGGLE_VALS.
    lda PL_BLK_IDX          ; freqIdx
    lsr                     ; A = freqIdx/2, C = freqIdx&1
    tax
    lda MUSIC_TOGGLE_IDX,x  ; nibble-pair byte
    bcc @ti_lo
    lsr
    lsr
    lsr
    lsr                     ; odd freqIdx: shift high nibble down
@ti_lo:
    and #$0F                ; nibble (0-15)
    asl                     ; nibble*2 = byte offset into TOGGLE_VALS
    tax
    lda MUSIC_TOGGLE_VALS,x
    sta PL_TMP              ; toggle period lo
    lda MUSIC_TOGGLE_VALS+1,x
    sta PL_BLK_LO           ; toggle period hi

    ; Store into PLAYER_TOGGLE for CIA handlers to use on reload
    ldx PL_CH
    lda PL_TMP
    sta PLAYER_TOGGLE_LO,x
    lda PL_BLK_LO
    sta PLAYER_TOGGLE_HI,x

    ; Skip arming if period is zero (no toggle on this note)
    ora PL_TMP
    beq @no_cia

    ; Arm: ch0→CIA1A, ch1→CIA1B
    cpx #0
    beq @arm_a
    lda PL_TMP
    sta CIA1_TB_LO
    lda PL_BLK_LO
    sta CIA1_TB_HI
    lda #$82                ; SET bit, enable Timer B IRQ source
    sta CIA1_ICR
    lda #$11
    sta CIA1_CRB
    lda #0
    sta PLAYER_CIA_PHASE+1
    beq @no_cia             ; always taken (Z=1 after sta #0)
@arm_a:
    lda PL_TMP
    sta CIA1_TA_LO
    lda PL_BLK_LO
    sta CIA1_TA_HI
    lda #$81                ; SET bit, enable Timer A IRQ source
    sta CIA1_ICR
    lda #$11
    sta CIA1_CRA
    lda #0
    sta PLAYER_CIA_PHASE
@no_cia:
    rts

@rest:
    ; ── rest start: A = restDurIdx ───────────────────────────────────────────
    tax                     ; X = restDurIdx
    lda MUSIC_REST_DURS,x
    ldx PL_CH
    sta ZP_DUR_CTR,x
    dec ZP_DUR_CTR,x        ; first rest frame consumed now
    lda #1
    sta PLAYER_IS_REST,x

    ldx PL_VBASE
    lda SID+SID_CTRL,x
    and #$FE
    sta SID+SID_CTRL,x
    ldx PL_CH
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

    pha                     ; save block byte before any A modification

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
    pla                     ; restore block byte
    cmp #$EE
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

pl_frame_base:
    .byte 0, 9, 18          ; PLAYER_FRAME stride-9 offset per channel

pl_sustain_vals:
    .byte 10, 12, 14, 15

pl_wave_ctrl:
    .byte $11, $21, $41, $81
