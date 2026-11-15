; CIA-1 initialization module
; Silences the CIA-1 timers and interrupts left running by the KERNAL

.include "include/cia.inc"

.segment "CODE"

.export clear_cia
.proc clear_cia
    lda #0
    sta CIA1_CRA
    sta CIA1_CRB
    lda #$7F
    sta CIA1_ICR
    lda CIA1_ICR
    rts
.endproc
