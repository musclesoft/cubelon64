; Zero page allocations for CUBELON demo
; $02-$1B available (26 bytes, avoiding KERNAL usage)

; Temporary variables (general purpose)
.zeropage
tmp1:       .res 1   ; $02
tmp2:       .res 1   ; $03
tmp3:       .res 1   ; $04
tmp4:       .res 1   ; $05

; Pointer variables
ptr1:       .res 2   ; $06-$07 (lo/hi)
ptr2:       .res 2   ; $08-$09 (lo/hi)
ptr3:       .res 2   ; $0A-$0B (lo/hi)
ptr4:       .res 2   ; $0C-$0D (lo/hi)

; Demo-specific variables
frameCounter: .res 2  ; $0E-$0F (16-bit frame counter)
partIndex:    .res 1  ; $10 (current demo part index)
effectTimer:  .res 2  ; $11-$12 (effect timing counter)
syncCounter:  .res 2  ; $13-$14 (music sync counter)

; Available for future use: $15-$1B (7 bytes)
