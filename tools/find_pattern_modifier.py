#!/usr/bin/env python3
"""
Find code that modifies the pattern data in Bank 2.
Pattern data is around $BE06 and $C046 (288 bytes each).
"""

# Check if any screen writes are actually to Bank 2 pattern area
# (Bank 2 is also mapped at $C000-$FFFF in ZX 128K)

with open('screen_trace.log', 'r') as f:
    bank2_writes = []
    for line in f:
        if '->' in line and '[' in line:
            # Extract write address
            addr_str = line.split('[')[1].split(']')[0]
            addr = int(addr_str, 16)

            # Check if write is in pattern region
            # Patterns at $BE06 and $C046, each 288 bytes (0x120)
            if (0xBE06 <= addr < 0xBE06+0x120) or (0xC046 <= addr < 0xC046+0x120):
                bank2_writes.append(line.strip())

if bank2_writes:
    print(f"Found {len(bank2_writes)} writes to pattern data:")
    for line in bank2_writes[:50]:
        print(line)
else:
    print("No writes to pattern data found in screen_trace.log")
    print("\nPattern data must be modified by:")
    print("1. Code outside the traced frame range")
    print("2. Direct memory manipulation not traced")
    print("3. Or patterns are truly static and animation comes from somewhere else")
