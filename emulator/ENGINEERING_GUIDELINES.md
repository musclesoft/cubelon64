# Engineering Guidelines for CUBELON Emulator

## THE RULE: Just Emulate the Computer

**NO:**
- ❌ Investigation tools (trace_*.go, check_*.go, show_*.go)
- ❌ Snapshots as shortcuts
- ❌ Hacks to "fix" things (manually setting bits, skipping loops)
- ❌ Assumptions about what should work
- ❌ Working on multiple things at once

**YES:**
- ✅ Boot from known state (ROM 0x0000 or post-BASIC loader)
- ✅ Run the emulator
- ✅ Show visible results (frames)
- ✅ When stuck, ASK the user
- ✅ One step at a time

## Current Task: Run CUBELON Demo

### What we know:
- TAP file has 16 blocks
- Block 0-1: BASIC loader (does: CLEAR 24575, LOAD CODE, USR 30720)
- Block 2-3: Header + 139 bytes of machine code → 0x7800
- Blocks 4+: Demo data loaded by machine code

### The Simple Approach:
1. Load block 3 (machine code) to 0x7800
2. Set PC=0x7800, SP=0x5FFF, IFF1=true, IM=1
3. Provide tape trap for blocks 4-15
4. Run and capture frames

### Current Status:
- Z80 CPU: ✅ Implemented
- Memory system: ✅ Working
- Tape loading trap: ✅ Working
- Next: Load block 3 and run from 0x7800

## When Something Doesn't Work:
1. Show the user what's happening
2. Ask what to do next
3. Don't investigate on your own
