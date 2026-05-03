#!/bin/bash
# Simple script to dump memory at specific frame using existing emulator

echo "Building modified emulator with memory dump..."

# Add memory dump code to run_cubelon.go temporarily
cat > /tmp/dump_patch.go << 'EOF'
// Add this after frame 300 in the main loop:
if frameCount == 300 {
    fmt.Println("\n=== Dumping memory at frame 300 ===")
    f, _ := os.Create("memory_dump_frame300.bin")
    for addr := uint16(0); addr < 0x10000; addr++ {
        f.Write([]byte{mem.Read(addr)})
    }
    f.Close()
    fmt.Println("Saved memory_dump_frame300.bin")
}
EOF

echo "Run the emulator manually with --stop-at=310 --no-frames --no-audio"
echo "Then use a disassembler on the memory dump"
echo ""
echo "Or: Use the existing AY command log analysis - we already have 123K commands"
echo "The music data is encoded in how those commands were generated"
