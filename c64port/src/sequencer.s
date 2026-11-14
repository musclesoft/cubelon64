; Sequencer module - Owns the demo timeline
;
; State counter: 0-39 (continuously looping)
;   states 0-20 are motion, 21-39 freeze (MOTION_STATES)
; Phase counter: increments every 40 frames (when state wraps)
; Frame counter: 0-65535 (16-bit), counts every frame and never goes back
; The timeline starts at frame START_FRAME, so the frame counter matches the
;   original demo's; before it the mode is freeze and nothing happens. The demo
;   loops by starting the timeline over (phase, state, events and scenes) on the
;   frame counter's next frame
; Mode: 0=motion, 1=freeze
; FRAME: the animation frame (0-40) to show while in motion mode; two phases
;   make one animation cycle: FRAME = (phase & 1) * 20 + state
; Event: what the other modules must start this frame (EVENT_*), from the
;   event table below, which is ordered by cycle and state, or the event of the
;   running scene on each of its frames
; Scene: one entry of the scene table below per 40 frame cycle, the table index being the
;   cycle number - SCENE_FIRST_PHASE. The scene shows in states 1-20 of the cycle;
;   SEQ_SCENE_STATE counts them from 1, or from 19 down to 0 for a backwards scene.
;   The demo starts over at the first cycle after the table
;
; Consumers (animate, reveal) only read FRAME, SEQ_MODE and SEQ_EVENT.

.include "include/shared.inc"

MOTION_STATES = 21
MODE_MOTION = 0
MODE_FREEZE = 1
START_FRAME = 159
MUSIC_FRAME = 161               ; The original demo's first music step
SCENE_FIRST_PHASE = 16          ; The morph's cycle: its state 0 is frame 799, START_FRAME + 16 * 40
SCENE_STATES = 20
SCENE_BACKWARDS = $80           ; In a table entry: the states run from 19 to 0
SCENE_CHECKER = $40             ; In a table entry: the checkerboard follows in states 21-24
SCENE_CUBES = $20               ; In a table entry: the marching cubes' motion plays in states 0-20
SCENE_UNCHECKER = $10           ; In a table entry: the checkerboard is undone in states 21-28
CUBES_INDEX = 16                ; The scene table's index of the cubes' scene, which repeats
CUBES_CYCLES = 8
BACKWARDS_CYCLES = 6            ; The cubes walk backwards after them: the next scene table entry
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
    sta SEQ_MODE        ; Mode = motion (0), wait_for_start overrides it
    lda #<INITIAL_FRAME
    sta SEQ_FRAME_LO
    lda #>INITIAL_FRAME
    sta SEQ_FRAME_HI
    jsr clear_timeline
    jmp enter_timeline
.endproc

.proc clear_timeline
    lda #0
    sta SEQ_STATE
    sta SEQ_PHASE
    sta SEQ_EVENT_INDEX
    sta SEQ_SCENE_STATE
    sta SEQ_SCENE_ENTRY
    rts
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
    lda SEQ_PHASE
    cmp event_phase,x
    bne @scene
    lda SEQ_STATE
    cmp event_state,x
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
    bit SEQ_SCENE_ENTRY
    bpl @state_ready
    lda #SCENE_STATES
    sec
    sbc SEQ_STATE
@state_ready:
    sta SEQ_SCENE_STATE
    stx SEQ_EVENT
    jmp @mode

@checker:
    ldy #EVENT_CHECKER
    bit SEQ_SCENE_ENTRY
    bvs @checker_state      ; SCENE_CHECKER is bit 6
    lda SEQ_SCENE_ENTRY
    and #SCENE_UNCHECKER
    beq @mode
    ldy #EVENT_UNCHECKER
    lda SEQ_STATE
    cmp #SCENE_STATES + CHECKER_STATES + 1
    bcs @mode
    bcc @checker_scene_state
@checker_state:
    lda SEQ_STATE
    cmp #SCENE_STATES + CHECKER_STATES + 1
    bcs @mode
@checker_scene_state:
    sbc #SCENE_STATES - 1   ; Carry clear: subtracts SCENE_STATES
    sta SEQ_SCENE_STATE
    sty SEQ_EVENT

@mode:
    lda SEQ_SCENE_ENTRY
    and #SCENE_CUBES
    bne @motion
    lda SEQ_EVENT_INDEX
    cmp #MOTION_ENDED_INDEX
    bcs @freeze
