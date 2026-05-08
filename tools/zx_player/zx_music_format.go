// ZX Spectrum Music Format - Reverse Engineered from CUBELON
//
// This recreates the original ZX Spectrum music player format and playback engine.
// The ZX version uses the AY-3-8912 sound chip on the Spectrum 128K.
//
// ARCHITECTURE:
// - The music player runs from interrupt handler at $7D26 (50Hz on ZX Spectrum)
// - Music routine at $980E initializes and updates the player
// - Routine at $857A appears to be a graphics/sprite routine (not music)
// - AY chip is programmed via ports $FFFD (register select) and $BFFD (data write)
//
// MUSIC DATA FORMAT:
// Based on analysis of extract_tracker_format.go and AY command logs, the format uses:
// 1. Frequency table (base pitches for all notes)
// 2. Pattern table (note sequences with pitch + duration)
// 3. Instrument table (ADSR envelopes, waveforms, effects)
// 4. Effect patterns (vibrato, portamento, arpeggio, etc.)
// 5. Stream data (sequencing patterns across 3 channels)
//
// PLAYBACK:
// - 50Hz update rate (one interrupt per frame)
// - Tick-based timing (typically 5 frames per tick = 10Hz note rate)
// - Per-frame frequency/volume/waveform updates for effects
// - Hardware envelope generator (R13) for volume modulation
//
package main

import (
	"fmt"
)

// ========== ZX SPECTRUM HARDWARE CONSTANTS ==========

const (
	// ZX Spectrum 128K timing
	ZX_CPU_CLOCK_HZ    = 3546895.0 // 3.546895 MHz
	ZX_AY_CLOCK_HZ     = ZX_CPU_CLOCK_HZ / 2.0 // AY runs at CPU/2
	ZX_INTERRUPT_HZ    = 50.0 // PAL frame rate
	ZX_FRAMES_PER_TICK = 5    // Typical tick rate = 10Hz (50/5)

	// AY-3-8912 registers
	AY_R0_TONE_A_LOW   = 0
	AY_R1_TONE_A_HIGH  = 1
	AY_R2_TONE_B_LOW   = 2
	AY_R3_TONE_B_HIGH  = 3
	AY_R4_TONE_C_LOW   = 4
	AY_R5_TONE_C_HIGH  = 5
	AY_R6_NOISE_PERIOD = 6
	AY_R7_MIXER        = 7
	AY_R8_VOL_A        = 8
	AY_R9_VOL_B        = 9
	AY_R10_VOL_C       = 10
	AY_R11_ENV_LOW     = 11
	AY_R12_ENV_HIGH    = 12
	AY_R13_ENV_SHAPE   = 13
)

// ========== MUSIC DATA STRUCTURES (ZX FORMAT) ==========

// ZXMusicData represents the complete music data in ZX Spectrum format
type ZXMusicData struct {
	// Global timing
	StartOffset  uint16 // Frame offset before music starts
	TickRate     uint8  // Frames per tick (5 = 10Hz updates)

	// Frequency table (base pitches)
	FreqTable    []uint16 // AY tone periods (12-bit values)

	// Pattern table (note sequences)
	Patterns     []ZXPattern

	// Instrument table (sound design)
	Instruments  []ZXInstrument

	// Effect tables
	VibratoTable      []int8    // Vibrato offsets per frame
	ArpeggioTable     []int8    // Arpeggio semitone offsets
	PortamentoTable   []uint16  // Portamento frequency ramps
	EnvelopeShapes    []uint8   // R13 envelope shape values

	// Sequencer (3 channels)
	Streams      [3][]byte // Bytecode: pattern IDs or commands
}

// ZXPattern represents a single note pattern (pitch + duration + effects)
type ZXPattern struct {
	FreqIndex     uint8  // Index into FreqTable (0xFF = rest)
	Duration      uint8  // Duration in ticks
	InstrumentID  uint8  // Index into Instruments
	EffectType    uint8  // 0=none, 1=vibrato, 2=portamento, 3=arpeggio, etc.
	EffectParam1  uint8  // Effect-specific parameter 1
	EffectParam2  uint8  // Effect-specific parameter 2
}

