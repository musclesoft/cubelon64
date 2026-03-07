#!/usr/bin/env python3
"""
Extract complete Part 3 algorithm: pattern table, drawing routine, and all pattern addresses.
"""
import json
import base64
import re

print("=== PART 3 ALGORITHM EXTRACTION ===\n")

# 1. Extract pattern table addresses from code trace
print("1. PATTERN TABLE ADDRESSES")
print("   Reading from code_trace.log...")

pattern_addrs = {}
with open('code_trace.log', 'r') as f:
    for line in f:
        # Look for line where SP is set from pattern table
        # Format: 85A9: EB D9 C1 71 | AF:0080 BC:089F DE:84C2 HL:C946 SP:C946
        match = re.match(r'85A9: .+ SP:([0-9A-F]{4})', line)
        if match:
            sp_addr = int(match.group(1), 16)
            pattern_addrs[sp_addr] = pattern_addrs.get(sp_addr, 0) + 1

print(f"\n   Found {len(pattern_addrs)} unique pattern addresses:")
for addr in sorted(pattern_addrs.keys())[:10]:
    print(f"      ${addr:04X}")
if len(pattern_addrs) > 10:
    print(f"      ... and {len(pattern_addrs)-10} more")

# 2. Extract drawing routine from trace
print("\n2. DRAWING ROUTINE DISASSEMBLY")
print("   Extracting from $859D-$866D...\n")

routine = []
with open('code_trace.log', 'r') as f:
    for line in f:
        match = re.match(r'([0-9A-F]{4}): ([0-9A-F]{2}) ([0-9A-F]{2})', line)
        if match:
            pc = int(match.group(1), 16)
            if 0x859D <= pc <= 0x866D and len(routine) < 50:
                routine.append(line.strip())

print("   First 50 instructions:")
for instr in routine[:50]:
    print(f"      {instr}")

# 3. Map pattern data in Bank 2
print("\n3. PATTERN DATA LOCATIONS IN BANK 2")
print("   Analyzing part3_tiles_900-1000.json...\n")

with open('part3_tiles_900-960.json', 'r') as f:
    tiles = json.load(f)

# Group frames by pattern address
addr_to_frames = {}
for tile in tiles:
    addr = tile['pattern_addr']
    if addr not in addr_to_frames:
        addr_to_frames[addr] = []
    addr_to_frames[addr].append(tile['frame'])

print(f"   Found {len(addr_to_frames)} unique pattern addresses in Bank 2:")
for addr in sorted(addr_to_frames.keys())[:20]:
    frames = addr_to_frames[addr]
    frame_range = f"{min(frames)}-{max(frames)}" if len(frames) > 1 else str(frames[0])
    print(f"      ${addr:04X}: frames {frame_range} ({len(frames)} frames)")

if len(addr_to_frames) > 20:
    print(f"      ... and {len(addr_to_frames)-20} more")

# 4. Extract unique pattern data
print("\n4. UNIQUE PATTERN DATA")

pattern_hashes = {}
unique_patterns = []

for tile in tiles:
    pattern_bytes = base64.b64decode(tile['pattern_bytes'])
    import hashlib
    h = hashlib.md5(pattern_bytes).hexdigest()

    if h not in pattern_hashes:
        pattern_hashes[h] = {
            'frames': [],
            'addr': tile['pattern_addr'],
            'data': pattern_bytes
        }
        unique_patterns.append(h)
    pattern_hashes[h]['frames'].append(tile['frame'])

print(f"\n   Found {len(unique_patterns)} unique patterns across 101 frames:")
for i, h in enumerate(unique_patterns[:10]):
    info = pattern_hashes[h]
    frame_list = info['frames'][:3]
    frame_str = ','.join(map(str, frame_list))
    if len(info['frames']) > 3:
        frame_str += f"... ({len(info['frames'])} total)"
    print(f"      Pattern {i}: ${info['addr']:04X} - frames {frame_str}")

if len(unique_patterns) > 10:
    print(f"      ... and {len(unique_patterns)-10} more patterns")

# 5. Analyze animation sequence
print("\n5. ANIMATION SEQUENCE")

frame_sequence = []
for tile in tiles:
    pattern_bytes = base64.b64decode(tile['pattern_bytes'])
    h = hashlib.md5(pattern_bytes).hexdigest()[:8]
    frame_sequence.append((tile['frame'], tile['pattern_addr'], h))

# Find where pattern changes
print("\n   Pattern changes:")
prev_hash = None
for frame, addr, h in frame_sequence[:50]:
    if h != prev_hash:
        print(f"      Frame {frame:4d}: ${addr:04X} [{h}]")
        prev_hash = h

print("\n=== EXTRACTION COMPLETE ===")
