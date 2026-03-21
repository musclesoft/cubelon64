; Sequencer module - Controls demo timing and state transitions
;
; State counter: 0-39 (continuously looping)
; Phase counter: increments every 40 frames (when state wraps)
; Frame counter: 0-65535 (16-bit)
; Mode: 0=motion, 1=freeze
;
; Based on analysis from docs/MAJOR_TRANSITIONS.md:
; - ZX Spectrum demo has 9 major sections
; - 314 total motion/freeze periods
; - Patterns: 19-motion/21-freeze, 29-frame freezes, long motion sections
;
; Current implementation: Simple 0-39 state counter + phase counter
; Future: Expand to full sequencer with motion/freeze control

.include "include/shared.inc"

.segment "CODE"

.export sequencer_init
.export sequencer_tick
.export sequencer_get_state
.export sequencer_get_phase

; ─── Initialize sequencer ────────────────────────────────────────────────────
; Sets up initial state: state=0, frame=0, phase=0, mode=motion
.proc sequencer_init
    lda #0
    sta SEQ_STATE       ; State = 0
    sta SEQ_FRAME_LO    ; Frame counter = 0
    sta SEQ_FRAME_HI
    sta SEQ_MODE        ; Mode = motion (0)
    sta SEQ_PHASE       ; Phase = 0
    rts
.endproc

; ─── Advance sequencer by one frame ──────────────────────────────────────────
; Call this once per frame
; Updates:
;   - SEQ_STATE: increments 0→1→2...→39→0
;   - SEQ_PHASE: increments when state wraps (every 40 frames)
;   - SEQ_FRAME_LO/HI: 16-bit frame counter (never wraps)
;   - SEQ_MODE: motion/freeze state (placeholder for future)
.proc sequencer_tick
    ; Increment state counter (0-39)
    lda SEQ_STATE
    clc
    adc #1
    cmp #40             ; Check if we've reached 40
    bcc @state_ok       ; If < 40, keep the value

    ; State wrapped to 0, increment phase counter
    inc SEQ_PHASE
    lda #0              ; Wrap state back to 0
@state_ok:
    sta SEQ_STATE

    ; Increment 16-bit frame counter
    inc SEQ_FRAME_LO
    bne @no_carry
    inc SEQ_FRAME_HI
@no_carry:

    ; Mode remains unchanged for now
    ; Future: Implement state machine based on frame counter
    ; Example from docs/MAJOR_TRANSITIONS.md:
    ;   Frames 3-166: freeze (intro)
    ;   Frames 167-179: motion (first motion period)
    ;   Frames 180-200: freeze (21-frame freeze)
    ;   etc.

    rts
.endproc

; ─── Get current sequencer state ─────────────────────────────────────────────
; Returns: A = current state (0-39)
.proc sequencer_get_state
    lda SEQ_STATE
    rts
.endproc

; ─── Get current phase counter ───────────────────────────────────────────────
; Returns: A = current phase (increments every 40 frames)
.proc sequencer_get_phase
    lda SEQ_PHASE
    rts
.endproc
