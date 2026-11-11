; sid.inc - SID chip register definitions for C64
; MOS 6581/8580 Sound Interface Device

; ═══════════════════════════════════════════════════════════════════════════
; SID Base Address
; ═══════════════════════════════════════════════════════════════════════════
SID = $D400

; ═══════════════════════════════════════════════════════════════════════════
; Voice Register Offsets
; Add one to a voice base: SID_VOICE1, SID_VOICE2 or SID_VOICE3
; ═══════════════════════════════════════════════════════════════════════════
SID_FREQ_LO = 0                 ; Frequency (low byte)
SID_FREQ_HI = 1                 ; Frequency (high byte)
SID_PW_LO   = 2                 ; Pulse width (low byte)
SID_PW_HI   = 3                 ; Pulse width (bits 0-3)
SID_CTRL    = 4                 ; Waveform, gate, sync, ring mod, test
SID_ATT_DEC = 5                 ; Attack (high nibble), decay (low nibble)
SID_SUS_REL = 6                 ; Sustain (high nibble), release (low nibble)

SID_VOICE_SIZE = 7
SID_VOICE1 = SID
SID_VOICE2 = SID + SID_VOICE_SIZE
SID_VOICE3 = SID + SID_VOICE_SIZE * 2

; ═══════════════════════════════════════════════════════════════════════════
; Voice 1 ($D400-$D406)
; ═══════════════════════════════════════════════════════════════════════════
SID_V1_FREQ_LO = SID_VOICE1 + SID_FREQ_LO
SID_V1_FREQ_HI = SID_VOICE1 + SID_FREQ_HI
SID_V1_PW_LO   = SID_VOICE1 + SID_PW_LO
SID_V1_PW_HI   = SID_VOICE1 + SID_PW_HI
SID_V1_CTRL    = SID_VOICE1 + SID_CTRL
SID_V1_ATT_DEC = SID_VOICE1 + SID_ATT_DEC
SID_V1_SUS_REL = SID_VOICE1 + SID_SUS_REL

; ═══════════════════════════════════════════════════════════════════════════
; Voice 2 ($D407-$D40D)
; ═══════════════════════════════════════════════════════════════════════════
SID_V2_FREQ_LO = SID_VOICE2 + SID_FREQ_LO
SID_V2_FREQ_HI = SID_VOICE2 + SID_FREQ_HI
SID_V2_PW_LO   = SID_VOICE2 + SID_PW_LO
SID_V2_PW_HI   = SID_VOICE2 + SID_PW_HI
SID_V2_CTRL    = SID_VOICE2 + SID_CTRL
SID_V2_ATT_DEC = SID_VOICE2 + SID_ATT_DEC
SID_V2_SUS_REL = SID_VOICE2 + SID_SUS_REL

; ═══════════════════════════════════════════════════════════════════════════
; Voice 3 ($D40E-$D414)
; ═══════════════════════════════════════════════════════════════════════════
SID_V3_FREQ_LO = SID_VOICE3 + SID_FREQ_LO
SID_V3_FREQ_HI = SID_VOICE3 + SID_FREQ_HI
SID_V3_PW_LO   = SID_VOICE3 + SID_PW_LO
SID_V3_PW_HI   = SID_VOICE3 + SID_PW_HI
SID_V3_CTRL    = SID_VOICE3 + SID_CTRL
SID_V3_ATT_DEC = SID_VOICE3 + SID_ATT_DEC
SID_V3_SUS_REL = SID_VOICE3 + SID_SUS_REL

; ═══════════════════════════════════════════════════════════════════════════
; Filter and Volume ($D415-$D418)
; ═══════════════════════════════════════════════════════════════════════════
SID_FILTER_LO  = SID + $15      ; Filter cutoff (bits 0-2)
SID_FILTER_HI  = SID + $16      ; Filter cutoff (bits 3-10)
SID_RES_FILTER = SID + $17      ; Resonance (high nibble), voice routing (low nibble)
SID_VOL        = SID + $18      ; Volume (low nibble), filter mode (high nibble)

; ═══════════════════════════════════════════════════════════════════════════
; Read-only Registers ($D419-$D41C)
; ═══════════════════════════════════════════════════════════════════════════
SID_POT_X      = SID + $19      ; Paddle X
SID_POT_Y      = SID + $1A      ; Paddle Y
SID_OSC3       = SID + $1B      ; Voice 3 waveform output
SID_ENV3       = SID + $1C      ; Voice 3 envelope output

; ═══════════════════════════════════════════════════════════════════════════
; Unused ($D41D-$D41F)
; Writes are ignored
; ═══════════════════════════════════════════════════════════════════════════
SID_UNUSED     = SID + $1D
