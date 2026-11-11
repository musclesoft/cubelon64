; irq.s - Shared IRQ handler for the VIC-II raster and CIA-1 timer interrupts
; CIA-1 timer underflows toggle the music waveform; raster interrupts drive
; the sprite multiplexer and, once it has moved the sprites, the per-frame
; tasks: the music step after the first band's interrupt, the SID update
; after the next to last band's, and the frame tick for the main loop after
; the last band's, when the bottom border starts.

.include "include/vic.inc"

.segment "CODE"

IRQ_VECTOR = $FFFE

MUSIC_STEP_BAND_Y = SPRITE_Y + BAND_HEIGHT
SID_UPDATE_BAND_Y = SPRITE_Y + BAND_HEIGHT * BANDS - BAND_HEIGHT
FRAME_TICK_BAND_Y = SPRITE_Y

; Installs the handler. Interrupts stay disabled; the caller enables them
; with cli.
; Modifies: A
.export irq_init
.proc irq_init
    lda #0
    sta FRAME_TICK
    lda #<irq_entry
    sta IRQ_VECTOR
    lda #>irq_entry
    sta IRQ_VECTOR+1
    rts
.endproc

; The VIC status is read first so that the raster path reaches the multiplexer
; with the exact cycle count it was written for. Reading CIA1_ICR acknowledges
; the CIA flags. The music write is harmless when no timer fired. cli lets a
; raster interrupt preempt the music write and the band tasks, which must
; finish within the 24 lines before the next raster interrupt.
.proc irq_entry
    bit VIC_IRQ_STATUS
    bmi @raster
    bit CIA1_ICR
    cli
    pha
    jsr music_player_write_ctrl
    pla
    rti
@raster:
    pha
    sprite_multiplex_irq
    bit CIA1_ICR
    txa
    pha
    tya
    pha
    ldx VIC_SPR0_Y
    cli
    jsr music_player_write_ctrl
    jsr run_band_task
    pla
    tay
    pla
    tax
    pla
    rti
.endproc

; X = the Y position the multiplexer has just moved the sprites to
.proc run_band_task
    cpx #MUSIC_STEP_BAND_Y
    beq music_step_task
    cpx #SID_UPDATE_BAND_Y
    beq sid_update_task
    cpx #FRAME_TICK_BAND_Y
    beq frame_tick_task
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

.proc sid_update_task
    lda MUSIC_ON
    beq @done
    inc VIC_BORDER
    jsr music_player_sid_update
    dec VIC_BORDER
@done:
    rts
.endproc

.proc frame_tick_task
    inc FRAME_TICK
    rts
.endproc
