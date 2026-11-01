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
; This is the "active" register file: player_tick's apply step copies
; PLAYER_NEXT_FRAME into here (and to SID hardware) at the start of every
; tick, one tick after compute_next_frame staged it. The two CIA hardware
; handlers (player_cia1a_fire/player_cia1b_fire) read CtrlA/CtrlB (+4/+5)
; straight from here, at their own asynchronous hardware cadence.
PLAYER_FRAME:      .res 27

; Staged output of compute_next_frame — same 9-field stride-9 shape as
; PLAYER_FRAME. Written this tick, applied (copied into PLAYER_FRAME/SID)
; at the START of the NEXT tick. Never written to SID hardware directly.
; A field left untouched this tick simply carries its last-staged value
; forward (persistent BSS, not cleared between ticks).
PLAYER_NEXT_FRAME: .res 27

; Pending immediate SID_CTRL pre-writes, per channel — the GateReset
; (gate 1→0) and TestPulse (test 0→1→0) sequences write SID_CTRL 1-2 times
; in a row within the SAME tick a note starts, before the note's own final
; ctrl value. Under the one-tick pipeline those writes can't land on
; hardware immediately, so they're queued here (in order) and replayed by
; apply_prev_frame at the very start of the NEXT tick, immediately before
; the normal CtrlA/CtrlB handoff — reproducing the exact same 1-2-then-final
; write sequence as before, just shifted one tick later as a block.
; COUNT=0: nothing pending. COUNT=1: replay PRE0 only. COUNT=2: replay
; PRE0 then PRE1.
PLAYER_CTRL_PRE_COUNT: .res 3
PLAYER_CTRL_PRE0:      .res 3
PLAYER_CTRL_PRE1:      .res 3

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
;   FM_PTR:      byte offset of the current entry (MUSIC_FREQ_MOD; DV: MUSIC_DELAYED_VIB template)
;   FM_END:      byte offset one past the last entry (seq/gliss/headCycle boundary)
;   FM_HOLD:     frames remaining in hold phase (DV/gliss: 12, counts to 0)
;   FM_PHASE:    DV phase counter 0-7
;   FM_FREQ_LO/HI: current FM output frequency (updated each sustain frame)
;   FM_BASE_LO/HI: note base frequency (for DV delta computation)
;   FM_CYC:      offset the entry pointer wraps to at FM_END (a seq holds its last entry)
PLAYER_FM_TYPE:    .res 3
PLAYER_FM_PTR:     .res 3
PLAYER_FM_END:     .res 3
PLAYER_FM_HOLD:    .res 3
PLAYER_FM_PHASE:   .res 3
PLAYER_FM_FREQ_LO: .res 3
PLAYER_FM_FREQ_HI: .res 3
PLAYER_FM_BASE_LO: .res 3
PLAYER_FM_BASE_HI: .res 3
PLAYER_FM_CYC:     .res 3

; NPT (NoisePlusTone) wave+toggle override state per channel:
;   NPT_ACTIVE:  1 if override in effect, 0 otherwise
;   NPT_WAVE:    offset of the waveByteLen byte in NPT_WAVE_SEQS
;   NPT_TOG:     offset of the next toggle uint16 in NPT_WAVE_SEQS
;   NPT_TOG_FC:     frames remaining in toggle sequence (0 = hold last)
;   NPT_TOG_LST_LO/HI: last toggle value (held when past end)
;   NPT_OCC:     byte cursor into a per-occurrence toggle list (frameCount $FF),
;                advanced by one entry pair for every note that uses the list
;   NF:              note frame counter (0-based, per channel)
PLAYER_NPT_ACTIVE:      .res 3
PLAYER_NPT_WAVE:        .res 3
PLAYER_NPT_TOG:         .res 3
PLAYER_NPT_OCC:         .res 3
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
PLAYER_CLEAR_END:

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

    ldx #PLAYER_CLEAR_END - PLAYER_NEXT_FRAME
    lda #0
@clr_state:
    sta PLAYER_NEXT_FRAME-1,x
    dex
    bne @clr_state

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

    ldx #2
    lda #0
