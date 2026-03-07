#!/usr/bin/env python3
"""
Analyze Part 3 screen trace to understand the drawing algorithm.
"""
import re
from collections import defaultdict

# Parse trace file
print("Analyzing Part 3 screen writes...")

# Track PC addresses that write to screen in Part 3 (frames 800-1000)
pc_counts = defaultdict(int)
frame_data = defaultdict(list)

with open('screen_trace.log', 'r') as f:
    for line in f:
        if line.startswith('F'):
            # Parse: F0800 PC:85AC | 71 2C | LD (HL),C | AF:0080 BC:0000 DE:FFBF HL:4884 | -> [4884]=00
            match = re.match(r'F(\d+) PC:([0-9A-F]+) \| ([0-9A-F ]+) \| (.+?) \| (.+?) \| -> \[([0-9A-F]+)\]=([0-9A-F]+)', line)
            if match:
                frame = int(match.group(1))
                pc = int(match.group(2), 16)
                instr = match.group(4).strip()
                addr = int(match.group(6), 16)
                val = int(match.group(7), 16)

                # Focus on Part 3 (frames 800-1000)
                if 800 <= frame <= 1000:
                    pc_counts[pc] += 1
                    frame_data[frame].append({
                        'pc': pc,
                        'instr': instr,
                        'addr': addr,
                        'val': val
                    })

print(f"\nTop 20 PC addresses that write to screen in Part 3 (frames 800-1000):")
top_pcs = sorted(pc_counts.items(), key=lambda x: x[1], reverse=True)[:20]
for pc, count in top_pcs:
    print(f"  ${pc:04X}: {count:6d} writes")

# Analyze drawing pattern for a single frame
print(f"\n\nFrame 900 drawing pattern:")
if 900 in frame_data:
    writes = frame_data[900]
    print(f"Total screen writes: {len(writes)}")

    # Group by PC to see instruction sequence
    pc_sequence = defaultdict(int)
    for w in writes:
        pc_sequence[w['pc']] += 1

    print(f"\nInstruction distribution:")
    for pc, count in sorted(pc_sequence.items(), key=lambda x: x[1], reverse=True)[:10]:
        instr = next(w['instr'] for w in writes if w['pc'] == pc)
        print(f"  ${pc:04X} {instr:16s}: {count:4d} times")

    # Find address range
    addrs = [w['addr'] for w in writes]
    print(f"\nScreen address range: ${min(addrs):04X} - ${max(addrs):04X}")

    # Show first 20 writes
    print(f"\nFirst 20 writes:")
    for w in writes[:20]:
        print(f"  ${w['pc']:04X} {w['instr']:16s} -> [${w['addr']:04X}] = ${w['val']:02X}")
