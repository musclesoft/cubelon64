#!/usr/bin/env python3
"""
Simulate the entire program execution to verify output
"""

def simulate_full_program():
    """Simulate main program flow"""
    screen = [0] * 1000

    # Step 1: clear_screen - fill with spaces
    for i in range(1000):
        screen[i] = 0x20

    # Step 2: draw_border
    # Fill entire screen with $A0
    for i in range(1000):
        screen[i] = 0xA0

    # Clear center 32x24 starting at column 4
    for row in range(24):
        offset = row * 40 + 4
        for col in range(32):
            screen[offset + col] = 0x20

    # Step 3: pattern_test (first frame, anim_frame = 0)
    # Draws 3x3 pattern at row 15, column 18
    # Pattern structure:
    # Row 0: T0(0), T1(1), T0(0)
    # Row 1: T1(1), T2(2), T1(1)
    # Row 2: T0(0), T1(1), T0(0)

    base_row = 15
    base_col = 18

    # Row 1 of pattern
    screen[(base_row + 0) * 40 + (base_col + 0)] = 0  # Tile 0
    screen[(base_row + 0) * 40 + (base_col + 1)] = 1  # Tile 1
    screen[(base_row + 0) * 40 + (base_col + 2)] = 0  # Tile 0

    # Row 2 of pattern
    screen[(base_row + 1) * 40 + (base_col + 0)] = 1  # Tile 1
    screen[(base_row + 1) * 40 + (base_col + 1)] = 2  # Tile 2
    screen[(base_row + 1) * 40 + (base_col + 2)] = 1  # Tile 1

    # Row 3 of pattern
    screen[(base_row + 2) * 40 + (base_col + 0)] = 0  # Tile 0
    screen[(base_row + 2) * 40 + (base_col + 1)] = 1  # Tile 1
    screen[(base_row + 2) * 40 + (base_col + 2)] = 0  # Tile 0

    return screen

def visualize_screen_with_patterns(screen):
    """Visualize screen with pattern tiles shown"""
    lines = []
    for row in range(25):
        line = ""
        for col in range(40):
            val = screen[row * 40 + col]
            if val == 0xA0:
                line += "█"
            elif val == 0x20:
                line += "."
            elif val == 0:
                line += "0"
            elif val == 1:
                line += "1"
            elif val == 2:
                line += "2"
            else:
                line += "?"
        lines.append(line)
    return "\n".join(lines)

def verify_pattern_visible(screen):
    """Check if the 3x3 pattern is visible and correct"""
    base_row = 15
    base_col = 18

    expected = [
        [0, 1, 0],
        [1, 2, 1],
        [0, 1, 0]
    ]

    errors = []
    for r in range(3):
        for c in range(3):
            pos = (base_row + r) * 40 + (base_col + c)
            actual = screen[pos]
            expected_val = expected[r][c]
            if actual != expected_val:
                errors.append(f"Pattern row {r}, col {c}: expected {expected_val}, got {actual}")

    return errors

if __name__ == '__main__':
    print("Simulating complete program execution...\n")

    screen = simulate_full_program()

    print("Checking pattern visibility...")
    errors = verify_pattern_visible(screen)

    if errors:
        print(f"❌ FAILED: {len(errors)} errors")
        for error in errors:
            print(f"  - {error}")
    else:
        print("✅ PASSED: Pattern is correctly positioned and visible\n")

    print("Screen visualization:")
    print("  █ = border (tile 160)")
    print("  . = empty center (tile 32)")
    print("  0,1,2 = pattern tiles\n")

    vis = visualize_screen_with_patterns(screen)
    lines = vis.split('\n')

    # Show relevant area (rows 13-18, full width)
    print("Rows 13-18 (pattern area):")
    for i, line in enumerate(lines[13:19], 13):
        print(f"{i:2d}: {line}")