// ZXInstrument defines the sound characteristics
type ZXInstrument struct {
	// Volume envelope (ADSR-like)
	Attack      uint8  // Frames to reach full volume
	Decay       uint8  // Frames to decay to sustain level
	Sustain     uint8  // Sustain volume (0-15)
	Release     uint8  // Frames to fade out after note off

	// Waveform
	UseTone     bool   // Enable tone generator
	UseNoise    bool   // Enable noise generator
	UseEnvelope bool   // Use hardware envelope (R13)
	EnvShape    uint8  // R13 envelope shape (if UseEnvelope=true)

	// Noise period (if UseNoise=true)
	NoisePeriod uint8  // R6 noise period value

	// Vibrato (optional)
	VibratoDelay  uint8  // Frames before vibrato starts
	VibratoDepth  uint8  // Vibrato depth in AY period units
	VibratoSpeed  uint8  // Vibrato speed (frames per cycle)
}

// ========== PLAYER STATE ==========

// ZXMusicPlayer maintains the playback state
type ZXMusicPlayer struct {
	// Music data
	Data *ZXMusicData

	// Global timing
	CurrentFrame uint32
	CurrentTick  uint32
	FrameCounter uint8  // Counts frames within a tick (0 to TickRate-1)

	// Per-channel state
	Channels [3]ZXChannelState

	// AY chip registers (shadow copy)
	AYRegisters [16]uint8
}

// ZXChannelState tracks the state of one channel
type ZXChannelState struct {
	// Stream position
	StreamPos     uint16  // Position in stream bytecode

	// Current note
	Active        bool    // Is a note playing?
	PatternID     uint8   // Current pattern ID
	Pattern       *ZXPattern // Current pattern data
	TicksRemaining uint8  // Ticks until next note
	FrameInNote   uint8   // Frame counter within current note

	// Base frequency
	BaseFreq      uint16  // Base AY period from FreqTable

	// Instrument
	Instrument    *ZXInstrument

	// Envelope state (ADSR)
	EnvPhase      uint8   // 0=attack, 1=decay, 2=sustain, 3=release
	EnvFrame      uint8   // Frame within current envelope phase
	CurrentVolume uint8   // Current volume (0-15)

	// Effect state
	EffectFrame   uint8   // Frame within effect pattern
	VibratoPhase  uint8   // Phase of vibrato oscillator
}

// ========== MUSIC PLAYER IMPLEMENTATION ==========

// NewZXMusicPlayer creates a new player instance
func NewZXMusicPlayer(data *ZXMusicData) *ZXMusicPlayer {
	player := &ZXMusicPlayer{
		Data: data,
	}

	// Initialize all channels
	for ch := 0; ch < 3; ch++ {
		player.Channels[ch].StreamPos = 0
	}

	return player
}

// Update is called every frame (50Hz on ZX Spectrum)
// This is the main playback routine that would run in the interrupt handler
func (player *ZXMusicPlayer) Update() {
	player.CurrentFrame++

	// Skip frames before music starts
	if player.CurrentFrame < uint32(player.Data.StartOffset) {
		return
	}

	// Update tick counter
	player.FrameCounter++
	if player.FrameCounter >= player.Data.TickRate {
		player.FrameCounter = 0
		player.CurrentTick++

		// Process new notes on tick boundaries
		for ch := 0; ch < 3; ch++ {
			player.updateChannelTick(ch)
		}
	}

	// Update per-frame effects (vibrato, envelopes, etc.)
	for ch := 0; ch < 3; ch++ {
		player.updateChannelFrame(ch)
	}

	// Write AY registers
	player.writeAYRegisters()
}

