; irq.s - Shared IRQ handler for the VIC-II raster and CIA-1 timer interrupts
; CIA-1 timer underflows toggle the music waveform; raster interrupts drive
; the sprite multiplexer.

.include "include/vic.inc"

.segment "CODE"

IRQ_VECTOR = $FFFE

CIA_TIMER_FLAGS = $03

.import sprite_multiplex_row: absolute

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

; Reading CIA1_ICR acknowledges the CIA flags, so it is read exactly once.
; No CIA flag set means the interrupt came from the VIC-II raster.
; Each handler saves the registers it uses besides A.
.proc irq_entry
    pha
    lda CIA1_ICR
    and #CIA_TIMER_FLAGS
    beq @raster

    lsr
    bcc @timer_b
    pha
    jsr player_cia1a_fire
    pla
@timer_b:
    lsr
    bcc @done
    jsr player_cia1b_fire
@done:
    pla
    rti

@raster:
    jsr sprite_multiplex_row
    lda #$01
    sta VIC_IRQ_STATUS
    pla
    rti
.endproc
