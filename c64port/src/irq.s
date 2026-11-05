; irq.s - Shared IRQ handler for the VIC-II raster and CIA-1 timer interrupts
; CIA-1 timer underflows toggle the music waveform; raster interrupts drive
; the sprite multiplexer.

.include "include/vic.inc"

.segment "CODE"

IRQ_VECTOR = $FFFE

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
; Its bit 7 is set while an enabled CIA source (the timers) is pending; clear
; means the interrupt came from the VIC-II raster.
; Each handler saves the registers it uses besides A.
.proc irq_entry
    dec VIC_BORDER
    pha
    bit CIA1_ICR
    bmi @cia
@raster:
    jsr sprite_multiplex_row
    lda #$01
    sta VIC_IRQ_STATUS
@cia:
    jsr music_player_write_ctrl
    pla
    inc VIC_BORDER
    rti
.endproc
