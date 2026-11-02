; SID tracker player
;
; Replays the tracker ROM format produced by tools/emit_asm.go.
; Call player_init once at startup, player_tick once per PAL frame.
;
; CIA1 Timer A drives waveform toggling for voice 1 (channel 0).
; CIA1 Timer B drives waveform toggling for voice 2 (channel 1).
; Voice 3 (channel 2) never uses CIA toggling.
;
; Stream reads use (zp,x) indirect addressing with X = 2*channel.
;
; Zero-page layout:
;   ZP_STREAM+2*ch  (ch=0,1,2): stream / block-body read pointer (lo, hi)
;   ZP_RETURN+2*ch            : stream resume pointer (lo, hi)
;   ZP_BLOCK_CTR+2*ch         : opcodes remaining in current block (0 = stream)
;   ZP_DUR_CTR+ch             : frames remaining in current note/rest (0 = fetch next)

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

; CIA1_PRB (bit6 = Timer A toggle output, bit7 = Timer B toggle output) is
; defined by the includer: $DC01 on hardware, a software byte in the IRQ harness.
CIA1_TA_LO  = $DC04
CIA1_TA_HI  = $DC05
CIA1_TB_LO  = $DC06
CIA1_TB_HI  = $DC07
CIA1_ICR    = $DC0D
CIA1_CRA    = $DC0E
CIA1_CRB    = $DC0F

; ── Zero-page ─────────────────────────────────────────────────────────────────

.segment "ZEROPAGE"

ZP_STREAM:    .res 6   ; (lo, hi) per channel at 2*ch
ZP_RETURN:    .res 6   ; (lo, hi) per channel at 2*ch
ZP_BLOCK_CTR: .res 6   ; at 2*ch
ZP_DUR_CTR:   .res 3   ; frames remaining (0 = fetch next stream byte)

; ── BSS ───────────────────────────────────────────────────────────────────────

.segment "BSS"

; Active CtrlA/CtrlB per channel: apply_prev_frame copies the staged pair
; here at the start of every tick. The two CIA hardware handlers
; (player_cia1a_fire/player_cia1b_fire) read them at their own asynchronous
; hardware cadence.
PLAYER_FRAME_CTRL_A: .res 3
PLAYER_FRAME_CTRL_B: .res 3

; Staged output of compute_next_frame, one array per SID field, indexed by
; channel. Written this tick, applied at the START of the NEXT tick. A field
; left untouched this tick simply carries its last-staged value forward
; (persistent BSS, not cleared between ticks).
PLAYER_NEXT_CTRL_A:  .res 3
PLAYER_NEXT_CTRL_B:  .res 3
PLAYER_NEXT_ATT_DEC: .res 3
PLAYER_NEXT_SUS_REL: .res 3

; Pending immediate SID_CTRL pre-writes, per channel — the GateReset
; (gate 1→0) and TestPulse (test 0→1→0) sequences write SID_CTRL 1-2 times
; in a row within the SAME tick a note starts, before the note's own final
; ctrl value. Under the one-tick pipeline those writes can't land on
; hardware immediately, so they're queued here and replayed by
; apply_prev_frame at the very start of the NEXT tick, immediately before
; the normal CtrlA/CtrlB handoff — reproducing the exact same 1-2-then-final
; write sequence as before, just shifted one tick later as a block.
; 0: nothing pending. $80: write $00 (GateReset). $89: write $09 then $01
; (TestPulse).
PLAYER_CTRL_PRE: .res 3

; CIA toggle periods (uint16 LE, 0 = CIA not running for this channel)
PLAYER_TOGGLE_LO:  .res 2
PLAYER_TOGGLE_HI:  .res 2

; Last period actually committed to CIA hardware by apply_toggle_period,
; per channel (ch0/ch1 only). apply_toggle_period now runs every tick (to
; stay synchronized with the ctrl pair), but its "force immediate reload"
; logic only makes sense when the period actually CHANGED since last
; commit — comparing an unchanged period against the live, naturally
; counting-down CIA counter would spuriously trigger a reload as soon as
; the counter's HI byte ticks below the target, corrupting the timer's
; natural cadence. Tracking the last-committed value lets apply_toggle_period
; skip its compare-and-maybe-force logic entirely when nothing changed.
PLAYER_TOGGLE_LAST_LO: .res 2
PLAYER_TOGGLE_LAST_HI: .res 2

; Block count (set at init from MUSIC_BLOCK_COUNT constant)
PLAYER_BLOCK_COUNT: .res 1