@clr_zp:
    sta ZP_RETURN_LO,x
    sta ZP_RETURN_HI,x
    sta ZP_BLOCK_CTR,x
    sta ZP_DUR_CTR,x
    dex
    bpl @clr_zp

    lda #$FF
    sta PLAYER_ATTDEC_NF1+0
    sta PLAYER_ATTDEC_NF1+1
    sta PLAYER_ATTDEC_NF1+2
    sta PLAYER_FM_TYPE+0
    sta PLAYER_FM_TYPE+1
    sta PLAYER_FM_TYPE+2
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; player_tick  — call once per PAL frame
; ─────────────────────────────────────────────────────────────────────────────
; Three-phase pipeline per channel, run channel-by-channel (each channel
; fully passes through all 3 phases before the next channel starts — no
; cross-channel dependency exists, so this ordering is equivalent to 3
; separate full loops but touches PL_CH/PL_VBASE/PL_FBASE fewer times):
;   1. apply_prev_frame:  copy PLAYER_NEXT_FRAME (staged last tick) into
;      PLAYER_FRAME and to SID hardware. For ch0/1, ctrl only updates
;      PLAYER_FRAME — never SID_CTRL directly.
;   2. CIA catch-up (ch0/1 only): JSR the exact same routine the CIA1
;      Timer A/B hardware IRQ calls, so a freshly-applied CtrlA/CtrlB pair
;      reaches SID_CTRL within this tick rather than waiting an arbitrary
;      (possibly ~13000-cycle) amount of time for the next real underflow.
;      The real CIA IRQ keeps firing independently between ticks too —
;      this call never replaces or caps that.
;   3. compute_next_frame: run note-start/sustain/rest logic, staging all
;      outputs into PLAYER_NEXT_FRAME for phase 1 to apply next tick.
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
    jsr player_cia1a_fire
    jmp @ch2_no_cia
@ch1_catchup:
    jsr player_cia1b_fire
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
; Phase 1: copies PLAYER_NEXT_FRAME (staged by compute_next_frame on the
; PREVIOUS tick) into PLAYER_FRAME and, for freq/PW/AttDec/SusRel, straight
; to SID hardware. For ch0/1's ctrl pair, only PLAYER_FRAME is updated —
; SID_CTRL is left for phase 2 (the CIA-fire catch-up call) to write, so
; this proc and the real CIA hardware IRQ are never both writing SID_CTRL
; for the same channel from two different, uncoordinated cadences. ch2 has
; no CIA, so its ctrl is written straight to SID here.
; Clobbers: A, X
; ─────────────────────────────────────────────────────────────────────────────
.proc apply_prev_frame
    stx PL_CH

    ; Set up voice_base and frame_base for this channel
    lda pl_voice_base,x
    sta PL_VBASE
    lda pl_frame_base,x
    sta PL_FBASE

    ; Replay any queued immediate ctrl pre-writes (GateReset/TestPulse's
    ; gate-off / test-bit-on-then-off sequence) FIRST, in the order they
    ; were staged last tick — reproducing the exact same multi-write
    ; sequence the original single-tick code did, just shifted one tick
    ; later as a block. The note's own final ctrl value follows via the
    ; normal CtrlA/CtrlB handoff below (ch2 direct; ch0/1 via phase 2's
    ; CIA-fire catch-up call, which runs right after this proc returns).
    ; Each pre-value is stored into BOTH PLAYER_FRAME ctrl slots first, so a
    ; CIA fire preempting this sequence rewrites the same pre-value instead
    ; of the stale ctrl pair.
    ldx PL_CH
    lda PLAYER_CTRL_PRE_COUNT,x
    beq @no_pre
    lda PLAYER_CTRL_PRE0,x
    ldx PL_FBASE
    sta PLAYER_FRAME+4,x
    sta PLAYER_FRAME+5,x
    ldx PL_VBASE
    sta SID+SID_CTRL,x
    ldx PL_CH
    lda PLAYER_CTRL_PRE_COUNT,x
    cmp #2
    bne @pre_done
    lda PLAYER_CTRL_PRE1,x
    ldx PL_FBASE
    sta PLAYER_FRAME+4,x
    sta PLAYER_FRAME+5,x
    ldx PL_VBASE
    sta SID+SID_CTRL,x
    ldx PL_CH
@pre_done:
    lda #0
    sta PLAYER_CTRL_PRE_COUNT,x
