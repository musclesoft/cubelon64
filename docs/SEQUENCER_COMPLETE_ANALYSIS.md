# CUBELON Sequencer - Complete Understanding

## Two-Level Architecture

The demo uses a **two-level timing system**:

1. **Main Sequencer** ($7F6D-$7F76) - Controls major scene transitions
2. **Animation Logic** (various routines) - Controls rapid motion/freeze cycles

This explains why the sequencer table is small (~80 bytes) but the motion script shows 314 periods.

## Main Sequencer - Major Milestones

**Location:** $7F6D-$7F76
**Table:** ~80 bytes at $7A7B
**Function:** Wait for major frame thresholds, then call scene-specific routines

### Observed Thresholds from Execution Trace:

| Threshold | Decimal | Frame Reached | Table Offset | Purpose |
|-----------|---------|---------------|--------------|---------|
| $0050 | 80 | Frame 79 | $7A81-$7A82 | First section transition |
| $009F | 159 | Frame 158 | $7A8D-$7A8E | Intro complete → main demo starts |
| $0320 | 800 | Not reached in trace | $7A92-$7A93 | Major scene change |

### Table Entry Format:

Based on execution traces, each table entry contains:

```
[threshold_lo] [threshold_hi] [data...]
```

**Example from trace:**
```
Frame 88: Loading from HL=$7A8C
  Byte at $7A8C = $9F → C register (low byte)
  Byte at $7A8D = $00 → B register (high byte)
  Result: BC = $009F (159)
```

### Table Pointer Advancement:

The table pointer (HL) advances by variable amounts:
- Frame 79: DE=$7A82 (after reaching threshold $0050)
- Frame 88: HL=$7A8C (advanced by 10 bytes)
- Frame 158: DE=$7A8D (after reaching threshold $009F)
- Frame 188: HL=$7A91 (advanced by 4 bytes)

This indicates entries have variable length depending on associated scene data.

## Animation Logic - Micro-Level Timing

The rapid 21-frame and 29-frame motion/freeze cycles are **NOT** controlled by the main sequencer. They are controlled by:

1. **Scene-specific animation routines** called from the table
2. **Internal timing counters** within each routine
3. **Pattern/effect update logic** that runs every frame

### Evidence:

From motion_script.txt:
- 314 total periods (157 motion + 157 freeze)
- 85 freeze periods of exactly 21 frames
- 51 freeze periods of exactly 29 frames

This rhythmic pattern suggests animation routines with built-in timing, not table-driven thresholds.

### How It Works:

```
Main Sequencer (slow):
  Frame   0-159: Run intro routine (from table)
  Frame 160-799: Run Part 2 routine (from table)
  Frame 800+:    Run Part 3 routine (from table)

Animation Routine (fast):
  While scene active:
    Animate for 19 frames
    Freeze for 21 frames
    Repeat
```

## Complete Table Structure

### Table at $7A7B (80 bytes):

```
Offset  Data          Interpretation
------  ----          --------------
7A7B:   C3 00 00      JMP $0000 or entry marker
7A7E:   04 79         Pointer to routine $7904
7A80:   C0            Flag/parameter
7A81:   50 00         THRESHOLD: $0050 (80) ← Reached at frame 79
7A83:   0C 79         Pointer to routine $790C
7A85:   40            Flag/parameter
7A86:   26 79         Pointer to routine $7926
7A88:   00 00         Data/flags
7A8A:   98 C1         Data/flags
7A8C:   9F 00         THRESHOLD: $009F (159) ← Loaded at frame 88, reached at frame 158
7A8E:   2E 79         Pointer to routine $792E
7A90:   C4            Flag/parameter
7A91:   20 03         THRESHOLD: $0320 (800) ← Loaded at frame 188
7A93:   42 79         Pointer to routine $7942
7A95:   44 5B 79      Data (possibly another pointer $795B with flag $44)
7A98:   C0 9F 0A      Data
7A9B:   63 79         Pointer to routine $7963
7A9D:   40 7C 79      Data (possibly pointer $797C with flag $40)
7AA0:   00 00 90      Data
7AA3:   46 84 79      Data (possibly pointer $7984 with flag $46)
7AA6:   C4 A0 0F      Data
7AA9:   8C 79         Pointer to routine $798C
...
7ACB:   44 09 7A      Data
7ACE:   C0 5F 18      Data
7AD1:   15 7A         Pointer $7A15
7AD3:   40 21 7A      Data (possibly pointer $7A21 with flag $40)
7AD6:   20 00 00      Data or END marker
7AD9:   00 00 ...     Padding
```

### Scene Routines ($79xx range):

Each threshold in the table is followed by a pointer to a scene routine:
- $7904, $790C, $7926, $792E, $7942, $795B, etc.

These routines handle:
- Graphics initialization
- Animation loops
- Internal timing (21-frame, 29-frame cycles)
- Effect-specific logic

## Summary

**Main Sequencer:**
- Simple wait loop checking frame counter against BC register
- Loads thresholds from compact table (~80 bytes)
- ~10-20 major scene transitions across entire demo
- Controls high-level scene progression

**Animation System:**
- Scene-specific code called from main sequencer
- Handles rapid motion/freeze timing internally
- 314 micro-transitions controlled by animation logic, not table
- Creates rhythmic 21-frame / 29-frame patterns

**For C64 Port:**
1. Use motion_script.txt for exact per-frame timing (314 periods)
2. Don't need to decode full sequencer table
3. Can implement as simple state machine: motion/freeze with frame counters
4. Or: port the scene routines that generate the patterns naturally

The sequencer table is for scene management, not per-period timing.