@motion:
    lda SEQ_STATE
    cmp #MOTION_STATES
    bcs @freeze
    lda #MODE_MOTION
    sta SEQ_MODE
    lda SEQ_PHASE
    lsr                     ; Carry set on odd phases
    lda SEQ_STATE
    bcc @frame_ready
    adc #19                 ; Carry set: +20
@frame_ready:
    bit SEQ_SCENE_ENTRY
    bpl @store_frame
    eor #$FF                ; Backwards: 40 - frame
    sec
    adc #2 * 20
@store_frame:
    sta FRAME
    rts
@freeze:
    lda #MODE_FREEZE
    sta SEQ_MODE
    rts
.endproc

; ─── Pick the scene of the cycle that starts now ──────────────────────────────
; The demo starts over when the table has run out
.proc advance_scene
    lda SEQ_PHASE
    sec
    sbc #SCENE_FIRST_PHASE
    bcc @before_scenes
    cmp #CUBES_INDEX + 1
    bcc @lookup
    cmp #CUBES_INDEX + CUBES_CYCLES
    bcs @cubes_backwards
    lda #CUBES_INDEX
    bpl @lookup
@cubes_backwards:
    cmp #CUBES_INDEX + CUBES_CYCLES + BACKWARDS_CYCLES
    bcs @after_repeats
    lda #CUBES_INDEX + 1
    bne @lookup
@after_repeats:
    sbc #CUBES_CYCLES + BACKWARDS_CYCLES - 2    ; Carry set: the repeated entries count once
@lookup:
    cmp #scene_table_end - scene_table - 1
    bcc @in_table
    lda #scene_table_end - scene_table - 1  ; The terminator, again and again
@in_table:
    tax
    lda scene_table,x
    sta SEQ_SCENE_ENTRY
@before_scenes:
    rts
.endproc

; Timeline events by cycle and state, ended by an unreachable one. The first is the frame
; START_FRAME, the music's first step is 2 frames later, and the marching cubes' last frame
; is 779, the state 20 of the cycle 15, so they rest from its state 21
event_phase:
    .byte 0, 0, 0, 0, 0, 15, 30, 30, 48, $FF
event_state:
    .byte 0, MUSIC_FRAME - START_FRAME, 3, 4, 5, MOTION_STATES, 30, 31, MOTION_STATES, $FF
event_kind:
    .byte EVENT_REVEAL, EVENT_MUSIC, EVENT_BUILD, EVENT_BUILD, EVENT_BUILD
    .byte EVENT_KEEP_SQUARES, EVENT_BUILD, EVENT_BUILD, EVENT_BUILD, EVENT_NONE

; Scenes by cycle, ended by 0. The spin turns three half turns, a mirrored quarter after each
; quarter; then everything plays backwards to the marching cubes, which then rotate on
scene_table:
    .byte EVENT_MORPH + SCENE_CHECKER
    .byte EVENT_SPLIT
    .byte EVENT_SPIN, EVENT_SPIN_MIRRORED, EVENT_SPIN, EVENT_SPIN_MIRRORED, EVENT_SPIN, EVENT_SPIN_MIRRORED
    .byte EVENT_SPIN_MIRRORED + SCENE_BACKWARDS, EVENT_SPIN + SCENE_BACKWARDS
    .byte EVENT_SPIN_MIRRORED + SCENE_BACKWARDS, EVENT_SPIN + SCENE_BACKWARDS
    .byte EVENT_SPIN_MIRRORED + SCENE_BACKWARDS, EVENT_SPIN + SCENE_BACKWARDS
    .byte EVENT_SPLIT + SCENE_BACKWARDS, EVENT_MORPH + SCENE_BACKWARDS + SCENE_UNCHECKER
    .byte SCENE_CUBES, SCENE_CUBES + SCENE_BACKWARDS
    .byte EVENT_MORPH + SCENE_CHECKER
    .byte EVENT_DIAMONDS, EVENT_DRIFT, EVENT_DIAMONDS + SCENE_BACKWARDS
    .byte EVENT_DIAMONDS_MIRRORED, EVENT_DRIFT_MIRRORED, EVENT_DIAMONDS_MIRRORED + SCENE_BACKWARDS
    .byte 0
scene_table_end:

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
