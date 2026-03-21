# CUBELON Major Transitions

## Frame Numbers of Major Section Changes

Based on sequencer trace and motion_script.txt analysis:

### Confirmed from Sequencer Trace:

| Frame   | Time   | Event                            | Threshold |
| ------- | ------ | -------------------------------- | --------- |
| **80**  | 1.60s  | First section transition         | $0050     |
| **159** | 3.18s  | **Intro ends, main demo starts** | $009F     |
| **800** | 16.00s | Major scene change               | $0320     |

### Identified from Motion Pattern Analysis:

| Frame    | Time    | Event                           | Pattern Change                        |
| -------- | ------- | ------------------------------- | ------------------------------------- |
| **2**    | 0.04s   | Demo starts                     | First motion frame                    |
| **3**    | 0.06s   | Intro freeze begins             | 164-frame freeze                      |
| **167**  | 3.34s   | Animation starts                | 19-motion/21-freeze pattern begins    |
| **2721** | 54.42s  | **Part 2: Long motion section** | 469 continuous frames (9.38s)         |
| **3190** | 63.80s  | Pattern shift                   | 11-frame freezes appear               |
| **4002** | 80.04s  | **Part 3: Rhythmic section**    | Four 291-frame segments               |
| **4322** | 86.44s  | Segment 2                       | 291-frame motion                      |
| **4642** | 92.84s  | Segment 3                       | 291-frame motion                      |
| **4962** | 99.24s  | Segment 4                       | 291-frame motion                      |
| **5280** | 105.60s | **Part 4: Longest motion**      | 880 continuous frames (17.6s!)        |
| **6160** | 123.20s | **Part 5: Flash section**       | 73-frame freezes with 7-frame flashes |
| **6561** | 131.22s | Pattern returns                 | Back to 19/21 rhythm                  |
| **7041** | 140.82s | **Part 6: Buildup**             | 103-frame continuous motion           |
| **7144** | 142.88s | Section transition              | 57-frame freeze                       |
| **7201** | 144.02s | **Part 7: Rapid flashing**      | 11-frame motion with 29-frame freezes |
| **9170** | 183.40s | **Demo ends**                   | Final 251-frame freeze (5.02s)        |
| **9420** | 188.40s | End credits/finish              | Demo complete                         |

## Section Summary

### Section 0: Intro (frames 2-166, 3.3s)

- Single motion frame followed by long freeze
- Setup/loading screen

### Section 1: Regular Pattern (frames 167-2720, 51.1s)

- Consistent 19-motion / 21-freeze rhythm
- 85 freeze periods of 21 frames
- Main cube rotation animation

### Section 2: Extended Motion (frames 2721-3189, 9.4s)

- **469 continuous motion frames**
- No freeze periods
- Complex animated effect

### Section 3: Rhythmic Motion (frames 4002-5280, 25.6s)

- Four identical 291-frame motion segments
- 29-frame freezes between segments
- Synchronized/choreographed animation

### Section 4: Climax (frames 5280-6159, 17.6s)

- **880 continuous motion frames** (longest in demo)
- Peak visual complexity

### Section 5: Flash Section (frames 6160-6560, 8.0s)

- Brief 7-frame motion flashes
- Long 73-frame freezes
- Strobe/flash effect

### Section 6: Return to Pattern (frames 6561-7040, 9.6s)

- Back to 19-motion / 21-freeze rhythm
- Winding down

### Section 7: Buildup (frames 7041-7200, 3.2s)

- 103-frame continuous motion
- Building to finale

### Section 8: Rapid Flash (frames 7201-9169, 39.4s)

- 11-frame motion bursts
- 29-frame freeze between each
- 50+ repetitions

### Section 9: Finale (frames 9170-9420, 5.0s)

- Final 251-frame freeze
- Credits or end screen

## Sequencer Table Relationship

The **main sequencer** (table at $7A7B) controls these major sections (1-9).

The **animation code** within each section generates the micro-level motion/freeze patterns.

For example:

- Section 1 code creates the 19/21 rhythm automatically
- Section 3 code creates the 291-frame segments automatically
- Section 8 code creates the 11/29 flash pattern automatically

This two-level system keeps the sequencer table compact (~80 bytes) while supporting 314 total motion/freeze periods.