// updateChannelTick processes tick-based events (new notes)
func (player *ZXMusicPlayer) updateChannelTick(ch int) {
	state := &player.Channels[ch]

	// Check if current note is finished
	if state.Active && state.TicksRemaining > 0 {
		state.TicksRemaining--
		if state.TicksRemaining > 0 {
			return // Note still playing
		}

		// Note finished - trigger release phase
		state.EnvPhase = 3 // Release
		state.EnvFrame = 0
	}

	// Fetch next pattern from stream
	if state.StreamPos >= uint16(len(player.Data.Streams[ch])) {
		state.Active = false
		return
	}

	patternID := player.Data.Streams[ch][state.StreamPos]
	state.StreamPos++

	// Handle stream commands (0x80+ are commands, 0x00-0x7F are pattern IDs)
	if patternID >= 0x80 {
		switch patternID {
		case 0xFF: // End of stream
			state.Active = false
			return
		case 0xFE: // Loop to start
			state.StreamPos = 0
			return
		// Add more commands as needed
		}
		return
	}

	// Load pattern
	if int(patternID) >= len(player.Data.Patterns) {
		return
	}
	pattern := &player.Data.Patterns[patternID]
	state.Pattern = pattern
	state.PatternID = patternID

	// Check for rest
	if pattern.FreqIndex == 0xFF {
		state.Active = false
		state.TicksRemaining = pattern.Duration
		return
	}

	// Start new note
	state.Active = true
	state.TicksRemaining = pattern.Duration
	state.FrameInNote = 0
	state.EffectFrame = 0
	state.VibratoPhase = 0

	// Load base frequency
	if int(pattern.FreqIndex) < len(player.Data.FreqTable) {
		state.BaseFreq = player.Data.FreqTable[pattern.FreqIndex]
	}

	// Load instrument
	if int(pattern.InstrumentID) < len(player.Data.Instruments) {
		state.Instrument = &player.Data.Instruments[pattern.InstrumentID]
	}

	// Reset envelope
	state.EnvPhase = 0 // Attack
	state.EnvFrame = 0
	state.CurrentVolume = 0
}

// updateChannelFrame processes per-frame updates (effects, envelopes)
func (player *ZXMusicPlayer) updateChannelFrame(ch int) {
	state := &player.Channels[ch]

	if !state.Active || state.Instrument == nil {
		return
	}

	state.FrameInNote++
	state.EffectFrame++

	// Update envelope (ADSR)
	player.updateEnvelope(state)

	// Apply effects (vibrato, portamento, etc.)
	player.updateEffects(state)
}

// updateEnvelope updates the volume envelope (ADSR)
func (player *ZXMusicPlayer) updateEnvelope(state *ZXChannelState) {
	inst := state.Instrument

	state.EnvFrame++

	switch state.EnvPhase {
	case 0: // Attack
		if inst.Attack == 0 {
			state.CurrentVolume = 15
			state.EnvPhase = 1
			state.EnvFrame = 0
		} else {
			state.CurrentVolume = uint8((int(state.EnvFrame) * 15) / int(inst.Attack))
			if state.EnvFrame >= inst.Attack {
				state.EnvPhase = 1
				state.EnvFrame = 0
			}
		}

	case 1: // Decay
		if inst.Decay == 0 {
			state.CurrentVolume = inst.Sustain
			state.EnvPhase = 2
			state.EnvFrame = 0
		} else {
			decay := 15 - int(state.EnvFrame)*(15-int(inst.Sustain))/int(inst.Decay)
			if decay < int(inst.Sustain) {
				decay = int(inst.Sustain)
			}
			state.CurrentVolume = uint8(decay)
			if state.EnvFrame >= inst.Decay {
				state.EnvPhase = 2
				state.EnvFrame = 0
			}
		}

	case 2: // Sustain
		state.CurrentVolume = inst.Sustain

	case 3: // Release
		if inst.Release == 0 {
			state.CurrentVolume = 0
			state.Active = false
		} else {
			release := int(inst.Sustain) - int(state.EnvFrame)*int(inst.Sustain)/int(inst.Release)
			if release < 0 {
				release = 0
			}
			state.CurrentVolume = uint8(release)
			if state.EnvFrame >= inst.Release {
				state.CurrentVolume = 0
				state.Active = false
			}
		}
	}

	// Clamp to 0-15
	if state.CurrentVolume > 15 {
		state.CurrentVolume = 15
	}
}

// updateEffects applies frequency modulation effects
func (player *ZXMusicPlayer) updateEffects(state *ZXChannelState) {
	pattern := state.Pattern
	if pattern == nil {
		return
	}

	// Apply effects based on type
	switch pattern.EffectType {
	case 0: // No effect
		// Use base frequency as-is

	case 1: // Vibrato
		player.applyVibrato(state, pattern)

	case 2: // Portamento
		player.applyPortamento(state, pattern)

	case 3: // Arpeggio
		player.applyArpeggio(state, pattern)
	}
}

