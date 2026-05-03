#!/usr/bin/env python3
"""Build music_data_log.bin from ay_commands.bin"""
import struct

# Load AY commands
with open('../emulator/ay_commands.bin', 'rb') as f:
    count = struct.unpack('<I', f.read(4))[0]
    commands = []
    for i in range(count):
        frame, reg, val = struct.unpack('<IBB', f.read(6))
        commands.append((frame, reg, val))

# Group by frame
frame_data = {}
for frame, reg, val in commands:
    if frame not in frame_data:
        frame_data[frame] = [0] * 14
    if reg < 14:
        frame_data[frame][reg] = val

# Write to data log format
with open('../emulator/music_data_log_from_ay.bin', 'wb') as f:
    for frame in sorted(frame_data.keys()):
        f.write(struct.pack('<I', frame))
        f.write(bytes(frame_data[frame]))

print(f"Built data table from {len(commands)} AY commands")
print(f"Covers {len(frame_data)} frames")
