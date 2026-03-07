#!/usr/bin/env python3
"""
Parallel brute force focusing on size parameter.
User observation: more red than blue pixels suggests square is too small.
"""

import math
import re
from multiprocessing import Pool, cpu_count
from rotating_square import parse_frame, compare_bitmaps
from test_8bit_precision import render_square_8bit

# Load animation file
with open("../emulator/frames/animation_318-336.txt", 'r') as f:
    content = f.read()

# Parse all test frames
frame_pattern = re.compile(r'=== Frame (\d+)')

def load_frame(target_frame):
    lines = content.split('\n')
    current_frame_lines = []
    current_frame_num = None

    for line in lines:
        match = frame_pattern.search(line)
        if match:
            if current_frame_num == target_frame and current_frame_lines:
                bitmap, _ = parse_frame(current_frame_lines)
                return bitmap
            current_frame_num = int(match.group(1))
            current_frame_lines = [line]
        else:
            current_frame_lines.append(line)

    if current_frame_num == target_frame:
        bitmap, _ = parse_frame(current_frame_lines)
        return bitmap
    return None

# Test parameters (from previous best)
test_configs = [
    (320, 12.0, 11.0, 8.0, 61.1),
    (322, 12.5, 11.5, 8.0, 70.9),
    (324, 11.5, 11.0, 8.0, 77.3),
    (326, 12.0, 11.0, 7.5, -4.2),
    (328, 12.5, 11.0, 8.0, 9.1),
    (330, 11.5, 11.5, 8.0, 17.3),
    (332, 12.0, 11.0, 8.0, 22.0),
    (334, 12.0, 11.5, 8.0, 35.2),
]

def test_params_worker(args):
    """Worker function for parallel testing"""
    frame_num, bitmap, cx, cy, size, angle = args

    rendered = render_square_8bit(cx, cy, size, math.radians(angle), 24, 24)
    matches, total = compare_bitmaps(bitmap, rendered)

    return (frame_num, cx, cy, size, angle, matches)

def optimize_frame(frame_config):
    """Optimize parameters for one frame"""
    frame_num, base_cx, base_cy, base_size, base_angle = frame_config

    bitmap = load_frame(frame_num)
    if not bitmap:
        return None

    # 8.8 precision
    step = 1.0 / 256.0

    # Build parameter space
    # Focus on LARGER sizes since we have more red (missing) pixels
    tasks = []

    # Test sizes from base-0.5 to base+1.5 (emphasize larger)
    for size_offset in range(-128, 385, 2):  # -0.5 to +1.5 in steps of 2/256
        size = base_size + size_offset * step

        if size < 1.0:
            continue

        # Fine cx/cy around base
        for cx_offset in range(-64, 65, 4):  # ±0.25 in steps of 4/256
            cx = base_cx + cx_offset * step

            for cy_offset in range(-64, 65, 4):
                cy = base_cy + cy_offset * step

                # Fine angle
                for angle_offset in range(-20, 21):  # ±2° in 0.1° steps
                    angle = base_angle + angle_offset * 0.1

                    tasks.append((frame_num, bitmap, cx, cy, size, angle))

    return tasks

print(f"Parallel brute force with emphasis on LARGER sizes")
print(f"CPUs available: {cpu_count()}")
print("=" * 70)

# Build all tasks
all_tasks = []
for config in test_configs:
    frame_num = config[0]
    print(f"Building parameter space for frame {frame_num}...")
    tasks = optimize_frame(config)
    if tasks:
        all_tasks.extend(tasks)
        print(f"  {len(tasks):,} combinations")

print(f"\nTotal combinations: {len(all_tasks):,}")
print(f"Testing with {cpu_count()} parallel workers...")
print()

# Run in parallel
best_results = {}

with Pool(cpu_count()) as pool:
    results = pool.imap_unordered(test_params_worker, all_tasks, chunksize=1000)

    tested = 0
    for frame_num, cx, cy, size, angle, matches in results:
        tested += 1

        if tested % 10000 == 0:
            print(f"  Tested {tested:,}/{len(all_tasks):,} ({100*tested/len(all_tasks):.1f}%)")

        if frame_num not in best_results or matches > best_results[frame_num]['matches']:
            best_results[frame_num] = {
                'matches': matches,
                'params': (cx, cy, size, angle)
            }

            match_pct = 100 * matches / 576
            if match_pct > 99.0:
                print(f"    Frame {frame_num}: {match_pct:.3f}% (cx={cx:.4f}, cy={cy:.4f}, size={size:.4f}, angle={angle:.2f}°)")

print("\n" + "=" * 70)
print("Best results:")

total_matches = 0
for frame_num in sorted(best_results.keys()):
    result = best_results[frame_num]
    matches = result['matches']
    cx, cy, size, angle = result['params']
    match_pct = 100 * matches / 576
    diff = 576 - matches

    total_matches += matches

    print(f"Frame {frame_num}: {match_pct:.3f}% ({matches}/576, diff={diff})")
    print(f"  cx={cx:.6f}, cy={cy:.6f}, size={size:.6f}, angle={angle:.2f}°")

    if matches == 576:
        print(f"  ✓ PERFECT 100%!")

avg_match = 100 * total_matches / (len(best_results) * 576)
avg_diff = (len(best_results) * 576 - total_matches) / len(best_results)

print(f"\nAverage: {avg_match:.3f}%")
print(f"Average diff: {avg_diff:.1f} pixels/frame")

perfect = sum(1 for r in best_results.values() if r['matches'] == 576)
if perfect > 0:
    print(f"\n✓ {perfect} perfect matches found!")
else:
    print(f"\nNo perfect matches - confirms pre-rendered bitmaps")

# Save best parameters
with open("best_params_parallel.txt", "w") as f:
    for frame_num in sorted(best_results.keys()):
        cx, cy, size, angle = best_results[frame_num]['params']
        matches = best_results[frame_num]['matches']
        f.write(f"{frame_num} {cx} {cy} {size} {angle} {matches}\n")

print(f"\nSaved to best_params_parallel.txt")
