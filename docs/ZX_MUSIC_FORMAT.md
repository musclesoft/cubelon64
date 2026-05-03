# ZX Spectrum Music Format - CUBELON

## Overview

This document describes the reverse-engineered music format used in the CUBELON ZX Spectrum demo. The music player runs on the ZX Spectrum 128K using the AY-3-8912 programmable sound generator (PSG).

## Hardware Platform

**ZX Spectrum 128K:**
- CPU: Z80 @ 3.546895 MHz
- Sound chip: AY-3-8912 @ 1.773447 MHz (CPU/2)
- Frame rate: 50 Hz (PAL)
- 3 tone channels + 1 noise channel
- Hardware envelope generator

**AY-3-8912 Registers:**
```
R0-R1:  Channel A tone period (12-bit)
R2-R3:  Channel B tone period (12-bit)
R4-R5:  Channel C tone period (12-bit)
R6:     Noise period (5-bit)
R7:     Mixer control (tone/noise enable per channel)
R8-R10: Volume control (4-bit) per channel
R11-R12: Envelope period (16-bit)
R13:    Envelope shape (4-bit)
```

## Music Player Architecture

### Interrupt-Driven Playback

The music player runs from the interrupt handler at **$7D26**, which is called at 50Hz (once per frame).

**Call chain:**
```
$7D26  - Interrupt handler (frame entry)
  └─> $980E  - Music update routine
      ├─> $857A  - Graphics/sprite routine (not music)
      ├─> $981D  - Music tick handler
      └─> $9851  - Frame counter decrement
```

### Timing System

**Frame-based timing:**
- 1 frame = 1/50 second = 20ms
- Music update runs every frame (50Hz)

**Tick-based sequencing:**
- Common tick rate: **5 frames per tick** = 10Hz note rate
- Allows for both fast effects (per-frame) and slower note changes (per-tick)

### Music Data Structure

Based on analysis of the existing `extract_tracker_format.go` implementation:

```go
type ZXMusicData struct {
    StartOffset  uint16    // Frame delay before music starts (172 frames in CUBELON)
    TickRate     uint8     // Frames per tick (5 = 10Hz)
    
    FreqTable    []uint16  // AY tone periods for all pitches (43 unique)
    Patterns     []Pattern // Note patterns (pitch + duration + effects)
    Instruments  []Instrument // Sound characteristics
    Streams      [3][]byte // Per-channel sequencer bytecode
}
```

## Frequency Encoding

**AY Tone Period Calculation:**
```
period = AY_CLOCK_HZ / (16 × frequency_hz)
       = 1773447 / (16 × frequency_hz)
```

**Example frequencies:**
- Middle C (261.63 Hz): period = 423
- A440: period = 253

**CUBELON uses 43 unique frequencies** covering approximately 3.5 octaves.

## Pattern System

Each pattern represents a single note or rest:

```go
type Pattern struct {
    FreqIndex    uint8  // Index into FreqTable (0xFF = rest)
    Duration     uint8  // Duration in ticks
    InstrumentID uint8  // Which instrument to use
    EffectType   uint8  // Effect type (0=none, 1=vibrato, 2=portamento, etc.)
    EffectParam1 uint8  // Effect parameter 1
    EffectParam2 uint8  // Effect parameter 2
}
```

**Common durations in CUBELON:**
- 30 frames (0.60s): 160 occurrences - most common (half note at 10Hz)
- 5 frames (0.10s): 117 occurrences - fast notes
- 10 frames (0.20s): 94 occurrences - quarter note
- 29 frames (0.58s): 66 occurrences - slightly shorter than 30

## Instrument System

Instruments define the sound characteristics:

```go
type Instrument struct {
    // Volume Envelope (ADSR)
    Attack      uint8  // Frames to reach full volume
    Decay       uint8  // Frames to decay to sustain level
    Sustain     uint8  // Sustain volume (0-15)
    Release     uint8  // Frames to fade out after note off
    
    // Waveform
    UseTone     bool   // Enable tone generator
    UseNoise    bool   // Enable noise generator
    UseEnvelope bool   // Use hardware envelope (R13)
    EnvShape    uint8  // R13 envelope shape (if UseEnvelope=true)
    
    // Noise (if UseNoise=true)
    NoisePeriod uint8  // R6 noise period value
    
    // Vibrato
    VibratoDelay uint8  // Frames before vibrato starts
    VibratoDepth uint8  // Vibrato depth in AY period units
    VibratoSpeed uint8  // Vibrato speed (frames per cycle)
}
```

## Effect System

Effects are applied per-frame to create pitch modulation:

**Effect Types:**
1. **None (0)**: Static pitch
2. **Vibrato (1)**: Periodic pitch oscillation
3. **Portamento (2)**: Smooth pitch slide
4. **Arpeggio (3)**: Rapid alternation between pitches

**Vibrato Implementation:**
- Delay period before vibrato starts
- Sine-wave approximation using lookup table
- Applied as offset to base AY period

## Stream Sequencer

