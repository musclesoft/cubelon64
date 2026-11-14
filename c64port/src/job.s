; job.s - The precomputation that runs in the main loop's idle time
;
; The job is a coroutine with its own stack ($0100-$017F; the main loop keeps the rest of the
; page). The main loop resumes it while it waits for the frame tick. The job gives control back
; at a checkpoint once the raster has reached JOB_DEADLINE_LINE or the tick has come, so the
; main loop's work starts on time. The job's routines call job_checkpoint between steps that are
; short enough to end before the tick. A checkpoint keeps all registers and flags, and does
; nothing when the code runs outside the job.

.include "include/shared_imports.inc"
.include "include/vic.inc"

JOB_STACK_TOP = $7F
JOB_DEADLINE_LINE = 205         ; The longest step between checkpoints must end before line 240

.segment "BSS"

JOB_SP: .res 1
MAIN_SP: .res 1
JOB_ACTIVE: .res 1
JOB_REQUESTED: .res 1
JOB_BUILT: .res 1

.segment "CODE"

.import morph_precompute: absolute
.import split_precompute: absolute
.import spin_precompute: absolute
.import diamond_precompute: absolute
.import drift_precompute: absolute

.export job_init
.export build_request
.export job_step
.export job_checkpoint

; ─── Prepare the job's stack so that the first resume starts job_main ────────
; Interrupts must be disabled.
.proc job_init
    lda #0
    sta JOB_ACTIVE
    sta JOB_REQUESTED
    sta JOB_BUILT
    tsx
    stx MAIN_SP
    ldx #JOB_STACK_TOP
    txs
    lda job_entry + 1
    pha
    lda job_entry
    pha
    lda #0
    pha                     ; Flags
    pha                     ; A
    pha                     ; X
    pha                     ; Y
    tsx
    stx JOB_SP
    ldx MAIN_SP
    txs
    rts
.endproc

; The address the first resume returns to
job_entry:
    .word job_main - 1

; ─── Run the job until its next checkpoint after the deadline, if it is early enough ─
.proc job_step
    lda VIC_RASTER
    cmp #JOB_DEADLINE_LINE
    bcs @done
    tsx
    stx MAIN_SP
    ldx JOB_SP
    txs
    lda #1
    sta JOB_ACTIVE
    pla
    tay
    pla
    tax
    pla
    plp
    rts
@done:
    rts
.endproc

; ─── Give control back to the main loop when the frame is about to end ───────
; Keeps A, X, Y and the flags.
.proc job_checkpoint
    php
    pha
    lda JOB_ACTIVE
    beq @continue
    lda FRAME_TICK
    bne @yield
    lda VIC_RASTER
    cmp #JOB_DEADLINE_LINE
    bcs @yield
@continue:
    pla
    plp
    rts
@yield:
    txa
    pha
    tya
    pha
    lda #0
    sta JOB_ACTIVE
    tsx
    stx JOB_SP
    ldx MAIN_SP
    txs
    rts
.endproc

; ─── Ask the job to build the images of the next scene ───────────────────────
; The handler of EVENT_BUILD, which the sequencer's timeline publishes in time for each scene, in the
; order of build_routines. A scene's images have a home, where the job overwrites the images that
; are not needed any more.
.proc build_request
    inc JOB_REQUESTED
    rts
.endproc

; ─── Build what has been asked for ───────────────────────────────────────────
; The job spends the idle time of the main loop: it builds as fast as the idle time allows, and
; waits for the next request at a checkpoint.
.proc job_main
@wait:
    jsr job_checkpoint
    lda JOB_BUILT
    cmp JOB_REQUESTED
    beq @wait
    asl
    tax
    jsr run_build
    inc JOB_BUILT
    jmp @wait
.endproc

; Runs the build routine X / 2: its address - 1 is pushed, so its rts returns to the caller
.proc run_build
    lda build_routines + 1,x
    pha
    lda build_routines,x
    pha
    rts
.endproc

build_routines:
    .word morph_precompute - 1
    .word split_precompute - 1
    .word spin_precompute - 1
    .word diamond_precompute - 1
    .word drift_precompute - 1
