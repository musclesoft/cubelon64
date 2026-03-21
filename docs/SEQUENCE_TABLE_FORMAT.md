# CUBELON Sequence Table Format

## Table Location

**Table pointer:** Stored at memory address $7902 (bank 2)
- At runtime (frame 200): Points to $7A7B

**Table data:** Located at $7A7B in memory

## Raw Table Data

```
Offset  +0  +1  +2  +3  +4  +5  +6  +7  +8  +9  +A  +B  +C  +D  +E  +F
------  --  --  --  --  --  --  --  --  --  --  --  --  --  --  --  --
7A7B:   C3  00  00  04  79  C0  50  00  0C  79  40  26  79  00  00  98
7A8B:   C1  9F  00  2E  79  C4  20  03  42  79  44  5B  79  C0  9F  0A
7A9B:   63  79  40  7C  79  00  00  90  46  84  79  C4  A0  0F  8C  79
7AAB:   00  12  90  C0  E0  10  AA  79  00  24  90  C4  20  12  CB  79
7ABB:   00  36  90  C0  60  13  DF  79  40  F9  79  C4  9F  14  01  7A
7ACB:   44  09  7A  C0  5F  18  15  7A  40  21  7A  20  00  00  00  00
```

## Table Entry Format (Tentative)

Based on the sequencer code at $7F69-$7F6B:
```asm
$7F69: LD C,(HL)    ; Load low byte from table
$7F6A: INC HL       ; Advance pointer
$7F6B: LD B,(HL)    ; Load high byte from table
```

The table contains 16-bit little-endian values loaded into BC register.

**Entry structure (hypothesis):**
- Each entry appears to be variable length
- Contains 16-bit threshold values (little-endian)
- Contains flag bytes that control state machine behavior
- Code pointer addresses (routines to call for each sequence state)

## Known Threshold Values

From BC register analysis, the sequencer uses these frame thresholds:
- 166 ($00A6) - End of intro freeze
- 179 ($00B3) - First motion period
- 200 ($00C8) - First 21-frame freeze
- 240 ($00F0) - Second 21-frame freeze
- And so on...

## Entry Parsing Strategy

The table entries are not uniform in length. The code uses bit flags in register A to determine:
```asm
$7F61: BIT 5,A    ; Check bit 5 (major state flag?)
$7F65: BIT 7,A    ; Check bit 7 (load new threshold?)
$7F78: BIT 6,A    ; Check bit 6 (next action?)
```

**Possible format:**
```
[flag_byte] [threshold_lo] [threshold_hi] [action_ptr_lo] [action_ptr_hi] ...
```

Or:
```
[threshold_lo] [threshold_hi] [flag_byte] [action_ptr_lo] [action_ptr_hi] ...
```

## Code Pointers in Table

Many of the byte sequences look like Z80 code addresses in the $79xx range:
- $7904 at offset +3,+4
- $790C at offset +7,+8
- $7926 at offset +A,+B
- $792E at offset +10,+11
- $7942 at offset +17,+18
- $795B at offset +1A,+1B
- $7963 at offset +1F,+20
- etc.

These are likely pointers to subroutines that execute specific demo effects for each sequence period.

## Table Size Estimation

The table continues until we see padding zeros at $7ACB onwards:
```
7ACB: 44 09 7A C0 5F 18 15 7A 40 21 7A 20 00 00 00 00
                                          ^^^^^^^^^^^^ padding
```

**Estimated size:** ~80 bytes ($7A7B to ~$7ACB)

This would accommodate approximately 314 periods we discovered in the motion analysis if entries average ~0.25 bytes per period (many periods share the same routine pointers and only differ in threshold values).

## Next Steps for Analysis

To fully understand the format, we need to:

1. Trace BC register values during execution to capture actual thresholds loaded
2. Track register A flag values to understand state machine transitions
3. Monitor HL register to see how the table pointer advances through entries
4. Disassemble the code routines pointed to by addresses in the table ($79xx range)

## Correlation with Motion Script

The motion script shows 314 periods (157 motion + 157 freeze) with these patterns:
- 85 freeze periods of 21 frames
- 51 freeze periods of 29 frames
- Various longer motion and freeze periods

The compact table size (80 bytes) suggests the table encodes:
- Default period durations (21, 29 frames)
- Override thresholds for special transitions
- Action routine pointers for different effects

Rather than storing every individual period explicitly, the table likely uses a compressed format with rules and defaults.
