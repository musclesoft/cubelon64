; KERNAL routine addresses for C64

; Standard KERNAL routines
CINT    = $FF81  ; Initialize screen editor
IOINIT  = $FF84  ; Initialize I/O devices
RAMTAS  = $FF87  ; Initialize RAM
RESTOR  = $FF8A  ; Restore I/O vectors
VECTOR  = $FF8D  ; Manage RAM vectors
SETMSG  = $FF90  ; Control KERNAL messages
SECOND  = $FF93  ; Send secondary address
TKSA    = $FF96  ; Talk secondary address
MEMTOP  = $FF99  ; Read/set top of memory
MEMBOT  = $FF9C  ; Read/set bottom of memory
SCNKEY  = $FFEA  ; Scan keyboard
SETTMO  = $FFA2  ; Set timeout
ACPTR   = $FFA5  ; Input byte from serial port
CIOUT   = $FFA8  ; Output byte to serial port
UNTLK   = $FFAB  ; Command serial bus to UNTALK
UNLSN   = $FFAE  ; Command serial bus to UNLISTEN
LISTEN  = $FFB1  ; Command serial bus to LISTEN
TALK    = $FFB4  ; Command serial bus to TALK
READST  = $FFB7  ; Read I/O status word
SETLFS  = $FFBA  ; Set logical file parameters
SETNAM  = $FFBD  ; Set filename
OPEN    = $FFC0  ; Open a logical file
CLOSE   = $FFC3  ; Close a logical file
CHKIN   = $FFC6  ; Set input channel
CHKOUT  = $FFC9  ; Set output channel
CLRCHN  = $FFCC  ; Restore I/O to default device
CHRIN   = $FFCF  ; Input character
CHROUT  = $FFD2  ; Output character
LOAD    = $FFD5  ; Load from device
SAVE    = $FFD8  ; Save to device
SETTIM  = $FFDB  ; Set real-time clock
RDTIM   = $FFDE  ; Read real-time clock
STOP    = $FFE1  ; Check STOP key
GETIN   = $FFE4  ; Get character from keyboard
CLALL   = $FFE7  ; Close all files
UDTIM   = $FFEA  ; Update real-time clock
SCREEN  = $FFED  ; Return screen size
PLOT    = $FFF0  ; Read/set cursor position
IOBASE  = $FFF3  ; Return I/O base address

; Zero page locations used by KERNAL
IOSTATUS = $90   ; I/O status byte
KEYQUEUE = $C6   ; Number of keys in keyboard queue

; Hardware vectors
NMI_VECTOR  = $FFFA  ; NMI vector
RESET_VECTOR = $FFFC ; Reset vector
IRQ_VECTOR  = $FFFE  ; IRQ vector

; CIA timers
CIA1        = $DC00  ; CIA 1 base
CIA2        = $DD00  ; CIA 2 base

; CIA 1 (keyboard, joystick)
CIA1_PRA    = $DC00  ; Port A data
CIA1_PRB    = $DC01  ; Port B data
CIA1_DDRA   = $DC02  ; Port A direction
CIA1_DDRB   = $DC03  ; Port B direction
CIA1_TA_LO  = $DC04  ; Timer A low byte
CIA1_TA_HI  = $DC05  ; Timer A high byte
CIA1_TB_LO  = $DC06  ; Timer B low byte
CIA1_TB_HI  = $DC07  ; Timer B high byte
CIA1_ICR    = $DC0D  ; Interrupt control
CIA1_CRA    = $DC0E  ; Control register A
CIA1_CRB    = $DC0F  ; Control register B

; CIA 2 (user port, RS-232, VIC bank)
CIA2_PRA    = $DD00  ; Port A data (VIC bank select)
CIA2_PRB    = $DD01  ; Port B data
CIA2_DDRA   = $DD02  ; Port A direction
CIA2_DDRB   = $DD03  ; Port B direction
CIA2_TA_LO  = $DD04  ; Timer A low byte
CIA2_TA_HI  = $DD05  ; Timer A high byte
CIA2_TB_LO  = $DD06  ; Timer B low byte
CIA2_TB_HI  = $DD07  ; Timer B high byte
CIA2_ICR    = $DD0D  ; Interrupt control
CIA2_CRA    = $DD0E  ; Control register A
CIA2_CRB    = $DD0F  ; Control register B

; Memory addresses
SCREEN_RAM  = $0400  ; Default screen RAM
COLOR_RAM   = $D800  ; Color RAM
SPRITE_PTRS = $07F8  ; Sprite pointers (screen + $3F8)
