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

CIA1_PRB    = $DC01     ; bit6 = Timer A toggle output, bit7 = Timer B toggle output
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

; Block count (set at init from MUSIC_BLOCK_COUNT constant)
PLAYER_BLOCK_COUNT: .res 1

; Per-channel done flag: set when stream exhausted, channel produces silence
PLAYER_DONE:      .res 3

; Per-channel rest flag (0=note, 1=rest) for sustain logic
PLAYER_IS_REST:    .res 3

; Gate-reset state per channel (persists across rests, matching the SID's
; own ADSR generator state, which isn't reset by a rest):
;   HAS_PREV_NOTE:  0 until the first note has played (skip reset on the very
;                   first note — nothing to reset from)
;   PREV_ENV_MODE:  envelope-mode flag of the last note that played
PLAYER_HAS_PREV_NOTE: .res 3
PLAYER_PREV_ENV_MODE: .res 3

; Sustain bitstream state per channel (stride-3 SoA):
;   SUS_PTR_LO/HI: pointer to current byte in body; == SUS_END when exhausted
;   SUS_END_LO/HI: pointer one past last body byte
;   SUS_BITPOS:    current bit position within body byte (0-7)
;   SUS_VAL:       current sustain nibble (written as SusRel = val << 4)
PLAYER_SUS_PTR_LO: .res 3
PLAYER_SUS_PTR_HI: .res 3
PLAYER_SUS_END_LO: .res 3
PLAYER_SUS_END_HI: .res 3
PLAYER_SUS_BITPOS: .res 3
PLAYER_SUS_VAL:    .res 3
PLAYER_ATTDEC_TAIL: .res 3  ; (attack<<4)|decay to write when tail fires
PLAYER_TAIL_CTR:    .res 3  ; tail fire countdown; 0 = no tail/already fired
PLAYER_ATTDEC_PRE:  .res 3  ; (attack<<4)|attack — pre-tail value; written every sustain frame
PLAYER_ATTDEC_NF1:  .res 3  ; attack<<4 to write on nf=1 if attack>0, else $FF (one-shot)

; FM state per channel (stride-3 SoA):
;   FM_TYPE:     compact FM type (0/1/3/4) or $FF=none
;   FM_PTR_LO/HI: current position in FM sequence/gliss data
;   FM_END_LO/HI: end of FM data (seq/gliss boundary check)
;   FM_HOLD:     frames remaining in hold phase (DV/gliss: 12, counts to 0)
;   FM_PHASE:    DV phase counter 0-7
;   FM_FREQ_LO/HI: current FM output frequency (updated each sustain frame)
;   FM_BASE_LO/HI: note base frequency (for DV delta computation)
;   FM_CYC_LO/HI: headCycle base pointer (for wrap)
PLAYER_FM_TYPE:    .res 3
PLAYER_FM_PTR_LO:  .res 3
PLAYER_FM_PTR_HI:  .res 3
PLAYER_FM_END_LO:  .res 3
PLAYER_FM_END_HI:  .res 3
PLAYER_FM_HOLD:    .res 3
PLAYER_FM_PHASE:   .res 3
PLAYER_FM_FREQ_LO: .res 3
PLAYER_FM_FREQ_HI: .res 3
PLAYER_FM_BASE_LO: .res 3
PLAYER_FM_BASE_HI: .res 3
PLAYER_FM_CYC_LO:  .res 3
PLAYER_FM_CYC_HI:  .res 3

; NPT (NoisePlusTone) wave+toggle override state per channel:
;   NPT_ACTIVE:  1 if override in effect, 0 otherwise
;   NPT_WAVE_LO/HI: ptr to waveByteLen byte in NPT_WAVE_SEQS
;   NPT_TOG_LO/HI:  ptr to current toggle uint16 in NPT_WAVE_SEQS
;   NPT_TOG_FC:     frames remaining in toggle sequence (0 = hold last)
;   NPT_TOG_LST_LO/HI: last toggle value (held when past end)
;   NF:              note frame counter (0-based, per channel)
PLAYER_NPT_ACTIVE:      .res 3
PLAYER_NPT_WAVE_LO:     .res 3
PLAYER_NPT_WAVE_HI:     .res 3
PLAYER_NPT_TOG_LO:      .res 3
PLAYER_NPT_TOG_HI:      .res 3
PLAYER_NPT_TOG_FC:      .res 3
PLAYER_NPT_TOG_LST_LO:  .res 3
PLAYER_NPT_TOG_LST_HI:  .res 3
PLAYER_NF:              .res 3  ; note frame counter (used for NPT wave indexing)

; Tone sequence state per channel (stride-3 SoA):
;   TSEQ_ACTIVE: 1 if tone seq is in effect for this channel, 0 otherwise
;   TSEQ_PTR_LO/HI: pointer to next (delta, hold) pair byte in MUSIC_TONE_SEQS
;   TSEQ_END_LO/HI: pointer one past last byte of this entry (no more steps when PTR >= END)
;   TSEQ_HOLD: frames remaining at current step (0 = already applied, no more steps)
;   TSEQ_PERIOD_LO/HI: current CIA period (uint16 LE)
PLAYER_TSEQ_ACTIVE:    .res 3
PLAYER_TSEQ_PTR_LO:    .res 3
PLAYER_TSEQ_PTR_HI:    .res 3
PLAYER_TSEQ_END_LO:    .res 3
PLAYER_TSEQ_END_HI:    .res 3
PLAYER_TSEQ_HOLD:      .res 3
PLAYER_TSEQ_PERIOD_LO: .res 3
PLAYER_TSEQ_PERIOD_HI: .res 3

; Scratch variables shared by player routines
PL_CH:        .res 1   ; current channel (0/1/2)
PL_VBASE:     .res 1   ; voice base for current channel (0, 7, or 14)
PL_FBASE:     .res 1   ; frame base for current channel (0, 9, or 18)
PL_TMP:       .res 1   ; scratch byte
PL_BLK_IDX:  .res 1   ; block index being entered
PL_BLK_LO:   .res 1   ; current block pointer lo
PL_BLK_HI:   .res 1   ; current block pointer hi
PL_PID:       .res 1   ; note pattern index (pid), saved at note-start

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
    sta PLAYER_DONE+0
    sta PLAYER_DONE+1
    sta PLAYER_DONE+2
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
    sta PLAYER_HAS_PREV_NOTE+0
    sta PLAYER_HAS_PREV_NOTE+1
    sta PLAYER_HAS_PREV_NOTE+2
    sta PLAYER_PREV_ENV_MODE+0
    sta PLAYER_PREV_ENV_MODE+1
    sta PLAYER_PREV_ENV_MODE+2
    sta PLAYER_SUS_PTR_LO+0
    sta PLAYER_SUS_PTR_LO+1
    sta PLAYER_SUS_PTR_LO+2
    sta PLAYER_SUS_PTR_HI+0
    sta PLAYER_SUS_PTR_HI+1
    sta PLAYER_SUS_PTR_HI+2
    sta PLAYER_SUS_END_LO+0
    sta PLAYER_SUS_END_LO+1
    sta PLAYER_SUS_END_LO+2
    sta PLAYER_SUS_END_HI+0
    sta PLAYER_SUS_END_HI+1
    sta PLAYER_SUS_END_HI+2
    sta PLAYER_SUS_BITPOS+0
    sta PLAYER_SUS_BITPOS+1
    sta PLAYER_SUS_BITPOS+2
    sta PLAYER_SUS_VAL+0
    sta PLAYER_SUS_VAL+1
    sta PLAYER_SUS_VAL+2
    sta PLAYER_ATTDEC_TAIL+0
    sta PLAYER_ATTDEC_TAIL+1
    sta PLAYER_ATTDEC_TAIL+2
    sta PLAYER_TAIL_CTR+0
    sta PLAYER_TAIL_CTR+1
    sta PLAYER_TAIL_CTR+2
    sta PLAYER_ATTDEC_PRE+0
    sta PLAYER_ATTDEC_PRE+1
    sta PLAYER_ATTDEC_PRE+2
    lda #$FF
    sta PLAYER_ATTDEC_NF1+0
    sta PLAYER_ATTDEC_NF1+1
    sta PLAYER_ATTDEC_NF1+2
    sta PLAYER_FM_TYPE+0
    sta PLAYER_FM_TYPE+1
    sta PLAYER_FM_TYPE+2
    lda #0
    sta PLAYER_FM_PTR_LO+0
    sta PLAYER_FM_PTR_LO+1
    sta PLAYER_FM_PTR_LO+2
    sta PLAYER_FM_PTR_HI+0
    sta PLAYER_FM_PTR_HI+1
    sta PLAYER_FM_PTR_HI+2
    sta PLAYER_FM_END_LO+0
    sta PLAYER_FM_END_LO+1
    sta PLAYER_FM_END_LO+2
    sta PLAYER_FM_END_HI+0
    sta PLAYER_FM_END_HI+1
    sta PLAYER_FM_END_HI+2
    sta PLAYER_FM_HOLD+0
    sta PLAYER_FM_HOLD+1
    sta PLAYER_FM_HOLD+2
    sta PLAYER_FM_PHASE+0
    sta PLAYER_FM_PHASE+1
    sta PLAYER_FM_PHASE+2
    sta PLAYER_FM_FREQ_LO+0
    sta PLAYER_FM_FREQ_LO+1
    sta PLAYER_FM_FREQ_LO+2
    sta PLAYER_FM_FREQ_HI+0
    sta PLAYER_FM_FREQ_HI+1
    sta PLAYER_FM_FREQ_HI+2
    sta PLAYER_FM_BASE_LO+0
    sta PLAYER_FM_BASE_LO+1
    sta PLAYER_FM_BASE_LO+2
    sta PLAYER_FM_BASE_HI+0
    sta PLAYER_FM_BASE_HI+1
    sta PLAYER_FM_BASE_HI+2
    sta PLAYER_FM_CYC_LO+0
    sta PLAYER_FM_CYC_LO+1
    sta PLAYER_FM_CYC_LO+2
    sta PLAYER_FM_CYC_HI+0
    sta PLAYER_FM_CYC_HI+1
    sta PLAYER_FM_CYC_HI+2
    sta PLAYER_NPT_ACTIVE+0
    sta PLAYER_NPT_ACTIVE+1
    sta PLAYER_NPT_ACTIVE+2
    sta PLAYER_NPT_WAVE_LO+0
    sta PLAYER_NPT_WAVE_LO+1
    sta PLAYER_NPT_WAVE_LO+2
    sta PLAYER_NPT_WAVE_HI+0
    sta PLAYER_NPT_WAVE_HI+1
    sta PLAYER_NPT_WAVE_HI+2
    sta PLAYER_NPT_TOG_LO+0
    sta PLAYER_NPT_TOG_LO+1
    sta PLAYER_NPT_TOG_LO+2
    sta PLAYER_NPT_TOG_HI+0
    sta PLAYER_NPT_TOG_HI+1
    sta PLAYER_NPT_TOG_HI+2
    sta PLAYER_NPT_TOG_FC+0
    sta PLAYER_NPT_TOG_FC+1
    sta PLAYER_NPT_TOG_FC+2
    sta PLAYER_NPT_TOG_LST_LO+0
    sta PLAYER_NPT_TOG_LST_LO+1
    sta PLAYER_NPT_TOG_LST_LO+2
    sta PLAYER_NPT_TOG_LST_HI+0
    sta PLAYER_NPT_TOG_LST_HI+1
    sta PLAYER_NPT_TOG_LST_HI+2
    sta PLAYER_NF+0
    sta PLAYER_NF+1
    sta PLAYER_NF+2
    sta PLAYER_TSEQ_ACTIVE+0
    sta PLAYER_TSEQ_ACTIVE+1
    sta PLAYER_TSEQ_ACTIVE+2
    sta PLAYER_TSEQ_PTR_LO+0
    sta PLAYER_TSEQ_PTR_LO+1
    sta PLAYER_TSEQ_PTR_LO+2
    sta PLAYER_TSEQ_PTR_HI+0
    sta PLAYER_TSEQ_PTR_HI+1
    sta PLAYER_TSEQ_PTR_HI+2
    sta PLAYER_TSEQ_END_LO+0
    sta PLAYER_TSEQ_END_LO+1
    sta PLAYER_TSEQ_END_LO+2
    sta PLAYER_TSEQ_END_HI+0
    sta PLAYER_TSEQ_END_HI+1
    sta PLAYER_TSEQ_END_HI+2
    sta PLAYER_TSEQ_HOLD+0
    sta PLAYER_TSEQ_HOLD+1
    sta PLAYER_TSEQ_HOLD+2
    sta PLAYER_TSEQ_PERIOD_LO+0
    sta PLAYER_TSEQ_PERIOD_LO+1
    sta PLAYER_TSEQ_PERIOD_LO+2
    sta PLAYER_TSEQ_PERIOD_HI+0
    sta PLAYER_TSEQ_PERIOD_HI+1
    sta PLAYER_TSEQ_PERIOD_HI+2
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
    bne @sustaining
    jmp @fetch
@sustaining:
    dec ZP_DUR_CTR,x

    ; Sustain: re-write SID registers to maintain state visible to emulator snapshot
    lda PLAYER_IS_REST,x
    beq @sustain_note
    jmp @sustain_rest

@sustain_note:
    ; Advance sustain bitstream and re-write SusRel
    ldx PL_CH
    jsr step_sustain        ; advances PLAYER_SUS_VAL[ch] if bitstream not exhausted
    ldx PL_CH
    lda PLAYER_SUS_VAL,x
    asl
    asl
    asl
    asl                     ; val << 4
    ldx PL_VBASE
    sta SID+SID_SUS_REL,x

    ; Advance FM and re-write freq
    ldx PL_CH
    jsr step_fm             ; updates PLAYER_FM_FREQ_LO/HI[ch]; clobbers A,X
    ldx PL_CH
    lda PLAYER_FM_FREQ_LO,x
    ldx PL_VBASE
    sta SID+SID_FREQ_LO,x
    ldx PL_CH
    lda PLAYER_FM_FREQ_HI,x
    ldx PL_VBASE
    sta SID+SID_FREQ_HI,x
    ; Advance NPT wave/toggle sequence (increments PLAYER_NF, updates PLAYER_FRAME ctrl)
    ldx PL_CH
    inc PLAYER_NF,x
    jsr step_npt
    ; Re-write ctrl from PLAYER_FRAME (stride-9 indexed by PL_FBASE)
    ; PW must track CtrlA's waveform: 0x800 only while pure pulse ($40) is
    ; active, else 0xFFF — a CIA fire in a previous frame or a different
    ; waveform's sustain frame may have left PW at the wrong value.
    ldx PL_FBASE
    lda PLAYER_FRAME+4,x    ; saved CtrlA (may have been updated by step_npt)
    pha
    and #$F0
    cmp #$40
    bne @sus_pw_default
    lda #$00
    ldx PL_VBASE
    sta SID+SID_PW_LO,x
    lda #$08
    sta SID+SID_PW_HI,x
    jmp @sus_pw_done
@sus_pw_default:
    lda #$FF
    ldx PL_VBASE
    sta SID+SID_PW_LO,x
    lda #$0F
    sta SID+SID_PW_HI,x
@sus_pw_done:
    pla
    ldx PL_VBASE
    sta SID+SID_CTRL,x
    ; Advance tone sequence and update PLAYER_TOGGLE (env-mode notes with tone override)
    ldx PL_CH
    jsr step_toneseq
    ; Apply any new toggle period to the live CIA latch (ch2 has no CIA)
    lda PL_CH
    cmp #2
    beq @attdec_start
    jsr apply_toggle_period
@attdec_start:
    ; AttDec: re-write every sustain frame.
    ; nf=1 special: ATTDEC_NF1 holds attack<<4 (one-shot); $FF means skip.
    ; After nf=1, write ATTDEC_PRE = (attack<<4)|attack each frame.
    ; On tail fire, write ATTDEC_TAIL and stop.
    ldx PL_CH
    lda PLAYER_TAIL_CTR,x
    beq @attdec_done        ; tail already fired (CTR=0), SID keeps tail value
    ; nf=1 check
    lda PLAYER_ATTDEC_NF1,x
    cmp #$FF
    beq @attdec_pre
    ; nf=1: write attack<<4, clear one-shot
    ldx PL_VBASE
    sta SID+SID_ATT_DEC,x
    ldx PL_CH
    lda #$FF
    sta PLAYER_ATTDEC_NF1,x
    jmp @attdec_tail_check
@attdec_pre:
    ; Normal pre-tail: write (attack<<4)|attack
    lda PLAYER_ATTDEC_PRE,x
    ldx PL_VBASE
    sta SID+SID_ATT_DEC,x
    ldx PL_CH
@attdec_tail_check:
    ; Decrement tail counter; fire when it hits 0
    dec PLAYER_TAIL_CTR,x
    bne @attdec_done
    lda PLAYER_ATTDEC_TAIL,x
    ldx PL_VBASE
    sta SID+SID_ATT_DEC,x
    ldx PL_CH
@attdec_done:
    rts

@sustain_rest:
    ldx PL_VBASE
    lda #0
    sta SID+SID_CTRL,x
    ldx PL_CH
    rts

@fetch:
    ; Check if stream is exhausted (no more blocks and stream ptr at end)
    ldx PL_CH
    lda PLAYER_DONE,x
    bne @done_silence
    jsr check_stream_done   ; sets PLAYER_DONE[PL_CH] if exhausted
    ldx PL_CH
    lda PLAYER_DONE,x
    beq @fetch_go
@done_silence:
    ; Stream exhausted: output silence (gate off)
    ldx PL_VBASE
    lda #0
    sta SID+SID_CTRL,x
    ldx PL_FBASE
    lda #0
    sta PLAYER_FRAME+4,x    ; CtrlA = gate off
    sta PLAYER_FRAME+5,x    ; CtrlB = same — CIA alternation is then a no-op
    ldx PL_CH
    rts
@fetch_go:
    jsr fetch_next          ; A = pid (C=0) or restDurIdx (C=1)
    bcc @note
    jmp @rest

@note:
    ; ── note start: A = pid ──────────────────────────────────────────────────
    tax                     ; X = pid
    sta PL_PID              ; save pid for FM init later

    lda NOTE_PAT_FREQ_BASE,x
    sta PL_BLK_IDX          ; save freqIdx for CIA toggle lookup
    asl
    tay                     ; Y = freqIdx * 2
    lda NOTE_PAT_PACK_BASE,x
    pha                     ; push packed byte (instrID<<4|durIdx)

    ldx PL_VBASE            ; X = voice_base for SID stores

    ; Write freq to SID and save to PLAYER_FRAME; also save as FM base freq
    lda MUSIC_FREQ_TABLE,y
    sta SID+SID_FREQ_LO,x   ; X = PL_VBASE here
    sta PL_TMP              ; save FreqLo
    lda MUSIC_FREQ_TABLE+1,y
    sta SID+SID_FREQ_HI,x
    pha                     ; save FreqHi on stack
    ; Save base freq for FM
    ldx PL_CH
    lda PL_TMP
    sta PLAYER_FM_BASE_LO,x
    sta PLAYER_FM_FREQ_LO,x
    pla                     ; FreqHi
    sta PLAYER_FM_BASE_HI,x
    sta PLAYER_FM_FREQ_HI,x
    pha                     ; push FreqHi back on stack
    ldx PL_VBASE            ; restore X = voice base for SID stores

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

    ; Gate reset / test pulse: the SID's ADSR generator does not restart the
    ; attack phase unless gate transitions 0→1, so back-to-back notes (gate
    ; held at 1 throughout) would otherwise inherit whatever envelope level
    ; the previous note left behind.
    lda MUSIC_INSTRUMENTS,y
    and #1                  ; envelope mode flag (current note)
    sta PL_BLK_HI            ; save current-note envelope flag (read again below)
    ldx PL_CH
    lda PLAYER_HAS_PREV_NOTE,x
    beq @gate_update         ; first note ever: nothing to reset from
    lda PLAYER_PREV_ENV_MODE,x
    and PL_BLK_HI
    beq @gate_reset          ; not (prev envelope AND current envelope) → full reset
    ; TestPulse: both envelope-mode — pulse the test bit (0x08) on the
    ; current ctrl register without touching gate; resets the oscillator
    ; phase without the full envelope restart a gate cycle would cause.
    ldx PL_VBASE
    lda SID+SID_CTRL,x
    ora #$08
    sta SID+SID_CTRL,x
    lda SID+SID_CTRL,x
    and #$F7
    sta SID+SID_CTRL,x
    jmp @gate_update
@gate_reset:
    ; GateReset: force gate off so the next write (gate on, below) is a
    ; clean 0→1 transition.
    ldx PL_VBASE
    lda SID+SID_CTRL,x
    and #$FE
    sta SID+SID_CTRL,x
@gate_update:
    ldx PL_CH
    lda #1
    sta PLAYER_HAS_PREV_NOTE,x
    lda PL_BLK_HI
    sta PLAYER_PREV_ENV_MODE,x

    ; wave ctrl: bits 7-5 of INSTRUMENTS[y] = waveCompact (0-3)
    lda MUSIC_INSTRUMENTS,y
    lsr
    lsr
    lsr
    lsr
    lsr                     ; waveCompact in bits 1-0
    and #3
    sta PL_BLK_HI            ; save waveCompact index (2 = pulse)
    tax
    lda pl_wave_ctrl,x      ; $11/$21/$41/$81
    pha                     ; save ctrl byte
    ; Pulse width: 0x800 (50% duty) only while a pure square/pulse wave is
    ; active (waveCompact index 2 → $40); 0xFFF (silent/no PWM effect)
    ; for every other waveform, including 0x05 (pulse+triangle).
    ldx PL_VBASE
    lda PL_BLK_HI
    cmp #2
    bne @note_pw_default
    lda #$00
    sta SID+SID_PW_LO,x
    lda #$08
    sta SID+SID_PW_HI,x
    jmp @note_pw_done
@note_pw_default:
    lda #$FF
    sta SID+SID_PW_LO,x
    lda #$0F
    sta SID+SID_PW_HI,x
@note_pw_done:
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
    sta PL_TMP              ; save CtrlA for the mirror-into-CtrlB default below

    ; CtrlB (lo-phase): $51 if envelope mode (instrument flag bit 0),
    ; else same as CtrlA — CIA alternation is then a harmless no-op.
    lda MUSIC_INSTRUMENTS,y
    and #1                  ; envelope mode flag
    beq @ctrlb_mirror
    lda #$51                ; pulse+triangle+gate (lo-phase for envelope toggle)
    bne @ctrlb_store
@ctrlb_mirror:
    lda PL_TMP              ; CtrlA
@ctrlb_store:
    sta PLAYER_FRAME+5,x    ; CtrlB

    ; AttDec: DS byte at INSTRUMENTS+1,y
    ; AttDec_init = (attack<<4)|attack  (hold A phase while attack is active)
    ; AttDec_tail = (attack<<4)|decay   (written when tail fires)
    lda MUSIC_INSTRUMENTS+1,y
    pha                     ; save DS byte
    and #1
    beq @no_atk
    lda #$10
    jmp @have_ad
@no_atk:
    lda #0
@have_ad:
    sta PL_TMP              ; attack nibble ($00 or $10)
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
    ; A = decay nibble (0 or 9), PL_TMP = attack nibble ($00 or $10)
    ora PL_TMP              ; A = AttDec_tail = (attack<<4)|decay
    ldx PL_CH
    sta PLAYER_ATTDEC_TAIL,x ; save for tail fire
    ; Store tail countdown from instrument byte 2
    lda MUSIC_INSTRUMENTS+2,y
    sta PLAYER_TAIL_CTR,x
    ; Set ATTDEC_NF1: attack<<4 if attack>0, else $FF (nf=1 special case)
    lda PL_TMP              ; attack nibble ($10 or $00)
    bne @nf1_atk
    lda #$FF
@nf1_atk:
    sta PLAYER_ATTDEC_NF1,x
    ; Write AttDec_init = (attack<<4)|attack to SID now
    lda PL_TMP              ; attack nibble
    beq @zero_atk_init
    lda #$11                ; attack=1 in both nibbles
    jmp @have_init
@zero_atk_init:
    lda #0
@have_init:
    ldx PL_CH
    sta PLAYER_ATTDEC_PRE,x   ; pre-tail value for sustain re-write
    ldx PL_VBASE
    sta SID+SID_ATT_DEC,x

    ; SusRel init: load sustain start value and init bitstream.
    ; Y = instrID*4 throughout. PL_CH and PL_VBASE valid.
    lda MUSIC_INSTRUMENTS+3,y  ; sus_def_idx
    sta PL_TMP                  ; save sus_def_idx
    lda MUSIC_INSTRUMENTS,y     ; merged: flags in bits 2-0
    and #1                      ; envelope mode bit
    beq @sus_no_env
    jmp @sus_static
@sus_no_env:
    lda PL_TMP                  ; sus_def_idx
    cmp #$FF
    bne @sus_dynamic
    jmp @sus_static             ; $FF → static
@sus_dynamic:

    ; Dynamic sustain: walk MUSIC_SUSTAIN_PATS to entry PL_TMP
    lda #<MUSIC_SUSTAIN_PATS
    sta PL_BLK_LO
    lda #>MUSIC_SUSTAIN_PATS
    sta PL_BLK_HI
    ldx PL_TMP                  ; X = count of entries to skip
    beq @sus_found
@sus_walk:
    lda PL_BLK_LO
    sta @sus_hdr+1
    lda PL_BLK_HI
    sta @sus_hdr+2
@sus_hdr:
    lda $0000                   ; self-modified: read hdr byte at PL_BLK_LO/HI
    and #$3F                    ; body length
    clc
    adc #1                      ; +1 for hdr byte
    adc PL_BLK_LO
    sta PL_BLK_LO
    bcc @sus_adv_ok
    inc PL_BLK_HI
@sus_adv_ok:
    dex
    bne @sus_walk

@sus_found:
    ; PL_BLK_LO/HI → sustain entry header byte
    lda PL_BLK_LO
    sta @sus_hdr2+1
    lda PL_BLK_HI
    sta @sus_hdr2+2
@sus_hdr2:
    lda $0000                   ; self-modified: hdr byte
    pha                         ; save hdr
    lsr
    lsr
    lsr
    lsr
    lsr
    lsr                         ; hdr >> 6 = start value index (0-3)
    tax
    lda pl_sustain_vals,x       ; initial sustain nibble
    ldx PL_CH
    sta PLAYER_SUS_VAL,x

    ; body_start = entry_header_addr + 1 (= @sus_hdr2 operand + 1)
    lda @sus_hdr2+1
    clc
    adc #1
    sta PLAYER_SUS_PTR_LO,x
    lda @sus_hdr2+2
    adc #0
    sta PLAYER_SUS_PTR_HI,x
    lda #0
    sta PLAYER_SUS_BITPOS,x

    ; body_end = body_start + body_len (hdr & $3F)
    pla                         ; restore hdr
    and #$3F                    ; body length
    clc
    adc PLAYER_SUS_PTR_LO,x
    sta PLAYER_SUS_END_LO,x
    lda PLAYER_SUS_PTR_HI,x
    adc #0
    sta PLAYER_SUS_END_HI,x

    ; Write initial SusRel to SID
    lda PLAYER_SUS_VAL,x
    asl
    asl
    asl
    asl
    ldx PL_VBASE
    sta SID+SID_SUS_REL,x
    jmp @sus_done

@sus_static:
    ; No bitstream: static sustain from DS bits 2-1. Mark PTR==END==0.
    ldx PL_CH
    lda MUSIC_INSTRUMENTS+1,y
    lsr
    and #3
    tax
    lda pl_sustain_vals,x
    ldx PL_CH
    sta PLAYER_SUS_VAL,x
    lda #0
    sta PLAYER_SUS_PTR_LO,x
    sta PLAYER_SUS_PTR_HI,x
    sta PLAYER_SUS_END_LO,x
    sta PLAYER_SUS_END_HI,x
    sta PLAYER_SUS_BITPOS,x
    lda PLAYER_SUS_VAL,x
    asl
    asl
    asl
    asl
    ldx PL_VBASE
    sta SID+SID_SUS_REL,x
@sus_done:

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
    bne @check_toggle
    jmp @no_cia
@check_toggle:

    ; Toggle period lookup: TOGGLE_IDX[freqIdx/2] gives a byte of two nibbles;
    ; even freqIdx uses low nibble, odd uses high nibble. Nibble indexes
    ; TOGGLE_VALS. This is always a valid nonzero period, even for notes with
    ; no real CIA alternation (CtrlA==CtrlB there, so toggling is a no-op).
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

    ; Arm: ch0→CIA1A, ch1→CIA1B. CRA/CRB=$17: LOAD+START (reload+run) plus
    ; PBON+OUTMODE=toggle (bits 1-2) so PB6/PB7 track phase in hardware.
    ; The toggle output is never reset by LOAD — it only changes on
    ; underflow — so phase is never reset at note-start either; it just
    ; carries over from wherever the previous note left it (matching real
    ; hardware and the Go reference model, which does the same).
    cpx #0
    beq @arm_a
    lda PL_TMP
    sta CIA1_TB_LO
    lda PL_BLK_LO
    sta CIA1_TB_HI
    lda #$82                ; SET bit, enable Timer B IRQ source
    sta CIA1_ICR
    lda #$17
    sta CIA1_CRB
    jmp @no_cia
@arm_a:
    lda PL_TMP
    sta CIA1_TA_LO
    lda PL_BLK_LO
    sta CIA1_TA_HI
    lda #$81                ; SET bit, enable Timer A IRQ source
    sta CIA1_ICR
    lda #$17
    sta CIA1_CRA
@no_cia:
    ; Reset note frame counter
    ldx PL_CH
    lda #0
    sta PLAYER_NF,x
    jsr init_fm_note
    jsr init_npt_note
    jsr init_toneseq_note
    ldx PL_CH
    jsr step_toneseq        ; apply first step so PLAYER_TOGGLE matches sim frame 0
    ldx PL_CH               ; step_toneseq clobbers X; player_tick's loop needs it restored
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
    lda #0
    sta SID+SID_CTRL,x
    ldx PL_FBASE
    lda #0
    sta PLAYER_FRAME+4,x    ; CtrlA = gate off
    sta PLAYER_FRAME+5,x    ; CtrlB = same — CIA alternation during rest is a no-op
    ldx PL_CH               ; player_tick's loop needs X restored before rts
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
; step_sustain  (X = channel, PL_CH = channel)
; Advance the sustain bitstream one output frame: read bits LSB-first until a
; 0-bit (= end of frame marker); decrement PLAYER_SUS_VAL for each 1-bit seen.
; If exhausted (PTR == END), returns immediately without change.
; Clobbers: A, X, PL_TMP. Caller must reload X (= PL_CH) after return.
; ─────────────────────────────────────────────────────────────────────────────
.proc step_sustain
    ; Check exhausted
    lda PLAYER_SUS_PTR_LO,x
    cmp PLAYER_SUS_END_LO,x
    bne @load_byte
    lda PLAYER_SUS_PTR_HI,x
    cmp PLAYER_SUS_END_HI,x
    bne @load_byte
    jmp @done
@load_byte:
    ; Load body byte at PTR into PL_TMP, pre-shifted by BITPOS
    lda PLAYER_SUS_PTR_LO,x
    sta @brd+1
    lda PLAYER_SUS_PTR_HI,x
    sta @brd+2
@brd:
    lda $0000               ; body byte (self-modified)
    ; Shift right by BITPOS so current bit is at bit 0 (X = channel)
    ldy PLAYER_SUS_BITPOS,x ; Y = bitpos; X keeps channel
    beq @bits_ready
@pre_shift:
    lsr
    dey
    bne @pre_shift
@bits_ready:
    ; A has body byte right-shifted to current bit position
    ; Loop: process one bit per iteration
@next_bit:
    lsr                     ; shift bit 0 into carry; A holds remaining bits
    bcc @zero_bit           ; carry=0 → this bit was 0, end of frame
    sta PL_TMP              ; save remaining bits (A is clobbered below)
    ; bit was 1: decrement sustain value
    ldx PL_CH
    lda PLAYER_SUS_VAL,x
    beq @skip_dec
    dec PLAYER_SUS_VAL,x
@skip_dec:
    ; advance bitpos
    ldx PL_CH
    inc PLAYER_SUS_BITPOS,x
    lda PLAYER_SUS_BITPOS,x
    cmp #8
    bne @same_byte          ; still in same byte
    ; wrapped to next byte
    lda #0
    sta PLAYER_SUS_BITPOS,x
    inc PLAYER_SUS_PTR_LO,x
    bne @chk_exhausted
    inc PLAYER_SUS_PTR_HI,x
@chk_exhausted:
    lda PLAYER_SUS_PTR_LO,x
    cmp PLAYER_SUS_END_LO,x
    bne @reload_byte
    lda PLAYER_SUS_PTR_HI,x
    cmp PLAYER_SUS_END_HI,x
    beq @done               ; exhausted
@reload_byte:
    lda PLAYER_SUS_PTR_LO,x
    sta @brd+1
    lda PLAYER_SUS_PTR_HI,x
    sta @brd+2
    ; bitpos just reset to 0 — read new byte from updated @brd, no pre-shift needed
    jmp @brd            ; @brd now reads new ptr; will fall into @bits_ready (X=0 → no shift)
@same_byte:
    ; PL_TMP holds the remaining body bits; restore A then process next bit
    lda PL_TMP
    jmp @next_bit
@zero_bit:
    ; 0-bit: end of frame — consume the bit and return
    ldx PL_CH
    inc PLAYER_SUS_BITPOS,x
    lda PLAYER_SUS_BITPOS,x
    cmp #8
    bne @done
    lda #0
    sta PLAYER_SUS_BITPOS,x
    inc PLAYER_SUS_PTR_LO,x
    bne @done
    inc PLAYER_SUS_PTR_HI,x
@done:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; apply_toggle_period  (PL_CH set to 0 or 1; ch2 never calls this)
; Writes PLAYER_TOGGLE_LO/HI[ch] to the channel's CIA latch. The 6526 always
; picks up a latch write on its next natural underflow, but if the new
; period is shorter than where the counter currently sits, that wait would
; be audibly late (e.g. glissando/vibrato pitch bends) — so this also forces
; an immediate reload (LOAD strobe) in that case, matching the "AY-like
; immediate frequency response" documented on the Go reference's
; CIATimer.UpdatePeriodWithImmediate. HI-byte-only compare, matching that
; reference's own 8-bit shortcut.
; Clobbers: A, X
; ─────────────────────────────────────────────────────────────────────────────
.proc apply_toggle_period
    ldx PL_CH
    lda PLAYER_TOGGLE_HI,x
    cpx #0
    beq @cia_a
    ; ch1 → CIA1 Timer B
    cmp CIA1_TB_HI          ; new period HI vs live counter HI
    lda PLAYER_TOGGLE_LO,x
    sta CIA1_TB_LO
    lda PLAYER_TOGGLE_HI,x
    sta CIA1_TB_HI
    bcs @done               ; new HI >= counter HI: natural underflow is soon enough
    lda #$17                ; LOAD+START+PBON+OUTMODE: force immediate reload
    sta CIA1_CRB
    rts
@cia_a:
    ; ch0 → CIA1 Timer A
    cmp CIA1_TA_HI          ; new period HI vs live counter HI
    lda PLAYER_TOGGLE_LO,x
    sta CIA1_TA_LO
    lda PLAYER_TOGGLE_HI,x
    sta CIA1_TA_HI
    bcs @done
    lda #$17                ; LOAD+START+PBON+OUTMODE: force immediate reload
    sta CIA1_CRA
@done:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; player_cia1a_fire
; ─────────────────────────────────────────────────────────────────────────────
; CIA1 Timer A runs in continuous mode with PBON+OUTMODE=toggle (CRA=$17):
; the 6526 auto-reloads from its own latch on every underflow and flips
; PB6 in hardware, with no CPU intervention. This handler reads that bit
; from PRB ($DC01) instead of tracking phase in software — there's no way
; to force the toggle to a known state (only underflows change it), so
; note-start never resets it either; phase just carries over continuously,
; matching real hardware and the Go reference model.
.proc player_cia1a_fire
    lda CIA1_PRB
    and #$40
    beq @lo

@hi:
    ; PW is set once at note-start and left untouched here: no CIA-armed
    ; CtrlA/CtrlB pair ever pairs pulse (0x04) with a different waveform in
    ; the other phase (NPT's mixed/noise-only states never use 0x04; an
    ; envelope-mode instrument with BaseWaveform=pulse would violate this,
    ; but none currently exist).
    lda PLAYER_FRAME + 4
    sta SID+SID_CTRL
    rts

@lo:
    lda PLAYER_FRAME + 5
    sta SID+SID_CTRL
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; player_cia1b_fire
; ─────────────────────────────────────────────────────────────────────────────
; CIA1 Timer B runs in continuous mode with PBON+OUTMODE=toggle (CRB=$17):
; see player_cia1a_fire — reads PB7 from PRB ($DC01) instead of tracking
; phase in software.
.proc player_cia1b_fire
    lda CIA1_PRB
    and #$80
    beq @lo

@hi:
    ; PW is set once at note-start and left untouched here: no CIA-armed
    ; CtrlA/CtrlB pair ever pairs pulse (0x04) with a different waveform in
    ; the other phase (NPT's mixed/noise-only states never use 0x04; an
    ; envelope-mode instrument with BaseWaveform=pulse would violate this,
    ; but none currently exist).
    lda PLAYER_FRAME + 9 + 4
    sta SID+7+SID_CTRL
    rts

@lo:
    lda PLAYER_FRAME + 9 + 5
    sta SID+7+SID_CTRL
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; check_stream_done  (PL_CH set)
; Sets PLAYER_DONE[PL_CH] = 1 if stream pointer >= stream end AND block ctr = 0.
; ─────────────────────────────────────────────────────────────────────────────
.proc check_stream_done
    ldx PL_CH
    lda ZP_BLOCK_CTR,x
    bne @not_done           ; inside a block: not done
    ; Check: ZP_STREAM_HI[ch] vs stream_end_hi[ch]
    lda ZP_STREAM_HI,x
    cmp pl_stream_end_hi,x
    bcc @not_done           ; stream hi < end hi: not done
    bne @set_done           ; stream hi > end hi: done
    ; stream hi == end hi: compare lo bytes
    lda ZP_STREAM_LO,x
    cmp pl_stream_end_lo,x
    bcc @not_done           ; stream lo < end lo: not done
@set_done:
    lda #1
    sta PLAYER_DONE,x
@not_done:
    rts
.endproc

; ── Tables ────────────────────────────────────────────────────────────────────

pl_stream_end_lo:
    .byte <MUSIC_STREAM_A_END, <MUSIC_STREAM_B_END, <MUSIC_STREAM_C_END

pl_stream_end_hi:
    .byte >MUSIC_STREAM_A_END, >MUSIC_STREAM_B_END, >MUSIC_STREAM_C_END

pl_voice_base:
    .byte 0, 7, 14

pl_frame_base:
    .byte 0, 9, 18          ; PLAYER_FRAME stride-9 offset per channel

pl_sustain_vals:
    .byte 10, 12, 14, 15

pl_wave_ctrl:
    .byte $11, $21, $41, $81

pl_dv_byte_off:
    .byte 0,2,0,0,4,6,4,0   ; DV template byte offset by phase

pl_dv_negate:
    .byte 0,0,0,0,1,1,1,0   ; DV negate flag (1=subtract delta) by phase

; ─────────────────────────────────────────────────────────────────────────────
; init_fm_note  (PL_CH, PL_PID, PLAYER_FM_BASE set by caller)
; Sets up PLAYER_FM_TYPE/PTR/END/HOLD/PHASE/FREQ/CYC for the new note.
; Clobbers: A, X, Y, PL_TMP, PL_BLK_LO, PL_BLK_HI, PL_BLK_IDX
; ─────────────────────────────────────────────────────────────────────────────
.proc init_fm_note
    lda PL_PID
    cmp #MUSIC_FREQ_MOD_COUNT
    bcc @has_mod
    ldx PL_CH
    lda #$FF
    sta PLAYER_FM_TYPE,x
    rts
@has_mod:
    ; A = pid; look up freqModIdx
    tax
    lda NOTE_PAT_FM_BASE,x  ; fmIdx for this pid
    ; look up byte offset into MUSIC_FREQ_MOD
    tax
    lda MUSIC_FM_OFFSETS,x  ; byte offset within FREQ_MOD section
    ; PL_BLK = &MUSIC_FREQ_MOD[offset]
    clc
    adc #<MUSIC_FREQ_MOD
    sta PL_BLK_LO
    lda #>MUSIC_FREQ_MOD
    adc #0
    sta PL_BLK_HI
    ; read FM header: type<<4 | nibble
    lda PL_BLK_LO
    sta @rd_hdr+1
    lda PL_BLK_HI
    sta @rd_hdr+2
@rd_hdr:
    lda $0000
    pha                     ; save full header
    lsr
    lsr
    lsr
    lsr                     ; type → bits 3-0
    ldx PL_CH
    sta PLAYER_FM_TYPE,x
    ; advance PL_BLK past header to data start
    inc PL_BLK_LO
    bne @blk_ok
    inc PL_BLK_HI
@blk_ok:
    pla                     ; restore header
    and #$0F                ; nibble
    sta PL_TMP
    ; dispatch on type
    ldx PL_CH
    lda PLAYER_FM_TYPE,x
    cmp #1
    beq @type_dv
    cmp #0
    beq @type_seq
    cmp #3
    beq @type_gliss
    cmp #4
    bne @fm_init_none
    jmp @type_headcycle
@fm_init_none:
    ; type 2 = noMod, or unknown → mark none
    lda #$FF
    sta PLAYER_FM_TYPE,x
    rts

@type_dv:
    ; DV: data[0]=dvIdx; template = MUSIC_DELAYED_VIB + dvIdx*8
    lda PL_BLK_LO
    sta @rd_dv+1
    lda PL_BLK_HI
    sta @rd_dv+2
@rd_dv:
    lda $0000               ; dvIdx
    asl
    asl
    asl                     ; dvIdx*8
    clc
    adc #<MUSIC_DELAYED_VIB
    sta PLAYER_FM_PTR_LO,x
    lda #>MUSIC_DELAYED_VIB
    adc #0
    sta PLAYER_FM_PTR_HI,x
    lda #12
    sta PLAYER_FM_HOLD,x
    lda #0
    sta PLAYER_FM_PHASE,x
    rts

@type_seq:
    ; seq: ptr = data start; end = ptr + nibble*2; freq = first entry
    lda PL_BLK_LO
    sta PLAYER_FM_PTR_LO,x
    lda PL_BLK_HI
    sta PLAYER_FM_PTR_HI,x
    ; read first freq (lo at ptr, hi at ptr+1)
    sta @rd_seqh+2
    lda PL_BLK_LO
    sta @rd_seql+1
    lda PL_BLK_HI
    sta @rd_seql+2
@rd_seql:
    lda $0000
    sta PLAYER_FM_FREQ_LO,x
    lda PL_BLK_LO
    clc
    adc #1
    sta @rd_seqh+1
@rd_seqh:
    lda $0000
    sta PLAYER_FM_FREQ_HI,x
    ; end = ptr + nibble*2
    lda PL_TMP
    asl
    clc
    adc PL_BLK_LO
    sta PLAYER_FM_END_LO,x
    lda PL_BLK_HI
    adc #0
    sta PLAYER_FM_END_HI,x
    rts

@type_gliss:
    ; holdGliss: ptr = data start; end = ptr + nibble (bytes); hold=12
    lda PL_BLK_LO
    sta PLAYER_FM_PTR_LO,x
    lda PL_BLK_HI
    sta PLAYER_FM_PTR_HI,x
    lda PL_TMP
    clc
    adc PL_BLK_LO
    sta PLAYER_FM_END_LO,x
    lda PL_BLK_HI
    adc #0
    sta PLAYER_FM_END_HI,x
    lda #12
    sta PLAYER_FM_HOLD,x
    rts

@type_headcycle:
    ; headCycle: data[0]=headLen, data[1]=cycleLen; ptr=data+2
    lda PL_BLK_LO
    sta @rd_hc0+1
    lda PL_BLK_HI
    sta @rd_hc0+2
@rd_hc0:
    lda $0000               ; headLen
    sta PL_TMP              ; save headLen
    lda PL_BLK_LO
    clc
    adc #1
    sta @rd_hc1+1
    lda PL_BLK_HI
    adc #0
    sta @rd_hc1+2
@rd_hc1:
    lda $0000               ; cycleLen
    sta PL_BLK_IDX          ; save cycleLen
    ; ptr = data + 2
    lda PL_BLK_LO
    clc
    adc #2
    sta PLAYER_FM_PTR_LO,x
    lda PL_BLK_HI
    adc #0
    sta PLAYER_FM_PTR_HI,x
    ; read first freq
    lda PLAYER_FM_PTR_LO,x
    sta @rd_hcfl+1
    lda PLAYER_FM_PTR_HI,x
    sta @rd_hcfl+2
@rd_hcfl:
    lda $0000               ; first freq lo
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_PTR_LO,x
    clc
    adc #1
    sta @rd_hcfh+1
    lda PLAYER_FM_PTR_HI,x
    adc #0
    sta @rd_hcfh+2
@rd_hcfh:
    lda $0000               ; first freq hi
    sta PLAYER_FM_FREQ_HI,x
    ; cycleBase = ptr + headLen*2
    lda PL_TMP              ; headLen
    asl                     ; headLen*2
    clc
    adc PLAYER_FM_PTR_LO,x
    sta PLAYER_FM_CYC_LO,x
    lda PLAYER_FM_PTR_HI,x
    adc #0
    sta PLAYER_FM_CYC_HI,x
    ; end = cycleBase + cycleLen*2
    lda PL_BLK_IDX          ; cycleLen
    asl
    clc
    adc PLAYER_FM_CYC_LO,x
    sta PLAYER_FM_END_LO,x
    lda PLAYER_FM_CYC_HI,x
    adc #0
    sta PLAYER_FM_END_HI,x
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; step_fm  (X = channel)
; Advances FM state by one frame and updates PLAYER_FM_FREQ_LO/HI[ch].
; Clobbers: A, X, Y, PL_TMP, PL_BLK_LO, PL_BLK_HI
; ─────────────────────────────────────────────────────────────────────────────
.proc step_fm
    lda PLAYER_FM_TYPE,x
    cmp #$FF
    beq @done_base          ; no FM: keep base freq
    cmp #1
    beq @type_dv
    cmp #0
    bne @fm_not_seq
    jmp @type_seq
@fm_not_seq:
    cmp #3
    bne @fm_not_gliss
    jmp @type_gliss
@fm_not_gliss:
    cmp #4
    bne @done_base
    jmp @type_headcycle
@done_base:
    rts

@type_dv:
    ; DV: hold 12 frames then 8-phase vibrato
    lda PLAYER_FM_HOLD,x
    beq @dv_vibrate
    dec PLAYER_FM_HOLD,x
    ; still in hold: return base freq
    lda PLAYER_FM_BASE_LO,x
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_BASE_HI,x
    sta PLAYER_FM_FREQ_HI,x
    rts
@dv_vibrate:
    lda PLAYER_FM_PHASE,x
    tay                     ; Y = phase
    cmp #3
    beq @dv_base_freq
    cmp #7
    beq @dv_base_freq
    ; compute delta: template at PLAYER_FM_PTR_LO/HI
    lda pl_dv_byte_off,y    ; byte offset into template
    clc
    adc PLAYER_FM_PTR_LO,x
    sta @dv_rd_lo+1
    lda PLAYER_FM_PTR_HI,x
    adc #0
    sta @dv_rd_lo+2
@dv_rd_lo:
    lda $0000               ; delta lo (self-modified)
    sta PL_BLK_LO
    lda @dv_rd_lo+1
    clc
    adc #1
    sta @dv_rd_lo+1
    sta @dv_rd_hi+1
    bcc @dv_hi_ok
    inc @dv_rd_lo+2
@dv_hi_ok:
    lda @dv_rd_lo+2
    sta @dv_rd_hi+2
@dv_rd_hi:
    lda $0000               ; delta hi (self-modified)
    sta PL_BLK_HI
    ; check negate flag
    lda pl_dv_negate,y
    beq @dv_add
    ; negate: two's complement of PL_BLK
    lda #0
    sec
    sbc PL_BLK_LO
    sta PL_BLK_LO
    lda #0
    sbc PL_BLK_HI
    sta PL_BLK_HI
@dv_add:
    ; freq = base + delta (signed 16-bit)
    lda PLAYER_FM_BASE_LO,x
    clc
    adc PL_BLK_LO
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_BASE_HI,x
    adc PL_BLK_HI
    sta PLAYER_FM_FREQ_HI,x
    jmp @dv_advance_phase
@dv_base_freq:
    lda PLAYER_FM_BASE_LO,x
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_BASE_HI,x
    sta PLAYER_FM_FREQ_HI,x
@dv_advance_phase:
    inc PLAYER_FM_PHASE,x
    lda PLAYER_FM_PHASE,x
    cmp #8
    bne @done_dv
    lda #0
    sta PLAYER_FM_PHASE,x
@done_dv:
    rts

@type_seq:
    ; seq: advance ptr each frame (after frame 0); hold at last entry
    lda PLAYER_FM_PTR_LO,x
    clc
    adc #2
    sta PL_BLK_LO
    lda PLAYER_FM_PTR_HI,x
    adc #0
    sta PL_BLK_HI
    ; check if next ptr is past end
    lda PL_BLK_LO
    cmp PLAYER_FM_END_LO,x
    bne @seq_advance
    lda PL_BLK_HI
    cmp PLAYER_FM_END_HI,x
    beq @seq_read           ; at end: stay put (hold last)
@seq_advance:
    lda PL_BLK_LO
    sta PLAYER_FM_PTR_LO,x
    lda PL_BLK_HI
    sta PLAYER_FM_PTR_HI,x
@seq_read:
    lda PLAYER_FM_PTR_LO,x
    sta @seq_rd_lo+1
    lda PLAYER_FM_PTR_HI,x
    sta @seq_rd_lo+2
@seq_rd_lo:
    lda $0000
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_PTR_LO,x
    clc
    adc #1
    sta @seq_rd_hi+1
    lda PLAYER_FM_PTR_HI,x
    adc #0
    sta @seq_rd_hi+2
@seq_rd_hi:
    lda $0000
    sta PLAYER_FM_FREQ_HI,x
    rts

@type_gliss:
    ; holdGliss: hold 12 frames, then add one delta per frame (signed)
    lda PLAYER_FM_HOLD,x
    beq @gliss_step
    dec PLAYER_FM_HOLD,x
    ; still in hold: keep current gliss freq (which = base freq at note start)
    rts
@gliss_step:
    ; check if exhausted
    lda PLAYER_FM_PTR_LO,x
    cmp PLAYER_FM_END_LO,x
    bne @gliss_not_done
    lda PLAYER_FM_PTR_HI,x
    cmp PLAYER_FM_END_HI,x
    beq @gliss_done
@gliss_not_done:
    ; read int16 delta at PTR, advance PTR by 2
    lda PLAYER_FM_PTR_LO,x
    sta @gl_rd_lo+1
    lda PLAYER_FM_PTR_HI,x
    sta @gl_rd_lo+2
@gl_rd_lo:
    lda $0000               ; delta lo
    sta PL_BLK_LO
    lda PLAYER_FM_PTR_LO,x
    clc
    adc #1
    sta @gl_rd_hi+1
    lda PLAYER_FM_PTR_HI,x
    adc #0
    sta @gl_rd_hi+2
@gl_rd_hi:
    lda $0000               ; delta hi
    sta PL_BLK_HI
    ; advance ptr by 2
    lda PLAYER_FM_PTR_LO,x
    clc
    adc #2
    sta PLAYER_FM_PTR_LO,x
    lda PLAYER_FM_PTR_HI,x
    adc #0
    sta PLAYER_FM_PTR_HI,x
    ; freq += delta (signed 16-bit)
    lda PLAYER_FM_FREQ_LO,x
    clc
    adc PL_BLK_LO
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_FREQ_HI,x
    adc PL_BLK_HI
    sta PLAYER_FM_FREQ_HI,x
@gliss_done:
    rts

@type_headcycle:
    ; headCycle: advance ptr each frame; wrap at cycleEnd back to cycleBase
    lda PLAYER_FM_PTR_LO,x
    clc
    adc #2
    sta PL_BLK_LO
    lda PLAYER_FM_PTR_HI,x
    adc #0
    sta PL_BLK_HI
    ; check if reached cycleEnd
    lda PL_BLK_LO
    cmp PLAYER_FM_END_LO,x
    bne @hc_advance
    lda PL_BLK_HI
    cmp PLAYER_FM_END_HI,x
    bne @hc_advance
    ; wrap to cycleBase
    lda PLAYER_FM_CYC_LO,x
    sta PL_BLK_LO
    lda PLAYER_FM_CYC_HI,x
    sta PL_BLK_HI
@hc_advance:
    lda PL_BLK_LO
    sta PLAYER_FM_PTR_LO,x
    lda PL_BLK_HI
    sta PLAYER_FM_PTR_HI,x
    ; read freq at new ptr
    sta @hc_rd_lo+2
    lda PL_BLK_LO
    sta @hc_rd_lo+1
@hc_rd_lo:
    lda $0000
    sta PLAYER_FM_FREQ_LO,x
    lda PL_BLK_LO
    clc
    adc #1
    sta @hc_rd_hi+1
    lda PL_BLK_HI
    adc #0
    sta @hc_rd_hi+2
@hc_rd_hi:
    lda $0000
    sta PLAYER_FM_FREQ_HI,x
@done:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; init_npt_note  (PL_CH, PL_PID set by caller)
; Scans MUSIC_NPT_WAVE_OVERRIDES for PL_PID.
; If found: sets up PLAYER_NPT_* state. If not found: clears NPT_ACTIVE.
; Clobbers: A, X, Y, PL_TMP, PL_BLK_LO, PL_BLK_HI, PL_BLK_IDX
; ─────────────────────────────────────────────────────────────────────────────
.proc init_npt_note
    lda PL_PID
    sta PL_TMP
    ldx PL_CH
    lda #0
    sta PLAYER_NPT_ACTIVE,x
    ldy #0
@scan_loop:
    cpy #MUSIC_NPT_OVR_COUNT * 2
    bne @scan_check
    jmp @no_override
@scan_check:
    lda MUSIC_NPT_WAVE_OVERRIDES,y
    cmp PL_TMP
    bne @scan_next
    jmp @found_pid
@scan_next:
    iny
    iny
    jmp @scan_loop
@found_pid:
    iny
    lda MUSIC_NPT_WAVE_OVERRIDES,y  ; seqIdx
    sta PL_BLK_IDX
    lda #<MUSIC_NPT_WAVE_SEQS
    sta PL_BLK_LO
    lda #>MUSIC_NPT_WAVE_SEQS
    sta PL_BLK_HI
    lda PL_BLK_IDX
    beq @seq_found
@walk_setup:
    lda PL_BLK_LO
    sta @walk_rd_wave+1
    lda PL_BLK_HI
    sta @walk_rd_wave+2
@walk_rd_wave:
    lda $0000               ; waveByteLen
    clc
    adc PL_BLK_LO
    adc #1
    sta PL_BLK_LO
    bcc @walk_at_fc
    inc PL_BLK_HI
@walk_at_fc:
    lda PL_BLK_LO
    sta @walk_rd_fc+1
    lda PL_BLK_HI
    sta @walk_rd_fc+2
@walk_rd_fc:
    lda $0000               ; frameCount
    asl
    clc
    adc #1
    adc PL_BLK_LO
    sta PL_BLK_LO
    bcc @walk_next_ok
    inc PL_BLK_HI
@walk_next_ok:
    dec PL_BLK_IDX
    bne @walk_setup
@seq_found:
    ldx PL_CH
    lda PL_BLK_LO
    sta PLAYER_NPT_WAVE_LO,x
    lda PL_BLK_HI
    sta PLAYER_NPT_WAVE_HI,x
    lda PL_BLK_LO
    sta @rd_wl+1
    lda PL_BLK_HI
    sta @rd_wl+2
@rd_wl:
    lda $0000               ; waveByteLen
    sta PL_TMP
    clc
    lda PL_BLK_LO
    adc PL_TMP
    adc #1
    sta PL_BLK_LO
    bcc @fc_hi_ok
    inc PL_BLK_HI
@fc_hi_ok:
    lda PL_BLK_LO
    sta @rd_fc+1
    lda PL_BLK_HI
    sta @rd_fc+2
@rd_fc:
    lda $0000               ; frameCount
    sta PL_TMP
    inc PL_BLK_LO
    bne @togdata_ok
    inc PL_BLK_HI
@togdata_ok:
    ; PL_BLK_LO/HI → toggle[0]; PL_TMP = frameCount
    ; Read toggle[0] and apply to PLAYER_TOGGLE (sim nf=0)
    lda PL_BLK_LO
    sta @rd_tog0_lo+1
    lda PL_BLK_HI
    sta @rd_tog0_lo+2
@rd_tog0_lo:
    lda $0000               ; toggle[0] lo
    ldx PL_CH
    sta PLAYER_TOGGLE_LO,x
    sta PLAYER_NPT_TOG_LST_LO,x
    lda @rd_tog0_lo+1
    clc
    adc #1
    sta @rd_tog0_lo+1
    sta @rd_tog0_hi+1
    bcc @rd_tog0_no_carry
    inc @rd_tog0_lo+2
@rd_tog0_no_carry:
    lda @rd_tog0_lo+2
    sta @rd_tog0_hi+2
@rd_tog0_hi_ok:
@rd_tog0_hi:
    lda $0000               ; toggle[0] hi (self-modified)
    sta PLAYER_TOGGLE_HI,x
    sta PLAYER_NPT_TOG_LST_HI,x
    ; advance ptr by 1 more to toggle[1]
    lda @rd_tog0_lo+1
    clc
    adc #1
    sta PLAYER_NPT_TOG_LO,x
    lda @rd_tog0_lo+2
    adc #0
    sta PLAYER_NPT_TOG_HI,x
    lda PL_TMP              ; frameCount
    beq @fc_zero
    sec
    sbc #1                  ; FC = frameCount - 1 (remaining after toggle[0] consumed)
    sta PLAYER_NPT_TOG_FC,x
    jmp @mark_active
@fc_zero:
    lda #0
    sta PLAYER_NPT_TOG_FC,x
@mark_active:
    lda #1
    sta PLAYER_NPT_ACTIVE,x
    ; Apply nf=0 wave state to PLAYER_FRAME ctrl slots (matches step_npt with NF=0)
    lda PLAYER_NPT_WAVE_LO,x
    sta @rd_w0+1
    lda PLAYER_NPT_WAVE_HI,x
    sta @rd_w0+2
@rd_w0:
    lda $0000               ; waveByteLen
    beq @w0_past_end        ; no wave bytes: treat as mixed
    ; wave byte 0 is at NPT_WAVE + 1; nf=0 → shift=0, state = byte & 3
    lda @rd_w0+1
    clc
    adc #1
    sta @rd_wb0+1
    lda @rd_w0+2
    adc #0
    sta @rd_wb0+2
@rd_wb0:
    lda $0000               ; wave byte 0
    and #3                  ; 2-bit state
    bne @w0_not_tone
    lda #$41                ; tone-only: CtrlA = pulse+gate
    sta PL_BLK_LO
    sta PL_BLK_HI           ; CtrlB = same
    jmp @w0_done
@w0_not_tone:
    cmp #1
    bne @w0_noise_only
    lda #$81                ; mixed: CtrlA = noise+gate
    sta PL_BLK_LO
    lda #$51                ; CtrlB = pulse+tri+gate
    sta PL_BLK_HI
    jmp @w0_done
@w0_noise_only:
    lda #$81
    sta PL_BLK_LO
    sta PL_BLK_HI
    jmp @w0_done
@w0_past_end:
    lda #$81                ; mixed
    sta PL_BLK_LO
    lda #$51
    sta PL_BLK_HI
@w0_done:
    lda PL_FBASE
    tax
    lda PL_BLK_LO
    sta PLAYER_FRAME+4,x    ; CtrlA
    lda PL_BLK_HI
    sta PLAYER_FRAME+5,x    ; CtrlB
@no_override:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; step_npt  (PL_CH set; PLAYER_NF already incremented by caller)
; Updates PLAYER_FRAME CtrlA slot and PLAYER_TOGGLE_LO/HI.
; No-op if PLAYER_NPT_ACTIVE[PL_CH]=0.
; Clobbers: A, X, Y, PL_TMP, PL_BLK_LO, PL_BLK_HI, PL_BLK_IDX
; ─────────────────────────────────────────────────────────────────────────────
.proc step_npt
    ldx PL_CH
    lda PLAYER_NPT_ACTIVE,x
    bne @npt_active
    rts
@npt_active:
    ; compute wave byte: byte_index = nf >> 2; shift = 2*(nf & 3)
    lda PLAYER_NF,x
    sta PL_TMP              ; nf
    lsr
    lsr
    sta PL_BLK_IDX          ; byte_index
    ; check byte_index >= waveByteLen
    lda PLAYER_NPT_WAVE_LO,x
    sta @rd_wl+1
    lda PLAYER_NPT_WAVE_HI,x
    sta @rd_wl+2
@rd_wl:
    lda $0000               ; waveByteLen
    cmp PL_BLK_IDX          ; A=waveByteLen; in-range if waveByteLen > byte_index
    beq @wave_past_end
    bcs @wave_in_range
@wave_past_end:
    lda #$81                ; past end: mixed → CtrlA = noise+gate
    sta PL_BLK_LO
    lda #$51                ; CtrlB = pulse+tri+gate
    sta PL_BLK_HI
    jmp @wave_done
@wave_in_range:
    ; ptr = wave_base + 1 + byte_index
    clc
    lda PLAYER_NPT_WAVE_LO,x
    adc PL_BLK_IDX
    adc #1
    sta @rd_wb+1
    lda PLAYER_NPT_WAVE_HI,x
    adc #0
    sta @rd_wb+2
@rd_wb:
    lda $0000               ; wave byte
    sta PL_BLK_LO           ; save wave byte
    ; shift right by 2*(nf & 3)
    lda PL_TMP
    and #3
    asl                     ; shift_amount
    beq @shift_done
    tay
@shift_loop:
    lsr PL_BLK_LO
    dey
    bne @shift_loop
@shift_done:
    lda PL_BLK_LO
    and #3                  ; 2-bit state: 0=tone, 1=mixed, 2=noise
    bne @not_tone
    lda #$41                ; tone-only: CtrlA = pulse+gate
    sta PL_BLK_LO
    sta PL_BLK_HI           ; CtrlB = same
    jmp @wave_done
@not_tone:
    cmp #1
    bne @noise_only
    lda #$81                ; mixed: CtrlA = noise+gate
    sta PL_BLK_LO
    lda #$51                ; CtrlB = pulse+tri+gate
    sta PL_BLK_HI
    jmp @wave_done
@noise_only:
    lda #$81                ; noise-only: CtrlA = noise+gate
    sta PL_BLK_LO
    sta PL_BLK_HI           ; CtrlB = same
@wave_done:
    ldx PL_FBASE
    lda PL_BLK_LO
    sta PLAYER_FRAME+4,x    ; CtrlA
    lda PL_BLK_HI
    sta PLAYER_FRAME+5,x    ; CtrlB
    ldx PL_CH
    lda PLAYER_NPT_TOG_FC,x
    beq @use_last
    ; read lo byte of current toggle uint16
    lda PLAYER_NPT_TOG_LO,x
    sta @rd_tlo+1
    lda PLAYER_NPT_TOG_HI,x
    sta @rd_tlo+2
@rd_tlo:
    lda $0000               ; toggle lo
    sta PLAYER_TOGGLE_LO,x
    sta PLAYER_NPT_TOG_LST_LO,x
    ; advance ptr to hi byte, update @rd_thi operand
    lda @rd_tlo+1
    clc
    adc #1
    sta @rd_tlo+1
    sta @rd_thi+1
    bcc @rd_tno_carry
    inc @rd_tlo+2
@rd_tno_carry:
    lda @rd_tlo+2
    sta @rd_thi+2
@rd_thi_ok:
@rd_thi:
    lda $0000               ; toggle hi (self-modified)
    sta PLAYER_TOGGLE_HI,x
    sta PLAYER_NPT_TOG_LST_HI,x
    ; advance ptr by 1 more to next toggle uint16
    lda @rd_tlo+1
    clc
    adc #1
    sta PLAYER_NPT_TOG_LO,x
    lda @rd_tlo+2
    adc #0
    sta PLAYER_NPT_TOG_HI,x
    dec PLAYER_NPT_TOG_FC,x
    bne @done
    ; FC just hit 0: but we already updated last values above, good
    jmp @done
@use_last:
    lda PLAYER_NPT_TOG_LST_LO,x
    sta PLAYER_TOGGLE_LO,x
    lda PLAYER_NPT_TOG_LST_HI,x
    sta PLAYER_TOGGLE_HI,x
@done:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; init_toneseq_note  (PL_CH, PL_PID set by caller)
; Scans MUSIC_TONE_OVERRIDES for PL_PID.
; If found: loads initial period from MUSIC_TOGGLE_VALS, applies first step,
;           sets up ptr/end/hold for subsequent steps.
; If not found: clears TSEQ_ACTIVE.
; Entry format in MUSIC_TONE_SEQS: toggleIdx(1) then (delta(1), hold(1)) pairs.
; Mirrors initToneSeq in simulate_tracker.go.
; Clobbers: A, X, Y, PL_TMP, PL_BLK_LO, PL_BLK_HI, PL_BLK_IDX
; ─────────────────────────────────────────────────────────────────────────────
.proc init_toneseq_note
    ldx PL_CH
    lda #0
    sta PLAYER_TSEQ_ACTIVE,x
    lda PL_PID
    sta PL_TMP
    ldy #0
@scan_loop:
    cpy #MUSIC_TONE_OVR_COUNT * 2
    bne @scan_check
    rts                         ; not found
@scan_check:
    lda MUSIC_TONE_OVERRIDES,y
    cmp PL_TMP
    beq @found_pid
    iny
    iny
    jmp @scan_loop
@found_pid:
    iny
    lda MUSIC_TONE_OVERRIDES,y  ; toneSeqIdx (byte offset into MUSIC_TONE_SEQS)
    clc
    adc #<MUSIC_TONE_SEQS
    sta PL_BLK_LO
    lda #>MUSIC_TONE_SEQS
    adc #0
    sta PL_BLK_HI
    ; read toggleIdx byte at entry_ptr[0]
    lda PL_BLK_LO
    sta @rd_ti+1
    lda PL_BLK_HI
    sta @rd_ti+2
@rd_ti:
    lda $0000               ; toggleIdx
    asl                     ; toggleIdx * 2 = byte offset into MUSIC_TOGGLE_VALS
    sta PL_BLK_IDX          ; save: toggleIdx*2 for TOGGLE_VALS lookup
    tax
    lda MUSIC_TOGGLE_VALS,x  ; initial period lo
    ldx PL_CH
    sta PLAYER_TSEQ_PERIOD_LO,x
    ldy PL_BLK_IDX           ; reload toggleIdx*2 for hi byte (Y safe, not clobbered since store)
    lda MUSIC_TOGGLE_VALS+1,y ; initial period hi
    sta PLAYER_TSEQ_PERIOD_HI,x
    ; advance ptr past toggleIdx byte to first (delta, hold) pair
    inc PL_BLK_LO
    bne @ptr_ok
    inc PL_BLK_HI
@ptr_ok:
    ; end = MUSIC_DELAYED_VIB (section 5, immediately follows section 4 in emitted data)
    lda #<MUSIC_DELAYED_VIB
    sta PLAYER_TSEQ_END_LO,x
    lda #>MUSIC_DELAYED_VIB
    sta PLAYER_TSEQ_END_HI,x
    ; check if first step exists (ptr < end)
    lda PL_BLK_LO
    cmp PLAYER_TSEQ_END_LO,x
    bne @has_step
    lda PL_BLK_HI
    cmp PLAYER_TSEQ_END_HI,x
    beq @no_steps
@has_step:
    ; apply first step: period += delta, hold = hold_byte, ptr += 2
    lda PL_BLK_LO
    sta @rd_d+1
    lda PL_BLK_HI
    sta @rd_d+2
@rd_d:
    lda $0000               ; delta byte
    clc
    adc PLAYER_TSEQ_PERIOD_LO,x
    sta PLAYER_TSEQ_PERIOD_LO,x
    lda PLAYER_TSEQ_PERIOD_HI,x
    adc #0
    sta PLAYER_TSEQ_PERIOD_HI,x
    lda PL_BLK_LO
    clc
    adc #1
    sta @rd_h+1
    lda PL_BLK_HI
    adc #0
    sta @rd_h+2
@rd_h:
    lda $0000               ; hold byte
    sta PLAYER_TSEQ_HOLD,x
    lda PL_BLK_LO
    clc
    adc #2
    sta PL_BLK_LO
    lda PL_BLK_HI
    adc #0
    sta PL_BLK_HI
    jmp @store_ptr
@no_steps:
    lda #0
    sta PLAYER_TSEQ_HOLD,x
@store_ptr:
    lda PL_BLK_LO
    sta PLAYER_TSEQ_PTR_LO,x
    lda PL_BLK_HI
    sta PLAYER_TSEQ_PTR_HI,x
    lda #1
    sta PLAYER_TSEQ_ACTIVE,x
    lda PLAYER_TSEQ_PERIOD_LO,x
    sta PLAYER_TOGGLE_LO,x
    lda PLAYER_TSEQ_PERIOD_HI,x
    sta PLAYER_TOGGLE_HI,x
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; step_toneseq  (X = channel / PL_CH set)
; Advances tone seq state by one frame and updates PLAYER_TOGGLE_LO/HI[ch].
; No-op if PLAYER_TSEQ_ACTIVE[ch]=0.
; Mirrors stepToneSeq in simulate_tracker.go.
; Clobbers: A, X, PL_TMP, PL_BLK_LO, PL_BLK_HI
; ─────────────────────────────────────────────────────────────────────────────
.proc step_toneseq
    lda PLAYER_TSEQ_ACTIVE,x
    bne @active
    rts
@active:
    lda PLAYER_TSEQ_HOLD,x
    beq @done               ; hold=0: no more steps, period is final
    dec PLAYER_TSEQ_HOLD,x
    bne @write_period       ; hold not yet 0: just write current period
    ; hold just hit 0: check if next step exists
    lda PLAYER_TSEQ_PTR_LO,x
    cmp PLAYER_TSEQ_END_LO,x
    bne @load_next_step
    lda PLAYER_TSEQ_PTR_HI,x
    cmp PLAYER_TSEQ_END_HI,x
    beq @write_period       ; ptr >= end: no more steps, period stays
@load_next_step:
    ; read delta at ptr, add to period
    lda PLAYER_TSEQ_PTR_LO,x
    sta @rd_d+1
    lda PLAYER_TSEQ_PTR_HI,x
    sta @rd_d+2
@rd_d:
    lda $0000               ; delta byte
    clc
    adc PLAYER_TSEQ_PERIOD_LO,x
    sta PLAYER_TSEQ_PERIOD_LO,x
    lda PLAYER_TSEQ_PERIOD_HI,x
    adc #0
    sta PLAYER_TSEQ_PERIOD_HI,x
    ; read hold at ptr+1
    lda PLAYER_TSEQ_PTR_LO,x
    clc
    adc #1
    sta @rd_h+1
    lda PLAYER_TSEQ_PTR_HI,x
    adc #0
    sta @rd_h+2
@rd_h:
    lda $0000               ; hold byte
    sta PLAYER_TSEQ_HOLD,x
    ; advance ptr by 2
    lda PLAYER_TSEQ_PTR_LO,x
    clc
    adc #2
    sta PLAYER_TSEQ_PTR_LO,x
    lda PLAYER_TSEQ_PTR_HI,x
    adc #0
    sta PLAYER_TSEQ_PTR_HI,x
@write_period:
    lda PLAYER_TSEQ_PERIOD_LO,x
    sta PLAYER_TOGGLE_LO,x
    lda PLAYER_TSEQ_PERIOD_HI,x
    sta PLAYER_TOGGLE_HI,x
@done:
    rts
.endproc
