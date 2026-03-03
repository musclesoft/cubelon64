#!/usr/bin/env python3
"""
Verify display logic by simulating what should be written to screen RAM
"""

def simulate_draw_border():
    """Simulate the draw_border routine"""
    screen = [0] * 1000  # 40x25 screen

    # Step 1: Fill with $A0 (160, solid block)
    for i in range(1000):
        screen[i] = 0xA0

    # Step 2: Clear center 32x24 area starting at column 4
    DISPLAY_WIDTH = 32
    DISPLAY_HEIGHT = 24
    DISPLAY_X = 4

    for row in range(DISPLAY_HEIGHT):
        offset = row * 40 + DISPLAY_X
        for col in range(DISPLAY_WIDTH):
            screen[offset + col] = 0x20  # Space

    return screen

def verify_screen(screen):
    """Verify the screen has correct pattern"""
    errors = []

    # Check borders (should be $A0)
    # Left border: columns 0-3
    for row in range(25):
        for col in range(4):
            pos = row * 40 + col
            if screen[pos] != 0xA0:
                errors.append(f"Row {row}, col {col}: expected $A0, got ${screen[pos]:02X}")

    # Right border: columns 36-39
    for row in range(25):
        for col in range(36, 40):
            pos = row * 40 + col
            if screen[pos] != 0xA0:
                errors.append(f"Row {row}, col {col}: expected $A0, got ${screen[pos]:02X}")

    # Bottom row (row 24): columns 4-35 should be $A0
    for col in range(4, 36):
        pos = 24 * 40 + col
        if screen[pos] != 0xA0:
            errors.append(f"Row 24, col {col}: expected $A0, got ${screen[pos]:02X}")

    # Center area (rows 0-23, columns 4-35): should be $20
    for row in range(24):
        for col in range(4, 36):
            pos = row * 40 + col
            if screen[pos] != 0x20:
                errors.append(f"Row {row}, col {col}: expected $20 (space), got ${screen[pos]:02X}")

    return errors

def visualize_screen(screen):
    """Create ASCII visualization"""
    lines = []
    for row in range(25):
        line = ""
        for col in range(40):
            val = screen[row * 40 + col]
            if val == 0xA0:
                line += "█"  # Solid block
            elif val == 0x20:
                line += " "  # Space
            else:
                line += "?"  # Unknown
        lines.append(line)
    return "\n".join(lines)

if __name__ == '__main__':
    print("Simulating draw_border routine...")
    screen = simulate_draw_border()

    print("\nVerifying screen layout...")
    errors = verify_screen(screen)

    if errors:
        print(f"❌ FAILED: {len(errors)} errors found:")
        for error in errors[:10]:  # Show first 10
            print(f"  - {error}")
    else:
        print("✅ PASSED: Screen layout is correct")

    print("\nVisualization (border should be █, center should be blank):")
    print(visualize_screen(screen))