; Gate-reset state per channel (persists across rests, matching the SID's
; own ADSR generator state, which isn't reset by a rest):
;   HAS_PREV_NOTE:  0 until the first note has played (skip reset on the very
;                   first note — nothing to reset from)
;   PREV_ENV_MODE:  envelope-mode flag of the last note that played
PLAYER_HAS_PREV_NOTE: .res 3
PLAYER_PREV_ENV_MODE: .res 3

; Sustain bitstream state per channel (stride-3 SoA):
;   SUS_PTR:  offset in MUSIC_SUSTAIN_PATS of the next body byte to load
;   SUS_END:  offset one past last body byte (PTR == END: nothing left to load)
;   SUS_BITS: remaining bits of the loaded byte, with a 1 sentinel above them
;             (0 = load next byte)
;   SUS_VAL:       current sustain nibble (written as SusRel = val << 4)
PLAYER_SUS_PTR:    .res 3
PLAYER_SUS_END:    .res 3
PLAYER_SUS_BITS:   .res 3
PLAYER_SUS_VAL:    .res 3
PLAYER_ATTDEC_TAIL: .res 3  ; (attack<<4)|decay to write when tail fires
PLAYER_TAIL_CTR:    .res 3  ; tail fire countdown; 0 = no tail/already fired
PLAYER_ATTDEC_PRE:  .res 3  ; (attack<<4)|attack — pre-tail value; written every sustain frame
PLAYER_ATTDEC_NF1:  .res 3  ; attack<<4 to write on nf=1 if attack>0, else $FF (one-shot)

; FM state per channel (stride-3 SoA):
;   FM_TYPE:     compact FM type (0/1/3/4) or $FF=none
;   FM_PTR:      byte offset of the current entry (MUSIC_FREQ_MOD; DV: step lo)
;   FM_END:      byte offset one past the last entry (seq/gliss/headCycle boundary; DV: step hi)
;   FM_HOLD:     frames remaining in hold phase (DV/gliss: 12, counts to 0)
;   FM_PHASE:    DV phase counter 0-7
;   FM_FREQ_LO/HI: current FM output frequency (updated each sustain frame)
;   FM_CYC:      offset the entry pointer wraps to at FM_END (a seq holds its last entry)
PLAYER_FM_TYPE:    .res 3
PLAYER_FM_PTR:     .res 3
PLAYER_FM_END:     .res 3
PLAYER_FM_HOLD:    .res 3
PLAYER_FM_PHASE:   .res 3
PLAYER_FM_FREQ_LO: .res 3
PLAYER_FM_FREQ_HI: .res 3
PLAYER_FM_CYC:     .res 3

; NPT (NoisePlusTone) wave+toggle override state per channel:
;   NPT_ACTIVE:  1 if override in effect, 0 otherwise
;   NPT_WAVE:    offset of the waveByteLen byte in NPT_WAVE_SEQS
;   NPT_TOG:     offset of the next toggle uint16 in NPT_WAVE_SEQS
;   NPT_TOG_FC:     frames remaining in toggle sequence (0 = hold last)
;   NPT_OCC:     byte cursor into a per-occurrence toggle list (frameCount $FF),
;                advanced by one list byte for every note that uses the list
;   NF:              note frame counter (0-based, per channel)
PLAYER_NPT_ACTIVE:      .res 3
PLAYER_NPT_WAVE:        .res 3
PLAYER_NPT_TOG:         .res 3
PLAYER_NPT_OCC:         .res 3
PLAYER_NPT_TOG_FC:      .res 3
PLAYER_NF:              .res 3  ; note frame counter (used for NPT wave indexing)

PLAYER_CLEAR_END:

; Scratch variables shared by player routines
PL_CH:        .res 1   ; current channel (0/1/2)
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
.export player_cia1a_apply_prb
.export player_cia1b_apply_prb

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

    ldx #PLAYER_CLEAR_END - PLAYER_NEXT_CTRL_A
    lda #0
@clr_state:
    sta PLAYER_NEXT_CTRL_A-1,x
    dex
    bne @clr_state

    lda #MUSIC_BLOCK_COUNT
    sta PLAYER_BLOCK_COUNT

    ldx #5
@set_streams:
    lda pl_stream_bounds,x
    sta ZP_STREAM,x
    dex
    bpl @set_streams

    ldx #ZP_DUR_CTR + 3 - ZP_BLOCK_CTR
    lda #0
@clr_zp:
    sta ZP_BLOCK_CTR-1,x
    dex
    bne @clr_zp

    ldx #2
    lda #$FF
@set_ff:
    sta PLAYER_ATTDEC_NF1,x
    sta PLAYER_FM_TYPE,x
    dex
    bpl @set_ff
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; player_tick  — call once per PAL frame
; ─────────────────────────────────────────────────────────────────────────────
; Three-phase pipeline per channel, run channel-by-channel (each channel
; fully passes through all 3 phases before the next channel starts — no
; cross-channel dependency exists, so this ordering is equivalent to 3
; separate full loops but touches PL_CH fewer times):
;   1. apply_prev_frame:  copy the PLAYER_NEXT_* fields (staged last tick) into
;      PLAYER_FRAME_CTRL_A/B and to SID hardware. For ch0/1, ctrl only updates
;      PLAYER_FRAME_CTRL_A/B — never SID_CTRL directly.
;   2. CIA catch-up (ch0/1 only): JSR the exact same routine the CIA1
;      Timer A/B hardware IRQ calls, so a freshly-applied CtrlA/CtrlB pair
;      reaches SID_CTRL within this tick rather than waiting an arbitrary
;      (possibly ~13000-cycle) amount of time for the next real underflow.
;      The real CIA IRQ keeps firing independently between ticks too —
;      this call never replaces or caps that.
;   3. compute_next_frame: run note-start/sustain/rest logic, staging all
;      outputs into PLAYER_NEXT_* for phase 1 to apply next tick.
; ─────────────────────────────────────────────────────────────────────────────
.proc player_tick
    ldx #0
@loop:
    jsr apply_prev_frame    ; sets PL_CH = X on entry; clobbers X internally
    lda PL_CH
    beq @ch0_catchup        ; PL_CH=0 → Timer A
    cmp #1
    beq @ch1_catchup        ; PL_CH=1 → Timer B
    jmp @ch2_no_cia         ; PL_CH=2 → no CIA
@ch0_catchup:
    php                     ; a CIA IRQ between the fire's phase read and its
    sei                     ; SID_CTRL write would be overwritten by the stale phase
    jsr player_cia1a_fire
    plp
    jmp @ch2_no_cia
@ch1_catchup:
    php
    sei
    jsr player_cia1b_fire
    plp
@ch2_no_cia:
    ldx PL_CH               ; apply_prev_frame/CIA handlers clobber X
    jsr compute_next_frame
    ldx PL_CH               ; compute_next_frame clobbers X; restore for loop counter
    inx
    cpx #3
    bne @loop
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; apply_prev_frame  (X = channel)
; Phase 1: copies PLAYER_NEXT_* (staged by compute_next_frame on the
; PREVIOUS tick) into PLAYER_FRAME_CTRL_A/B and, for freq/PW/AttDec/SusRel, straight
; to SID hardware. For ch0/1's ctrl pair, only PLAYER_FRAME_CTRL_A/B is updated —
; SID_CTRL is left for phase 2 (the CIA-fire catch-up call) to write, so
; this proc and the real CIA hardware IRQ are never both writing SID_CTRL
; for the same channel from two different, uncoordinated cadences. ch2 has
; no CIA, so its ctrl is written straight to SID here.
; Clobbers: A, X
; ─────────────────────────────────────────────────────────────────────────────
.proc apply_prev_frame
    stx PL_CH
    ldy pl_voice_base,x     ; Y = SID voice base

    ; Replay any queued immediate ctrl pre-writes (GateReset/TestPulse's
    ; gate-off / test-bit-on-then-off sequence) FIRST, in the order they
    ; were staged last tick. The note's own final ctrl value follows via the
    ; normal CtrlA/CtrlB handoff below. Each pre-value is stored into BOTH
    ; active ctrl slots first, so a CIA fire preempting this sequence
    ; rewrites the same pre-value instead of the stale ctrl pair.
    lda PLAYER_CTRL_PRE,x
    beq @no_pre
    and #$0F
    jsr @pre_write
    cmp #$09
    bne @pre_done
    lda #$01
    jsr @pre_write
@pre_done:
    lda #0
    sta PLAYER_CTRL_PRE,x
@no_pre:
    lda PLAYER_FM_FREQ_LO,x
    sta SID+SID_FREQ_LO,y
    lda PLAYER_FM_FREQ_HI,x
    sta SID+SID_FREQ_HI,y
    ; Pulse width: 0x800 only while pure pulse ($40) is the staged CtrlA
    ; waveform, else 0xFFF. Silence leaves the width as it was.
    lda PLAYER_NEXT_CTRL_A,x
    beq @no_pw
    and #$F0
    eor #$40
    beq @pw_lo
    lda #$FF
@pw_lo:
    sta SID+SID_PW_LO,y
    and #$07
    ora #$08
    sta SID+SID_PW_HI,y
@no_pw:
    lda PLAYER_NEXT_ATT_DEC,x
    sta SID+SID_ATT_DEC,y
    lda PLAYER_NEXT_SUS_REL,x
    sta SID+SID_SUS_REL,y

    lda PLAYER_NEXT_CTRL_A,x
    sta PLAYER_FRAME_CTRL_A,x
    lda PLAYER_NEXT_CTRL_B,x
    sta PLAYER_FRAME_CTRL_B,x

    ; ch2 has no CIA fire handler to write SID_CTRL for it in phase 2 —
    ; write CtrlA straight to hardware here instead.
    cpx #2
    bne @ch0_1
    lda PLAYER_FRAME_CTRL_A,x
    sta SID+SID_CTRL,y
    rts
@ch0_1:
    ; Commit the staged CIA toggle period in the SAME tick the ctrl pair
    ; above became active. Skip during rest/silence (CtrlA=0): calling it
    ; then would keep re-forcing a reload from the stale period, causing a
    ; CIA fire-rate runaway for the whole rest duration.
    lda PLAYER_FRAME_CTRL_A,x
    beq @done
    jsr apply_toggle_period
@done:
    rts

@pre_write:
    sta PLAYER_FRAME_CTRL_A,x
    sta PLAYER_FRAME_CTRL_B,x
    sta SID+SID_CTRL,y
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; compute_next_frame  (X = channel)
; Phase 3: on every tick, decrement duration counter.
;   If counter was > 0: sustain (advance FM/NPT/sustain-bitstream state).
;   If counter was 0: fetch next stream entry, set new counter.
; All results are staged into PLAYER_NEXT_* for apply_prev_frame to
; apply on the NEXT tick — this proc never writes SID hardware or
; PLAYER_FRAME directly, except CIA timer PERIOD arming (PLAYER_TOGGLE_LO/HI,
; CIA1_TA/TB_LO/HI, CIA1_CRA/CRB via apply_toggle_period and the note-start
; arm block below) — period is orthogonal to the ctrl waveform PAIR this
; pipeline stages, and already has its own immediate-vs-natural-reload
; logic designed to apply as soon as the new period is known.
; ─────────────────────────────────────────────────────────────────────────────
.proc compute_next_frame
    stx PL_CH
    lda ZP_DUR_CTR,x
    bne @sustaining
    jmp @fetch
@sustaining:
    dec ZP_DUR_CTR,x
    lda PLAYER_NEXT_CTRL_A,x    ; a staged note always has the gate bit, a rest has CtrlA 0
    bne @sustain_note
    jmp stage_silence

@sustain_note:
    jsr step_sustain

    jsr step_fm
    inc PLAYER_NF,x
    jsr step_npt            ; may restage ctrl
    ; AttDec: stage every sustain frame.
    ; nf=1 special: ATTDEC_NF1 holds attack<<4 (one-shot); $FF means skip.
    ; After nf=1, stage ATTDEC_PRE = (attack<<4)|attack each frame.
    ; On tail fire, stage ATTDEC_TAIL and stop.
    lda PLAYER_TAIL_CTR,x
    beq @attdec_done        ; tail already fired (CTR=0), next tick keeps applying last value
    lda PLAYER_ATTDEC_NF1,x
    cmp #$FF
    beq @attdec_pre
    sta PLAYER_NEXT_ATT_DEC,x
    lda #$FF
    sta PLAYER_ATTDEC_NF1,x
    bne @attdec_tail_check
@attdec_pre:
    lda PLAYER_ATTDEC_PRE,x
    sta PLAYER_NEXT_ATT_DEC,x
@attdec_tail_check:
    dec PLAYER_TAIL_CTR,x
    bne @attdec_done
    lda PLAYER_ATTDEC_TAIL,x
    sta PLAYER_NEXT_ATT_DEC,x
@attdec_done:
    ; ADSR delay bug: the last FRAMES_BEFORE_NOTE_OFF frames of every note
    ; run with D=0 so the envelope rate counter has wrapped and settled below
    ; the R=0 period before the next note's gate-off/gate-on. A write that
    ; lowers the rate period below the counter freezes the envelope for up to
    ; ~1.7 frames, so the lead must be written this early.
    lda ZP_DUR_CTR,x
    cmp #FRAMES_BEFORE_NOTE_OFF
    bcs @attdec_ret
    lda PLAYER_NEXT_ATT_DEC,x
    and #$F0
    sta PLAYER_NEXT_ATT_DEC,x
@attdec_ret:
    rts

@fetch:
    txa
    asl
    tax                     ; X = 2*channel
    lda ZP_BLOCK_CTR,x
    clc
    bne @fetch_go_check     ; inside a block: not done
    lda ZP_STREAM,x
    cmp pl_stream_bounds+2,x
    lda ZP_STREAM+1,x
    sbc pl_stream_bounds+3,x   ; C=1 when the stream pointer reached its end
@fetch_go_check:
    ldx PL_CH
    bcc @fetch_go
    jmp stage_silence
@fetch_go:
    jsr fetch_next          ; A = pid (C=0) or restDurIdx (C=1)
    bcs @rest

    ; ── note start: A = pid ──────────────────────────────────────────────────
    ; Determine current-note envelope-mode flag first (needed for the
    ; GateReset/TestPulse decision below) without disturbing pid.
    pha                     ; save pid
    tax
    lda NOTE_PAT_PACK_BASE,x
    lsr
    lsr
    and #$3C                ; instrID*4
    tay
    lda MUSIC_INSTRUMENTS,y
    and #2                  ; envelope mode flag (current note)
    sta PL_BLK_HI

    ; Gate reset / test pulse: the SID's ADSR generator does not restart the
    ; attack phase unless gate transitions 0→1, so back-to-back notes (gate
    ; held at 1 throughout) would otherwise inherit whatever envelope level
    ; the previous note left behind. Both cases queue 1-2 literal ctrl bytes
    ; (waveform forced to 0) as PLAYER_CTRL_PRE for apply_prev_frame to
    ; replay at the start of the NEXT tick, before the note's own ctrl value.
    ldx PL_CH
    lda PLAYER_HAS_PREV_NOTE,x
    beq @gate_done           ; first note ever: nothing to reset from
    lda PLAYER_PREV_ENV_MODE,x
    and PL_BLK_HI
    beq @gate_reset          ; not (prev envelope AND current envelope) → GateReset

    ; TestPulse: queue test-bit-on then test-bit-off (gate=1, waveform=0).
    lda #$89
    sta PLAYER_CTRL_PRE,x
    bne @gate_done
@gate_reset:
    ; GateReset: queue a single gate-off write.
    lda #$80
    sta PLAYER_CTRL_PRE,x
@gate_done:
    lda #1
    sta PLAYER_HAS_PREV_NOTE,x
    lda PL_BLK_HI
    sta PLAYER_PREV_ENV_MODE,x
    pla                      ; pid
    jmp init_note_from_pid

@rest:
    ; ── rest start: A = restDurIdx ───────────────────────────────────────────
    tax
    lda MUSIC_REST_DURS,x
    ldx PL_CH
    sta ZP_DUR_CTR,x
    dec ZP_DUR_CTR,x        ; first rest frame consumed now
    ; fall through
.endproc

; stage_silence  (X = channel): gate off, no-op CIA alternation
.proc stage_silence
    lda #0
    sta PLAYER_NEXT_CTRL_A,x
    sta PLAYER_NEXT_CTRL_B,x
    rts
.endproc
; ─────────────────────────────────────────────────────────────────────────────
; init_note_from_pid  (A = pid; PL_CH set by caller)
; Computes a note's full register set and stages it into PLAYER_NEXT_*
; (AttDec/SusRel/CtrlA/CtrlB), plus the CIA toggle-period arm (which
; stays immediate — see compute_next_frame's header comment) and FM/NPT
; init. Called from @note in the same tick the note is fetched —
; any GateReset/TestPulse transient bytes are queued separately (see
; PLAYER_CTRL_PRE) for apply_prev_frame to replay ahead of
; this note's own ctrl value next tick.
; Clobbers: A, X, Y, PL_PID, PL_TMP, PL_BLK_IDX, PL_BLK_LO, PL_BLK_HI
; ─────────────────────────────────────────────────────────────────────────────
.proc init_note_from_pid
    sta PL_PID              ; save pid for FM init later
    tax
    lda NOTE_PAT_FREQ_BASE,x
    sta PL_BLK_IDX          ; save freqIdx for CIA toggle lookup
    asl
    tay                     ; Y = freqIdx * 2
    lda NOTE_PAT_PACK_BASE,x
    pha                     ; push packed byte (instrID<<4|durIdx)
    ldx PL_CH

    lda MUSIC_FREQ_TABLE,y
    sta PLAYER_FM_FREQ_LO,x
    lda MUSIC_FREQ_TABLE+1,y
    sta PLAYER_FM_FREQ_HI,x

    pla
    pha
    lsr
    lsr
    and #$3C                ; instrID*4
    tay                     ; Y = instrument record index

    ; CtrlA: wave<<4 | gate, with the envelope/NPT flags (bits 2-1) masked off
    lda MUSIC_INSTRUMENTS,y
    sta PL_BLK_LO
    and #$F1
    sta PLAYER_NEXT_CTRL_A,x

    ; CtrlB (lo-phase): $51 if envelope mode (instrument flag bit 1),
    ; else same as CtrlA — CIA alternation is then a harmless no-op.
    lda MUSIC_INSTRUMENTS,y
    and #2
    beq @ctrlb_mirror
    lda #$51                ; pulse+triangle+gate (lo-phase for envelope toggle)
    bne @ctrlb_store
@ctrlb_mirror:
    lda PLAYER_NEXT_CTRL_A,x
@ctrlb_store:
    sta PLAYER_NEXT_CTRL_B,x

    ; AttDec: DS byte at INSTRUMENTS+1,y = (attack<<4)|decay with the static
    ; sustain index in bits 2-1.
    ; AttDec_init = (attack<<4)|attack  (hold A phase while attack is active)
    ; AttDec_tail = (attack<<4)|decay   (written when tail fires)
    lda MUSIC_INSTRUMENTS+2,y
    sta PLAYER_TAIL_CTR,x
    lda MUSIC_INSTRUMENTS+1,y
    and #$19
    sta PLAYER_ATTDEC_TAIL,x
    and #$10
    sta PL_TMP              ; attack nibble ($00 or $10)
    ; ATTDEC_NF1: attack<<4 if attack>0, else $FF (nf=1 special case)
    bne @nf1_atk
    lda #$FF
@nf1_atk:
    sta PLAYER_ATTDEC_NF1,x
    lda PL_TMP
    lsr
    lsr
    lsr
    lsr
    ora PL_TMP              ; (attack<<4)|attack
    sta PLAYER_ATTDEC_PRE,x
    sta PLAYER_NEXT_ATT_DEC,x

    ; SusRel init: load sustain start value and init bitstream.
    ; Y = instrID*4 throughout. PL_CH valid.
    ldx PL_CH
    lda #0
    sta PLAYER_SUS_PTR,x
    sta PLAYER_SUS_END,x
    sta PLAYER_SUS_BITS,x
    lda MUSIC_INSTRUMENTS+3,y   ; byte offset of the pattern in MUSIC_SUSTAIN_PATS
    tay
    lda MUSIC_SUSTAIN_PATS,y
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
    iny                         ; body start
    tya
    sta PLAYER_SUS_PTR,x
    pla
    and #$3F                    ; body length
    clc
    adc PLAYER_SUS_PTR,x
    sta PLAYER_SUS_END,x
    jsr stage_sus_rel

    ; Set duration counter from DUR_TABLE[durIdx]
    pla                     ; pop packed byte
    and #$0F                ; durIdx
    tax
    lda MUSIC_DUR_TABLE,x
    ldx PL_CH
    sta ZP_DUR_CTR,x
    dec ZP_DUR_CTR,x        ; first frame consumed now
    lda #0
    sta PLAYER_NF,x

    ; Stage the new CIA toggle period (ch0/ch1 only; ch2 has no toggle).
    ; apply_toggle_period commits it from apply_prev_frame next tick.
    cpx #2
    beq @no_cia

    ; TOGGLE_IDX[freqIdx/2] holds two nibbles (even freqIdx low, odd high)
    ; indexing TOGGLE_VALS. Always a valid nonzero period, even for notes
    ; with no real CIA alternation (CtrlA==CtrlB there, so toggling is a no-op).
    lda PL_BLK_IDX
    lsr
    tax
    lda MUSIC_TOGGLE_IDX,x
    bcc @ti_lo
    lsr
    lsr
    lsr
    lsr
@ti_lo:
    and #$0F
    asl
    tay
    ldx PL_CH
    lda MUSIC_TOGGLE_VALS,y
    sta PLAYER_TOGGLE_LO,x
    lda MUSIC_TOGGLE_VALS+1,y
    sta PLAYER_TOGGLE_HI,x
@no_cia:
    jsr init_fm_note
    jmp init_npt_note
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; fetch_next  (X = channel)
; Returns: A = pid (C=0) or restDurIdx (C=1).
; ─────────────────────────────────────────────────────────────────────────────
.proc fetch_next
    txa
    asl
    tax                     ; X = 2*channel
@again:
    lda ZP_BLOCK_CTR,x
    bne @from_block
    jsr read_byte
    cmp #$EE
    bcs @s_rest             ; b >= 0xEE → rest
    cmp PLAYER_BLOCK_COUNT
    bcs @s_note             ; b >= blockCount → direct note (pid = b - blockCount)

    ; ── enter block ──────────────────────────────────────────────────────────
    sta PL_BLK_IDX
    lda ZP_STREAM,x
    sta ZP_RETURN,x
    lda ZP_STREAM+1,x
    sta ZP_RETURN+1,x

    ; Seek to block: sum BLOCK_LENGTHS[0..idx-1] from MUSIC_BLOCKS base
    lda #<MUSIC_BLOCKS
    sta ZP_STREAM,x
    lda #>MUSIC_BLOCKS
    sta ZP_STREAM+1,x
    ldy #0
@blk_seek:
    cpy PL_BLK_IDX
    beq @blk_found
    lda ZP_STREAM,x
    clc
    adc MUSIC_BLOCK_LENGTHS,y
    sta ZP_STREAM,x
    bcc @sk_hi_ok
    inc ZP_STREAM+1,x
@sk_hi_ok:
    iny
    bne @blk_seek
@blk_found:
    jsr read_byte           ; block length
    sta ZP_BLOCK_CTR,x
    jmp @again              ; read first byte from block body

@s_rest:
    sbc #$EE                ; restDurIdx = b - 0xEE (C=1: rest)
    rts

@s_note:
    sbc PLAYER_BLOCK_COUNT  ; pid = b - blockCount
    clc                     ; C=0 note
    rts

    ; ── block body read ───────────────────────────────────────────────────────
@from_block:
    jsr read_byte
    pha
    dec ZP_BLOCK_CTR,x
    bne @bb_not_done
    lda ZP_RETURN,x         ; restore stream pointer
    sta ZP_STREAM,x
    lda ZP_RETURN+1,x
    sta ZP_STREAM+1,x
@bb_not_done:
    pla
    cmp #$EE                ; b >= 0xEE → rest (durIdx = ~b); else pid
    bcs @blk_rest
    clc                     ; C=0 note, A=pid
    rts
@blk_rest:
    eor #$FF                ; C=1 rest
    rts
.endproc

; read_byte  (X = 2*channel): A = next byte at ZP_STREAM[ch], pointer advanced
.proc read_byte
    lda (ZP_STREAM,x)
    inc ZP_STREAM,x
    bne @done
    inc ZP_STREAM+1,x
@done:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; step_sustain  (X = channel, PL_CH = channel)
; Advance the sustain bitstream one output frame: read bits LSB-first until a
; 0-bit (= end of frame marker); decrement PLAYER_SUS_VAL for each 1-bit seen.
; If exhausted (PTR == END), the value is unchanged. Either way stages SusRel
; (val << 4); falls through into stage_sus_rel, which init_note_from_pid calls for the start value.
; Clobbers: A, X, PL_TMP. Caller must reload X (= PL_CH) after return.
; ─────────────────────────────────────────────────────────────────────────────
.proc step_sustain
@next_bit:
    lsr PLAYER_SUS_BITS,x
    bne @have_bit
    lda PLAYER_SUS_PTR,x
    cmp PLAYER_SUS_END,x
    beq @done
    tay
    inc PLAYER_SUS_PTR,x
    lda MUSIC_SUSTAIN_PATS,y
    sec
    ror                     ; bit 0 → carry, sentinel above the other 7 bits
    sta PLAYER_SUS_BITS,x
@have_bit:
    bcc @done               ; 0-bit: end of frame
    lda PLAYER_SUS_VAL,x
    beq @next_bit
    dec PLAYER_SUS_VAL,x
    jmp @next_bit
@done:
.endproc

.proc stage_sus_rel
    lda PLAYER_SUS_VAL,x
    asl
    asl
    asl
    asl                     ; val << 4
    sta PLAYER_NEXT_SUS_REL,x
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; apply_toggle_period  (PL_CH set to 0 or 1; ch2 never calls this)
; Called from apply_prev_frame every tick — commits PLAYER_TOGGLE_LO/HI[ch]
; (staged by compute_next_frame the PREVIOUS tick, same one-tick pipeline as
; the ctrl pair, so a note's new toggle period and its new CtrlA/CtrlB pair
; always land on hardware together, never one tick apart — that mismatch
; would let the CIA alternate the OLD waveform pair at the NEW note's
; period, or vice versa) to the channel's CIA latch.
; First checks PLAYER_TOGGLE_LO/HI against PLAYER_TOGGLE_LAST_LO/HI (the
; period actually committed last time this ran) — if unchanged, does
; nothing at all. This is essential now that this proc runs every tick
; rather than only when a period genuinely changes: comparing an UNCHANGED
; period's HI byte against the live, naturally-counting-down CIA counter
; would spuriously read "new HI < counter HI" as soon as the counter ticks
; below that byte, forcing a reload that was never warranted and corrupting
; the timer's natural cadence (observed as wildly wrong toggle periods).
; When the period DOES change: the 6526 always picks up a latch write on
; its next natural underflow, but if the new period is shorter than where
; the counter currently sits, that wait would be audibly late (e.g.
; glissando/vibrato pitch bends) — so this forces an immediate reload
; (LOAD strobe) in that case, matching the "AY-like immediate frequency
; response" documented on the Go reference's CIATimer.UpdatePeriodWithImmediate.
; Also forces a reload if the timer isn't running yet (e.g. the very first
; note ever on this channel — no natural underflow would ever come).
; HI-byte-only compare, matching that reference's own 8-bit shortcut.
; Clobbers: A, X
; ─────────────────────────────────────────────────────────────────────────────
.proc apply_toggle_period
    ldx PL_CH
    lda PLAYER_TOGGLE_LO,x
    cmp PLAYER_TOGGLE_LAST_LO,x
    bne @changed
    lda PLAYER_TOGGLE_HI,x
    cmp PLAYER_TOGGLE_LAST_HI,x
    beq @unchanged
@changed:
    lda PLAYER_TOGGLE_LO,x
    sta PLAYER_TOGGLE_LAST_LO,x
    lda PLAYER_TOGGLE_HI,x
    sta PLAYER_TOGGLE_LAST_HI,x
    txa
    asl
    tay                     ; Y = 2*ch: ch0 → Timer A, ch1 → Timer B
    lda PLAYER_TOGGLE_HI,x
    cmp CIA1_TA_HI,y        ; new period HI vs live counter HI
    lda PLAYER_TOGGLE_LO,x
    sta CIA1_TA_LO,y
    lda PLAYER_TOGGLE_HI,x
    sta CIA1_TA_HI,y
    lda pl_cia_icr_set,x    ; SET bit, enable the timer's IRQ source
    sta CIA1_ICR
    bcc @force              ; new HI < counter HI: force immediate reload
    lda CIA1_CRA,x
    and #1
    beq @force              ; timer currently stopped: force immediate reload
@unchanged:
    rts
@force:
    lda #$17                ; LOAD+START+PBON+OUTMODE: force immediate reload
    sta CIA1_CRA,x
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
; player_cia1a_apply_prb is the entry point for harnesses/emulators that
; don't simulate the 6526's PBON+OUTMODE hardware pin-toggle (e.g. websid):
; they track the flip in a software byte and run their own
; "BIT <that byte>" before jumping in here — same V flag, no AND needed,
; so this body serves both the real hardware read and the simulated one.
.proc player_cia1a_fire
    bit CIA1_PRB
player_cia1a_apply_prb:
    bvc @lo

@hi:
    ; PW is set once at note-start and left untouched here: no CIA-armed
    ; CtrlA/CtrlB pair ever pairs pulse (0x04) with a different waveform in
    ; the other phase (NPT's mixed/noise-only states never use 0x04; an
    ; envelope-mode instrument with BaseWaveform=pulse would violate this,
    ; but none currently exist).
    lda PLAYER_FRAME_CTRL_A
    sta SID+SID_CTRL
    rts

@lo:
    lda PLAYER_FRAME_CTRL_B
    sta SID+SID_CTRL
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; player_cia1b_fire / player_cia1b_apply_prb
; ─────────────────────────────────────────────────────────────────────────────
; CIA1 Timer B runs in continuous mode with PBON+OUTMODE=toggle (CRB=$17):
; see player_cia1a_fire — reads PB7 from PRB ($DC01) instead of tracking
; phase in software, and see player_cia1a_apply_prb for the harness entry
; point convention (BIT's N flag mirrors bit7 directly, no AND needed).
.proc player_cia1b_fire
    bit CIA1_PRB
player_cia1b_apply_prb:
    bmi @hi

@lo:
    lda PLAYER_FRAME_CTRL_B+1
    sta SID+7+SID_CTRL
    rts

@hi:
    ; PW is set once at note-start and left untouched here: no CIA-armed
    ; CtrlA/CtrlB pair ever pairs pulse (0x04) with a different waveform in
    ; the other phase (NPT's mixed/noise-only states never use 0x04; an
    ; envelope-mode instrument with BaseWaveform=pulse would violate this,
    ; but none currently exist).
    lda PLAYER_FRAME_CTRL_A+1
    sta SID+7+SID_CTRL
    rts
.endproc

; ── Tables ────────────────────────────────────────────────────────────────────

; each stream starts where the previous one ends
pl_stream_bounds:
    .word MUSIC_STREAM_A, MUSIC_STREAM_A_END, MUSIC_STREAM_B_END, MUSIC_STREAM_C_END

pl_cia_icr_set:
    .byte $81, $82

pl_voice_base:
    .byte 0, 7, 14

pl_sustain_vals:
    .byte 10, 12, 14, 15

; NPT wave state (0 tone-only, 1 mixed, 2 noise-only) → CtrlA / CtrlB
pl_npt_ctrl_a:
    .byte $41, $81, $81
pl_npt_ctrl_b:
    .byte $41, $51, $81

; ─────────────────────────────────────────────────────────────────────────────
; init_fm_note  (PL_CH, PL_PID, PLAYER_FM_FREQ set by caller)
; Sets up PLAYER_FM_TYPE/PTR/END/HOLD/PHASE/FREQ/CYC for the new note.
; A seq is a headCycle whose cycle is its last entry.
; Clobbers: A, X, Y, PL_TMP, PL_BLK_IDX
; ─────────────────────────────────────────────────────────────────────────────
.proc init_fm_note
    lda PL_PID
    cmp #MUSIC_FREQ_MOD_COUNT
    ldx PL_CH
    bcs @no_record
    ldx PL_PID
    lda NOTE_PAT_FM_BASE,x
    tax
    lda MUSIC_FM_OFFSETS,x  ; byte offset within FREQ_MOD section
    ldx PL_CH
    tay
    lda MUSIC_FREQ_MOD,y    ; header: type<<4 | nibble
    iny                     ; Y = data start
    pha
    lsr
    lsr
    lsr
    lsr
    sta PLAYER_FM_TYPE,x
    pla
    and #$0F
    sta PL_TMP              ; nibble
    lda PLAYER_FM_TYPE,x
    bne @type_headcycle
    ; seq: nibble entries; head = nibble-1, cycle = the last entry
    ldx PL_TMP
    dex
    stx PL_TMP
    lda #1
    bne @cycle_setup
@no_record:
    lda PL_BLK_LO           ; instrument byte 0: bit 3 = vibrato
    and #8
    bne @type_dv
@fm_none:
    lda #$FF
    sta PLAYER_FM_TYPE,x
    rts

@type_dv:
    lda #1
    sta PLAYER_FM_TYPE,x
    ; DV: step = ((top + top>>4)^2) >> 6 with top = base freq >> 7
    lda PLAYER_FM_FREQ_LO,x
    asl
    lda PLAYER_FM_FREQ_HI,x
    rol
    sta PL_TMP
    lsr
    lsr
    lsr
    lsr
    clc
    adc PL_TMP
    sta PL_TMP
    sta PL_BLK_IDX
    lda #0
    ldy #8
    lsr PL_TMP
@square:
    bcc @no_add
    clc
    adc PL_BLK_IDX
@no_add:
    ror
    ror PL_TMP
    dey
    bne @square
    sta PLAYER_FM_END,x
    lda PL_TMP
    sta PLAYER_FM_PTR,x
    ldy #6
@shift:
    lsr PLAYER_FM_END,x
    ror PLAYER_FM_PTR,x
    dey
    bne @shift
    lda #0
    sta PLAYER_FM_PHASE,x
    lda #12
    sta PLAYER_FM_HOLD,x
    rts

@type_headcycle:
    ; headCycle: data[0]=headLen, data[1]=cycleLen; ptr=data+2
    lda MUSIC_FREQ_MOD,y
    sta PL_TMP
    lda MUSIC_FREQ_MOD+1,y
    iny
    iny
@cycle_setup:
    sta PL_BLK_IDX          ; cycleLen
    ldx PL_CH
    tya
    sta PLAYER_FM_PTR,x
    lda PL_TMP              ; headLen
    asl
    clc
    adc PLAYER_FM_PTR,x
    sta PLAYER_FM_CYC,x
    lda PL_BLK_IDX
    asl
    clc
    adc PLAYER_FM_CYC,x
    sta PLAYER_FM_END,x
    jmp fm_load_freq
.endproc

; fm_load_freq  (X = channel, Y = offset into MUSIC_FREQ_MOD)
.proc fm_load_freq
    lda MUSIC_FREQ_MOD,y
    sta PLAYER_FM_FREQ_LO,x
    lda MUSIC_FREQ_MOD+1,y
    sta PLAYER_FM_FREQ_HI,x
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; step_fm  (X = channel)
; Advances FM state by one frame and updates PLAYER_FM_FREQ_LO/HI[ch].
; Clobbers: A, X, Y, PL_BLK_LO, PL_BLK_HI
; ─────────────────────────────────────────────────────────────────────────────
.proc step_fm
    lda PLAYER_FM_TYPE,x
    bpl @has_fm
    rts                     ; no FM: keep base freq
@has_fm:
    cmp #1
    beq @type_dv

    ; seq / headCycle: advance each frame, wrap at end
    lda PLAYER_FM_PTR,x
    clc
    adc #2
    cmp PLAYER_FM_END,x
    bne @cycle_advance
    lda PLAYER_FM_CYC,x
@cycle_advance:
    sta PLAYER_FM_PTR,x
    tay
    jmp fm_load_freq

@type_dv:
    ; DV: hold 12 frames, then step up, up, down x4, up, up (triangle of 1,2,1,0,-1,-2,-1,0)
    lda PLAYER_FM_HOLD,x
    beq @dv_vibrate
    dec PLAYER_FM_HOLD,x
    rts
@dv_vibrate:
    lda PLAYER_FM_PHASE,x
    inc PLAYER_FM_PHASE,x
    clc
    adc #2
    and #4
    bne @dv_down
    lda PLAYER_FM_FREQ_LO,x
    clc
    adc PLAYER_FM_PTR,x
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_FREQ_HI,x
    adc PLAYER_FM_END,x
    sta PLAYER_FM_FREQ_HI,x
    rts
@dv_down:
    lda PLAYER_FM_FREQ_LO,x
    sec
    sbc PLAYER_FM_PTR,x
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_FREQ_HI,x
    sbc PLAYER_FM_END,x
    sta PLAYER_FM_FREQ_HI,x
    rts

.endproc

; ─────────────────────────────────────────────────────────────────────────────
; init_npt_note  (PL_CH, PL_PID set by caller)
; Scans MUSIC_NPT_WAVE_OVERRIDES (pid, offset) for PL_PID. The offset indexes
; a sequence in MUSIC_NPT_WAVE_SEQS: waveByteLen, packed wave states, then a
; frame count and toggle uint16s. A frame count of $FF marks a per-occurrence
; list instead: each note using it takes the next two entries (first frame,
; then held).
; If found: sets up PLAYER_NPT_* state. If not found: clears NPT_ACTIVE.
; Clobbers: A, X, Y, PL_TMP
; ─────────────────────────────────────────────────────────────────────────────
.proc init_npt_note
    ldx PL_CH
    lda #0
    sta PLAYER_NPT_ACTIVE,x
    tay
@scan:
    cpy #MUSIC_NPT_OVR_COUNT * 2
    beq @done
    lda MUSIC_NPT_WAVE_OVERRIDES,y
    iny
    iny
    cmp PL_PID
    bne @scan
    lda MUSIC_NPT_WAVE_OVERRIDES-1,y
    sta PLAYER_NPT_WAVE,x
    tay
    clc
    adc MUSIC_NPT_WAVE_SEQS,y   ; + waveByteLen
    adc #1
    tay                         ; Y → frameCount
    lda MUSIC_NPT_WAVE_SEQS,y
    iny                         ; Y → toggle[0]
    cmp #$FF
    bne @counted
    sty PL_TMP                  ; list start
    tya
    clc
    adc PLAYER_NPT_OCC,x
    tay
    inc PLAYER_NPT_OCC,x
    lda MUSIC_NPT_WAVE_SEQS,y   ; offset of this note's pair
    clc
    adc PL_TMP
    tay
    lda #2
@counted:
    sec
    sbc #1                      ; frames remaining after toggle[0]
    sta PLAYER_NPT_TOG_FC,x
    jsr npt_read_toggle
    lda #1
    sta PLAYER_NPT_ACTIVE,x
    jmp npt_stage_wave
@done:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; npt_read_toggle  (X = channel, Y = offset of a toggle uint16)
; Loads it as the channel's toggle period and leaves
; PLAYER_NPT_TOG pointing at the next entry.
; ─────────────────────────────────────────────────────────────────────────────
.proc npt_read_toggle
    lda MUSIC_NPT_WAVE_SEQS,y
    sta PLAYER_TOGGLE_LO,x
    lda MUSIC_NPT_WAVE_SEQS+1,y
    sta PLAYER_TOGGLE_HI,x
    iny
    iny
    tya
    sta PLAYER_NPT_TOG,x
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; npt_stage_wave  (X = channel, PLAYER_NF = note frame)
; Stages the CtrlA/CtrlB pair for the frame's wave state: 2 bits per frame,
; LSB first; frames past the packed bytes are mixed. A sequence without packed
; bytes (an envelope note's toggle-only sequence) leaves the instrument's pair.
; Clobbers: A, X, Y, PL_TMP
; ─────────────────────────────────────────────────────────────────────────────
.proc npt_stage_wave
    ldy PLAYER_NPT_WAVE,x
    lda MUSIC_NPT_WAVE_SEQS,y   ; waveByteLen
    beq @keep                   ; 0: keep the instrument's CtrlA/CtrlB
    sta PL_TMP
    lda PLAYER_NF,x
    lsr
    lsr                         ; byte index
    cmp PL_TMP
    bcs @mixed
    sta PL_TMP
    tya
    clc
    adc PL_TMP
    tay
    lda PLAYER_NF,x
    and #3
    tax                         ; X = nf & 3: shift by 2 bits per step
    lda MUSIC_NPT_WAVE_SEQS+1,y
    inx
@shift:
    dex
    beq @shifted
    lsr
    lsr
    bpl @shift
@shifted:
    and #3
    jmp @stage
@mixed:
    lda #1
@stage:
    tay
    ldx PL_CH
    lda pl_npt_ctrl_a,y
    sta PLAYER_NEXT_CTRL_A,x
    lda pl_npt_ctrl_b,y
    sta PLAYER_NEXT_CTRL_B,x
@keep:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; step_npt  (PL_CH set; PLAYER_NF already incremented by caller)
; Stages PLAYER_NEXT_CTRL_A/B and updates PLAYER_TOGGLE_LO/HI
; (toggle period, applied immediately by the caller via apply_toggle_period).
; No-op if PLAYER_NPT_ACTIVE[PL_CH]=0.
; Clobbers: A, X, Y, PL_TMP
; ─────────────────────────────────────────────────────────────────────────────
.proc step_npt
    ldx PL_CH
    lda PLAYER_NPT_ACTIVE,x
    beq @done
    jsr npt_stage_wave
    ldx PL_CH
    lda PLAYER_NPT_TOG_FC,x
    beq @done               ; past the list: the last toggle stays
    dec PLAYER_NPT_TOG_FC,x
    ldy PLAYER_NPT_TOG,x
    jmp npt_read_toggle
@done:
    rts
.endproc
