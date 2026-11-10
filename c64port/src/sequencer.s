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
;   event table below, which is ordered by frame counter, or EVENT_MORPH on
;   every frame of the morph
; Morph: the state (1-20) of the squares to diamonds morph; it starts at
;   MORPH_FRAME and the cubes stop at MOTION_END_FRAME
; Split: the state (1-20) of the large square splitting into two kites; it starts at
;   SPLIT_FRAME, after CHECKER_FRAME made the grid a checkerboard
;
; Consumers (animate, reveal) only read FRAME, SEQ_MODE and SEQ_EVENT.

.include "include/shared.inc"

MOTION_STATES = 21
MODE_MOTION = 0
MODE_FREEZE = 1
START_FRAME = 159
MUSIC_FRAME = 161               ; The original demo's first music step
MOTION_END_FRAME = 780          ; The cubes rest on their last animation frame (779) from here
MORPH_FRAME = 800               ; Morph state 1; state N shows at frame 799 + N
CHECKER_FRAME = 820             ; The morph is over and its last tile is symmetric
SPLIT_FRAME = 840               ; Split state 1; state N shows at frame 839 + N
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
    sta SEQ_MORPH
    sta SEQ_SPLIT
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
    lda #EVENT_NONE
    sta SEQ_EVENT
    ldx SEQ_EVENT_INDEX
    lda SEQ_FRAME_LO
    cmp event_frame_lo,x
    bne @morph
    lda SEQ_FRAME_HI
    cmp event_frame_hi,x
    bne @morph
    lda event_kind,x
    sta SEQ_EVENT
    inc SEQ_EVENT_INDEX

@morph:
    lda SEQ_EVENT_INDEX
    cmp #MORPH_STARTED_INDEX
    bcc @split
    lda SEQ_MORPH
    cmp #MORPH_STATES
    bcs @split
    inc SEQ_MORPH
    lda #EVENT_MORPH
    sta SEQ_EVENT

@split:
    lda SEQ_EVENT_INDEX
    cmp #SPLIT_STARTED_INDEX
    bcc @mode
    lda SEQ_SPLIT
    cmp #SPLIT_STATES
    bcs @mode
    inc SEQ_SPLIT
    lda #EVENT_SPLIT
    sta SEQ_EVENT

@mode:
    lda SEQ_EVENT_INDEX
    cmp #MOTION_ENDED_INDEX
    bcs @freeze
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
    rts
@freeze:
    lda #MODE_FREEZE
    sta SEQ_MODE
    rts
.endproc

; Timeline events by frame counter, ended by an unreachable frame
event_frame_lo:
    .byte <START_FRAME, <MUSIC_FRAME, <MOTION_END_FRAME, <MORPH_FRAME, <CHECKER_FRAME, <SPLIT_FRAME, $FF
event_frame_hi:
    .byte >START_FRAME, >MUSIC_FRAME, >MOTION_END_FRAME, >MORPH_FRAME, >CHECKER_FRAME, >SPLIT_FRAME, $FF
event_kind:
    .byte EVENT_REVEAL, EVENT_MUSIC, EVENT_NONE, EVENT_NONE, EVENT_CHECKER, EVENT_NONE, EVENT_NONE

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