// applyVibrato applies vibrato effect
func (player *ZXMusicPlayer) applyVibrato(state *ZXChannelState, pattern *ZXPattern) {
	inst := state.Instrument
	if inst == nil {
		return
	}

	// Check vibrato delay
	if state.FrameInNote < uint8(inst.VibratoDelay) {
		return
	}

	// Update vibrato phase
	if inst.VibratoSpeed > 0 {
		state.VibratoPhase = uint8((state.EffectFrame - uint8(inst.VibratoDelay)) % inst.VibratoSpeed)
	}

	// Calculate vibrato offset (simple sine approximation)
	vibratoPos := (state.VibratoPhase * 8) / inst.VibratoSpeed // 0-7 phase
	var offset int16
	switch vibratoPos {
	case 0, 7:
		offset = 0
	case 1:
		offset = int16(inst.VibratoDepth) / 2
	case 2:
		offset = int16(inst.VibratoDepth)
	case 3:
		offset = int16(inst.VibratoDepth) / 2
	case 4:
		offset = 0
	case 5:
		offset = -int16(inst.VibratoDepth) / 2
	case 6:
		offset = -int16(inst.VibratoDepth)
	}

	// Apply offset to base frequency
	newFreq := int16(state.BaseFreq) + offset
	if newFreq < 0 {
		newFreq = 0
	}
	if newFreq > 4095 {
		newFreq = 4095
	}
	state.BaseFreq = uint16(newFreq)
}

// applyPortamento applies pitch slide effect
func (player *ZXMusicPlayer) applyPortamento(state *ZXChannelState, pattern *ZXPattern) {
	// TODO: Implement portamento (frequency ramp)
}

// applyArpeggio applies arpeggio effect
func (player *ZXMusicPlayer) applyArpeggio(state *ZXChannelState, pattern *ZXPattern) {
	// TODO: Implement arpeggio (rapid pitch changes)
}

// writeAYRegisters writes the current state to AY registers
func (player *ZXMusicPlayer) writeAYRegisters() {
	for ch := 0; ch < 3; ch++ {
		state := &player.Channels[ch]

		if !state.Active {
			// Silence channel
			player.AYRegisters[AY_R8_VOL_A+ch] = 0
			continue
		}

		// Write frequency (tone period)
		freq := state.BaseFreq
		player.AYRegisters[AY_R0_TONE_A_LOW+ch*2] = uint8(freq & 0xFF)
		player.AYRegisters[AY_R1_TONE_A_HIGH+ch*2] = uint8((freq >> 8) & 0x0F)

		// Write volume
		if state.Instrument.UseEnvelope {
			player.AYRegisters[AY_R8_VOL_A+ch] = 0x10 // Enable envelope mode
		} else {
			player.AYRegisters[AY_R8_VOL_A+ch] = state.CurrentVolume & 0x0F
		}

		// Setup mixer (enable tone/noise)
		mixerBit := uint8(1 << ch) // Tone enable bit
		noiseBit := uint8(0x08 << ch) // Noise enable bit

		if state.Instrument.UseTone {
			player.AYRegisters[AY_R7_MIXER] &^= mixerBit // Clear bit = enable
		} else {
			player.AYRegisters[AY_R7_MIXER] |= mixerBit // Set bit = disable
		}

		if state.Instrument.UseNoise {
			player.AYRegisters[AY_R7_MIXER] &^= noiseBit // Clear bit = enable
			player.AYRegisters[AY_R6_NOISE_PERIOD] = state.Instrument.NoisePeriod
		} else {
			player.AYRegisters[AY_R7_MIXER] |= noiseBit // Set bit = disable
		}

		// Setup hardware envelope (if used)
		if state.Instrument.UseEnvelope && state.EnvPhase == 0 && state.EnvFrame == 0 {
			// Trigger envelope on note start
			player.AYRegisters[AY_R13_ENV_SHAPE] = state.Instrument.EnvShape
		}
	}
}

// GetAYCommand returns the next AY register write command
// This can be used to generate AYCommand logs for compatibility with existing code
func (player *ZXMusicPlayer) GetAYCommands() []AYRegisterWrite {
	commands := []AYRegisterWrite{}

	for reg := 0; reg < 14; reg++ {
		commands = append(commands, AYRegisterWrite{
			Frame:    player.CurrentFrame,
			Register: uint8(reg),
			Value:    player.AYRegisters[reg],
		})
	}

	return commands
}

// AYRegisterWrite represents a single AY register write
type AYRegisterWrite struct {
	Frame    uint32
	Register uint8
	Value    uint8
}

