; job.s - The precomputation that runs in the main program's time
;
; The main program does nothing else: it works off the build requests and, when there are none, builds the rotating
; cubes' images, with every cycle the interrupts leave. The frame's own work runs in the raster interrupt chain
; (frame_work in main.s), which interrupts the job wherever it is, so the job's routines never yield. What the job
; and the frame's work both use must therefore not keep state in memory (see image_address).

.include "include/shared_imports.inc"
.include "include/vic.inc"

.segment "BSS"

JOB_REQUESTED: .res 1
JOB_BUILT: .res 1

.segment "CODE"

.import morph_precompute: absolute
.import split_precompute: absolute
.import spin_precompute: absolute
.import diamond_precompute: absolute
.import drift_precompute: absolute
.import drift_precompute_mirrored: absolute
.import merge_precompute: absolute
.import flower_precompute: absolute
.import flower_march_render: absolute
.import flower_between_precompute: absolute
.import flower_dissolve_precompute: absolute
.import cube_build_next: absolute

.export job_init
.export job_main
.export build_request

.proc job_init
    lda #0
    sta JOB_REQUESTED
    sta JOB_BUILT
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

; ─── Build what has been asked for, forever ──────────────────────────────────
.proc job_main
    lda JOB_BUILT
    cmp JOB_REQUESTED
    bne @requested
    jsr cube_build_next
    jmp job_main
@requested:
    asl
    tax
    jsr @run
    inc JOB_BUILT
    jmp job_main
@run:
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
    .word drift_precompute_mirrored - 1
    .word drift_precompute - 1
    .word drift_precompute_mirrored - 1
    .word drift_precompute - 1
    .word merge_precompute - 1
    .word flower_precompute - 1
    .word flower_march_render - 1
    .word flower_between_precompute - 1
    .word flower_dissolve_precompute - 1
