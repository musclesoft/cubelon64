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
JOB_DONE: .res 1

.segment "CODE"

.import morph_precompute: absolute
.import split_precompute: absolute
.import spin_precompute: absolute
.import diamond_precompute: absolute

.export job_init
.export job_step
.export job_checkpoint

; ─── Prepare the job's stack so that the first resume starts job_main ────────
; Interrupts must be disabled.
.proc job_init
    lda #0
    sta JOB_ACTIVE
    sta JOB_DONE
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
    lda JOB_DONE
    bne @done
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

.proc job_main
    jsr morph_precompute
    jsr split_precompute
    jsr spin_precompute
    jsr diamond_precompute
    lda #1
    sta JOB_DONE
    lda #0
    sta JOB_ACTIVE
    ldx MAIN_SP
    txs
    rts
.endproc