Each of the 3 channels has a bytecode stream:

**Stream format:**
```
0x00-0x7F: Play pattern ID
0x80-0xFE: Commands (loop, jump, etc.)
0xFF:      End of stream
```

**Example stream:**
```
[0x12, 0x13, 0x14, 0xFF]  // Play patterns 18, 19, 20, then end
```

## Music Statistics (CUBELON)

**From AY command analysis:**
- Start offset: 172 frames (3.44 seconds)
- Total duration: 185 seconds (3:05)
- Total notes: 589
- Unique frequencies: 43
- AY commands: 123,027

**Per-channel activity:**
- Channel A: 127 notes (melody)
- Channel B: 197 notes (harmony/bass)
- Channel C: 265 notes (rhythm/effects)

## Data Size Analysis

**Original ZX Spectrum format (estimated from analysis):**
- Frequency table: 86 bytes (43 entries × 2)
- Note data: ~2356 bytes (589 notes × 4)
- Instruments: ~64 bytes (8 instruments × 8)
- Stream data: ~589 bytes (pattern references)
- Effect patterns: ~100 bytes
- **Total: ~3195 bytes (3.1 KB) uncompressed**

**Note:** The original ZX binary uses compressed data blocks in the TAP file. The actual storage format is not fully reverse-engineered, but the music player expands the data at runtime. The 123,027 AY register writes during playback are generated from this compressed source data.

## Player Implementation

### Per-Frame Update

```go
func (player *ZXMusicPlayer) Update() {
    // 1. Increment frame counter
    player.CurrentFrame++
    
    // 2. Skip if before music start
    if player.CurrentFrame < player.Data.StartOffset {
        return
    }
    
    // 3. Check for tick boundary
    player.FrameCounter++
    if player.FrameCounter >= player.Data.TickRate {
        player.FrameCounter = 0
        player.CurrentTick++
        
        // 4. Process new notes at tick boundaries
        for ch := 0; ch < 3; ch++ {
            player.updateChannelTick(ch)
        }
    }
    
    // 5. Update per-frame effects (envelopes, vibrato)
    for ch := 0; ch < 3; ch++ {
        player.updateChannelFrame(ch)
    }
    
    // 6. Write to AY registers
    player.writeAYRegisters()
}
```

### Envelope State Machine

```
ATTACK → DECAY → SUSTAIN → RELEASE
  ↓       ↓        ↓          ↓
  0→15   15→S      S         S→0
```

Each phase duration controlled by instrument ADSR parameters.

## Memory Layout (Original ZX Implementation)

Based on disassembly analysis:

```
$7D26-$7E1E:  Interrupt handler
$7EC4:        Frame counter (decrements each frame)
$7F6D-$7F76:  Main sequencer
$8184:        Frame counter (16-bit)
$8186-$8189:  Player state variables
$857A-$8670:  Graphics routine (sprite/screen update)
$980E-$982B:  Music player entry point
$9851-$9855:  Frame counter update
$9B1A:        Tick counter
```

## AY-3-8912 Programming

**Register Write Sequence:**
```z80
LD BC, $FFFD        ; Select register port
OUT (C), A          ; Write register number
LD BC, $BFFD        ; Select data port
OUT (C), A          ; Write value
```

**Typical register update order per frame:**
1. Tone periods (R0-R5) - frequency
2. Mixer control (R7) - enable/disable channels
3. Volume (R8-R10) - amplitude
4. Envelope (R11-R13) - if using hardware envelope

## Comparison with Tracker Formats

**Similarities:**
- Pattern-based sequencing
- Instrument table
- Stream/order list

**Differences:**
- **No rows**: Uses continuous frame/tick timing instead of row-based
- **Per-frame updates**: Effects calculated every frame (50Hz) vs. per-row
- **Hardware envelope**: Uses AY chip's envelope generator instead of software volume tables
- **Compact**: Optimized for small memory footprint

## Porting to C64/SID

The existing `extract_tracker_format.go` already handles conversion to C64 format:

**Key differences:**
- SID has 3 voices vs AY's 3 channels + 1 noise
- SID frequency encoding: direct Hz value vs. AY period
- SID ADSR: 4-bit hardware envelope vs. software envelope
- SID waveforms: pulse/sawtooth/triangle/noise vs. AY tone/noise + envelope shape

## References

**Implementation files:**
- `tools/zx_music_format.go` - Recreated ZX player
- `tools/zx_music_extractor.go` - Music data extraction
- `tools/extract_tracker_format.go` - Full tracker conversion system
- `emulator/spectrum/ay_logger.go` - AY command capture
- `emulator/spectrum/audio.go` - AY emulation

**Disassembly:**
- `emulator/full_disassembly.asm` - Complete Z80 disassembly
- `docs/SEQUENCER_COMPLETE_ANALYSIS.md` - Sequencer timing analysis

**AY-3-8912 Documentation:**
- General Instrument AY-3-8912 datasheet
- Yamaha YM2149 (compatible chip) documentation
