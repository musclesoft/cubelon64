; player_harness_irq.s — IRQ-driven harness for SID player replay.
;
; Assembled to produce a C64 PRG suitable for wrapping in a PSID v2NG header
; with play=0 (self-installed interrupts). The init address calls sid_init;
; the C64 environment calls it once, then lets IRQs drive the music.
;
; IRQ sources:
;   CIA1 Timer A → player_cia1a_fire (waveform toggle for ch0)
;   CIA1 Timer B → player_cia1b_fire (waveform toggle for ch1)
;   VIC raster   → player_tick (one tick per PAL frame = 50 Hz)
;
; The VIC raster fires at line $FF (just past the visible area) every frame.
; CIA1 timers fire one-shot, reloaded by their own handlers.
;
; IRQ routing: KERNAL ROM stays mapped ($FFFE/$FFFF are ROM, not RAM).
; The KERNAL IRQ entry at $EA31 saves A/X/Y and JMPs through ($0314/$0315).
; Our handler ends with JMP $EA81 so the KERNAL restores A/X/Y and RTIs.
; CIA1 timer IRQ sources are enabled with ICR write $83/$84 at arm time.

.include "include/shared.inc"
.include "music_data.inc"
.include "player.s"

; ── Hardware ──────────────────────────────────────────────────────────────────

VIC_RASTER      = $D012
VIC_CTRL1       = $D011
VIC_IMR         = $D01A     ; interrupt mask register
VIC_IRR         = $D019     ; interrupt request register (ack)
CIA1_ICR        = $DC0D

; KERNAL IRQ exit (restores A/X/Y, RTI)
KERNAL_IRQ_EXIT = $EA81

; ── Init ──────────────────────────────────────────────────────────────────────

.segment "CODE"

.export sid_init

.proc sid_init
    jsr player_init

    ; Set up VIC raster IRQ at line $FF
    lda #$FF
    sta VIC_RASTER
    lda VIC_CTRL1
    and #$7F                ; raster bit 8 = 0
    sta VIC_CTRL1
    lda #$01
    sta VIC_IMR             ; enable raster interrupt

    ; Mask CIA1 timer IRQs for now (player_init already does this,
    ; but re-clear here to be safe after VIC setup).
    lda #$7F
    sta CIA1_ICR

    ; Point KERNAL user IRQ vector to our dispatcher.
    ; $FFFE/$FFFF are Kernal ROM — writes go nowhere.
    ; The Kernal IRQ handler at $EA31 saves A/X/Y then JMPs ($0314).
    lda #<irq_dispatch
    sta $0314
    lda #>irq_dispatch
    sta $0315

    ; Enable IRQs
    cli
    rts
.endproc

; ── IRQ dispatcher ────────────────────────────────────────────────────────────
; On entry: A/X/Y already saved by KERNAL at $EA31.
; On exit: JMP $EA81 — KERNAL restores A/X/Y and RTIs.
;
; CIA1_ICR is read first (reading clears the flags atomically).
; If no CIA timer is pending, the interrupt must be a VIC raster.
; This avoids depending on VIC_IRR ($D019) being readable in SID players
; that do not fully simulate VIC registers.

.proc irq_dispatch
    lda CIA1_ICR
    and #$03
    beq @raster            ; 0 → no CIA timer fired → VIC raster
    cmp #$01
    beq @cia_a
    cmp #$02
    beq @cia_b
    ; both timers fired
    jsr player_cia1a_fire
    jsr player_cia1b_fire
    jmp KERNAL_IRQ_EXIT

@cia_a:
    jsr player_cia1a_fire
    jmp KERNAL_IRQ_EXIT
@cia_b:
    jsr player_cia1b_fire
    jmp KERNAL_IRQ_EXIT

@raster:
    lda #$01
    sta VIC_IRR             ; ack VIC raster (harmless if SID player ignores it)
    jsr player_tick
    jmp KERNAL_IRQ_EXIT
.endproc
