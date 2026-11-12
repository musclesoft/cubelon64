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
;   event table below, which is ordered by frame counter, or the event of the
;   running scene on each of its frames
; Scene: one entry of the scene table below per 40 frame cycle, from SCENE_FIRST_PHASE on, in
;   states 1-20 of the cycle; SEQ_SCENE_STATE counts them from 1. The table lists the scenes
;   forward (the morph is followed by the checkerboard, in states 21-24 of its cycle); once it
;   has run out, the cycles walk it backwards, each scene's states from 19 down to 0, and the
;   demo starts over after its first entry. The cubes stop at MOTION_END_FRAME
;
; Consumers (animate, reveal) only read FRAME, SEQ_MODE and SEQ_EVENT.

.include "include/shared.inc"

MOTION_STATES = 21
MODE_MOTION = 0
MODE_FREEZE = 1
START_FRAME = 159
MUSIC_FRAME = 161               ; The original demo's first music step
MOTION_END_FRAME = 780          ; The cubes rest on their last animation frame (779) from here
SCENE_FIRST_PHASE = 16          ; The morph's cycle: its state 0 is frame 799, START_FRAME + 16 * 40
SCENE_STATES = 20
SCENE_CHECKER = $40             ; In a table entry: the checkerboard follows in states 21-24
SCENE_KIND_MASK = $0F
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
    sta SEQ_SCENE_STATE
    sta SEQ_SCENE_INDEX
    sta SEQ_SCENE_ENTRY
    sta SEQ_SCENE_BACKWARDS
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
    bne @scene
    lda SEQ_FRAME_HI
    cmp event_frame_hi,x
    bne @scene
    lda event_kind,x
    sta SEQ_EVENT
    inc SEQ_EVENT_INDEX

@scene:
    lda SEQ_STATE
    bne @scene_state
    jsr advance_scene

@scene_state:
    lda SEQ_SCENE_ENTRY
    beq @mode
    and #SCENE_KIND_MASK
    tax
    lda SEQ_STATE
    beq @mode
    cmp #SCENE_STATES + 1
    bcs @checker
    ldy SEQ_SCENE_BACKWARDS
    beq @state_ready
    lda #SCENE_STATES
    sec
    sbc SEQ_STATE
@state_ready:
    sta SEQ_SCENE_STATE
    stx SEQ_EVENT
    jmp @mode

@checker:
    ldy SEQ_SCENE_BACKWARDS
    bne @mode
    ldy SEQ_SCENE_ENTRY
    cpy #SCENE_CHECKER
    bcc @mode
    cmp #SCENE_STATES + CHECKER_STATES + 1
    bcs @mode
    sbc #SCENE_STATES - 1   ; Carry clear: subtracts SCENE_STATES
    sta SEQ_SCENE_STATE
    lda #EVENT_CHECKER
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

; ─── Pick the scene of the cycle that starts now ──────────────────────────────
; Forward through the table, then backwards through it, then the demo starts over
.proc advance_scene
    lda SEQ_PHASE
    cmp #SCENE_FIRST_PHASE
    bcc @before_scenes
    bne @next
    ldx #0
    beq @load
@next:
    lda SEQ_SCENE_ENTRY
    beq @restart
    ldx SEQ_SCENE_INDEX
    lda SEQ_SCENE_BACKWARDS
    bne @backwards
    inx
    lda scene_table,x
    bne @load
    lda #1
    sta SEQ_SCENE_BACKWARDS
    dex
    bpl @load
@backwards:
    dex
    bpl @load
@restart:
    lda #0
    sta SEQ_SCENE_ENTRY
    lda #EVENT_RESTART
    sta SEQ_EVENT
@before_scenes:
    rts
@load:
    stx SEQ_SCENE_INDEX
    lda scene_table,x
    sta SEQ_SCENE_ENTRY
    rts
.endproc

; Timeline events by frame counter, ended by an unreachable frame
event_frame_lo:
    .byte <START_FRAME, <MUSIC_FRAME, <MOTION_END_FRAME, $FF
event_frame_hi:
    .byte >START_FRAME, >MUSIC_FRAME, >MOTION_END_FRAME, $FF
event_kind:
    .byte EVENT_REVEAL, EVENT_MUSIC, EVENT_KEEP_SQUARES, EVENT_NONE

; Scenes in order, ended by 0. The spin turns three half turns, a mirrored quarter after each
; quarter
scene_table:
    .byte EVENT_MORPH + SCENE_CHECKER, EVENT_SPLIT
    .byte EVENT_SPIN, EVENT_SPIN_MIRRORED, EVENT_SPIN, EVENT_SPIN_MIRRORED, EVENT_SPIN, EVENT_SPIN_MIRRORED
    .byte 0

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