@no_pre:

    ; FreqLo/Hi (X must be PL_FBASE to index PLAYER_NEXT_FRAME's stride-9 array)
    ldx PL_FBASE
    lda PLAYER_NEXT_FRAME+0,x
    sta PL_TMP
    lda PLAYER_NEXT_FRAME+1,x
    sta PL_BLK_LO
    ldx PL_VBASE
    lda PL_TMP
    sta SID+SID_FREQ_LO,x
    lda PL_BLK_LO
    sta SID+SID_FREQ_HI,x

    ; PWLo/Hi
    ldx PL_FBASE
    lda PLAYER_NEXT_FRAME+2,x
    sta PL_TMP
    lda PLAYER_NEXT_FRAME+3,x
    sta PL_BLK_LO
    ldx PL_VBASE
    lda PL_TMP
    sta SID+SID_PW_LO,x
    lda PL_BLK_LO
    sta SID+SID_PW_HI,x

    ; AttDec, SusRel
    ldx PL_FBASE
    lda PLAYER_NEXT_FRAME+6,x
    sta PL_TMP
    lda PLAYER_NEXT_FRAME+7,x
    sta PL_BLK_LO
    ldx PL_VBASE
    lda PL_TMP
    sta SID+SID_ATT_DEC,x
    lda PL_BLK_LO
    sta SID+SID_SUS_REL,x

    ; CtrlA/CtrlB: update state memory (PLAYER_FRAME) unconditionally.
    ldx PL_FBASE
    lda PLAYER_NEXT_FRAME+4,x
    sta PLAYER_FRAME+4,x    ; CtrlA
    lda PLAYER_NEXT_FRAME+5,x
    sta PLAYER_FRAME+5,x    ; CtrlB

    ; ch2 has no CIA fire handler to write SID_CTRL for it in phase 2 —
    ; write CtrlA straight to hardware here instead. X is still PL_FBASE
    ; from above, so PLAYER_FRAME+4,x is this channel's CtrlA.
    ldy PL_CH
    cpy #2
    bne @ch0_1
    lda PLAYER_FRAME+4,x    ; CtrlA (X = PL_FBASE)
    ldx PL_VBASE
    sta SID+SID_CTRL,x
    rts
@ch0_1:
    ; Commit the staged CIA toggle period now too, in the SAME tick the
    ; ctrl pair above became active — apply_toggle_period must never run
    ; a tick apart from this, or the CIA would briefly alternate the OLD
    ; waveform pair at the NEW period (or vice versa). Skip entirely
    ; during rest/silence (CtrlA=0): the original code never called this
    ; during a rest either (PLAYER_TOGGLE_LO/HI just sits stale, harmless
    ; since ctrl is 0/0) — calling it unconditionally every tick would
    ; instead keep re-forcing a reload from that stale period, causing a
    ; CIA fire-rate runaway for the whole rest duration.
    lda PLAYER_FRAME+4,x    ; CtrlA (X = PL_FBASE)
    beq @done
    jsr apply_toggle_period
@done:
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; compute_next_frame  (X = channel)
; Phase 3: on every tick, decrement duration counter.
;   If counter was > 0: sustain (advance FM/NPT/sustain-bitstream state).
;   If counter was 0: fetch next stream entry, set new counter.
; All results are staged into PLAYER_NEXT_FRAME for apply_prev_frame to
; apply on the NEXT tick — this proc never writes SID hardware or
; PLAYER_FRAME directly, except CIA timer PERIOD arming (PLAYER_TOGGLE_LO/HI,
; CIA1_TA/TB_LO/HI, CIA1_CRA/CRB via apply_toggle_period and the note-start
; arm block below) — period is orthogonal to the ctrl waveform PAIR this
; pipeline stages, and already has its own immediate-vs-natural-reload
; logic designed to apply as soon as the new period is known.
; ─────────────────────────────────────────────────────────────────────────────
.proc compute_next_frame
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
    ; Advance sustain bitstream and stage SusRel
    ldx PL_CH
    jsr step_sustain        ; advances PLAYER_SUS_VAL[ch] if bitstream not exhausted
    ldx PL_CH
    lda PLAYER_SUS_VAL,x
    asl
    asl
    asl
    asl                     ; val << 4
    ldx PL_FBASE
    sta PLAYER_NEXT_FRAME+7,x

    ; Advance FM and stage freq
    ldx PL_CH
    jsr step_fm             ; updates PLAYER_FM_FREQ_LO/HI[ch]; clobbers A,X
    ldx PL_CH
    lda PLAYER_FM_FREQ_LO,x
    ldx PL_FBASE
    sta PLAYER_NEXT_FRAME+0,x
    ldx PL_CH
    lda PLAYER_FM_FREQ_HI,x
    ldx PL_FBASE
    sta PLAYER_NEXT_FRAME+1,x
    ; Advance NPT wave/toggle sequence (increments PLAYER_NF, stages ctrl
    ; into PLAYER_NEXT_FRAME — see step_npt)
    ldx PL_CH
    inc PLAYER_NF,x
    jsr step_npt
    ; Stage PW from the newly-staged CtrlA (PLAYER_NEXT_FRAME+4, just
    ; written by step_npt above, or carried over from last tick if NPT is
    ; inactive for this note): 0x800 only while pure pulse ($40) is active,
    ; else 0xFFF.
    ldx PL_FBASE
    lda PLAYER_NEXT_FRAME+4,x    ; staged CtrlA (may have been updated by step_npt)
    and #$F0
    cmp #$40
    bne @sus_pw_default
    lda #$00
    sta PLAYER_NEXT_FRAME+2,x
    lda #$08
    sta PLAYER_NEXT_FRAME+3,x
    jmp @sus_pw_done
@sus_pw_default:
    lda #$FF
    sta PLAYER_NEXT_FRAME+2,x
    lda #$0F
    sta PLAYER_NEXT_FRAME+3,x
@sus_pw_done:
    ; Advance tone sequence and stage PLAYER_TOGGLE (env-mode notes with
    ; tone override). apply_toggle_period commits this to CIA hardware from
    ; apply_prev_frame next tick, synchronized with the ctrl pair — not
    ; called here (see apply_toggle_period's header comment).
    ldx PL_CH
    jsr step_toneseq
@attdec_start:
    ; AttDec: stage every sustain frame.
    ; nf=1 special: ATTDEC_NF1 holds attack<<4 (one-shot); $FF means skip.
    ; After nf=1, stage ATTDEC_PRE = (attack<<4)|attack each frame.
    ; On tail fire, stage ATTDEC_TAIL and stop.
    ldx PL_CH
    lda PLAYER_TAIL_CTR,x
    beq @attdec_done        ; tail already fired (CTR=0), next tick keeps applying last value
    ; nf=1 check
    lda PLAYER_ATTDEC_NF1,x
    cmp #$FF
    beq @attdec_pre
    ; nf=1: stage attack<<4, clear one-shot
    ldx PL_FBASE
    sta PLAYER_NEXT_FRAME+6,x
    ldx PL_CH
    lda #$FF
    sta PLAYER_ATTDEC_NF1,x
    jmp @attdec_tail_check
@attdec_pre:
    ; Normal pre-tail: stage (attack<<4)|attack
    lda PLAYER_ATTDEC_PRE,x
    ldx PL_FBASE
    sta PLAYER_NEXT_FRAME+6,x
    ldx PL_CH
@attdec_tail_check:
    ; Decrement tail counter; fire when it hits 0
    dec PLAYER_TAIL_CTR,x
    bne @attdec_done
    lda PLAYER_ATTDEC_TAIL,x
    ldx PL_FBASE
    sta PLAYER_NEXT_FRAME+6,x
    ldx PL_CH
@attdec_done:
    ; ADSR delay bug: the last FRAMES_BEFORE_NOTE_OFF frames of every note
    ; run with D=0 so the envelope rate counter has wrapped and settled below
    ; the R=0 period before the next note's gate-off/gate-on. A write that
    ; lowers the rate period below the counter freezes the envelope for up to
    ; ~1.7 frames, so the lead must be written this early.
    ldx PL_CH
    lda ZP_DUR_CTR,x
    cmp #FRAMES_BEFORE_NOTE_OFF
    bcs @attdec_ret
    ldx PL_FBASE
    lda PLAYER_NEXT_FRAME+6,x
    and #$F0
    sta PLAYER_NEXT_FRAME+6,x
@attdec_ret:
    rts

@sustain_rest:
    ldx PL_FBASE
    lda #0
    sta PLAYER_NEXT_FRAME+4,x    ; NextCtrlA = gate off
    sta PLAYER_NEXT_FRAME+5,x    ; NextCtrlB = same — no-op alternation
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
    ; Stream exhausted: stage silence (gate off)
    ldx PL_FBASE
    lda #0
    sta PLAYER_NEXT_FRAME+4,x    ; NextCtrlA = gate off
    sta PLAYER_NEXT_FRAME+5,x    ; NextCtrlB = same — no-op alternation
    ldx PL_CH
    rts
@fetch_go:
    jsr fetch_next          ; A = pid (C=0) or restDurIdx (C=1)
    bcc @note
    jmp @rest

@note:
    ; ── note start: A = pid ──────────────────────────────────────────────────
    ; Determine current-note envelope-mode flag first (needed for the
    ; GateReset/TestPulse decision below) without disturbing pid.
    pha                     ; save pid
    tax                     ; X = pid
    lda NOTE_PAT_PACK_BASE,x
    lsr
    lsr
    lsr
    lsr                     ; A = instrID
    asl
    asl                     ; A = instrID*4
    tay
    lda MUSIC_INSTRUMENTS,y
    and #1                  ; envelope mode flag (current note)
    sta PL_BLK_HI

    ; Gate reset / test pulse: the SID's ADSR generator does not restart the
    ; attack phase unless gate transitions 0→1, so back-to-back notes (gate
    ; held at 1 throughout) would otherwise inherit whatever envelope level
    ; the previous note left behind. Both cases queue 1-2 self-contained
    ; literal ctrl bytes (waveform forced to 0) into PLAYER_CTRL_PRE0/1 for
    ; apply_prev_frame to replay at the start of NEXT tick, immediately
    ; before the note's own final ctrl value — reproducing the exact same
    ; multi-write sequence the original single-tick code did, just applied
    ; to hardware one tick later as a block. The note itself (freq/PW/ADSR/
    ; ctrl) is still computed in THIS tick, same frame-counting as before.
    ldx PL_CH
    lda PLAYER_HAS_PREV_NOTE,x
    beq @gate_none           ; first note ever: nothing to reset from
    lda PLAYER_PREV_ENV_MODE,x
    and PL_BLK_HI
    beq @gate_reset          ; not (prev envelope AND current envelope) → GateReset

    ; TestPulse: queue test-bit-on then test-bit-off (gate=1, waveform=0
    ; throughout — matches the literal-byte replacement for the old
    ; live-SID_CTRL-read-modify-write).
    lda #2
    sta PLAYER_CTRL_PRE_COUNT,x
    lda #$09
    sta PLAYER_CTRL_PRE0,x
    lda #$01
    sta PLAYER_CTRL_PRE1,x
    jmp @gate_done
@gate_reset:
    ; GateReset: queue a single gate-off write.
    lda #1
    sta PLAYER_CTRL_PRE_COUNT,x
    lda #$00
    sta PLAYER_CTRL_PRE0,x
@gate_done:
    lda #1
    sta PLAYER_HAS_PREV_NOTE,x
    lda PL_BLK_HI
    sta PLAYER_PREV_ENV_MODE,x
    pla                      ; pid
    jsr init_note_from_pid
    rts
@gate_none:
    ; First note ever: no reset needed.
    lda #1
    sta PLAYER_HAS_PREV_NOTE,x
    lda PL_BLK_HI
    sta PLAYER_PREV_ENV_MODE,x
    pla                      ; pid
    jsr init_note_from_pid
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

    ldx PL_FBASE
    lda #0
    sta PLAYER_NEXT_FRAME+4,x    ; NextCtrlA = gate off
    sta PLAYER_NEXT_FRAME+5,x    ; NextCtrlB = same — no-op alternation
    ldx PL_CH               ; player_tick's loop needs X restored before rts
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; init_note_from_pid  (A = pid; PL_CH/PL_VBASE/PL_FBASE set by caller)
; Computes a note's full register set and stages it into PLAYER_NEXT_FRAME
; (freq/PW/AttDec/SusRel/CtrlA/CtrlB), plus the CIA toggle-period arm (which
; stays immediate — see compute_next_frame's header comment) and FM/NPT/
; tone-seq init. Called from @note in the same tick the note is fetched —
; any GateReset/TestPulse transient bytes are queued separately (see
; PLAYER_CTRL_PRE0/1/PRE_COUNT) for apply_prev_frame to replay ahead of
; this note's own ctrl value next tick.
; Clobbers: A, X, Y, PL_PID, PL_TMP, PL_BLK_IDX, PL_BLK_LO, PL_BLK_HI
; ─────────────────────────────────────────────────────────────────────────────
.proc init_note_from_pid
    sta PL_PID              ; save pid for FM init later
    tax                     ; X = pid

    lda NOTE_PAT_FREQ_BASE,x
    sta PL_BLK_IDX          ; save freqIdx for CIA toggle lookup
    asl
    tay                     ; Y = freqIdx * 2
    lda NOTE_PAT_PACK_BASE,x
    pha                     ; push packed byte (instrID<<4|durIdx)

    ldx PL_FBASE            ; X = frame_base for PLAYER_NEXT_FRAME stores

    ; Stage freq into PLAYER_NEXT_FRAME; also save as FM base freq
    lda MUSIC_FREQ_TABLE,y
    sta PLAYER_NEXT_FRAME+0,x   ; NextFreqLo (X = PL_FBASE here)
    sta PL_TMP              ; save FreqLo
    lda MUSIC_FREQ_TABLE+1,y
    sta PLAYER_NEXT_FRAME+1,x   ; NextFreqHi
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
    ldx PL_FBASE            ; restore X = frame base

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
    sta PL_BLK_HI            ; save waveCompact index (2 = pulse)
    tax
    lda pl_wave_ctrl,x      ; $11/$21/$41/$81
    pha                     ; save ctrl byte
    ; Pulse width: 0x800 (50% duty) only while a pure square/pulse wave is
    ; active (waveCompact index 2 → $40); 0xFFF (silent/no PWM effect)
    ; for every other waveform, including 0x05 (pulse+triangle).
    ldx PL_FBASE
    lda PL_BLK_HI
    cmp #2
    bne @note_pw_default
    lda #$00
    sta PLAYER_NEXT_FRAME+2,x
    lda #$08
    sta PLAYER_NEXT_FRAME+3,x
    jmp @note_pw_done
@note_pw_default:
    lda #$FF
    sta PLAYER_NEXT_FRAME+2,x
    lda #$0F
    sta PLAYER_NEXT_FRAME+3,x
@note_pw_done:
    pla                     ; CtrlA
    sta PL_TMP              ; save CtrlA (staged below once CtrlB is known)

    ; Stage CtrlA
    sta PLAYER_NEXT_FRAME+4,x    ; NextCtrlA (X = PL_FBASE)

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
    sta PLAYER_NEXT_FRAME+5,x    ; NextCtrlB

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
    ; Stage AttDec_init = (attack<<4)|attack
    lda PL_TMP              ; attack nibble
    beq @zero_atk_init
    lda #$11                ; attack=1 in both nibbles
    jmp @have_init
@zero_atk_init:
    lda #0
@have_init:
    ldx PL_CH
    sta PLAYER_ATTDEC_PRE,x   ; pre-tail value for sustain re-stage
    ldx PL_FBASE
    sta PLAYER_NEXT_FRAME+6,x  ; NextAttDec

    ; SusRel init: load sustain start value and init bitstream.
    ; Y = instrID*4 throughout. PL_CH and PL_FBASE valid.
    ldx PL_CH
    lda #0
    sta PLAYER_SUS_PTR,x
    sta PLAYER_SUS_END,x
    sta PLAYER_SUS_BITS,x
    lda MUSIC_INSTRUMENTS,y     ; merged: flags in bits 2-0
    and #1                      ; envelope mode bit
    bne @sus_static
    lda MUSIC_INSTRUMENTS+3,y   ; sus_def_idx
    cmp #$FF
    beq @sus_static

    ; Dynamic sustain: walk MUSIC_SUSTAIN_PATS to entry sus_def_idx
    ldy #0
    tax
    beq @sus_found
@sus_walk:
    lda MUSIC_SUSTAIN_PATS,y
    and #$3F                    ; body length
    sta PL_TMP
    tya
    sec
    adc PL_TMP                  ; + body length + 1 for hdr byte
    tay
    dex
    bne @sus_walk
@sus_found:
    ldx PL_CH
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
    jmp @sus_stage

@sus_static:
    ; No bitstream: static sustain from DS bits 2-1.
    lda MUSIC_INSTRUMENTS+1,y
    lsr
    and #3
    tax
    lda pl_sustain_vals,x
    ldx PL_CH
    sta PLAYER_SUS_VAL,x
@sus_stage:
    lda PLAYER_SUS_VAL,x
    asl
    asl
    asl
    asl
    ldx PL_FBASE
    sta PLAYER_NEXT_FRAME+7,x
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

    ; Stage the new CIA toggle period (ch0/ch1 only; ch2 has no toggle).
    ; apply_toggle_period commits this to hardware from apply_prev_frame
    ; next tick, synchronized with the ctrl pair (see its header comment)
    ; — no immediate CIA1_TA/TB/CRA/CRB writes here.
    cpx #2
    beq @no_cia

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

    ; Store into PLAYER_TOGGLE for apply_toggle_period to commit next tick.
    ldx PL_CH
    lda PL_TMP
    sta PLAYER_TOGGLE_LO,x
    lda PL_BLK_LO
    sta PLAYER_TOGGLE_HI,x
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
    cpx #0
    beq @cia_a
    ; ch1 → CIA1 Timer B
    cmp CIA1_TB_HI          ; new period HI vs live counter HI
    lda PLAYER_TOGGLE_LO,x
    sta CIA1_TB_LO
    lda PLAYER_TOGGLE_HI,x
    sta CIA1_TB_HI
    lda #$82                ; SET bit, enable Timer B IRQ source
    sta CIA1_ICR
    bcc @force_b            ; new HI < counter HI: force immediate reload
    lda CIA1_CRB
    and #1
    beq @force_b            ; timer currently stopped: force immediate reload
    rts
@force_b:
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
    lda #$81                ; SET bit, enable Timer A IRQ source
    sta CIA1_ICR
    bcc @force_a            ; new HI < counter HI: force immediate reload
    lda CIA1_CRA
    and #1
    beq @force_a            ; timer currently stopped: force immediate reload
    rts
@force_a:
    lda #$17                ; LOAD+START+PBON+OUTMODE: force immediate reload
    sta CIA1_CRA
    rts
@unchanged:
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
    lda PLAYER_FRAME + 4
    sta SID+SID_CTRL
    rts

@lo:
    lda PLAYER_FRAME + 5
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
    lda PLAYER_FRAME + 9 + 5
    sta SID+7+SID_CTRL
    rts

@hi:
    ; PW is set once at note-start and left untouched here: no CIA-armed
    ; CtrlA/CtrlB pair ever pairs pulse (0x04) with a different waveform in
    ; the other phase (NPT's mixed/noise-only states never use 0x04; an
    ; envelope-mode instrument with BaseWaveform=pulse would violate this,
    ; but none currently exist).
    lda PLAYER_FRAME + 9 + 4
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

; NPT wave state (0 tone-only, 1 mixed, 2 noise-only) → CtrlA / CtrlB
pl_npt_ctrl_a:
    .byte $41, $81, $81
pl_npt_ctrl_b:
    .byte $41, $51, $81

pl_dv_byte_off:
    .byte 0,2,0,0,4,6,4,0   ; DV template byte offset by phase

pl_dv_negate:
    .byte 0,0,0,0,1,1,1,0   ; DV negate flag (1=subtract delta) by phase

; ─────────────────────────────────────────────────────────────────────────────
; init_fm_note  (PL_CH, PL_PID, PLAYER_FM_BASE set by caller)
; Sets up PLAYER_FM_TYPE/PTR/END/HOLD/PHASE/FREQ/CYC for the new note.
; A seq is a headCycle whose cycle is its last entry.
; Clobbers: A, X, Y, PL_TMP, PL_BLK_IDX
; ─────────────────────────────────────────────────────────────────────────────
.proc init_fm_note
    lda PL_PID
    cmp #MUSIC_FREQ_MOD_COUNT
    ldx PL_CH
    bcs @fm_none
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
    cmp #1
    beq @type_dv
    cmp #3
    beq @type_gliss
    cmp #4
    beq @type_headcycle
    cmp #0
    beq @type_seq
@fm_none:
    lda #$FF
    sta PLAYER_FM_TYPE,x
    rts

@type_dv:
    ; DV: data[0]=dvIdx; template = MUSIC_DELAYED_VIB + dvIdx*8
    lda MUSIC_FREQ_MOD,y
    asl
    asl
    asl
    sta PLAYER_FM_PTR,x
    lda #0
    sta PLAYER_FM_PHASE,x
    beq @hold_12

@type_gliss:
    ; holdGliss: ptr = data start; end = ptr + nibble (bytes); hold=12
    tya
    sta PLAYER_FM_PTR,x
    clc
    adc PL_TMP
    sta PLAYER_FM_END,x
@hold_12:
    lda #12
    sta PLAYER_FM_HOLD,x
    rts

@type_seq:
    ; seq: nibble entries; head = nibble-1, cycle = the last entry
    ldx PL_TMP
    dex
    stx PL_TMP
    lda #1
    bne @cycle_setup

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
    cmp #3
    beq @type_gliss

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

@type_gliss:
    ; holdGliss: hold 12 frames, then add one delta per frame (signed)
    lda PLAYER_FM_HOLD,x
    beq @gliss_step
    dec PLAYER_FM_HOLD,x
    rts
@gliss_step:
    lda PLAYER_FM_PTR,x
    cmp PLAYER_FM_END,x
    beq @gliss_done
    tay
    lda PLAYER_FM_FREQ_LO,x
    clc
    adc MUSIC_FREQ_MOD,y
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_FREQ_HI,x
    adc MUSIC_FREQ_MOD+1,y
    sta PLAYER_FM_FREQ_HI,x
    inc PLAYER_FM_PTR,x
    inc PLAYER_FM_PTR,x
@gliss_done:
    rts

@type_dv:
    ; DV: hold 12 frames then 8-phase vibrato
    lda PLAYER_FM_HOLD,x
    beq @dv_vibrate
    dec PLAYER_FM_HOLD,x
    lda PLAYER_FM_BASE_LO,x ; still in hold: return base freq
    sta PLAYER_FM_FREQ_LO,x
    lda PLAYER_FM_BASE_HI,x
    sta PLAYER_FM_FREQ_HI,x
    rts
@dv_vibrate:
    ldy PLAYER_FM_PHASE,x
    cpy #3
    beq @dv_base_freq
    cpy #7
    beq @dv_base_freq
    lda pl_dv_byte_off,y    ; byte offset into template
    clc
    adc PLAYER_FM_PTR,x
    tay
    lda MUSIC_DELAYED_VIB,y
    sta PL_BLK_LO
    lda MUSIC_DELAYED_VIB+1,y
    sta PL_BLK_HI
    ldy PLAYER_FM_PHASE,x
    lda pl_dv_negate,y
    beq @dv_add
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
    bne @done_base
    lda #0
    sta PLAYER_FM_PHASE,x
@done_base:
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
    tya
    clc
    adc PLAYER_NPT_OCC,x
    tay
    lda PLAYER_NPT_OCC,x
    clc
    adc #4
    sta PLAYER_NPT_OCC,x
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
; Loads it as the channel's toggle period and as the held value, and leaves
; PLAYER_NPT_TOG pointing at the next entry.
; ─────────────────────────────────────────────────────────────────────────────
.proc npt_read_toggle
    lda MUSIC_NPT_WAVE_SEQS,y
    sta PLAYER_TOGGLE_LO,x
    sta PLAYER_NPT_TOG_LST_LO,x
    lda MUSIC_NPT_WAVE_SEQS+1,y
    sta PLAYER_TOGGLE_HI,x
    sta PLAYER_NPT_TOG_LST_HI,x
    iny
    iny
    tya
    sta PLAYER_NPT_TOG,x
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; npt_stage_wave  (X = channel, PLAYER_NF = note frame)
; Stages the CtrlA/CtrlB pair for the frame's wave state: 2 bits per frame,
; LSB first; frames past the packed bytes are mixed.
; Clobbers: A, X, Y, PL_TMP
; ─────────────────────────────────────────────────────────────────────────────
.proc npt_stage_wave
    lda PLAYER_NF,x
    lsr
    lsr
    sta PL_TMP                  ; byte index
    ldy PLAYER_NPT_WAVE,x
    lda MUSIC_NPT_WAVE_SEQS,y   ; waveByteLen
    cmp PL_TMP
    beq @mixed
    bcc @mixed
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
    ldx PL_FBASE
    lda pl_npt_ctrl_a,y
    sta PLAYER_NEXT_FRAME+4,x
    lda pl_npt_ctrl_b,y
    sta PLAYER_NEXT_FRAME+5,x
    rts
.endproc

; ─────────────────────────────────────────────────────────────────────────────
; step_npt  (PL_CH set; PLAYER_NF already incremented by caller)
; Stages PLAYER_NEXT_FRAME's ctrl pair and updates PLAYER_TOGGLE_LO/HI
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
    beq @hold
    dec PLAYER_NPT_TOG_FC,x
    ldy PLAYER_NPT_TOG,x
    jmp npt_read_toggle
@hold:
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
