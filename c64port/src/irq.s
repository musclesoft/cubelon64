; irq.s - Shared IRQ handler for the VIC-II raster and CIA-1 timer interrupts
; CIA-1 timer underflows toggle the music waveform; raster interrupts drive
; the sprite multiplexer and, once it has moved the sprites, the per-frame
; tasks: the music step after the first band's interrupt, and after the
; last band's, when the bottom border starts, first the SID update and then
; the work of the frame (frame_work), which the job in the main program yields to.

.include "include/vic.inc"

.segment "CODE"

IRQ_VECTOR = $FFFE

.import frame_work: absolute

MUSIC_STEP_BAND_Y = SPRITE_Y + BAND_HEIGHT
FRAME_END_BAND_Y = SPRITE_Y

; Installs the handler. Interrupts stay disabled; the caller enables them
; with cli.
; Modifies: A
.export irq_init
.proc irq_init
    lda #0
    sta FRAME_BUSY
    lda #<irq_entry
    sta IRQ_VECTOR
    lda #>irq_entry
    sta IRQ_VECTOR+1
    rts
.endproc

; Makes the vector point at the handler of the gapless multiplexer or of the simple one
; Modifies: A
.export irq_use_gapless
.proc irq_use_gapless
    assert_flag I, 1
    lda #<irq_entry
    sta IRQ_VECTOR
    lda #>irq_entry
    sta IRQ_VECTOR+1
    rts
.endproc

.export irq_use_simple
.proc irq_use_simple
    assert_flag I, 1
    lda #<irq_entry_simple
    sta IRQ_VECTOR
    lda #>irq_entry_simple
    sta IRQ_VECTOR+1
    rts
.endproc

; A is pushed first and the VIC status read next, so that the raster path reaches the
; multiplexer with the exact cycle count it was written for. A CIA interrupt writes the
; waveform inline with interrupts still masked from the interrupt entry, which keeps it
; short and protects the phase read and the SID store from a second CIA fire. A raster
; interrupt that came in meanwhile is then handled right here instead of after the rti.
; The raster path reads CIA1_ICR to acknowledge the CIA flags, makes the music write
; (harmless when no timer fired; it masks interrupts itself) and restores interrupts
; with cli, which lets a raster interrupt preempt the band tasks, which must
; finish within the 24 lines before the next raster interrupt.
.align no_cross_page_branch
.proc irq_entry
    pha
    bit VIC_IRQ_STATUS
    bmi @raster
    bit CIA1_ICR
    write_ctrl_ch0 ; TODO: handle ch1 for the last note
    bit VIC_IRQ_STATUS
    bmi @raster
    pla
    rti
@raster:
    sprite_multiplex_irq
    bit CIA1_ICR
    txa
    pha
    tya
    pha
; The simple multiplexer needs X for its band index, so it saves X and Y before its stores and
; jumps here, past the saves above, to the pops below
raster_tasks:
    ldx VIC_SPR0_Y
    jsr music_player_write_ctrl
    cli
    jsr run_band_task
    pla
    tay
    pla
    tax
    pla
    rti
.endproc

; The same for the simple multiplexer, which is not cycle exact and so needs no particular cycle
; count from the entry. The music's waveform is written between its stores, which keep the
; interrupts masked for about 130 cycles.
.align no_cross_page_branch
.proc irq_entry_simple
    pha
    bit VIC_IRQ_STATUS
    bmi @raster
    bit CIA1_ICR
    write_ctrl_ch0 ; TODO: handle ch1 for the last note
    bit VIC_IRQ_STATUS
    bmi @raster
    pla
    rti
@raster:
    txa
    pha
    tya
    pha
    simple_sprite_irq
    bit CIA1_ICR
    jmp raster_tasks
.endproc

; X = the Y position the multiplexer has just moved the sprites to
; The dissolve's sprites are one line below the bands (an odd Y), the bands' Y are even
.proc run_band_task
    txa
    and #$FE
    tax
    cpx #MUSIC_STEP_BAND_Y
    beq music_step_task
    cpx #FRAME_END_BAND_Y
    beq frame_end_task
    rts
.endproc

.proc music_step_task
    lda MUSIC_ON
    beq @done               ; The music starts at its timeline event
    inc VIC_BORDER
    jsr music_player_step
    dec VIC_BORDER
@done:
    rts
.endproc

.proc frame_end_task
    lda MUSIC_ON
    beq @tick
    inc VIC_BORDER
    jsr music_player_sid_update
    dec VIC_BORDER
@tick:
    lda FRAME_BUSY
    bne @late               ; The last frame's work is still running: this frame is lost
    inc FRAME_BUSY
    jsr frame_work
    dec FRAME_BUSY
@late:
    rts
.endproc
