#!/usr/bin/env python3
"""
Extract channel pointers and data for ALL patterns by running emulator
and dumping memory at every pattern transition.
"""
import subprocess
import os
import struct

# Read the sequence to find all pattern transitions
with open('tools/zx_music_fresh.bin', 'rb') as f:
    mem = f.read()

sequence = mem[0:432]

# Calculate all pattern transitions
frame = 2
seq_ptr = 0
transitions = []

while frame < 9424 and seq_ptr < len(sequence):  # Full song length
    pat_idx = sequence[seq_ptr]
    seq_ptr += 1
    if seq_ptr >= len(sequence):
        seq_ptr = 0  # Loop

    # Get pattern tick count
    rotated = pat_idx
    for i in range(3):
        carry = rotated & 1
        rotated = (rotated >> 1) | (carry << 7)

    pattern_offset = 0x0400 + rotated * 29
    if pattern_offset < len(mem):
        tick = mem[pattern_offset]
        if tick > 0:  # Skip 0-tick patterns
            transitions.append((frame, pat_idx, tick))
            frame += tick
        else:
            # 0-tick pattern, skip to next
            pass
    else:
        break

print(f"Found {len(transitions)} pattern transitions")
print(f"First 10: {transitions[:10]}")
print(f"Last 10: {transitions[-10:]}")

# We can't dump ALL transitions (too many), so dump key ones
# Sample: every 50th frame, plus first 10 patterns
key_frames = [t[0] for t in transitions[:20]]  # First 20 patterns
key_frames += [f for f in range(200, 9424, 200)]  # Every 200 frames

print(f"\nWill dump {len(key_frames)} key frames")
print(f"Key frames: {key_frames[:20]}...")

# Create a Go program modification to dump these
print("\nGenerate emulator dumps:")
print(f"go run -C emulator run_cubelon.go --max-frames=9424 --no-frames")
print("(with modified code to dump at key frames)")
