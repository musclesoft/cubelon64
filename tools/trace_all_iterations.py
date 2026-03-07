#!/usr/bin/env python3
"""
Trace all 8 DJNZ iterations to see complete 576-byte write pattern.

This shows which pattern bytes overwrite which screen positions.
"""

import json

# Load single-iteration write sequence
with open('part3_write_sequence.json') as f:
    single_iter = json.load(f)

writes_per_iter = single_iter['writes']
print(f"Bytes per iteration: {len(writes_per_iter)}")
print()

# Simulate all 8 iterations
all_writes = []
for iter_num in range(8):
    pattern_byte_offset = iter_num * len(writes_per_iter)

    for write in writes_per_iter:
        all_writes.append({
            'pattern_byte': pattern_byte_offset + write['pattern_byte'],
            'screen_addr': write['screen_addr'],
            'y': write['y'],
            'x_byte': write['x_byte'],
            'iteration': iter_num,
            'register': write['register']
        })

print(f"Total writes across 8 iterations: {len(all_writes)}")
print()

# Check for overwrites
from collections import defaultdict
addr_to_writes = defaultdict(list)
for w in all_writes:
    addr_to_writes[w['screen_addr']].append(w['pattern_byte'])

unique_addrs = len(addr_to_writes)
overwritten_addrs = sum(1 for writes in addr_to_writes.values() if len(writes) > 1)

print(f"Unique screen addresses: {unique_addrs}")
print(f"Addresses written multiple times: {overwritten_addrs}")
print(f"Addresses written once: {unique_addrs - overwritten_addrs}")
print()

# Show some examples of overwrites
print("Example overwrites (first 10):")
print(f"{'Addr':<8} {'Y':<5} {'XByte':<7} {'Pattern bytes'}")
print("-" * 60)
for addr in sorted(addr_to_writes.keys())[:10]:
    writes_list = addr_to_writes[addr]
    if len(writes_list) > 1:
        # Find y, x_byte from first write to this address
        first_write = [w for w in all_writes if w['screen_addr'] == addr][0]
        y = first_write['y']
        x_byte = first_write['x_byte']
        print(f"${addr:04X}    {y:<5} {x_byte:<7} {writes_list}")

print()

# For source-only decoding, we need the FINAL value at each position
# (i.e., the last write to each address)
final_values = {}
for addr, pattern_bytes in addr_to_writes.items():
    final_values[addr] = pattern_bytes[-1]  # Last write wins

print(f"Final permutation table (last write to each address):")
print(f"  Covers {len(final_values)} unique screen addresses")
print(f"  Uses pattern bytes: {sorted(set(final_values.values()))[:20]}... ({len(set(final_values.values()))} unique)")
print()

# Check if all 288 tile bytes are covered
tile_bytes = 48 * 6  # 48 rows × 6 bytes/row = 288
if len(final_values) == tile_bytes:
    print(f"✅ Perfect! All {tile_bytes} tile bytes are written")
else:
    print(f"⚠️  Expected {tile_bytes} bytes but got {len(final_values)}")

# Save complete trace
output = {
    'description': 'Complete 8-iteration write trace for Part 3',
    'iterations': 8,
    'writes_per_iteration': len(writes_per_iter),
    'total_writes': len(all_writes),
    'unique_addresses': unique_addrs,
    'all_writes': all_writes,
    'final_values': [{'screen_addr': addr, 'pattern_byte': pb,
                      'y': [w for w in all_writes if w['screen_addr'] == addr][0]['y'],
                      'x_byte': [w for w in all_writes if w['screen_addr'] == addr][0]['x_byte']}
                     for addr, pb in sorted(final_values.items(), key=lambda x: x[1])]
}

with open('part3_complete_trace.json', 'w') as f:
    json.dump(output, f, indent=2)

print(f"✅ Saved part3_complete_trace.json")
