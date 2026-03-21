# CUBELON Sequencer Architecture

## Overview

The CUBELON demo uses a table-driven sequencer to control the timing of motion and freeze periods. The sequencer runs every frame and uses frame counter thresholds to determine when to advance to the next state.

## Key Memory Locations

- **$8184**: Frame counter (16-bit, little-endian)
  - Incremented every frame in interrupt handler at $7E15
  - Read by sequencer wait loop at $7F6D

- **$7902**: Sequence table pointer (16-bit)
  - Points to current position in sequence table
  - Loaded at sequencer initialization

## Sequencer Wait Loop

The core wait loop runs every frame after initialization:

```asm
; === WAIT LOOP (executed every frame) ===
$7F6D: LD HL,($8184)    ; Load current frame counter into HL
$7F70: OR A             ; Clear carry flag
$7F71: SBC HL,BC        ; Compute: frame_counter - threshold
$7F73: JP M,$7F6D       ; If negative (frame < threshold), loop back
; === END WAIT LOOP ===
```

**How it works:**
1. BC register contains the target frame threshold
2. HL is loaded with current frame counter
3. If frame_counter < threshold, the subtraction yields negative result
4. JP M (jump if minus/negative) loops back to wait more
5. When frame_counter >= threshold, execution continues past the loop

**Performance:**
- Executed 504 times during frame 900 (typical steady-state frame)
- Extremely tight loop - only 4 instructions

## Table-Driven Architecture

The sequencer loads thresholds from a table in memory:

```asm
; === THRESHOLD LOAD (executed at state transitions) ===
$7F60: INC HL           ; Advance table pointer
$7F61: CB 6F            ; BIT 5,A (check state flag)
$7F63: JR NZ,$7F9E      ; Branch based on state
$7F65: CB 7F            ; BIT 7,A (check another state flag)
$7F67: JR Z,$7F78       ; Skip threshold load if flag clear

$7F69: LD C,(HL)        ; Load low byte of threshold from table
$7F6A: INC HL           ; Advance pointer
$7F6B: LD B,(HL)        ; Load high byte of threshold from table
$7F6C: EX DE,HL         ; Save table pointer in DE

; ... wait loop executes here ...

$7F76: EX DE,HL         ; Restore table pointer
$7F77: INC HL           ; Advance to next entry
```

**Table format:**
- 16-bit frame thresholds stored little-endian
- State flags control sequencer behavior
- Table entries: [threshold_low, threshold_high, state_flags, ...]

**Threshold values observed:**
- 21 frames (85 occurrences in motion script)
- 29 frames (51 occurrences)
- 166, 179, 200, etc.

## Mapping to Motion/Freeze Periods

From motion_script.txt, we identified 314 periods with freeze threshold of 4 frames:

**Freeze periods with 21 frames:**
```
FREEZE   180   200   21 0.42   ; threshold = 200
FREEZE   220   240   21 0.42   ; threshold = 240
FREEZE   260   280   21 0.42   ; threshold = 280
... (85 total)
```

**Freeze periods with 29 frames:**
```
FREEZE  4293  4321   29 0.58   ; threshold = 4321
FREEZE  4613  4641   29 0.58   ; threshold = 4641
... (51 total)
```

**Correlation:**
- Sequencer BC thresholds = END frame of freeze periods
- Motion periods: code between thresholds actively animates
- Freeze periods: code waits at threshold, minimal animation

## State Flags (Register A)

The sequencer uses bit flags in register A to control behavior:

```asm
$7F61: BIT 5,A    ; Check bit 5 (state flag)
$7F65: BIT 7,A    ; Check bit 7 (state flag)
$7F78: BIT 6,A    ; Check bit 6 (state flag)
```

**Likely purposes:**
- Bit 5: Major state transition flag?
- Bit 7: Load new threshold from table
- Bit 6: Unknown action at state boundary

## Initialization

Based on trace logs:
- Before frame 166: PC in intro code ($82xx range)
- Frame 167+: PC enters wait loop at $7F6D
- BC loaded with threshold 179 for first post-intro period

This matches motion script:
```
FREEZE     3   166  164 3.28   ; intro period
MOTION   167   179   13 0.26   ; first motion, threshold = 179
```

## Implementation Notes for C64 Port

1. **Frame counter**: Maintain 16-bit counter, increment every frame
2. **Threshold table**: Port the sequence table from ZX Spectrum
3. **Wait logic**: Each frame, check `frame_counter >= threshold`
4. **State machine**: Use flags to control threshold loading and state transitions
5. **Table format**: [threshold_hi, threshold_lo, flags] per entry

The table-driven approach makes the sequencer data-driven and easy to modify. The C64 port can use the exact same threshold values extracted from motion_script.txt.

## Summary

The sequencer is remarkably simple:
- One 16-bit register (BC) holds target frame
- One 4-instruction loop waits until that frame
- One table pointer (HL) steps through thresholds
- State flags (register A) control transitions

This architecture explains the precise 21-frame and 29-frame freeze periods we observed - they're hardcoded in the sequence table!
