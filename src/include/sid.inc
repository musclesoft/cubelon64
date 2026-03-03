; SID (Sound Interface Device) register definitions for C64
; $D400-$D41C

; Voice 1
SID_V1_FREQ_LO  = $D400  ; Voice 1 frequency low byte
SID_V1_FREQ_HI  = $D401  ; Voice 1 frequency high byte
SID_V1_PW_LO    = $D402  ; Voice 1 pulse width low byte
SID_V1_PW_HI    = $D403  ; Voice 1 pulse width high byte
SID_V1_CTRL     = $D404  ; Voice 1 control register
SID_V1_AD       = $D405  ; Voice 1 attack/decay
SID_V1_SR       = $D406  ; Voice 1 sustain/release

; Voice 2
SID_V2_FREQ_LO  = $D407  ; Voice 2 frequency low byte
SID_V2_FREQ_HI  = $D408  ; Voice 2 frequency high byte
SID_V2_PW_LO    = $D409  ; Voice 2 pulse width low byte
SID_V2_PW_HI    = $D40A  ; Voice 2 pulse width high byte
SID_V2_CTRL     = $D40B  ; Voice 2 control register
SID_V2_AD       = $D40C  ; Voice 2 attack/decay
SID_V2_SR       = $D40D  ; Voice 2 sustain/release

; Voice 3
SID_V3_FREQ_LO  = $D40E  ; Voice 3 frequency low byte
SID_V3_FREQ_HI  = $D40F  ; Voice 3 frequency high byte
SID_V3_PW_LO    = $D410  ; Voice 3 pulse width low byte
SID_V3_PW_HI    = $D411  ; Voice 3 pulse width high byte
SID_V3_CTRL     = $D412  ; Voice 3 control register
SID_V3_AD       = $D413  ; Voice 3 attack/decay
SID_V3_SR       = $D414  ; Voice 3 sustain/release

; Filter and volume
SID_FC_LO       = $D415  ; Filter cutoff frequency low
SID_FC_HI       = $D416  ; Filter cutoff frequency high
SID_RES_FILT    = $D417  ; Resonance and filter routing
SID_MODE_VOL    = $D418  ; Filter mode and main volume

; Misc
SID_POT_X       = $D419  ; Paddle X (read only)
SID_POT_Y       = $D41A  ; Paddle Y (read only)
SID_OSC3        = $D41B  ; Oscillator 3 output (read only)
SID_ENV3        = $D41C  ; Envelope 3 output (read only)

; Control register bits
SID_CTRL_GATE   = $01    ; Gate bit
SID_CTRL_SYNC   = $02    ; Sync bit
SID_CTRL_RING   = $04    ; Ring modulation
SID_CTRL_TEST   = $08    ; Test bit
SID_CTRL_TRI    = $10    ; Triangle waveform
SID_CTRL_SAW    = $20    ; Sawtooth waveform
SID_CTRL_PULSE  = $40    ; Pulse waveform
SID_CTRL_NOISE  = $80    ; Noise waveform

; Filter routing bits
SID_FILT_V1     = $01    ; Filter voice 1
SID_FILT_V2     = $02    ; Filter voice 2
SID_FILT_V3     = $04    ; Filter voice 3
SID_FILT_EXT    = $08    ; Filter external input

; Filter mode bits
SID_MODE_LP     = $10    ; Low pass filter
SID_MODE_BP     = $20    ; Band pass filter
SID_MODE_HP     = $40    ; High pass filter
SID_MODE_3OFF   = $80    ; Voice 3 off
