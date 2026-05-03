#!/usr/bin/env python3
"""Apply RRCA transformation to Ch13 data"""
import struct

def rrca(val):
    """Rotate right circular"""
    carry = val & 1
    return (val >> 1) | (carry << 7)

# Load and transform Ch13 data
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
        # Apply RRCA transformation
        transformed = rrca(val)
        ch13_data[frame] = transformed

print(f"Transformed {len(ch13_data)} Ch13 values with RRCA")

# Test: frame 162 should be $0A
if 162 in ch13_data:
    print(f"Frame 162: original=$14 transformed=${ch13_data[162]:02X} (expected $0A)")

# Load and update main data log
with open('emulator/music_data_log.bin', 'rb') as f:
    data = f.read()

output = bytearray()
offset = 0
while offset + 18 <= len(data):
    frame = struct.unpack('<I', data[offset:offset+4])[0]
    values = list(data[offset+4:offset+18])
    
    # Update Ch13 with transformed value
    if frame in ch13_data:
        values[13] = ch13_data[frame]
    
    output.extend(struct.pack('<I', frame))
    output.extend(bytes(values))
    offset += 18

# Write merged data
with open('emulator/music_data_log.bin', 'wb') as f:
    f.write(output)

print(f"Applied RRCA to Ch13 for {len(output) // 18} frames")
