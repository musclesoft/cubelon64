#!/usr/bin/env python3
"""Merge Ch13 data into main data log"""
import struct

# Load Ch13 data
ch13_data = {}
with open('emulator/music_data_ch13.bin', 'rb') as f:
    while True:
        frame_bytes = f.read(4)
        if len(frame_bytes) < 4:
            break
        frame = struct.unpack('<I', frame_bytes)[0]
        val_bytes = f.read(1)
        if len(val_bytes) < 1:
            break
        val = struct.unpack('B', val_bytes)[0]
        ch13_data[frame] = val

print(f"Loaded {len(ch13_data)} Ch13 values")

# Load and update main data log
with open('emulator/music_data_log.bin', 'rb') as f:
    data = f.read()

# Parse and update
output = bytearray()
offset = 0
while offset + 18 <= len(data):
    frame = struct.unpack('<I', data[offset:offset+4])[0]
    values = list(data[offset+4:offset+18])
    
    # Update Ch13 if we have data for this frame
    if frame in ch13_data:
        values[13] = ch13_data[frame]
    
    output.extend(struct.pack('<I', frame))
    output.extend(bytes(values))
    offset += 18

# Write merged data
with open('emulator/music_data_log.bin', 'wb') as f:
    f.write(output)

print(f"Merged Ch13 data into {len(output) // 18} frames")
