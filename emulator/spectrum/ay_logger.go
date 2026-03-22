// AY Command Logger
//
// Logs all AY-3-8912 register writes during emulation for later analysis and replay.
// Used by run_cubelon.go with --log-ay-commands flag to capture audio data for
// AY-to-SID translation.
//
// The binary format stores each command as: [frame:uint32][register:byte][value:byte]
// Commands can be replayed through the AY or SID emulator for comparison.
//
package spectrum

import (
	"encoding/binary"
	"fmt"
	"os"
)

// AYCommand represents a single AY register write
type AYCommand struct {
	Frame    uint32 // Frame number when write occurred
	Register byte   // AY register (0-15)
	Value    byte   // Value written
}

// AYLogger logs all AY register writes for later replay
type AYLogger struct {
	Commands []AYCommand
	Enabled  bool
}

// NewAYLogger creates a new AY command logger
func NewAYLogger() *AYLogger {
	return &AYLogger{
		Commands: make([]AYCommand, 0, 100000), // Pre-allocate for typical demo
		Enabled:  false,
	}
}

// LogWrite records an AY register write
func (log *AYLogger) LogWrite(frame uint32, register byte, value byte) {
	if !log.Enabled {
		return
	}
	log.Commands = append(log.Commands, AYCommand{
		Frame:    frame,
		Register: register,
		Value:    value,
	})
}

// SaveBinary saves the command log to a binary file
// Format: [count:uint32][command1][command2]...
// Each command: [frame:uint32][register:byte][value:byte]
func (log *AYLogger) SaveBinary(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Write count
	count := uint32(len(log.Commands))
	if err := binary.Write(f, binary.LittleEndian, count); err != nil {
		return fmt.Errorf("write count: %w", err)
	}

	// Write commands
	for _, cmd := range log.Commands {
		if err := binary.Write(f, binary.LittleEndian, cmd.Frame); err != nil {
			return fmt.Errorf("write frame: %w", err)
		}
		if err := binary.Write(f, binary.LittleEndian, cmd.Register); err != nil {
			return fmt.Errorf("write register: %w", err)
		}
		if err := binary.Write(f, binary.LittleEndian, cmd.Value); err != nil {
			return fmt.Errorf("write value: %w", err)
		}
	}

	return nil
}

// LoadBinary loads command log from a binary file
func LoadAYCommands(filename string) ([]AYCommand, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	// Read count
	var count uint32
	if err := binary.Read(f, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("read count: %w", err)
	}

	// Read commands
	commands := make([]AYCommand, count)
	for i := uint32(0); i < count; i++ {
		if err := binary.Read(f, binary.LittleEndian, &commands[i].Frame); err != nil {
			return nil, fmt.Errorf("read frame %d: %w", i, err)
		}
		if err := binary.Read(f, binary.LittleEndian, &commands[i].Register); err != nil {
			return nil, fmt.Errorf("read register %d: %w", i, err)
		}
		if err := binary.Read(f, binary.LittleEndian, &commands[i].Value); err != nil {
			return nil, fmt.Errorf("read value %d: %w", i, err)
		}
	}

	return commands, nil
}

// SaveText saves the command log to a human-readable text file
func (log *AYLogger) SaveText(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "# AY Command Log\n")
	fmt.Fprintf(f, "# Total commands: %d\n", len(log.Commands))
	fmt.Fprintf(f, "# Format: frame register value\n\n")

	regNames := []string{
		"R0_TONE_A_LOW", "R1_TONE_A_HIGH",
		"R2_TONE_B_LOW", "R3_TONE_B_HIGH",
		"R4_TONE_C_LOW", "R5_TONE_C_HIGH",
		"R6_NOISE_PERIOD",
		"R7_MIXER",
		"R8_VOL_A", "R9_VOL_B", "R10_VOL_C",
		"R11_ENV_LOW", "R12_ENV_HIGH",
		"R13_ENV_SHAPE",
		"R14_IO_A", "R15_IO_B",
	}

	for _, cmd := range log.Commands {
		regName := "UNKNOWN"
		if cmd.Register < 16 {
			regName = regNames[cmd.Register]
		}
		fmt.Fprintf(f, "%6d  R%-2d %-16s  0x%02X\n",
			cmd.Frame, cmd.Register, regName, cmd.Value)
	}

	return nil
}
