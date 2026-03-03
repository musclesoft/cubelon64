#!/usr/bin/env python3
"""
Connect to VICE monitor and extract screen RAM to visualize display
"""
import socket
import time
import sys

def connect_vice_monitor(host='localhost', port=6510):
    """Connect to VICE remote monitor"""
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(5)
        sock.connect((host, port))
        return sock
    except Exception as e:
        print(f"Failed to connect to VICE monitor: {e}")
        return None

def send_command(sock, cmd):
    """Send command to VICE monitor and get response"""
    sock.sendall((cmd + '\n').encode('ascii'))
    time.sleep(0.1)
    response = b''
    try:
        while True:
            chunk = sock.recv(4096)
            if not chunk:
                break
            response += chunk
            if b'\n(C:' in response or b'\n>' in response:
                break
    except socket.timeout:
        pass
    return response.decode('ascii', errors='ignore')

def extract_screen_ram(sock):
    """Extract screen RAM ($0400-$07E7)"""
    screen_data = []

    # Read screen RAM in chunks
    for offset in range(0, 1000, 256):
        addr = 0x0400 + offset
        count = min(256, 1000 - offset)
        cmd = f"m {addr:04x} {addr + count - 1:04x}"
        response = send_command(sock, cmd)

        # Parse hexdump output
        for line in response.split('\n'):
            if line.startswith('>C:'):
                parts = line.split()
                if len(parts) >= 2:
                    # Skip address, get hex bytes
                    for i in range(1, len(parts)):
                        if parts[i].startswith('>'): break
                        try:
                            screen_data.append(int(parts[i], 16))
                        except ValueError:
                            pass

    return screen_data[:1000]

def extract_charset(sock, addr=0x2000):
    """Extract charset from memory"""
    charset_data = []

    # Read 2KB of charset
    for offset in range(0, 2048, 256):
        cmd_addr = addr + offset
        cmd = f"m {cmd_addr:04x} {cmd_addr + 255:04x}"
        response = send_command(sock, cmd)

        for line in response.split('\n'):
            if line.startswith('>C:'):
                parts = line.split()
                for i in range(1, len(parts)):
                    if parts[i].startswith('>'): break
                    try:
                        charset_data.append(int(parts[i], 16))
                    except ValueError:
                        pass

    return charset_data[:2048]

def render_char(char_data):
    """Render an 8x8 character as ASCII art"""
    lines = []
    for byte in char_data:
        line = ""
        for bit in range(7, -1, -1):
            if byte & (1 << bit):
                line += "█"
            else:
                line += " "
        lines.append(line)
    return lines

def visualize_screen(screen_data, charset_data):
    """Visualize the entire screen"""
    output = []

    # Pad screen data to 1000 bytes if needed
    while len(screen_data) < 1000:
        screen_data.append(32)  # Space

    for row in range(25):
        # Each row is 8 scan lines
        char_lines = [[] for _ in range(8)]

        for col in range(40):
            char_code = screen_data[row * 40 + col]

            # Get character data (8 bytes)
            char_offset = char_code * 8
            if char_offset + 8 <= len(charset_data):
                char_bytes = charset_data[char_offset:char_offset + 8]
                rendered = render_char(char_bytes)
                for i, line in enumerate(rendered):
                    char_lines[i].append(line)
            else:
                for i in range(8):
                    char_lines[i].append("????????")

        # Combine character lines
        for line_parts in char_lines:
            output.append("".join(line_parts))

    return "\n".join(output)

if __name__ == '__main__':
    print("Attempting to connect to VICE monitor...")
    print("(Start VICE with: x64sc -remotemonitor ...)")

    sock = connect_vice_monitor()
    if not sock:
        print("\nFailed to connect. Make sure VICE is running with remote monitor enabled:")
        print("  x64sc -remotemonitor -autostartprgmode 1 build/self_diagnostic.prg")
        sys.exit(1)

    print("Connected! Extracting screen RAM...")
    screen_data = extract_screen_ram(sock)
    print(f"Extracted {len(screen_data)} bytes from screen RAM")

    print("Extracting charset from $2000...")
    charset_data = extract_charset(sock, 0x2000)
    print(f"Extracted {len(charset_data)} bytes of charset data")

    sock.close()

    print("\nVisualizing display:\n")
    print("=" * 320)
    display = visualize_screen(screen_data, charset_data)
    print(display)
    print("=" * 320)

    # Save to file
    with open('/tmp/vice_screen_capture.txt', 'w') as f:
        f.write(display)
    print("\nSaved to /tmp/vice_screen_capture.txt")