// ========== EXAMPLE: Create sample music data ==========

func CreateSampleZXMusic() *ZXMusicData {
	data := &ZXMusicData{
		StartOffset: 162, // From analysis: music starts at frame 162
		TickRate:    5,   // 5 frames per tick = 10Hz note rate
	}

	// Build frequency table (AY tone periods for musical notes)
	// AY period = AY_CLOCK / (16 * frequency_hz)
	// For middle C (261.63 Hz): period = 1773447 / (16 * 261.63) ≈ 423
	data.FreqTable = []uint16{
		1721, 1623, 1531, 1445, 1364, 1287, 1214, 1146, // C2-B2
		1081, 1020, 963,  910,  859,  810,  765,  722,  // C3-B3
		681,  643,  607,  573,  541,  511,  482,  455,  // C4-B4
		429,  405,  382,  361,  341,  322,  304,  287,  // C5-B5 (middle C = 429)
		270,  255,  241,  227,  215,  202,  191,  180,  // C6-B6
		170,  161,  152,  // C7-D7
	}

	// Create instruments
	data.Instruments = []ZXInstrument{
		// Instrument 0: Simple tone
		{
			Attack: 1, Decay: 5, Sustain: 12, Release: 10,
			UseTone: true, UseNoise: false, UseEnvelope: false,
			VibratoDelay: 0, VibratoDepth: 0, VibratoSpeed: 0,
		},
		// Instrument 1: Percussion
		{
			Attack: 0, Decay: 10, Sustain: 0, Release: 0,
			UseTone: false, UseNoise: true, UseEnvelope: false,
			NoisePeriod: 16,
		},
		// Instrument 2: Envelope tone
		{
			Attack: 0, Decay: 0, Sustain: 15, Release: 0,
			UseTone: true, UseNoise: false, UseEnvelope: true,
			EnvShape: 0x0E, // /\/\ repeating
		},
	}

	// Create patterns (notes)
	data.Patterns = []ZXPattern{
		{FreqIndex: 24, Duration: 4, InstrumentID: 0, EffectType: 0}, // C4 quarter note
		{FreqIndex: 28, Duration: 4, InstrumentID: 0, EffectType: 0}, // E4 quarter note
		{FreqIndex: 31, Duration: 4, InstrumentID: 0, EffectType: 0}, // G4 quarter note
		{FreqIndex: 0xFF, Duration: 4, InstrumentID: 0, EffectType: 0}, // Rest
	}

	// Create streams (sequencing)
	data.Streams[0] = []byte{0, 1, 2, 3, 0, 1, 2, 3, 0xFF} // Simple melody
	data.Streams[1] = []byte{0xFF} // Channel B silent
	data.Streams[2] = []byte{0xFF} // Channel C silent

	return data
}

// ========== MAIN ==========

func main() {
	fmt.Println("ZX Spectrum Music Player - CUBELON Format")
	fmt.Println("==========================================")

	// Create sample music
	musicData := CreateSampleZXMusic()

	// Create player
	player := NewZXMusicPlayer(musicData)

	// Simulate playback for 300 frames (6 seconds)
	for frame := 0; frame < 300; frame++ {
		player.Update()

		// Print AY register state every 50 frames
		if frame%50 == 0 {
			fmt.Printf("\nFrame %d (Tick %d):\n", frame, player.CurrentTick)
			for ch := 0; ch < 3; ch++ {
				state := &player.Channels[ch]
				if state.Active {
					freq := uint16(player.AYRegisters[AY_R0_TONE_A_LOW+ch*2]) |
					       (uint16(player.AYRegisters[AY_R1_TONE_A_HIGH+ch*2]) << 8)
					vol := player.AYRegisters[AY_R8_VOL_A+ch]
					fmt.Printf("  Ch%d: Freq=%d Vol=%d Phase=%d\n", ch, freq, vol, state.EnvPhase)
				} else {
					fmt.Printf("  Ch%d: Inactive\n", ch)
				}
			}
		}
	}

	fmt.Println("\n✓ Playback simulation complete")
	fmt.Printf("Total frames: %d (%.1f seconds)\n", player.CurrentFrame, float64(player.CurrentFrame)/50.0)
	fmt.Printf("Total ticks: %d\n", player.CurrentTick)
}
