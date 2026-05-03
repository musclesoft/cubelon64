# ZX Spectrum Music Format - Tools

This directory contains tools for working with the CUBELON ZX Spectrum music format.

## Quick Start

### 1. Extract Music Statistics

Analyze the captured AY register commands from CUBELON:

```bash
go run zx_music_extractor.go ../emulator/ay_commands.bin
```

**Output:**
- Total notes: 589
- Unique frequencies: 43
- Music duration: 185 seconds
- Per-channel activity breakdown
- Common note durations

### 2. Run the Player Simulation

Test the ZX music player with sample data:

```bash
go run zx_music_format.go
```

**Output:**
- Frame-by-frame playback simulation
- AY register state per channel
- Envelope and effect updates

## Tools Overview

### zx_music_format.go (602 lines)

**Complete ZX Spectrum music player implementation.**

Key components:
- `ZXMusicData` - Music data structure
- `ZXMusicPlayer` - Playback engine
- `ZXChannelState` - Per-channel state tracking
- ADSR envelope generator
- Vibrato/portamento/arpeggio effects
- AY-3-8912 register output

**Usage example:**
```go
// Create music data
musicData := CreateSampleZXMusic()

// Create player
player := NewZXMusicPlayer(musicData)

// Simulate 50Hz playback
for frame := 0; frame < 300; frame++ {
    player.Update()
    
    // Get AY register writes
    commands := player.GetAYCommands()
}
```

### zx_music_extractor.go (265 lines)

**Music analysis tool for AY command logs.**

Functions:
- `LoadAYCommands()` - Load binary command log
- `ExtractNotesFromCommands()` - Extract note events
- `BuildFrequencyTable()` - Build frequency table
- `AnalyzeNoteDurations()` - Analyze timing patterns
- `AnalyzeChannelActivity()` - Per-channel statistics

**Usage example:**
```go
commands, _ := LoadAYCommands("ay_commands.bin")
notes := ExtractNotesFromCommands(commands)
freqTable := BuildFrequencyTable(notes)
```

## Integration with Existing Code

These tools integrate with:

### emulator/spectrum/ay_logger.go
- Captures AY register writes during emulation
- Saves to binary format
- Compatible with extractor

### emulator/spectrum/audio.go
- Complete AY-3-8912 emulation
- Envelope generator
- Noise generator
- WAV file output

## IMPORTANT: Relationship to C64 Tracker System

**`tools/extract_tracker_format.go` is a SEPARATE, DIFFERENT system:**

- **ZX Music Format (this directory):**
  - Recreation of the ORIGINAL ZX Spectrum music format
  - Designed for AY-3-8912 chip
  - ~3.1 KB estimated data size
  - Frame-accurate interrupt-driven playback

- **C64 Tracker System (extract_tracker_format.go):**
  - NEW format designed for C64/SID from scratch
  - NOT the original ZX format
  - 5401 bytes current size
  - Analyzes AY commands to extract patterns
  - Creates optimized C64-specific format

**These are two completely different approaches to playing the same music. Do not confuse them!**

## Format Specification

See **docs/ZX_MUSIC_FORMAT.md** for complete format documentation:
- Hardware specifications (ZX 128K, AY-3-8912)
- Data structure layouts
- Register programming
- Timing systems
- Effect implementations

## Architecture

See **docs/ZX_MUSIC_ARCHITECTURE.txt** for system diagrams:
- Original ZX implementation flow
- Music data structure
- Go recreation architecture
- Timing diagrams
- ADSR envelope visualization
- Integration overview

## Complete Recreation Guide

See **docs/ZX_TO_GO_RECREATION.md** for:
- Overview of what was recreated
- Original vs. Go architecture comparison
- Usage examples
- Verification procedures
- Future enhancements

## File Summary

**New files created:**
- `zx_music_format.go` - Player implementation
- `zx_music_extractor.go` - Analysis tool
- `../docs/ZX_MUSIC_FORMAT.md` - Format spec
- `../docs/ZX_TO_GO_RECREATION.md` - Recreation guide
- `../docs/ZX_MUSIC_ARCHITECTURE.txt` - System diagrams
- `README_ZX_MUSIC.md` - This file

**Total: ~900 lines of code + comprehensive documentation**

## Development Workflow

### Capturing Music from CUBELON

1. Run emulator with AY logging:
   ```bash
   go run ../emulator/run_cubelon.go --log-ay-commands
   ```

2. This creates `ay_commands.bin` (123,027 commands)

3. Extract music statistics:
   ```bash
   go run zx_music_extractor.go ../emulator/ay_commands.bin
   ```

### Creating New Music

1. Define frequency table (musical notes)
2. Create instruments (ADSR + effects)
3. Build patterns (note sequences)
4. Sequence patterns into streams
5. Test with player simulation

### Note on C64/SID Conversion

The `extract_tracker_format.go` tool is a **separate system** that creates a **new tracker format** for C64. It is NOT a direct conversion of the ZX format. See the "IMPORTANT" section above for details.

## Technical Details

### Timing

- **Frame rate**: 50 Hz (PAL)
- **Tick rate**: 5 frames = 10 Hz note updates
- **Music start**: Frame 172 (3.44 seconds)
- **Total duration**: 185 seconds (9,250 frames)

### AY-3-8912 Specifications

- **CPU clock**: 3.546895 MHz
- **AY clock**: 1.773447 MHz (CPU/2)
- **Channels**: 3 tone + 1 noise
- **Tone period**: 12-bit (0-4095)
- **Volume**: 4-bit (0-15) per channel
- **Envelope**: 16 shapes, 16-bit period

### Music Statistics (CUBELON)

- **Total notes**: 589
- **Unique frequencies**: 43
- **Channel A**: 127 notes (melody)
- **Channel B**: 197 notes (harmony)
- **Channel C**: 265 notes (rhythm)
- **Most common duration**: 30 frames (0.60s)

## References

### Code Files
- `zx_music_format.go` - This directory
- `zx_music_extractor.go` - This directory
- `../emulator/spectrum/ay_logger.go` - AY command capture
- `../emulator/spectrum/audio.go` - AY emulation
- `extract_tracker_format.go` - Full conversion system

### Documentation
- `../docs/ZX_MUSIC_FORMAT.md` - Format specification
- `../docs/ZX_TO_GO_RECREATION.md` - Recreation guide
- `../docs/ZX_MUSIC_ARCHITECTURE.txt` - System diagrams
- `../docs/SEQUENCER_COMPLETE_ANALYSIS.md` - Timing analysis

### External Resources
- AY-3-8912 datasheet (General Instrument)
- YM2149 datasheet (Yamaha, compatible chip)
- ZX Spectrum 128K technical manual
- Ayumi audio library (accurate volume tables)

## Questions?

For detailed technical questions, see the comprehensive documentation in `docs/`.

For implementation examples, see the inline comments in `zx_music_format.go`.

For analysis methodology, see `zx_music_extractor.go`.
