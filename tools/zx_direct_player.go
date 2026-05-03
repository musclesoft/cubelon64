package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// DirectPlayer just loads and plays back ay_commands.bin
type DirectPlayer struct {
	commands [][3]uint32 // [frame, register, value]
}

func NewDirectPlayer(commandFile string) (*DirectPlayer, error) {
	data, err := os.ReadFile(commandFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read command file: %w", err)
	}

	// Format: [count:uint32][command1:6 bytes][command2:6 bytes]...
	count := binary.LittleEndian.Uint32(data[0:4])
	commands := make([][3]uint32, count)

	for i := uint32(0); i < count; i++ {
		offset := 4 + i*6
		frame := binary.LittleEndian.Uint32(data[offset : offset+4])
		reg := data[offset+4]
		val := data[offset+5]
		commands[i] = [3]uint32{frame, uint32(reg), uint32(val)}
	}

	return &DirectPlayer{commands: commands}, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run zx_direct_player.go <ay_commands.bin>")
		os.Exit(1)
	}

	player, err := NewDirectPlayer(os.Args[1])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded %d commands\n", len(player.commands))
	
	// Show first few commands
	for i := 0; i < 20 && i < len(player.commands); i++ {
		cmd := player.commands[i]
		fmt.Printf("[%d, R%d, $%02X]\n", cmd[0], cmd[1], cmd[2])
	}
}
