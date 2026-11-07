; irq.s - Shared IRQ handler for the VIC-II raster and CIA-1 timer interrupts
; CIA-1 timer underflows toggle the music waveform; raster interrupts drive
; the sprite multiplexer.

.include "include/vic.inc"

.segment "CODE"

IRQ_VECTOR = $FFFE

; Installs the handler. Interrupts stay disabled; the caller enables them
; with cli.
; Modifies: A
.export irq_init
.proc irq_init
    lda #<irq_entry
    sta IRQ_VECTOR
    lda #>irq_entry
    sta IRQ_VECTOR+1
    rts
.endproc

; The VIC status is read first so that the raster path reaches the multiplexer
; with the exact cycle count it was written for. Reading CIA1_ICR acknowledges
; the CIA flags. The music write is harmless when no timer fired. cli lets a
; raster interrupt preempt the music write.
.proc irq_entry
    bit VIC_IRQ_STATUS
    bmi @raster
@music:
    bit CIA1_ICR
    cli
    pha
    jsr music_player_write_ctrl
    pla
    rti
@raster:
    pha
    sprite_multiplex_irq
    pla
    jmp @music
.endproc
