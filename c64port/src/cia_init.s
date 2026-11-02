; CIA-1 initialization module
; Silences the CIA-1 timers and interrupts left running by the KERNAL

CIA1_CRA = $DC0E
CIA1_CRB = $DC0F
CIA1_ICR = $DC0D

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
