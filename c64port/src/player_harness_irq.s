; player_harness_irq.s — IRQ-driven harness for SID player replay.
;
; Assembled to produce a C64 PRG suitable for wrapping in a PSID v2NG header
; with play=0 (self-installed interrupts). The init address calls sid_init;
; the C64 environment calls it once, then lets IRQs drive the music.
;
; IRQ sources:
;   CIA1 Timer A/B → music_player_write_ctrl (waveform toggle for ch0 and ch1)
;   VIC raster   → music_player_sid_update + music_player_step (one tick per PAL frame = 50 Hz)
;
; The VIC raster fires at line $FF (just past the visible area) every frame.
; CIA1 timers fire one-shot, reloaded by their own handlers.
;
; IRQ routing: KERNAL and BASIC ROMs are banked out ($01 = $35), so the
; hardware IRQ vector at $FFFE/$FFFF is RAM and points straight at our
; dispatcher, which saves and restores A/X/Y itself. sid_init never returns
; (it idles after setup), so the player has no chance to map the KERNAL back in.
; CIA1 timer IRQ sources are enabled with ICR write $83/$84 at arm time.

.include "include/shared.inc"
.include "music_data.inc"
.segment "BSS"
HARNESS_PRB_SIM: .res 1
.segment "CODE"
CIA1_PRB = HARNESS_PRB_SIM

.include "player.s"

; ── Hardware ──────────────────────────────────────────────────────────────────

VIC_RASTER      = $D012
VIC_CTRL1       = $D011
VIC_IMR         = $D01A     ; interrupt mask register
VIC_IRR         = $D019     ; interrupt request register (ack)
CIA1_ICR        = $DC0D

CPU_PORT        = $01
CPU_PORT_RAM_IO = $35       ; RAM everywhere except the I/O area
IRQ_VECTOR      = $FFFE

; Software-tracked CIA1_PRB toggle bits (bit6=Timer A, bit7=Timer B), packed
; to match the real PRB layout. Some SID players (e.g. websid) fire the CIA
; timer IRQ without emulating the 6526's PBON+OUTMODE hardware pin-toggle,
; so CIA1_PRB ($DC01) can't be trusted to reflect it. Since CRA/CRB always
; run in continuous toggle mode, the bit flips exactly once per underflow —
; the same underflow that fires this IRQ — so this harness tracks it itself
; and, instead of reading the register, points CIA1_PRB (used by player.s's
; music_player_write_ctrl) at this byte.

; ── Init ──────────────────────────────────────────────────────────────────────

.segment "CODE"

.export sid_init
.export sid_setup

.proc sid_init
    jsr sid_setup
idle:
    jmp idle
.endproc

.proc sid_setup
    sei

    ; Bank out KERNAL/BASIC and install our own hardware IRQ vector.
    lda #CPU_PORT_RAM_IO
    sta CPU_PORT
    lda #<irq_entry
    sta IRQ_VECTOR
    lda #>irq_entry
    sta IRQ_VECTOR+1

    jsr music_player_init

    lda #$C0
    sta HARNESS_PRB_SIM

    ; Set up VIC raster IRQ at line $FF
    lda #$FF
    sta VIC_RASTER
    lda VIC_CTRL1
    and #$7F                ; raster bit 8 = 0
    sta VIC_CTRL1
    lda #$01
    sta VIC_IMR             ; enable raster interrupt

    ; Prime the one-tick pipeline: stage frame 0 now so the first raster
    ; tick applies it instead of silence.
    jsr music_player_step

    ; Enable IRQs
    cli
    rts
.endproc

; ── IRQ handler ───────────────────────────────────────────────────────────────
; The CIA path only needs A; the raster path also saves X and Y.
;
; CIA1_ICR is read first (reading clears the flags atomically).
; If no CIA timer is pending, the interrupt must be a VIC raster.
; This avoids depending on VIC_IRR ($D019) being readable in SID players
; that do not fully simulate VIC registers.

.proc irq_entry
    pha
    lda CIA1_ICR
    and #$03
    beq @raster            ; 0 → no CIA timer fired → VIC raster
    lsr                     ; each fire is exactly one underflow: flag A → C, flag B → bit 0
    ror                     ; A → bit 7, B → C
    ror                     ; B → bit 7, A → bit 6: the PRB toggle layout
    eor HARNESS_PRB_SIM
    sta HARNESS_PRB_SIM
    jsr music_player_write_ctrl
    pla
    rti

@raster:
    txa
    pha
    tya
    pha
    lda #$01
    sta VIC_IRR             ; ack VIC raster (harmless if SID player ignores it)
    cli                     ; let CIA waveform toggles preempt the tick
    jsr music_player_sid_update
    jsr music_player_step
@raster_done:
    pla
    tay
    pla
    tax
    pla
    rti
.endproc
