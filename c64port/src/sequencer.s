; Sequencer module - Owns the demo timeline
;
; State counter: 0-39 (continuously looping)
;   states 0-20 are motion, 21-39 freeze (MOTION_STATES)
; Phase counter: increments every 40 frames (when state wraps)
; Frame counter: 0-65535 (16-bit)
; The timeline starts at frame START_FRAME, so the frame counter matches the
;   original demo's; before it the mode is freeze and nothing happens
; Mode: 0=motion, 1=freeze
; FRAME: the animation frame (0-40) to show while in motion mode; two phases
;   make one animation cycle: FRAME = (phase & 1) * 20 + state
; Event: what the other modules must start this frame (EVENT_*), from the
;   event table below, which is ordered by frame counter
;
; Consumers (animate, reveal) only read FRAME, SEQ_MODE and SEQ_EVENT.

.include "include/shared.inc"

MOTION_STATES = 21
MODE_MOTION = 0
MODE_FREEZE = 1
START_FRAME = 159
MUSIC_FRAME = 161               ; The original demo's first music step
INITIAL_FRAME = START_FRAME     ; Start at 0 to play the idle frames before the timeline

.segment "CODE"

.export sequencer_init
.export sequencer_tick
.export sequencer_get_state
.export sequencer_get_phase

; ─── Initialize sequencer ────────────────────────────────────────────────────
; Sets up initial state: state=0, frame=INITIAL_FRAME, phase=0
.proc sequencer_init
    lda #0
    sta SEQ_STATE       ; State = 0
    sta SEQ_MODE        ; Mode = motion (0), wait_for_start overrides it
    sta SEQ_PHASE       ; Phase = 0
    sta SEQ_EVENT_INDEX
    lda #<INITIAL_FRAME
    sta SEQ_FRAME_LO
    lda #>INITIAL_FRAME
    sta SEQ_FRAME_HI
    jmp enter_timeline
.endproc

; ─── Advance sequencer by one frame ──────────────────────────────────────────
; Call this once per frame
; Updates:
;   - SEQ_STATE: increments 0→1→2...→39→0
;   - SEQ_PHASE: increments when state wraps (every 40 frames)
;   - SEQ_FRAME_LO/HI: 16-bit frame counter (never wraps)
;   - SEQ_MODE, FRAME, SEQ_EVENT: see publish_timeline
.proc sequencer_tick
    ; Increment 16-bit frame counter
    inc SEQ_FRAME_LO
    bne @no_carry
    inc SEQ_FRAME_HI
@no_carry:
    jmp enter_timeline
.endproc

; ─── Idle before START_FRAME, publish its first frame, advance after it ─────
.proc enter_timeline
    lda SEQ_FRAME_HI
    cmp #>START_FRAME
    bne @start_compared
    lda SEQ_FRAME_LO
    cmp #<START_FRAME
@start_compared:
    bcc wait_for_start
    beq publish_timeline    ; The first timeline frame keeps state 0

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

    jmp publish_timeline
.endproc

; ─── Idle until the timeline starts ──────────────────────────────────────────
.proc wait_for_start
    lda #MODE_FREEZE
    sta SEQ_MODE
    lda #$FF                ; No animation frame yet
    sta FRAME
    lda #EVENT_NONE
    sta SEQ_EVENT
    rts
.endproc

; ─── Derive mode, FRAME and event from state, phase and frame counter ────────
.proc publish_timeline
    lda SEQ_STATE
    cmp #MOTION_STATES
    bcs @freeze
    lda #MODE_MOTION
    sta SEQ_MODE
    lda SEQ_PHASE
    lsr                     ; Carry set on odd phases
    lda SEQ_STATE
    bcc @store_frame
    adc #19                 ; Carry set: +20
@store_frame:
    sta FRAME
    jmp @event
@freeze:
    lda #MODE_FREEZE
    sta SEQ_MODE

@event:
    lda #EVENT_NONE
    sta SEQ_EVENT
    ldx SEQ_EVENT_INDEX
    lda SEQ_FRAME_LO
    cmp event_frame_lo,x
    bne @done
    lda SEQ_FRAME_HI
    cmp event_frame_hi,x
    bne @done
    lda event_kind,x
    sta SEQ_EVENT
    inc SEQ_EVENT_INDEX
@done:
    rts
.endproc

; Timeline events by frame counter, ended by an unreachable frame
event_frame_lo:
    .byte <START_FRAME, <MUSIC_FRAME, $FF
event_frame_hi:
    .byte >START_FRAME, >MUSIC_FRAME, $FF
event_kind:
    .byte EVENT_REVEAL, EVENT_MUSIC, EVENT_NONE

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
